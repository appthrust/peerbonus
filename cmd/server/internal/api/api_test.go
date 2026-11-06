package api

import (
	"bytes"
	"cmp"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/appserver-go/appserver/pgstore/testutil"
	"github.com/appthrust/appserver-go/appserver/platformclient"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
	"github.com/stretchr/testify/require"
)

type harness struct {
	t                    *testing.T
	server               *httptest.Server
	api                  *API
	db                   *sql.DB
	clock                atomic.Int64
	directoryStatus      atomic.Int32
	platformCalls        atomic.Int32
	mu                   sync.Mutex
	people               map[string]contract.Person
	memberships          map[string][]contract.PersonMembership
	groupPeople          map[string][]string
	directoryCalls       []string
	directoryFailurePath string
	lastBearer           string
	batches              []int
}

func token(subject string) string {
	claims, _ := json.Marshal(map[string]string{"sub": subject, "email": subject + "@example.test", "name": subject})
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims) + "."
}
func subjectOf(bearer string) string {
	parts := strings.Split(bearer, ".")
	if len(parts) != 3 {
		return ""
	}
	body, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Subject string `json:"sub"`
	}
	_ = json.Unmarshal(body, &claims)
	return claims.Subject
}
func newHarness(t *testing.T) *harness {
	t.Helper()
	database, dsn := testutil.Database(t, store.Open)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	h := &harness{t: t, db: db, people: map[string]contract.Person{}, memberships: map[string][]contract.PersonMembership{}, groupPeople: map[string][]string{}}
	h.setTime("2026-10-19T03:00:00Z")
	for _, subject := range []string{"writer", "clapper", "admin", "reader", "subject"} {
		sub := subject
		h.people["person-"+subject] = contract.Person{ID: "person-" + subject, Subject: &sub, DisplayName: "Directory " + subject, Email: subject + "@directory.test"}
	}
	h.people["person-recipient"] = contract.Person{ID: "person-recipient", DisplayName: "Pending Recipient", Email: "recipient@directory.test", Pending: true}
	directory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.lastBearer = r.Header.Get("Authorization")
		h.directoryCalls = append(h.directoryCalls, r.URL.RequestURI())
		if status := int(h.directoryStatus.Load()); status != 0 && (h.directoryFailurePath == "" || h.directoryFailurePath == r.URL.Path) {
			httpx.WriteJSON(w, status, map[string]string{"error": "identity_conflict", "message": "Email belongs to a different subject"})
			return
		}
		switch r.URL.Path {
		case "/api/directory/v1/me":
			person, ok := h.people["person-"+subjectOf(h.lastBearer)]
			if !ok {
				httpx.WriteError(w, httpx.NotFound(), "directory")
				return
			}
			httpx.WriteJSON(w, 200, contract.MeResponse{Person: person})
		case "/api/directory/v1/people":
			items := []contract.Person{}
			if groupID := r.URL.Query().Get("groupId"); groupID != "" {
				for _, id := range h.groupPeople[groupID] {
					if person, ok := h.people[id]; ok {
						items = append(items, person)
					}
				}
			} else if ids := r.URL.Query().Get("ids"); ids != "" {
				ids := strings.Split(ids, ",")
				h.batches = append(h.batches, len(ids))
				if len(ids) > 100 {
					httpx.WriteError(w, httpx.Invalid("too many IDs"), "directory")
					return
				}
				for _, id := range ids {
					if person, ok := h.people[id]; ok {
						items = append(items, person)
					}
				}
			} else {
				for _, person := range h.people {
					items = append(items, person)
				}
			}
			slices.SortFunc(items, func(a, b contract.Person) int {
				if name := cmp.Compare(a.DisplayName, b.DisplayName); name != 0 {
					return name
				}
				return cmp.Compare(a.ID, b.ID)
			})
			if limit, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && limit < len(items) {
				items = items[:limit]
			}
			httpx.WriteJSON(w, 200, contract.PersonList{Items: items})
		default:
			if id, ok := strings.CutPrefix(r.URL.Path, "/api/directory/v1/people/"); ok {
				person, found := h.people[id]
				if !found {
					httpx.WriteError(w, httpx.NotFound(), "directory")
					return
				}
				httpx.WriteJSON(w, 200, contract.PersonProfile{Person: person, Memberships: h.memberships[id]})
				return
			}
			t.Errorf("unexpected Directory route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(directory.Close)
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.platformCalls.Add(1)
		if r.Header.Get("Authorization") == "" {
			t.Error("platform request missing bearer")
		}
		switch r.URL.Path {
		case "/api/v1/projects/pj-test/app-installations/peerbonus":
			httpx.WriteJSON(w, 200, map[string]any{"id": "peerbonus", "links": []any{}})
		case "/api/v1/projects/pj-test":
			httpx.WriteJSON(w, 200, map[string]string{"teamSlug": "test-team"})
		case "/api/v1/teams":
			role := "Viewer"
			if subjectOf(r.Header.Get("Authorization")) == "admin" {
				role = "Admin"
			}
			httpx.WriteJSON(w, 200, map[string]any{"items": []any{map[string]string{"id": "team-id", "slug": "test-team", "role": role}}})
		default:
			t.Errorf("unexpected platform route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(platform.Close)
	auth, err := identity.New(identity.Trust{})
	require.NoError(t, err)
	upstream := platformclient.New(platform.URL, "pj-test")
	links := link.NewClient(link.NewResolver(upstream, "peerbonus", func(key string) (string, bool) {
		if key == "PEERBONUS_DIRECTORY_URL" {
			return directory.URL, true
		}
		return "", false
	}, map[string]link.Required{"directory": {URLEnv: "PEERBONUS_DIRECTORY_URL", PublicURLEnv: "PEERBONUS_DIRECTORY_PUBLIC_URL"}}), nil)
	h.api = New(database, auth, upstream, links, "https://directory.example.test", SlackOptions{}).(*API)
	h.api.now = func() time.Time { return time.Unix(0, h.clock.Load()) }
	h.server = httptest.NewServer(h.api)
	t.Cleanup(h.server.Close)
	return h
}
func (h *harness) setTime(value string) {
	h.t.Helper()
	now, err := time.Parse(time.RFC3339, value)
	require.NoError(h.t, err)
	h.clock.Store(now.UnixNano())
}
func (h *harness) count(table string) int {
	h.t.Helper()
	var count int
	require.NoError(h.t, h.db.QueryRowContext(h.t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count))
	return count
}
func (h *harness) request(bearer, method, path string, body io.Reader, key, contentType string, status int) (http.Header, []byte) {
	h.t.Helper()
	r, err := http.NewRequest(method, h.server.URL+"/api/peerbonus/v1"+path, body)
	require.NoError(h.t, err)
	if bearer != "" {
		r.Header.Set("Authorization", bearer)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	response, err := h.server.Client().Do(r)
	require.NoError(h.t, err)
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	require.NoError(h.t, err)
	require.Equal(h.t, status, response.StatusCode, "%s %s: %s", method, path, payload)
	if method == http.MethodGet && strings.HasPrefix(path, "/avatars/") && status == http.StatusOK {
		require.Equal(h.t, "private, max-age=86400", response.Header.Get("Cache-Control"))
	} else {
		require.Equal(h.t, "no-store", response.Header.Get("Cache-Control"))
	}
	return response.Header, payload
}
func call[T any](h *harness, subject, method, path string, body any, status int, key ...string) T {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = bytes.NewReader(encoded)
	}
	bearer := ""
	if subject != "" {
		bearer = "Bearer " + token(subject)
	}
	idempotency := ""
	if len(key) > 0 {
		idempotency = key[0]
	}
	_, payload := h.request(bearer, method, path, reader, idempotency, "application/json", status)
	var result T
	if status != 204 {
		require.NoError(h.t, json.Unmarshal(payload, &result), "%s", payload)
	}
	return result
}
func postInput(points int) map[string]any {
	return map[string]any{"recipientPersonId": "person-recipient", "points": points, "message": " Thank you! "}
}

func TestMeResolveDegradedAndRejected(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, "unauthenticated", call[httpx.Error](h, "", "GET", "/me", nil, 401).Code)
	me := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.True(t, me.Directory.Linked)
	require.False(t, me.Roles.PeerbonusAdmin)
	require.Equal(t, "Directory writer", me.Member.DisplayName)
	require.Equal(t, "writer@directory.test", me.Member.Email)
	require.False(t, me.Member.Pending)
	require.Equal(t, "https://directory.example.test/people/person-writer", *me.Member.DirectoryURL)
	require.Equal(t, 400, me.Stats.Period.Remaining)
	require.Equal(t, 120, me.Period.MaxPointsPerPost)
	require.Equal(t, 0, h.count("periods"))
	var seen, synced time.Time
	require.NoError(t, h.db.QueryRowContext(t.Context(), "SELECT last_seen_at,directory_synced_at FROM members WHERE id=$1", me.Member.ID).Scan(&seen, &synced))
	require.Equal(t, h.clock.Load(), seen.UnixNano())
	require.Equal(t, h.clock.Load(), synced.UnixNano())
	program := call[map[string]any](h, "writer", "GET", "/program", nil, 200)
	require.NotContains(t, program, "pointUnitValue")
	require.NotContains(t, program, "currency")
	h.directoryStatus.Store(503)
	h.setTime("2026-10-19T03:01:00Z")
	degraded := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.False(t, degraded.Directory.Linked)
	require.Equal(t, me.Member, degraded.Member)
	var degradedSynced time.Time
	require.NoError(t, h.db.QueryRowContext(t.Context(), "SELECT last_seen_at,directory_synced_at FROM members WHERE id=$1", me.Member.ID).Scan(&seen, &degradedSynced))
	require.Equal(t, h.clock.Load(), seen.UnixNano())
	require.Equal(t, synced, degradedSynced, "a cached upsert must not claim a Directory refresh")
	require.Equal(t, "directory_unavailable", call[httpx.Error](h, "unknown", "GET", "/me", nil, 503).Code)
	h.directoryStatus.Store(409)
	require.Equal(t, "identity_conflict", call[httpx.Error](h, "writer", "GET", "/me", nil, 409).Code)
	require.Equal(t, 0, h.count("periods"))
}
func TestSignedIdentityAndVerbatimDirectoryBearer(t *testing.T) {
	h := newHarness(t)
	issuer := testutil.NewOIDCIssuer(t)
	auth, err := identity.New(identity.Trust{OIDCIssuer: issuer.Server.URL, OIDCCAPEM: issuer.CAPEM, OIDCAudiences: []string{"appthrust-platform-api"}})
	require.NoError(t, err)
	h.api.identity = auth
	bearer := "bEaReR  " + strings.TrimPrefix(issuer.Bearer(t), "Bearer ")
	_, body := h.request(bearer, "GET", "/me", nil, "", "", 200)
	var me meResponse
	require.NoError(t, json.Unmarshal(body, &me))
	require.Equal(t, "person-subject", me.Member.PersonID)
	h.mu.Lock()
	got := h.lastBearer
	h.mu.Unlock()
	require.Equal(t, bearer, got)
	h.request("Bearer "+token("subject"), "GET", "/me", nil, "", "", 401)
}
func TestMembersSearchIsReadOnly(t *testing.T) {
	h := newHarness(t)
	result := call[struct {
		Items []candidate `json:"items"`
	}](h, "writer", "GET", "/members?q=person&limit=20", nil, 200)
	require.Len(t, result.Items, 5)
	for _, person := range result.Items {
		require.NotEqual(t, "person-writer", person.PersonID)
		require.NotNil(t, person.Groups)
	}
	require.Equal(t, 0, h.count("members"))
	require.Equal(t, 0, h.count("periods"))
	require.Equal(t, int32(0), h.platformCalls.Load())
	for _, path := range []string{"/members?q=" + strings.Repeat("a", 65), "/members?q=a&limit=0", "/members?q=a&limit=21"} {
		call[httpx.Error](h, "writer", "GET", path, nil, 400)
	}
	h.directoryStatus.Store(503)
	require.Equal(t, "directory_unavailable", call[httpx.Error](h, "writer", "GET", "/members?q=person", nil, 503).Code)
}
func TestPostResolutionReplayClapsAndDeletion(t *testing.T) {
	h := newHarness(t)
	post := call[postResponse](h, "writer", "POST", "/posts", postInput(120), 201, "post-1")
	require.Equal(t, "Thank you!", post.Message)
	require.NotNil(t, post.Values)
	require.True(t, post.Recipient.Pending)
	require.Equal(t, "person-recipient", post.Recipient.PersonID)
	require.Equal(t, 2, h.count("ledger"))
	clapPath := "/posts/" + post.ID + "/claps"
	type clapResult struct {
		Clap clapResponse `json:"clap"`
		Post postResponse `json:"post"`
	}
	first := call[clapResult](h, "clapper", "POST", clapPath, nil, 201, "clap-1")
	replay := call[clapResult](h, "clapper", "POST", clapPath, nil, 201, "clap-1")
	require.Equal(t, first, replay)
	call[clapResult](h, "clapper", "POST", clapPath, nil, 201, "clap-2")
	replayed := call[postResponse](h, "writer", "POST", "/posts", postInput(120), 201, "post-1")
	require.Equal(t, post.ID, replayed.ID)
	require.Equal(t, 2, replayed.ClapCount)
	require.Equal(t, 4, replayed.ClapPoints)
	require.Equal(t, []memberResponse{first.Clap.Member, first.Clap.Member}, replayed.Clappers, "clappers follows clap events, including repeated members")
	require.Equal(t, 8, h.count("ledger"))
	require.Equal(t, 1, h.count("posts"))
	require.Equal(t, "own_post", call[httpx.Error](h, "writer", "POST", clapPath, nil, 403, "own-clap").Code)
	claps := call[struct {
		Items []clapResponse `json:"items"`
	}](h, "writer", "GET", clapPath, nil, 200)
	require.Len(t, claps.Items, 2)
	me := call[meResponse](h, "clapper", "GET", "/me", nil, 200)
	require.Equal(t, 396, me.Stats.Period.Remaining)
	require.Equal(t, 2, me.Stats.Period.Claps)
	profile := call[struct {
		Member memberResponse `json:"member"`
		Stats  memberStats    `json:"stats"`
	}](h, "reader", "GET", "/members/"+post.Recipient.ID, nil, 200)
	require.Equal(t, 122, profile.Stats.Month.ReceivedPoints)
	for _, path := range []string{"/admin/program", "/admin/statements", "/admin/statements/2026-10", "/admin/statements/2026-10/export.csv", "/admin/members?month=2026-10"} {
		require.Equal(t, "forbidden", call[httpx.Error](h, "writer", "GET", path, nil, 403).Code)
	}
	call[httpx.Error](h, "writer", "PATCH", "/admin/program", map[string]any{"weeklyAllowance": 500}, 403)
	call[httpx.Error](h, "writer", "POST", "/admin/statements/2026-10/close", nil, 403)
	call[httpx.Error](h, "writer", "DELETE", "/posts/"+post.ID, nil, 403)
	call[any](h, "admin", "DELETE", "/posts/"+post.ID, nil, 204)
	call[httpx.Error](h, "writer", "GET", "/posts/"+post.ID, nil, 404)
	call[httpx.Error](h, "clapper", "POST", clapPath, nil, 404, "deleted-clap")
	call[httpx.Error](h, "admin", "DELETE", "/posts/"+post.ID, nil, 404)
	me = call[meResponse](h, "clapper", "GET", "/me", nil, 200)
	require.Equal(t, 400, me.Stats.Period.Remaining)
	require.Equal(t, 16, h.count("ledger"))
}
func TestRecipientResolutionFailureModes(t *testing.T) {
	h := newHarness(t)
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	h.directoryStatus.Store(503)
	require.Equal(t, "directory_unavailable", call[httpx.Error](h, "writer", "POST", "/posts", postInput(5), 503, "unknown").Code)
	h.directoryStatus.Store(0)
	call[postResponse](h, "writer", "POST", "/posts", postInput(5), 201, "known")
	h.directoryStatus.Store(503)
	call[postResponse](h, "writer", "POST", "/posts", postInput(5), 201, "cached")
	h.directoryStatus.Store(409)
	require.Equal(t, "identity_conflict", call[httpx.Error](h, "writer", "POST", "/posts", postInput(5), 409, "rejected").Code)
	h.directoryStatus.Store(0)
	h.mu.Lock()
	delete(h.people, "person-recipient")
	h.mu.Unlock()
	require.Equal(t, "recipient_not_found", call[httpx.Error](h, "writer", "POST", "/posts", postInput(5), 404, "gone").Code)
	input := postInput(5)
	input["recipientPersonId"] = "person-writer"
	require.Equal(t, "self_target", call[httpx.Error](h, "writer", "POST", "/posts", input, 400, "self").Code)
}
func TestProgramFrozenPeriodAndRequestValidation(t *testing.T) {
	h := newHarness(t)
	call[meResponse](h, "writer", "GET", "/me", nil, 200)
	call[programAdminResponse](h, "admin", "PATCH", "/admin/program", map[string]any{"weeklyAllowance": 20, "maxPointsPerPost": 10, "values": []any{map[string]any{"key": "thanks", "label": "Thanks"}}}, 200)
	call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "first")
	call[programAdminResponse](h, "admin", "PATCH", "/admin/program", map[string]any{"weeklyAllowance": 100, "maxPointsPerPost": 90}, 200)
	me := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.Equal(t, 90, me.Program.MaxPointsPerPost)
	require.Equal(t, 10, me.Period.MaxPointsPerPost)
	require.Equal(t, 20, me.Stats.Period.Allowance)
	require.True(t, me.Program.TimeZoneLocked)
	call[httpx.Error](h, "writer", "POST", "/posts", postInput(11), 400, "too-many")
	call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "second")
	insufficient := call[struct {
		Error     string `json:"error"`
		Remaining int    `json:"remaining"`
		Required  int    `json:"required"`
	}](h, "writer", "POST", "/posts", postInput(1), 409, "insufficient")
	require.Equal(t, "insufficient_points", insufficient.Error)
	require.Equal(t, 0, insufficient.Remaining)
	require.Equal(t, 1, insufficient.Required)
	require.Equal(t, "timezone_locked", call[httpx.Error](h, "admin", "PATCH", "/admin/program", map[string]any{"timeZone": "UTC"}, 409).Code)
	for i, input := range []any{map[string]any{"recipientPersonId": "person-recipient", "points": 1.5, "message": "bad"}, map[string]any{"recipientPersonId": "person-recipient", "points": 1, "message": " "}, map[string]any{"recipientPersonId": "person-recipient", "points": 1, "message": "bad\u0001"}, map[string]any{"recipientPersonId": "person-recipient", "points": 1, "message": "a", "values": []string{"thanks", "thanks"}}, map[string]any{"recipientPersonId": "person-recipient", "points": 1, "message": "a", "unexpected": true}} {
		call[httpx.Error](h, "writer", "POST", "/posts", input, 400, fmt.Sprint("invalid-", i))
	}
	unknown := postInput(1)
	unknown["values"] = []string{"unknown"}
	require.Equal(t, "unknown_value", call[httpx.Error](h, "writer", "POST", "/posts", unknown, 400, "unknown-value").Code)
	call[httpx.Error](h, "writer", "POST", "/posts", postInput(1), 400)
	call[httpx.Error](h, "writer", "POST", "/posts", postInput(1), 400, strings.Repeat("x", 129))
	h.request("Bearer "+token("writer"), "POST", "/posts", strings.NewReader(`{}`), "content-type", "", 400)
	h.request("Bearer "+token("writer"), "POST", "/posts", strings.NewReader(`{"message":"`+strings.Repeat("x", 64<<10)+`"}`), "oversized", "application/json", 400)
	for _, path := range []string{"/posts?limit=0", "/posts?limit=51", "/posts?cursor=bad", "/posts?period=2026-W00", "/posts?month=2026-13"} {
		call[httpx.Error](h, "reader", "GET", path, nil, 400)
	}
}
func TestTimelinePagingAndFilters(t *testing.T) {
	h := newHarness(t)
	for i := range 5 {
		call[postResponse](h, "writer", "POST", "/posts", postInput(1), 201, fmt.Sprint(i))
	}
	type page struct {
		Items      []postResponse `json:"items"`
		NextCursor *string        `json:"nextCursor"`
	}
	path := "/posts?limit=2"
	seen := map[string]bool{}
	for range 3 {
		result := call[page](h, "reader", "GET", path, nil, 200)
		for _, post := range result.Items {
			require.False(t, seen[post.ID])
			seen[post.ID] = true
		}
		if result.NextCursor == nil {
			break
		}
		path = "/posts?limit=2&cursor=" + url.QueryEscape(*result.NextCursor)
	}
	require.Len(t, seen, 5)
	me := call[meResponse](h, "writer", "GET", "/me", nil, 200)
	require.Len(t, call[page](h, "reader", "GET", "/posts?member="+me.Member.ID, nil, 200).Items, 5)
	require.Empty(t, call[page](h, "reader", "GET", "/posts?received="+me.Member.ID, nil, 200).Items)
}

func TestCloseStatementCSVAndFrozenDeletion(t *testing.T) {
	h := newHarness(t)
	post := call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "payroll")
	call[any](h, "clapper", "POST", "/posts/"+post.ID+"/claps", nil, 201, "payroll-clap")
	require.Equal(t, "statement_open", call[httpx.Error](h, "admin", "GET", "/admin/statements/2026-10/export.csv", nil, 409).Code)
	require.Equal(t, "month_not_ended", call[httpx.Error](h, "admin", "POST", "/admin/statements/2026-10/close", nil, 409).Code)
	h.setTime("2026-11-01T00:00:00Z")
	h.mu.Lock()
	p := h.people["person-recipient"]
	p.DisplayName = "Updated, Recipient"
	p.Email = "payroll@directory.test"
	h.people[p.ID] = p
	h.mu.Unlock()
	call[programAdminResponse](h, "admin", "PATCH", "/admin/program", map[string]any{"pointUnitValue": "2.50", "currency": "JPY"}, 200)
	statement := call[statementResponse](h, "admin", "POST", "/admin/statements/2026-10/close", nil, 200)
	require.True(t, statement.Closed)
	require.True(t, statement.DirectoryRefreshed)
	require.Equal(t, 12, statement.TotalPoints)
	require.Equal(t, "30.00", statement.TotalAmount)
	require.Len(t, statement.Lines, 2)
	require.NotNil(t, statement.ClosedBy)
	require.Equal(t, "person-admin", statement.ClosedBy.PersonID)
	require.Equal(t, statement, call[statementResponse](h, "admin", "POST", "/admin/statements/2026-10/close", nil, 200))
	headers, payload := h.request("Bearer "+token("admin"), "GET", "/admin/statements/2026-10/export.csv", nil, "", "", 200)
	require.Equal(t, "text/csv; charset=utf-8", headers.Get("Content-Type"))
	require.Equal(t, `attachment; filename="peerbonus-2026-10.csv"`, headers.Get("Content-Disposition"))
	require.True(t, bytes.HasPrefix(payload, []byte{0xef, 0xbb, 0xbf}))
	rows, err := csv.NewReader(bytes.NewReader(payload[3:])).ReadAll()
	require.NoError(t, err)
	require.Equal(t, []string{"month", "person_id", "email", "display_name", "pending", "points", "unit_value", "currency", "amount"}, rows[0])
	found := false
	for _, row := range rows[1:] {
		if row[1] == "person-recipient" {
			found = true
			require.Equal(t, []string{"2026-10", "person-recipient", "payroll@directory.test", "Updated, Recipient", "true", "11", "2.50", "JPY", "27.50"}, row)
		}
	}
	require.True(t, found)
	require.Equal(t, "period_closed", call[httpx.Error](h, "clapper", "POST", "/posts/"+post.ID+"/claps", nil, 409, "closed-clap").Code)
	call[any](h, "admin", "DELETE", "/posts/"+post.ID, nil, 204)
	require.Equal(t, 5, h.count("ledger"))
	require.Equal(t, statement, call[statementResponse](h, "admin", "GET", "/admin/statements/2026-10", nil, 200))
	call[httpx.Error](h, "admin", "POST", "/admin/statements/2026-13/close", nil, 400)
	members := call[participationResponse](h, "admin", "GET", "/admin/members?month=2026-10", nil, 200)
	require.Equal(t, "2026-10", members.Month)
	require.NotEmpty(t, members.Items)
	list := call[struct {
		Items []statementSummary `json:"items"`
	}](h, "admin", "GET", "/admin/statements", nil, 200)
	require.Equal(t, "2026-11", list.Items[0].Month)
}
func TestCloseDegradedDirectoryAndMonthOrdering(t *testing.T) {
	h := newHarness(t)
	call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "october")
	call[meResponse](h, "admin", "GET", "/me", nil, 200)
	h.setTime("2026-11-19T00:00:00Z")
	call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "november")
	h.setTime("2026-12-01T00:00:00Z")
	require.Equal(t, "previous_month_open", call[httpx.Error](h, "admin", "POST", "/admin/statements/2026-11/close", nil, 409).Code)
	h.directoryStatus.Store(503)
	closed := call[statementResponse](h, "admin", "POST", "/admin/statements/2026-10/close", nil, 200)
	require.True(t, closed.Closed)
	require.False(t, closed.DirectoryRefreshed)
	require.Equal(t, "Pending Recipient", closed.Lines[0].Member.DisplayName)
}

