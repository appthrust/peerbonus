package api

import (
	"testing"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAdminParticipationDirectoryRosterAndCache(t *testing.T) {
	h := newHarness(t)
	h.mu.Lock()
	h.people["directory-only"] = contract.Person{ID: "directory-only", DisplayName: "No activity", Email: "new@example.test", Pending: true}
	h.mu.Unlock()
	post := call[postResponse](h, "writer", "POST", "/posts", postInput(20), 201, "participation")
	call[postResponse](h, "clapper", "POST", "/posts/"+post.ID+"/claps", nil, 201, "participation-clap")
	call[httpx.Error](h, "reader", "GET", "/admin/members?month=2026-10", nil, 403)
	call[httpx.Error](h, "admin", "GET", "/admin/members", nil, 400)
	call[recipientsResponse](h, "admin", "GET", "/recipients", nil, 200)
	calls := len(h.directoryRequests())
	result := call[participationResponse](h, "admin", "GET", "/admin/members?month=2026-10", nil, 200)
	require.Equal(t, calls, len(h.directoryRequests()), "picker and participation must share the people cache")
	require.True(t, result.DirectoryAvailable)
	require.Equal(t, 7, result.Summary.People)
	require.Equal(t, 1, result.Summary.Senders)
	require.Equal(t, 1, result.Summary.Recipients)
	require.Equal(t, 3, result.Summary.Active)
	require.Equal(t, 3, h.count("members"), "reading the directory roster must not create members")
	require.Equal(t, "person-recipient", result.Items[0].PersonID)
	require.Equal(t, 21, result.Items[0].ReceivedPoints)
	found := false
	for _, item := range result.Items {
		if item.PersonID == "directory-only" {
			found = true
			require.Nil(t, item.Member)
			require.True(t, item.Pending)
			require.Zero(t, item.SentPosts+item.ReceivedPosts+item.ClapsGiven)
		}
	}
	require.True(t, found)
	call[participationResponse](h, "admin", "GET", "/admin/members?month=2026-09", nil, 200)
	require.Equal(t, calls, len(h.directoryRequests()))

	h.mu.Lock()
	delete(h.people, "person-recipient")
	writer := h.people["person-writer"]
	writer.DisplayName = "Renamed writer"
	h.people[writer.ID] = writer
	h.mu.Unlock()
	h.setTime("2026-10-19T03:01:00Z")
	result = call[participationResponse](h, "admin", "GET", "/admin/members?month=2026-10", nil, 200)
	require.Equal(t, 7, result.Summary.People)
	require.Equal(t, "person-recipient", result.Items[len(result.Items)-1].PersonID)
	require.NotNil(t, result.Items[len(result.Items)-1].Member)
	require.Equal(t, "Renamed writer", result.Items[0].DisplayName)
	h.directoryStatus.Store(503)
	h.setTime("2026-10-19T03:02:00Z")
	degraded := call[participationResponse](h, "admin", "GET", "/admin/members?month=2026-10", nil, 200)
	require.False(t, degraded.DirectoryAvailable)
	require.Len(t, degraded.Items, 3)
	require.Equal(t, 3, degraded.Summary.People)
	require.Equal(t, 3, degraded.Summary.Active)
	for _, item := range degraded.Items {
		require.NotNil(t, item.Member)
	}
}

func TestAdminInsightsWeeksValuesAndAuthority(t *testing.T) {
	h := newHarness(t)
	call[httpx.Error](h, "reader", "GET", "/admin/insights?month=2026-10", nil, 403)
	for _, path := range []string{"/admin/insights", "/admin/insights?month=2026-13", "/admin/insights?month=2026-1"} {
		require.Equal(t, "invalid_month", call[httpx.Error](h, "admin", "GET", path, nil, 400).Code)
	}
	call[programAdminResponse](h, "admin", "PATCH", "/admin/program", map[string]any{
		"values": []map[string]string{{"key": "a", "label": "A"}, {"key": "z", "label": "Z"}},
	}, 200)
	first := postInput(10)
	first["values"] = []string{"a", "z"}
	post := call[postResponse](h, "writer", "POST", "/posts", first, 201, "insights-one")
	second := postInput(20)
	second["values"] = []string{"z"}
	call[postResponse](h, "writer", "POST", "/posts", second, 201, "insights-two")
	call[postResponse](h, "clapper", "POST", "/posts/"+post.ID+"/claps", nil, 201, "insights-clap")
	result := call[insightsResponse](h, "admin", "GET", "/admin/insights?month=2026-10", nil, 200)
	require.Equal(t, 2, result.Posts)
	require.Equal(t, 32, result.Points)
	require.Equal(t, 1, result.Claps)
	require.Equal(t, 1, result.Senders)
	require.Equal(t, 1, result.Recipients)
	require.Equal(t, []valueCountResponse{{Key: "z", Posts: 2, Points: 30}, {Key: "a", Posts: 1, Points: 10}}, result.Values)
	require.Len(t, result.Weeks, 5)
	require.Equal(t, "2026-W40", result.Weeks[0].Key)
	require.Equal(t, "2026-W44", result.Weeks[4].Key)
	require.Equal(t, "2026-09-27T15:00:00Z", result.Weeks[0].StartsAt.UTC().Format("2006-01-02T15:04:05Z"))
	require.Equal(t, "2026-11-01T15:00:00Z", result.Weeks[4].EndsAt.UTC().Format("2006-01-02T15:04:05Z"))
	require.Zero(t, result.Weeks[0].Posts+result.Weeks[0].Points+result.Weeks[0].Claps)
	require.Equal(t, 32, result.Weeks[3].Points)
	require.Equal(t, 30, result.TopRecipients[0].Points, "rankings use post points, not clap credits")
	empty := call[insightsResponse](h, "admin", "GET", "/admin/insights?month=2021-02", nil, 200)
	require.Len(t, empty.Weeks, 4)
	require.Equal(t, []valueCountResponse{}, empty.Values)
	require.Equal(t, []partnerResponse{}, empty.TopRecipients)
}

func TestMemberProfileGroupsPartnersAndDegradedDirectory(t *testing.T) {
	h := newHarness(t)
	post := call[postResponse](h, "writer", "POST", "/posts", postInput(30), 201, "profile")
	group := contract.GroupRef{ID: "team", Name: "Team", Path: "company/team"}
	h.mu.Lock()
	h.memberships["person-recipient"] = []contract.PersonMembership{
		{Group: group}, {Group: group},
		{Group: contract.GroupRef{ID: "inherited", Name: "Inherited", Path: "company"}, InheritedFrom: new("team")},
	}
	h.mu.Unlock()
	type profileResponse struct {
		Member memberResponse   `json:"member"`
		Groups []RecipientGroup `json:"groups"`
		Stats  memberStats      `json:"stats"`
	}
	path := "/members/" + post.Recipient.ID
	result := call[profileResponse](h, "reader", "GET", path, nil, 200)
	require.Equal(t, []RecipientGroup{RecipientGroup(group)}, result.Groups)
	require.Len(t, result.Stats.Partners.ReceivedFrom, 1)
	require.Equal(t, post.Sender.ID, result.Stats.Partners.ReceivedFrom[0].Member.ID)
	require.Equal(t, 30, result.Stats.Partners.ReceivedFrom[0].Points)
	require.Equal(t, []partnerResponse{}, result.Stats.Partners.SentTo)
	require.Len(t, result.Stats.Months, 6)
	calls := len(h.directoryRequests())
	h.setTime("2026-10-19T03:00:59Z")
	require.Equal(t, result, call[profileResponse](h, "reader", "GET", path, nil, 200))
	require.Equal(t, calls, len(h.directoryRequests()))
	call[profileResponse](h, "writer", "GET", path, nil, 200)
	require.Equal(t, calls+1, len(h.directoryRequests()), "profile cache must be caller-isolated")
	h.directoryStatus.Store(409)
	h.setTime("2026-10-19T03:01:00Z")
	degraded := call[profileResponse](h, "reader", "GET", path, nil, 200)
	require.Equal(t, []RecipientGroup{}, degraded.Groups)
	require.Equal(t, result.Stats, degraded.Stats)
	h.directoryStatus.Store(0)
	require.Equal(t, result, call[profileResponse](h, "reader", "GET", path, nil, 200))
}

func TestStatementCloseBlockersMatchClose(t *testing.T) {
	h := newHarness(t)
	h.setTime("2026-09-19T03:00:00Z")
	reversed := call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "september")
	call[any](h, "admin", "DELETE", "/posts/"+reversed.ID, nil, 204)
	h.setTime("2026-10-19T03:00:00Z")
	call[postResponse](h, "writer", "POST", "/posts", postInput(20), 201, "october")
	preview := call[statementResponse](h, "admin", "GET", "/admin/statements/2026-10", nil, 200)
	require.Equal(t, []string{"month_not_ended", "previous_month_open"}, preview.CloseBlockers)
	require.Equal(t, new("2026-09"), preview.PreviousOpenMonth, "even zero-net ledger months must close sequentially")
	require.Equal(t, preview.CloseBlockers[0], call[httpx.Error](h, "admin", "POST", "/admin/statements/2026-10/close", nil, 409).Code)
	closed := call[statementResponse](h, "admin", "POST", "/admin/statements/2026-09/close", nil, 200)
	require.Equal(t, []string{}, closed.CloseBlockers)
	require.Nil(t, closed.PreviousOpenMonth)
	h.setTime("2026-10-31T14:59:59Z")
	preview = call[statementResponse](h, "admin", "GET", "/admin/statements/2026-10", nil, 200)
	require.Equal(t, []string{"month_not_ended"}, preview.CloseBlockers)
	h.setTime("2026-10-31T15:00:00Z")
	preview = call[statementResponse](h, "admin", "GET", "/admin/statements/2026-10", nil, 200)
	require.Equal(t, []string{}, preview.CloseBlockers)
	require.Nil(t, preview.PreviousOpenMonth)
	h.setTime("2026-11-30T15:00:00Z")
	preview = call[statementResponse](h, "admin", "GET", "/admin/statements/2026-11", nil, 200)
	require.Equal(t, []string{"previous_month_open"}, preview.CloseBlockers)
	require.Equal(t, new("2026-10"), preview.PreviousOpenMonth)
	require.Equal(t, preview.CloseBlockers[0], call[httpx.Error](h, "admin", "POST", "/admin/statements/2026-11/close", nil, 409).Code)
	call[statementResponse](h, "admin", "POST", "/admin/statements/2026-10/close", nil, 200)
	closed = call[statementResponse](h, "admin", "POST", "/admin/statements/2026-11/close", nil, 200)
	require.Equal(t, []string{}, closed.CloseBlockers)
	require.Nil(t, closed.PreviousOpenMonth)
	require.Equal(t, closed, call[statementResponse](h, "admin", "GET", "/admin/statements/2026-11", nil, 200))
}

