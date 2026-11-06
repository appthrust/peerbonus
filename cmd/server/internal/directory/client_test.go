package directory

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/stretchr/testify/require"
)

func testClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	links := link.NewClient(link.NewResolver(nil, "peerbonus", func(key string) (string, bool) {
		if key == "PEERBONUS_DIRECTORY_URL" {
			return server.URL, true
		}
		return "", false
	}, map[string]link.Required{"directory": {URLEnv: "PEERBONUS_DIRECTORY_URL"}}), nil)
	return New(links)
}
func TestMeSearchAndPeopleBatchingForwardBearer(t *testing.T) {
	bearer := "bEaReR  unchanged"
	batches := []int{}
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, bearer, r.Header.Get("Authorization"))
		require.Equal(t, "GET", r.Method)
		switch r.URL.Path {
		case "/api/directory/v1/me":
			httpx.WriteJSON(w, 200, contract.MeResponse{Person: contract.Person{ID: "self"}})
		case "/api/directory/v1/people":
			items := []contract.Person{}
			if ids := r.URL.Query().Get("ids"); ids != "" {
				parts := strings.Split(ids, ",")
				batches = append(batches, len(parts))
				require.LessOrEqual(t, len(parts), 100)
				for _, id := range parts {
					items = append(items, contract.Person{ID: id})
				}
			} else {
				require.Equal(t, "田中 & friends", r.URL.Query().Get("q"))
				require.Equal(t, "12", r.URL.Query().Get("limit"))
				items = append(items, contract.Person{ID: "found"})
			}
			httpx.WriteJSON(w, 200, contract.PersonList{Items: items})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	person, err := client.Me(t.Context(), bearer)
	require.NoError(t, err)
	require.Equal(t, "self", person.ID)
	people, err := client.Search(t.Context(), bearer, "田中 & friends", 12)
	require.NoError(t, err)
	require.Equal(t, "found", people[0].ID)
	ids := make([]string, 205)
	for i := range ids {
		ids[i] = fmt.Sprintf("person-%d", i)
	}
	people, err = client.PeopleByIDs(t.Context(), bearer, ids)
	require.NoError(t, err)
	require.Len(t, people, 205)
	require.Equal(t, []int{100, 100, 5}, batches)
	people, err = client.PeopleByIDs(t.Context(), bearer, nil)
	require.NoError(t, err)
	require.Empty(t, people)
	require.Equal(t, []int{100, 100, 5}, batches)
}
func TestPeopleBatchFailurePreservesSuccessfulProfiles(t *testing.T) {
	calls := 0
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		httpx.WriteJSON(w, 200, contract.PersonList{Items: []contract.Person{{ID: "first"}}})
	}))
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	people, err := client.PeopleByIDs(t.Context(), "Bearer token", ids)
	require.ErrorIs(t, err, link.ErrUnavailable)
	require.Equal(t, []contract.Person{{ID: "first"}}, people)
}
func TestDirectoryRejectedAndAbsent(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, 409, map[string]string{"error": "identity_conflict", "message": "Conflicting identity"})
	}))
	_, err := client.Me(t.Context(), "Bearer token")
	var rejected *link.RejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, 409, rejected.Status)
	require.Equal(t, "identity_conflict", rejected.Code)
	_, err = New(nil).Me(t.Context(), "Bearer token")
	require.ErrorIs(t, err, link.ErrAbsent)
	_, err = New(nil).Search(t.Context(), "Bearer token", "a", 10)
	require.ErrorIs(t, err, link.ErrAbsent)
	_, err = New(nil).PeopleByIDs(t.Context(), "Bearer token", []string{"a"})
	require.ErrorIs(t, err, link.ErrAbsent)
	_, err = New(nil).Profile(t.Context(), "Bearer token", "a")
	require.ErrorIs(t, err, link.ErrAbsent)
	_, err = New(nil).PeopleInGroup(t.Context(), "Bearer token", "group", 100)
	require.ErrorIs(t, err, link.ErrAbsent)
	_, err = New(nil).ListAll(t.Context(), "Bearer token", 500)
	require.ErrorIs(t, err, link.ErrAbsent)
}

func TestProfileGroupAndAllPeople(t *testing.T) {
	bearer := "bEaReR  unchanged"
	calls := 0
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, bearer, r.Header.Get("Authorization"))
		require.Equal(t, http.MethodGet, r.Method)
		switch r.URL.Path {
		case "/api/directory/v1/people/person/a b":
			require.Equal(t, "/api/directory/v1/people/person%2Fa%20b", r.URL.EscapedPath())
			httpx.WriteJSON(w, 200, contract.PersonProfile{
				Person:      contract.Person{ID: "person/a b"},
				Memberships: []contract.PersonMembership{{Group: contract.GroupRef{ID: "team", Name: "Team", Path: "org/team"}}},
			})
		case "/api/directory/v1/people":
			require.False(t, r.URL.Query().Has("q"))
			if r.URL.Query().Has("groupId") {
				require.Equal(t, "team & friends", r.URL.Query().Get("groupId"))
				require.Equal(t, "100", r.URL.Query().Get("limit"))
			} else {
				require.Equal(t, "500", r.URL.Query().Get("limit"))
			}
			httpx.WriteJSON(w, 200, contract.PersonList{Items: []contract.Person{{ID: "found"}}})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	profile, err := client.Profile(t.Context(), bearer, "person/a b")
	require.NoError(t, err)
	require.Equal(t, "org/team", profile.Memberships[0].Group.Path)
	people, err := client.PeopleInGroup(t.Context(), bearer, "team & friends", 100)
	require.NoError(t, err)
	require.Equal(t, "found", people[0].ID)
	people, err = client.ListAll(t.Context(), bearer, 500)
	require.NoError(t, err)
	require.Equal(t, "found", people[0].ID)
	people, err = client.Search(t.Context(), bearer, "", 500)
	require.NoError(t, err)
	require.Equal(t, "found", people[0].ID)
	require.Equal(t, 4, calls)
}

func TestProfileRejectsMismatchedPerson(t *testing.T) {
	client := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, 200, contract.PersonProfile{Person: contract.Person{ID: "other"}})
	}))
	_, err := client.Profile(t.Context(), "Bearer token", "self")
	require.ErrorIs(t, err, link.ErrUnavailable)
}