func TestOpenAPIContractIncludingSlackWithoutDatabase(t *testing.T) {
	auth, err := identity.New(identity.Trust{})
	require.NoError(t, err)
	handler := New(nil, auth, nil, nil, "", SlackOptions{})
	request := httptest.NewRequest(http.MethodGet, "/api/peerbonus/v1/openapi.json", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	require.Equal(t, 401, w.Code)
	request.Header.Set("Authorization", "Bearer "+token("reader"))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	require.Equal(t, 200, w.Code)
	var document struct {
		Definition string `json:"x-appthrust-definition"`
		Paths      map[string]map[string]struct {
			Link       bool `json:"x-appthrust-link"`
			Parameters []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &document))
	require.Equal(t, "peerbonus", document.Definition)
	for path, operations := range document.Paths {
		if get, ok := operations["get"]; ok {
			require.True(t, get.Link, path)
		}
	}
	for _, path := range []string{"/posts", "/posts/{id}/claps"} {
		found := false
		for _, parameter := range document.Paths[path]["post"].Parameters {
			if parameter.Name == "Idempotency-Key" {
				found = parameter.Required
			}
		}
		require.True(t, found, path)
	}
	require.Contains(t, document.Paths, "/recipients")
	for _, parameter := range document.Paths["/members"]["get"].Parameters {
		if parameter.Name == "q" {
			require.False(t, parameter.Required)
		}
	}
	require.Len(t, document.Paths, 25)
	for _, path := range []string{"/admin/slack", "/admin/slack/verify", "/admin/slack/channels", "/admin/slack/test", "/admin/slack/deliveries"} {
		require.Contains(t, document.Paths, path)
	}
	require.Contains(t, document.Paths, "/avatars/{personId}")
	require.Contains(t, document.Paths, "/inbox")
	require.Contains(t, document.Paths, "/inbox/seen")
	require.Contains(t, document.Paths, "/admin/insights")
	require.Contains(t, document.Paths, "/admin/network")
}

func TestInboxPostClapSeenAndIsolation(t *testing.T) {
	h := newHarness(t)
	type inboxPage struct {
		Items      []inboxItemResponse `json:"items"`
		NextCursor *string             `json:"nextCursor"`
		SeenAt     *time.Time          `json:"seenAt"`
	}
	type seenResponse struct {
		SeenAt time.Time `json:"seenAt"`
		Unread int       `json:"unread"`
	}
	require.Equal(t, "unauthenticated", call[httpx.Error](h, "", "GET", "/inbox", nil, 401).Code)
	empty := call[inboxPage](h, "reader", "GET", "/inbox", nil, 200)
	require.NotNil(t, empty.Items)
	require.Empty(t, empty.Items)
	require.Nil(t, empty.SeenAt)
	input := postInput(10)
	input["recipientPersonId"] = "person-reader"
	post := call[postResponse](h, "writer", "POST", "/posts", input, 201, "inbox-post")
	h.setTime("2026-10-19T03:01:00Z")
	clapped := call[struct {
		Clap clapResponse `json:"clap"`
	}](h, "clapper", "POST", "/posts/"+post.ID+"/claps", nil, 201, "inbox-clap")
	first := call[inboxPage](h, "reader", "GET", "/inbox?limit=1", nil, 200)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.NextCursor)
	require.Nil(t, first.SeenAt)
	require.Equal(t, clapped.Clap.ID, first.Items[0].ID)
	require.Equal(t, domain.InboxClap, first.Items[0].Kind)
	require.Equal(t, clapped.Clap.Member, first.Items[0].Actor)
	require.Equal(t, post.ID, first.Items[0].Post.ID)
	require.Equal(t, []memberResponse{clapped.Clap.Member}, first.Items[0].Post.Clappers)
	require.True(t, first.Items[0].Unread)
	second := call[inboxPage](h, "reader", "GET", "/inbox?limit=1&cursor="+url.QueryEscape(*first.NextCursor), nil, 200)
	require.Len(t, second.Items, 1)
	require.Nil(t, second.NextCursor)
	require.Equal(t, post.ID, second.Items[0].ID)
	require.Equal(t, domain.InboxPost, second.Items[0].Kind)
	require.Equal(t, post.Sender, second.Items[0].Actor)
	require.True(t, second.Items[0].Unread)
	require.Equal(t, int32(0), h.platformCalls.Load(), "inbox routes do not need platform roles")
	require.Equal(t, 2, call[meResponse](h, "reader", "GET", "/me", nil, 200).Inbox.Unread)
	require.Equal(t, 1, call[meResponse](h, "writer", "GET", "/me", nil, 200).Inbox.Unread)

	h.setTime("2026-10-19T03:02:00Z")
	marked := call[seenResponse](h, "reader", "POST", "/inbox/seen", nil, 200)
	require.Equal(t, h.clock.Load(), marked.SeenAt.UnixNano())
	require.Zero(t, marked.Unread)
	after := call[inboxPage](h, "reader", "GET", "/inbox", nil, 200)
	require.True(t, marked.SeenAt.Equal(*after.SeenAt))
	require.Len(t, after.Items, 2)
	for _, item := range after.Items {
		require.False(t, item.Unread)
	}
	require.Zero(t, call[meResponse](h, "reader", "GET", "/me", nil, 200).Inbox.Unread)
	require.Equal(t, 1, call[meResponse](h, "writer", "GET", "/me", nil, 200).Inbox.Unread)
	h.setTime("2026-10-19T03:01:30Z")
	older := call[seenResponse](h, "reader", "POST", "/inbox/seen", map[string]any{}, 200)
	require.True(t, marked.SeenAt.Equal(older.SeenAt))
	h.setTime("2026-10-19T03:03:00Z")
	call[postResponse](h, "writer", "POST", "/posts", input, 201, "inbox-new")
	require.Equal(t, 1, call[meResponse](h, "reader", "GET", "/me", nil, 200).Inbox.Unread)
	for _, path := range []string{"/inbox?limit=0", "/inbox?limit=51", "/inbox?cursor=invalid"} {
		require.Equal(t, "invalid_request", call[httpx.Error](h, "reader", "GET", path, nil, 400).Code)
	}
	require.Equal(t, "invalid_request", call[httpx.Error](h, "reader", "POST", "/inbox/seen", map[string]any{"seenAt": "2027-01-01T00:00:00Z"}, 400).Code)
}

