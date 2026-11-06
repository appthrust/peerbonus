package api

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/peerbonus/cmd/server/internal/directory"
	"github.com/stretchr/testify/require"
)

func candidateIDs(items []candidate) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.PersonID)
	}
	return ids
}

func (h *harness) directoryRequests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.directoryCalls)
}

func TestRecipientsTeamsAndMergedGroups(t *testing.T) {
	h := newHarness(t)
	groups := []contract.GroupRef{
		{ID: "root", Name: "Company", Path: "company"},
		{ID: "z", Name: "Zeta", Path: "company/z"},
		{ID: "deep", Name: "Team", Path: "company/sales/team"},
		{ID: "a", Name: "Alpha", Path: "company/a"},
		{ID: "b", Name: "Beta", Path: "company/b"},
		{ID: "c", Name: "Charlie", Path: "company/c"},
	}
	h.mu.Lock()
	for _, group := range groups {
		h.memberships["person-writer"] = append(h.memberships["person-writer"], contract.PersonMembership{Group: group})
	}
	h.memberships["person-writer"] = append(h.memberships["person-writer"], contract.PersonMembership{
		Group: contract.GroupRef{ID: "inherited", Name: "Inherited", Path: "company/sales/team/child"}, InheritedFrom: new("deep"),
	})
	h.groupPeople["deep"] = []string{"person-recipient", "person-writer", "person-clapper"}
	h.groupPeople["a"] = []string{"person-recipient", "person-reader"}
	h.mu.Unlock()

	result := call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
	require.True(t, result.DirectoryAvailable)
	require.Equal(t, "person-writer", result.Me.PersonID)
	require.Empty(t, result.Recent)
	require.NotNil(t, result.Recent)
	groupIDs := []string{}
	for _, group := range result.Me.Groups {
		groupIDs = append(groupIDs, group.ID)
	}
	require.Equal(t, []string{"deep", "a", "b", "c", "z", "root"}, groupIDs)
	require.Len(t, result.Teams, 5)
	require.Equal(t, []string{"person-clapper", "person-recipient"}, candidateIDs(result.Teams[0].Members))
	merged := []RecipientGroup{RecipientGroup(groups[2]), RecipientGroup(groups[3])}
	require.Equal(t, merged, result.Teams[0].Members[1].Groups)
	require.True(t, result.Teams[0].Members[1].Pending)
	require.Equal(t, merged, result.Teams[1].Members[1].Groups)
	for _, team := range result.Teams[2:] {
		require.NotNil(t, team.Members)
		require.Empty(t, team.Members)
	}
	require.Equal(t, 5, result.Everyone.Total)
	require.Equal(t, []string{"person-admin", "person-clapper", "person-reader", "person-subject", "person-recipient"}, candidateIDs(result.Everyone.Items))
	require.Equal(t, merged, result.Everyone.Items[4].Groups)
	require.Equal(t, []RecipientGroup{}, result.Everyone.Items[0].Groups)
	require.Equal(t, 0, h.count("members"))
	require.Equal(t, []string{
		"/api/directory/v1/me", "/api/directory/v1/people/person-writer",
		"/api/directory/v1/people?groupId=deep&limit=100", "/api/directory/v1/people?groupId=a&limit=100",
		"/api/directory/v1/people?groupId=b&limit=100", "/api/directory/v1/people?groupId=c&limit=100",
		"/api/directory/v1/people?groupId=z&limit=100", "/api/directory/v1/people?limit=500",
	}, h.directoryRequests())

	// Posts are never part of the Directory cache and must appear immediately.
	call[postResponse](h, "writer", "POST", "/posts", postInput(1), 201, "recent")
	calls := len(h.directoryRequests())
	result = call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
	require.Len(t, result.Recent, 1)
	require.Equal(t, "person-recipient", result.Recent[0].PersonID)
	require.Equal(t, merged, result.Recent[0].Groups)
	require.Equal(t, calls, len(h.directoryRequests()))
}

func TestRecipientsRecentDistinctVisibleAndBounded(t *testing.T) {
	h := newHarness(t)
	for i := range 10 {
		id := fmt.Sprintf("recent-%02d", i)
		h.mu.Lock()
		h.people[id] = contract.Person{ID: id, DisplayName: id, Email: id + "@example.test"}
		h.mu.Unlock()
		h.setTime(fmt.Sprintf("2026-10-19T03:%02d:00Z", i))
		input := postInput(1)
		input["recipientPersonId"] = id
		call[postResponse](h, "writer", "POST", "/posts", input, 201, id)
	}
	h.setTime("2026-10-19T03:11:00Z")
	input := postInput(1)
	input["recipientPersonId"] = "recent-02"
	call[postResponse](h, "writer", "POST", "/posts", input, 201, "repeat")
	deleted := call[postResponse](h, "writer", "POST", "/posts", postInput(1), 201, "deleted")
	call[any](h, "admin", "DELETE", "/posts/"+deleted.ID, nil, 204)
	call[postResponse](h, "clapper", "POST", "/posts", postInput(1), 201, "other-sender")
	result := call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
	require.Equal(t, []string{"recent-02", "recent-09", "recent-08", "recent-07", "recent-06", "recent-05", "recent-04", "recent-03"}, candidateIDs(result.Recent))
	for _, person := range result.Recent {
		require.True(t, person.Pending)
		require.Equal(t, []RecipientGroup{}, person.Groups)
	}
}

