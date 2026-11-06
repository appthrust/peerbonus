package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/peerbonus/cmd/server/internal/directory"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
	"github.com/stretchr/testify/require"
)

func (h *harness) memberSyncRequests() []string {
	h.t.Helper()
	var ids []string
	for _, path := range h.directoryRequests() {
		u, err := url.Parse(path)
		require.NoError(h.t, err)
		if u.Path == "/api/directory/v1/people" && u.Query().Get("ids") != "" {
			ids = append(ids, u.Query().Get("ids"))
		}
	}
	return ids
}

func TestMeRefreshesStoredMembers(t *testing.T) {
	h := newHarness(t)
	now := h.api.now().UTC()
	stored, err := h.api.store.UpsertMember(t.Context(), store.MemberInput{PersonID: "person-recipient", DisplayName: "Old name", Email: "old@example.test"}, now, false)
	require.NoError(t, err)
	require.Nil(t, stored.AvatarVersion)
	require.Nil(t, stored.DirectorySyncedAt)
	version := now.Add(-time.Hour)
	h.mu.Lock()
	person := h.people[stored.PersonID]
	person.AvatarVersion = &version
	h.people[person.ID] = person
	h.mu.Unlock()

	me := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.True(t, me.Directory.Linked)
	profile := call[struct{ Member memberResponse }](h, "writer", "GET", "/members/"+stored.ID, nil, 200)
	require.Equal(t, avatarURL(person.ID, &version), profile.Member.AvatarURL)
	require.Equal(t, person.DisplayName, profile.Member.DisplayName)
	require.Equal(t, person.Email, profile.Member.Email)
	require.Equal(t, []string{person.ID}, h.memberSyncRequests(), "the caller was already refreshed by /me")
	refreshed, err := h.api.store.MemberByID(t.Context(), stored.ID)
	require.NoError(t, err)
	require.Equal(t, &now, refreshed.DirectorySyncedAt)

	// A new stale row makes another fetch observable, even though the first
	// batch has already refreshed every pre-existing member.
	_, err = h.api.store.UpsertMember(t.Context(), store.MemberInput{PersonID: "person-clapper", DisplayName: "Old clapper"}, now, false)
	require.NoError(t, err)
	h.setTime("2026-10-19T03:00:59Z")
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	call[meResponse](h, "reader", "GET", "/me", nil, 200)
	require.Equal(t, []string{person.ID}, h.memberSyncRequests(), "the cooldown is shared by all callers")
	h.setTime("2026-10-19T03:01:00Z")
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.Len(t, h.memberSyncRequests(), 1, "more than 60 seconds must pass before another attempt")
	h.setTime("2026-10-19T03:01:01Z")
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.Equal(t, []string{person.ID, "person-clapper"}, h.memberSyncRequests())
}

func TestMeMemberRefreshFailureIsBestEffort(t *testing.T) {
	h := newHarness(t)
	stored, err := h.api.store.UpsertMember(t.Context(), store.MemberInput{PersonID: "person-recipient", DisplayName: "Stored name", Email: "stored@example.test"}, h.api.now(), false)
	require.NoError(t, err)
	h.mu.Lock()
	h.directoryFailurePath = "/api/directory/v1/people"
	h.mu.Unlock()
	h.directoryStatus.Store(503)

	me := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.True(t, me.Directory.Linked)
	require.Equal(t, []string{stored.PersonID}, h.memberSyncRequests())
	unchanged, err := h.api.store.MemberByID(t.Context(), stored.ID)
	require.NoError(t, err)
	require.Equal(t, stored, unchanged)
	call[meResponse](h, "reader", "GET", "/me", nil, 200)
	require.Len(t, h.memberSyncRequests(), 1, "failed refreshes also observe the cooldown")

	h.directoryStatus.Store(0)
	h.setTime("2026-10-19T03:01:01Z")
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.Equal(t, []string{stored.PersonID, stored.PersonID}, h.memberSyncRequests())
	refreshed, err := h.api.store.MemberByID(t.Context(), stored.ID)
	require.NoError(t, err)
	require.Equal(t, "Pending Recipient", refreshed.DisplayName)
}