func TestPostSearchAndNewestClappers(t *testing.T) {
	h := newHarness(t)
	input := postInput(10)
	input["message"] = `100%_\\ Care`
	post := call[postResponse](h, "writer", "POST", "/posts", input, 201, "search-post")
	require.NotNil(t, post.Clappers)
	require.Empty(t, post.Clappers)
	call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "other-post")
	for i, subject := range []string{"clapper", "admin", "reader", "subject"} {
		h.setTime(fmt.Sprintf("2026-10-19T03:00:0%dZ", i+1))
		call[map[string]any](h, subject, "POST", "/posts/"+post.ID+"/claps", nil, 201, "clap-"+subject)
	}
	current := call[postResponse](h, "reader", "GET", "/posts/"+post.ID, nil, 200)
	require.Equal(t, 4, current.ClapCount)
	require.Equal(t, 1, current.ClappedByMe)
	require.Len(t, current.Clappers, 3)
	require.Equal(t, []string{"person-subject", "person-reader", "person-admin"}, []string{current.Clappers[0].PersonID, current.Clappers[1].PersonID, current.Clappers[2].PersonID})
	type postPage struct {
		Items []postResponse `json:"items"`
	}
	for _, query := range []string{"cArE", "%", "_", `\`, "　 care  "} {
		page := call[postPage](h, "reader", "GET", "/posts?q="+url.QueryEscape(query), nil, 200)
		require.Len(t, page.Items, 1, query)
		require.Equal(t, current, page.Items[0])
	}
	for _, query := range []string{"DIRECTORY WRITER", "pending recipient"} {
		page := call[postPage](h, "reader", "GET", "/posts?q="+url.QueryEscape(query), nil, 200)
		require.Len(t, page.Items, 2, query)
	}
	page := call[postPage](h, "reader", "GET", "/posts?q="+url.QueryEscape(strings.Repeat("あ", 100)), nil, 200)
	require.Empty(t, page.Items)
	require.Equal(t, "invalid_request", call[httpx.Error](h, "reader", "GET", "/posts?q="+url.QueryEscape(strings.Repeat("あ", 101)), nil, 400).Code)
}

func TestV2InputAndResponseContractWithoutDatabase(t *testing.T) {
	a := &API{}
	for _, query := range []string{strings.Repeat("a", 101), strings.Repeat("あ", 101)} {
		_, _, err := a.posts(&request{r: httptest.NewRequest(http.MethodGet, "/posts?q="+url.QueryEscape(query), nil)})
		require.Error(t, err)
		w := httptest.NewRecorder()
		writeError(w, err)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), `"error":"invalid_request"`)
		require.Contains(t, w.Body.String(), "Query must contain at most 100 characters")
	}
	for _, limit := range []string{"0", "51", "invalid"} {
		_, _, err := a.inbox(&request{r: httptest.NewRequest(http.MethodGet, "/inbox?limit="+limit, nil)})
		require.Error(t, err)
	}
	require.NotNil(t, a.renderPost(domain.Post{}).Clappers)
	stats := a.renderMemberStats(domain.MemberStats{})
	require.NotNil(t, stats.Partners.SentTo)
	require.NotNil(t, stats.Partners.ReceivedFrom)
	require.NotNil(t, stats.Values)
	require.NotNil(t, stats.Months)
	statement := a.renderStatement(domain.Statement{})
	require.NotNil(t, statement.CloseBlockers)
	require.Nil(t, statement.PreviousOpenMonth)
	insights := a.renderInsights(domain.Insights{})
	require.NotNil(t, insights.Values)
	require.NotNil(t, insights.Weeks)
	require.NotNil(t, insights.TopRecipients)
	require.NotNil(t, insights.TopSenders)
	require.NotNil(t, a.renderParticipation(domain.Participation{}).Items)
}
