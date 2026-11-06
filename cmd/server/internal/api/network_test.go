package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"testing"
	"time"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/peerbonus/cmd/server/internal/directory"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

// Keep the network-specific Directory operations in this slice's fixture.
// Writes use newHarness's real store and existing Directory fixture first.
func networkDirectory(h *harness) {
	h.t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.directoryCalls = append(h.directoryCalls, r.URL.RequestURI())
		require.Equal(h.t, "Bearer "+token("admin"), r.Header.Get("Authorization"))
		require.Equal(h.t, "500", r.URL.Query().Get("limit"))
		if code := h.directoryStatus.Load(); code != 0 && (h.directoryFailurePath == "" || h.directoryFailurePath == r.URL.Path) {
			httpx.WriteJSON(w, int(code), map[string]string{"error": "unavailable"})
			return
		}
		switch r.URL.Path {
		case "/api/directory/v1/groups":
			require.Len(h.t, r.URL.Query(), 1, "groups must not be restricted by parent or name")
			httpx.WriteJSON(w, 200, contract.GroupList{Items: []contract.Group{
				{ID: "sales", Name: "Sales", Path: "company/sales"},
				{ID: "empty", Name: "Empty", Path: "company/empty"},
			}})
		case "/api/directory/v1/people":
			people := []contract.Person{}
			if groupID := r.URL.Query().Get("groupId"); groupID != "" {
				for _, id := range h.groupPeople[groupID] {
					people = append(people, h.people[id])
				}
			} else {
				for _, person := range h.people {
					people = append(people, person)
				}
			}
			httpx.WriteJSON(w, 200, contract.PersonList{Items: people})
		default:
			h.t.Errorf("unexpected network Directory route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	h.t.Cleanup(provider.Close)
	links := link.NewClient(link.NewResolver(nil, "peerbonus", func(string) (string, bool) {
		return provider.URL, true
	}, map[string]link.Required{"directory": {URLEnv: "URL"}}), nil)
	h.api.directory = directory.New(links)
}

func networkFixture(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	post := call[postResponse](h, "writer", "POST", "/posts", postInput(20), 201, "network-post")
	h.setTime("2026-10-19T03:01:00Z")
	call[postResponse](h, "clapper", "POST", "/posts/"+post.ID+"/claps", nil, 201, "network-clap")
	h.mu.Lock()
	version := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	h.people["directory-only"] = contract.Person{ID: "directory-only", DisplayName: "No activity", Pending: true, AvatarVersion: &version}
	h.groupPeople["sales"] = []string{"person-writer", "person-recipient", "directory-only"}
	h.mu.Unlock()
	networkDirectory(h)
	return h
}

func TestAdminNetworkAuthorityAndValidation(t *testing.T) {
	h := newHarness(t)
	call[httpx.Error](h, "", "GET", "/admin/network?month=2026-10", nil, 401)
	call[httpx.Error](h, "reader", "GET", "/admin/network?month=2026-10", nil, 403)
	for _, path := range []string{"/admin/network", "/admin/network?month=2026-1", "/admin/network?month=2026-13", "/admin/network?month=2026-10-01"} {
		err := call[httpx.Error](h, "admin", "GET", path, nil, 400)
		require.Equal(t, "invalid_request", err.Code)
	}
	require.Empty(t, h.directoryRequests())
	networkDirectory(h)
	require.Equal(t, "group_not_found", call[httpx.Error](h, "admin", "GET", "/admin/network?month=2026-10&group=unknown", nil, 404).Code)
	require.Zero(t, h.count("members"))
	require.Zero(t, h.count("periods"))
}

func TestAdminNetworkRosterGroupsAndReadOnly(t *testing.T) {
	h := networkFixture(t)
	members, periods := h.count("members"), h.count("periods")
	path := "/admin/network?month=2026-10"
	result := call[networkResponse](h, "admin", "GET", path, nil, 200)
	require.True(t, result.DirectoryAvailable)
	require.Nil(t, result.Group)
	require.Equal(t, networkSummaryResponse{People: 7, Connected: 3, Isolated: 4, Posts: 1, Claps: 1}, result.Summary)
	require.Equal(t, networkThresholdsResponse(domain.DefaultNetworkThresholds), result.Thresholds)
	require.Equal(t, "2026-10-19T03:01:00Z", result.AsOf.Format(time.RFC3339))
	require.Equal(t, "2026-10-18T15:00:00Z", result.RecentSince.Format(time.RFC3339))
	require.Equal(t, []networkGroupResponse{{ID: "empty", Name: "Empty", Path: "company/empty"}, {ID: "sales", Name: "Sales", Path: "company/sales"}}, result.Groups)
	nodes := map[string]networkNodeResponse{}
	for _, node := range result.Nodes {
		require.NotContains(t, nodes, node.PersonID, "N1")
		nodes[node.PersonID] = node
	}
	isolated := nodes["directory-only"]
	require.Nil(t, isolated.MemberID)
	require.Zero(t, isolated.Connections)
	require.True(t, isolated.Pending)
	require.Equal(t, []domain.NetworkSignal{domain.SignalIsolated}, isolated.Signals)
	require.Equal(t, new("/api/peerbonus/v1/avatars/directory-only?v=2026-10-19T00%3A00%3A00Z"), isolated.AvatarURL)
	require.Equal(t, 21, nodes["person-recipient"].ReceivedPoints)
	require.Equal(t, 1.0, nodes["person-recipient"].Share)
	require.Len(t, result.Edges, 3)
	for i, edge := range result.Edges {
		require.Contains(t, nodes, edge.From, "N2")
		require.Contains(t, nodes, edge.To, "N2")
		require.Less(t, edge.From, edge.To)
		if i > 0 {
			require.Less(t, result.Edges[i-1].From+"/"+result.Edges[i-1].To, edge.From+"/"+edge.To)
		}
	}
	require.Equal(t, []string{"person-clapper", "person-writer", "person-recipient"}, []string{result.Nodes[0].PersonID, result.Nodes[1].PersonID, result.Nodes[2].PersonID})
	// Repeated reads reuse the existing caller-scoped people cache, not a graph cache.
	before := len(h.directoryRequests())
	call[networkResponse](h, "admin", "GET", path, nil, 200)
	require.Len(t, h.directoryRequests(), before+1, "only groups are fetched again")
	filtered := call[networkResponse](h, "admin", "GET", path+"&group=sales", nil, 200)
	require.Equal(t, "sales", filtered.Group.ID)
	require.Equal(t, networkSummaryResponse{People: 3, Connected: 2, Isolated: 1, Posts: 1, Claps: 1}, filtered.Summary)
	require.Len(t, filtered.Edges, 1)
	require.Equal(t, "person-recipient", filtered.Edges[0].From)
	require.Equal(t, "person-writer", filtered.Edges[0].To)
	for _, node := range filtered.Nodes {
		require.Equal(t, nodes[node.PersonID].Connections, node.Connections, "Project-wide degree survives the filter")
	}
	empty := call[networkResponse](h, "admin", "GET", path+"&group=empty", nil, 200)
	require.Empty(t, empty.Nodes)
	require.NotNil(t, empty.Nodes)
	require.Empty(t, empty.Edges)
	require.NotNil(t, empty.Edges)
	require.Equal(t, members, h.count("members"), "N5")
	require.Equal(t, periods, h.count("periods"), "N5")
	// Members absent from Directory remain in the unfiltered view only.
	h.mu.Lock()
	delete(h.people, "person-clapper")
	h.mu.Unlock()
	h.setTime("2026-10-19T03:02:01Z")
	retained := call[networkResponse](h, "admin", "GET", path, nil, 200)
	require.Equal(t, result.Summary, retained.Summary)
	require.Equal(t, result.Edges, retained.Edges)
}

func TestAdminNetworkDirectoryFailureIgnoresFilter(t *testing.T) {
	for _, failedPath := range []string{"/api/directory/v1/groups", "/api/directory/v1/people"} {
		t.Run(failedPath, func(t *testing.T) {
			h := networkFixture(t)
			h.mu.Lock()
			h.directoryFailurePath = failedPath
			h.mu.Unlock()
			h.directoryStatus.Store(503)
			result := call[networkResponse](h, "admin", "GET", "/admin/network?month=2026-10&group=sales", nil, 200)
			require.False(t, result.DirectoryAvailable)
			require.Nil(t, result.Group)
			require.Equal(t, []networkGroupResponse{}, result.Groups)
			require.Equal(t, networkSummaryResponse{People: 3, Connected: 3, Posts: 1, Claps: 1}, result.Summary)
			require.Len(t, result.Edges, 3)
			for _, node := range result.Nodes {
				require.NotNil(t, node.MemberID)
			}
		})
	}
}

func TestMergeNetworkDeduplicatesAndSorts(t *testing.T) {
	facts := domain.NetworkFacts{Members: map[string]domain.NetworkMemberFacts{
		"m-z": {Member: domain.Member{ID: "m-z", PersonID: "a", DisplayName: "Same"}, Connections: 1},
		"m-a": {Member: domain.Member{ID: "m-a", PersonID: "z", DisplayName: "Same"}, Connections: 1},
	}, Edges: []domain.NetworkMemberEdge{{From: "m-a", To: "m-z", Posts: 1}}}
	result := mergeNetwork(facts, []contract.Person{{ID: "a", DisplayName: "Same"}, {ID: "a", DisplayName: "Duplicate"}}, nil, nil, true)
	require.Len(t, result.Nodes, 2)
	require.Equal(t, "a", result.Nodes[0].PersonID)
	require.Equal(t, "z", result.Nodes[1].PersonID)
	require.Equal(t, "a", result.Edges[0].From, "person order may reverse member order")
	require.Equal(t, "z", result.Edges[0].To)
	body, err := json.Marshal(renderNetwork(domain.Network{Nodes: []domain.NetworkNode{{PersonID: "none"}}}))
	require.NoError(t, err)
	require.Contains(t, string(body), `"signals":[]`)
	require.Contains(t, string(body), `"decline":null`)
}

func TestAdminNetworkCurlProof(t *testing.T) {
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl is needed for the optional command-line HTTP proof")
	}
	h := networkFixture(t)
	command := exec.CommandContext(t.Context(), curl, "--fail-with-body", "--silent", "--show-error", "--max-time", "10", "--header", "Authorization: Bearer "+token("admin"), h.server.URL+"/api/peerbonus/v1/admin/network?month=2026-10&group=sales")
	payload, err := command.CombinedOutput()
	require.NoError(t, err, "%s", payload)
	var result networkResponse
	require.NoError(t, json.Unmarshal(payload, &result))
	require.True(t, result.DirectoryAvailable)
	require.Len(t, result.Edges, 1)
	require.True(t, slices.ContainsFunc(result.Nodes, func(node networkNodeResponse) bool { return node.PersonID == "directory-only" && node.Connections == 0 }))
	t.Logf("curl happy-path response: %s", payload)
}