func TestRecipientsDirectoryDegradedAndRejected(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int32
		path   string
	}{
		{name: "unavailable", status: 503},
		{name: "forbidden", status: 403},
		{name: "unauthorized", status: 401},
		{name: "profile", status: 503, path: "/api/directory/v1/people/person-writer"},
		{name: "people", status: 503, path: "/api/directory/v1/people"},
		{name: "absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			call[postResponse](h, "writer", "POST", "/posts", postInput(1), 201, "known")
			h.mu.Lock()
			h.directoryFailurePath = tc.path
			h.mu.Unlock()
			h.directoryStatus.Store(tc.status)
			if tc.name == "absent" {
				h.api.directory = directory.New(nil)
			}
			result := call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
			require.False(t, result.DirectoryAvailable)
			require.Equal(t, "person-writer", result.Me.PersonID)
			require.Equal(t, []RecipientGroup{}, result.Me.Groups)
			require.Equal(t, []recipientTeam{}, result.Teams)
			require.Equal(t, recipientEveryone{Items: []candidate{}}, result.Everyone)
			require.Equal(t, []string{"person-recipient"}, candidateIDs(result.Recent))
			require.Equal(t, []RecipientGroup{}, result.Recent[0].Groups)
			if tc.path == "" {
				unknown := call[recipientsResponse](h, "unknown", "GET", "/recipients", nil, 200)
				require.False(t, unknown.DirectoryAvailable)
				require.Equal(t, "", unknown.Me.PersonID)
				require.Equal(t, []candidate{}, unknown.Recent)
			}
		})
	}
	h := newHarness(t)
	h.directoryStatus.Store(409)
	require.Equal(t, "identity_conflict", call[httpx.Error](h, "writer", "GET", "/recipients", nil, 409).Code)
	h.directoryStatus.Store(0)
	require.True(t, call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200).DirectoryAvailable)
}

func TestRecipientsCacheIsolationAndExpiry(t *testing.T) {
	h := newHarness(t)
	first := call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
	calls := len(h.directoryRequests())
	h.setTime("2026-10-19T03:00:59Z")
	require.Equal(t, first, call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200))
	require.Equal(t, calls, len(h.directoryRequests()))
	other := call[recipientsResponse](h, "reader", "GET", "/recipients", nil, 200)
	require.Equal(t, "person-reader", other.Me.PersonID)
	require.NotContains(t, candidateIDs(other.Everyone.Items), "person-reader")
	require.Contains(t, candidateIDs(other.Everyone.Items), "person-writer")
	require.Greater(t, len(h.directoryRequests()), calls)
	h.setTime("2026-10-19T03:01:00Z")
	h.directoryStatus.Store(503)
	degraded := call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
	require.False(t, degraded.DirectoryAvailable)
	require.Empty(t, degraded.Everyone.Items)
	h.directoryStatus.Store(0)
	require.True(t, call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200).DirectoryAvailable)
}

func TestRecipientsCacheBounded(t *testing.T) {
	now := time.Date(2026, time.October, 19, 0, 0, 0, 0, time.UTC)
	a := &API{now: func() time.Time { return now }}
	for i := range recipientsCacheLimit + 1 {
		a.cacheRecipients(fmt.Sprint(i), directoryRecipients{})
	}
	require.Len(t, a.recipientsCache, recipientsCacheLimit)
	now = now.Add(60 * time.Second)
	a.cacheRecipients("new", directoryRecipients{})
	require.Len(t, a.recipientsCache, 1)
	require.Contains(t, a.recipientsCache, "new")
}

func TestRecipientsEveryoneAndTeamBounds(t *testing.T) {
	h := newHarness(t)
	h.mu.Lock()
	h.memberships["person-writer"] = []contract.PersonMembership{{Group: contract.GroupRef{ID: "team", Name: "Team", Path: "team"}}}
	for i := range 510 {
		id := fmt.Sprintf("person-%03d", i)
		h.people[id] = contract.Person{ID: id, DisplayName: fmt.Sprintf("A%03d", i), Email: id + "@example.test"}
		h.groupPeople["team"] = append(h.groupPeople["team"], id)
	}
	h.mu.Unlock()
	result := call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
	require.Equal(t, 500, result.Everyone.Total)
	require.Len(t, result.Everyone.Items, 100)
	require.Len(t, result.Teams[0].Members, 100)
	require.Equal(t, "person-000", result.Everyone.Items[0].PersonID)
	require.Equal(t, "person-099", result.Everyone.Items[99].PersonID)
	require.Equal(t, result.Teams[0].Members, result.Everyone.Items)
}

func TestMembersWithoutQuery(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{"/members", "/members?q=", "/members?q=%20%20"} {
		result := call[struct {
			Items []candidate `json:"items"`
		}](h, "writer", "GET", path, nil, 200)
		require.Equal(t, []string{"person-admin", "person-clapper", "person-reader", "person-subject", "person-recipient"}, candidateIDs(result.Items))
		for _, person := range result.Items {
			require.Equal(t, []RecipientGroup{}, person.Groups)
		}
	}
	limited := call[struct {
		Items []candidate `json:"items"`
	}](h, "writer", "GET", "/members?limit=2", nil, 200)
	require.Equal(t, []string{"person-admin", "person-clapper"}, candidateIDs(limited.Items))
	for _, path := range h.directoryRequests() {
		require.False(t, strings.Contains(path, "q="))
	}
	require.Equal(t, 0, h.count("members"))
}