func TestParticipationMergeOrderingAndSummary(t *testing.T) {
	activity := map[string]domain.Participant{
		"sender": {Member: &domain.Member{ID: "sender", PersonID: "sender", DisplayName: "Old name"}, SentPosts: 1, SentPoints: 10, ReceivedPoints: 1},
		"gone":   {Member: &domain.Member{ID: "gone", PersonID: "gone", DisplayName: "Gone"}, ReceivedPosts: 1, ReceivedPoints: 50},
	}
	people := []contract.Person{{ID: "new", DisplayName: "New", Pending: true}, {ID: "sender", DisplayName: "Current name"}}
	result := mergeParticipation("2026-10", activity, people, true)
	require.Equal(t, domain.ParticipationSummary{People: 3, Senders: 1, Recipients: 1, Active: 2}, result.Summary)
	require.Equal(t, "Current name", result.Items[0].DisplayName)
	require.Equal(t, "new", result.Items[1].PersonID)
	require.Nil(t, result.Items[1].Member)
	require.True(t, result.Items[1].Pending)
	require.Equal(t, "gone", result.Items[2].PersonID)
	degraded := mergeParticipation("2026-10", activity, people, false)
	require.Len(t, degraded.Items, 2)
	require.Equal(t, "gone", degraded.Items[0].PersonID)
	require.False(t, degraded.DirectoryAvailable)
}