func TestMeMemberRefreshBatchesMissingPeople(t *testing.T) {
	h := newHarness(t)
	_, err := h.db.ExecContext(t.Context(), `INSERT INTO members (id,person_id,display_name,email)
 SELECT 'missing-' || n, 'missing-person-' || n, 'Missing member', 'missing@example.test' FROM generate_series(1,101) n`)
	require.NoError(t, err)
	now := h.api.now().UTC()
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	requests := h.memberSyncRequests()
	require.Len(t, requests, 1)
	require.Len(t, strings.Split(requests[0], ","), 100)
	var stamped, unchanged int
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT
 COUNT(*) FILTER (WHERE directory_synced_at=$1),
 COUNT(*) FILTER (WHERE display_name='Missing member' AND email='missing@example.test' AND avatar_version IS NULL AND subject IS NULL)
 FROM members WHERE person_id LIKE 'missing-person-%'`, now).Scan(&stamped, &unchanged))
	require.Equal(t, 100, stamped)
	require.Equal(t, 101, unchanged)
	h.setTime("2026-10-19T03:01:01Z")
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	requests = h.memberSyncRequests()
	require.Len(t, requests, 2)
	require.NotContains(t, strings.Split(requests[0], ","), requests[1], "missing people must not monopolize later batches")
	require.Len(t, strings.Split(requests[1], ","), 1)
}

func TestMeDegradedDoesNotStampNeverSyncedMember(t *testing.T) {
	h := newHarness(t)
	stored, err := h.api.store.UpsertMember(t.Context(), store.MemberInput{PersonID: "person-writer", Subject: new("writer"), DisplayName: "Cached writer"}, h.api.now(), false)
	require.NoError(t, err)
	h.directoryStatus.Store(503)
	me := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.False(t, me.Directory.Linked)
	member, err := h.api.store.MemberByID(t.Context(), stored.ID)
	require.NoError(t, err)
	require.Nil(t, member.DirectorySyncedAt)
	require.NotNil(t, member.LastSeenAt)
	require.Empty(t, h.memberSyncRequests())
}

func TestMeMemberRefreshReservesCooldownBeforeDirectoryCall(t *testing.T) {
	h := newHarness(t)
	_, err := h.api.store.UpsertMember(t.Context(), store.MemberInput{PersonID: "person-recipient", DisplayName: "Cached recipient"}, h.api.now(), false)
	require.NoError(t, err)
	started, release := make(chan struct{}), make(chan struct{})
	var batches atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/directory/v1/me":
			subject := subjectOf(r.Header.Get("Authorization"))
			httpx.WriteJSON(w, 200, contract.MeResponse{Person: contract.Person{ID: "person-" + subject, Subject: &subject, DisplayName: subject}})
		case "/api/directory/v1/people":
			if batches.Add(1) == 1 {
				close(started)
			}
			select {
			case <-release:
				httpx.WriteJSON(w, 200, contract.PersonList{})
			case <-r.Context().Done():
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	t.Cleanup(func() { close(release) })
	links := link.NewClient(link.NewResolver(nil, "peerbonus", func(string) (string, bool) {
		return provider.URL, true
	}, map[string]link.Required{"directory": {URLEnv: "URL"}}), nil)
	h.api.directory = directory.New(links)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	first, err := http.NewRequestWithContext(ctx, http.MethodGet, h.server.URL+"/api/peerbonus/v1/me", nil)
	require.NoError(t, err)
	first.Header.Set("Authorization", "Bearer "+token("writer"))
	done := make(chan error, 1)
	go func() {
		response, err := h.server.Client().Do(first)
		if err == nil {
			_, err = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first /me did not reach the Directory refresh")
	}
	second, err := http.NewRequestWithContext(ctx, http.MethodGet, h.server.URL+"/api/peerbonus/v1/me", nil)
	require.NoError(t, err)
	second.Header.Set("Authorization", "Bearer "+token("reader"))
	response, err := h.server.Client().Do(second)
	require.NoError(t, err, "another /me must finish while the first refresh is still blocked")
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.EqualValues(t, 1, batches.Load())
	cancel()
	<-done
}
