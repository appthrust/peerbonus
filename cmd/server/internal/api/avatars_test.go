package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/peerbonus/cmd/server/internal/directory"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAvatarRelay(t *testing.T) {
	h := newHarness(t)
	var picture bytes.Buffer
	require.NoError(t, png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	var status atomic.Int32
	var oversized atomic.Bool
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/directory/v1/people/person-recipient/avatar", r.URL.Path)
		require.Empty(t, r.URL.RawQuery, "the browser cache key must not reach Directory")
		require.Equal(t, "Bearer "+token("reader"), r.Header.Get("Authorization"))
		require.Equal(t, "image/*", r.Header.Get("Accept"))
		require.Empty(t, r.Header.Get("If-None-Match"))
		if code := status.Load(); code != 0 {
			httpx.WriteJSON(w, int(code), map[string]string{"error": "directory_error"})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", `"picture"`)
		if oversized.Load() {
			_, _ = w.Write(make([]byte, (1<<20)+1))
			return
		}
		_, _ = w.Write(picture.Bytes())
	}))
	t.Cleanup(provider.Close)
	links := link.NewClient(link.NewResolver(nil, "peerbonus", func(string) (string, bool) {
		return provider.URL, true
	}, map[string]link.Required{"directory": {URLEnv: "URL"}}), nil)
	h.api.directory = directory.New(links)
	path := "/avatars/person-recipient?v=ignored"
	require.Equal(t, "unauthenticated", call[httpx.Error](h, "", "GET", path, nil, 401).Code)
	require.Zero(t, calls.Load())
	for range 2 {
		headers, body := h.request("Bearer "+token("reader"), "GET", path, nil, "", "", 200)
		require.Equal(t, "image/png", headers.Get("Content-Type"))
		require.Equal(t, "private, max-age=86400", headers.Get("Cache-Control"))
		require.Equal(t, picture.Bytes(), body)
	}
	require.EqualValues(t, 2, calls.Load())
	require.Zero(t, h.platformCalls.Load(), "avatar reads must not resolve roles")
	require.Zero(t, h.count("members"), "avatar reads must not create members")
	for _, failure := range []struct {
		upstream int
		status   int
		code     string
	}{
		{404, 404, "not_found"},
		{401, 403, "forbidden"},
		{403, 403, "forbidden"},
		{503, 503, "directory_unavailable"},
	} {
		status.Store(int32(failure.upstream))
		require.Equal(t, failure.code, call[httpx.Error](h, "reader", "GET", path, nil, failure.status).Code)
	}
	status.Store(0)
	oversized.Store(true)
	require.Equal(t, "directory_unavailable", call[httpx.Error](h, "reader", "GET", path, nil, 503).Code)
	provider.Close()
	require.Equal(t, "directory_unavailable", call[httpx.Error](h, "reader", "GET", path, nil, 503).Code)
	h.api.directory = directory.New(nil)
	require.Equal(t, "directory_unavailable", call[httpx.Error](h, "reader", "GET", path, nil, 503).Code)
}

func TestAvatarURLRendering(t *testing.T) {
	version, err := time.Parse(time.RFC3339, "2026-10-19T12:34:56+09:00")
	require.NoError(t, err)
	person := contract.Person{ID: "person/a b", AvatarVersion: &version}
	input := memberInput(person, version)
	require.Equal(t, &version, input.AvatarVersion)
	require.Equal(t, &version, input.DirectorySyncedAt)
	a := &API{}
	member := a.renderMember(domain.Member{PersonID: person.ID, AvatarVersion: input.AvatarVersion})
	want := "/api/peerbonus/v1/avatars/person%2Fa%20b?v=2026-10-19T12%3A34%3A56%2B09%3A00"
	require.Equal(t, &want, member.AvatarURL)
	candidates := recipientCandidates([]contract.Person{person}, "self", "subject", 10)
	require.Len(t, candidates, 1)
	require.Equal(t, &want, candidates[0].AvatarURL)
	for _, value := range []any{a.renderMember(domain.Member{}), candidate{}, participantResponse{}} {
		body, err := json.Marshal(value)
		require.NoError(t, err)
		require.Contains(t, string(body), `"avatarUrl":null`)
	}
}

func TestMemberAvatarRefreshAndCandidates(t *testing.T) {
	h := newHarness(t)
	version := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	h.mu.Lock()
	for id, person := range h.people {
		person.AvatarVersion = &version
		h.people[id] = person
	}
	h.mu.Unlock()
	me := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.Equal(t, avatarURL("person-writer", &version), me.Member.AvatarURL)
	search := call[struct{ Items []candidate }](h, "writer", "GET", "/members", nil, 200)
	for _, person := range search.Items {
		require.Equal(t, avatarURL(person.PersonID, &version), person.AvatarURL)
	}
	post := call[postResponse](h, "writer", "POST", "/posts", postInput(20), 201, "avatar-post")
	require.Equal(t, avatarURL("person-recipient", &version), post.Recipient.AvatarURL)
	recipient, err := h.api.store.MemberByID(t.Context(), post.Recipient.ID)
	require.NoError(t, err)
	require.Equal(t, &version, recipient.DirectorySyncedAt)
	picker := call[recipientsResponse](h, "writer", "GET", "/recipients", nil, 200)
	require.Len(t, picker.Recent, 1)
	require.Equal(t, post.Recipient.AvatarURL, picker.Recent[0].AvatarURL)
	for _, person := range picker.Everyone.Items {
		require.Equal(t, avatarURL(person.PersonID, &version), person.AvatarURL)
	}
	h.directoryStatus.Store(503)
	degraded := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.Equal(t, me.Member.AvatarURL, degraded.Member.AvatarURL)
	h.directoryStatus.Store(0)
	updated := version.Add(time.Hour)
	h.mu.Lock()
	person := h.people["person-recipient"]
	person.AvatarVersion = &updated
	h.people[person.ID] = person
	h.mu.Unlock()
	h.setTime("2026-11-01T00:00:00Z")
	closed := call[statementResponse](h, "admin", "POST", "/admin/statements/2026-10/close", nil, 200)
	require.True(t, closed.DirectoryRefreshed)
	require.Len(t, closed.Lines, 1)
	require.Equal(t, avatarURL(person.ID, &updated), closed.Lines[0].Member.AvatarURL)
	member, err := h.api.store.MemberByPersonID(t.Context(), person.ID)
	require.NoError(t, err)
	require.True(t, updated.Equal(*member.AvatarVersion))
	require.Equal(t, h.api.now().UTC(), *member.DirectorySyncedAt)
}

func TestParticipationAvatarsIncludeDirectoryOnlyPeople(t *testing.T) {
	version := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	stored := version.Add(-time.Hour)
	activity := map[string]domain.Participant{
		"member": {Member: &domain.Member{ID: "member", PersonID: "known", AvatarVersion: &stored}},
	}
	people := []contract.Person{{ID: "known", AvatarVersion: &version}, {ID: "directory-only", AvatarVersion: &version}}
	a := &API{}
	result := a.renderParticipation(mergeParticipation("2026-10", activity, people, true))
	require.Len(t, result.Items, 2)
	for _, person := range result.Items {
		require.Equal(t, avatarURL(person.PersonID, &version), person.AvatarURL)
		if person.PersonID == "directory-only" {
			require.Nil(t, person.Member)
		}
	}
	degraded := a.renderParticipation(mergeParticipation("2026-10", activity, nil, false))
	require.Len(t, degraded.Items, 1)
	require.Equal(t, avatarURL("known", &stored), degraded.Items[0].AvatarURL)
}
