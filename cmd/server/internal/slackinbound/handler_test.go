package slackinbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/slackinbound/slackinboundtest"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
	"github.com/stretchr/testify/require"
)

const testSecret = "inbound-test-secret"

var testNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

type fakeStore struct {
	integration            domain.SlackIntegration
	integrationErr         error
	member                 domain.Member
	memberErr              error
	memberLookups          []string
	program                domain.Program
	period                 domain.Period
	remaining              int
	remainingMember        string
	remainingNow           time.Time
	posts                  []store.CreatePostInput
	postErr                error
	claps                  []store.CreateClapInput
	clapErr                error
	searchQ, searchExclude string
	searchLimit            int
	members                []domain.Member
}

func (s *fakeStore) SlackIntegration(context.Context) (domain.SlackIntegration, error) {
	return s.integration, s.integrationErr
}
func (s *fakeStore) MemberBySlackUser(_ context.Context, user string) (domain.Member, error) {
	s.memberLookups = append(s.memberLookups, user)
	return s.member, s.memberErr
}
func (s *fakeStore) MemberByEmail(context.Context, string) (domain.Member, error) {
	panic("an inbound email must never establish identity")
}
func (s *fakeStore) MemberByID(context.Context, string) (domain.Member, error) {
	panic("cached Slack identity already supplies the member")
}
func (s *fakeStore) SearchMembers(_ context.Context, q, exclude string, limit int) ([]domain.Member, error) {
	s.searchQ, s.searchExclude, s.searchLimit = q, exclude, limit
	return s.members, nil
}
func (s *fakeStore) CreatePost(_ context.Context, in store.CreatePostInput) (domain.Post, bool, error) {
	s.posts = append(s.posts, in)
	return domain.Post{ID: "post-created"}, false, s.postErr
}
func (s *fakeStore) CreateClap(_ context.Context, in store.CreateClapInput) (domain.Clap, domain.Post, bool, error) {
	s.claps = append(s.claps, in)
	return domain.Clap{ID: "clap-created"}, domain.Post{ID: in.PostID}, false, s.clapErr
}
func (s *fakeStore) Program(context.Context) (domain.Program, error) { return s.program, nil }
func (s *fakeStore) CurrentPeriod(context.Context, time.Time) (domain.Period, error) {
	return s.period, nil
}
func (s *fakeStore) GiveRemaining(_ context.Context, member string, now time.Time) (int, error) {
	s.remainingMember, s.remainingNow = member, now
	return s.remaining, nil
}

type fakeSlack struct {
	trigger       string
	view          json.RawMessage
	deadline      time.Time
	openErr       error
	channel, user string
	messages      []slack.Message
}

func (s *fakeSlack) OpenView(ctx context.Context, trigger string, view json.RawMessage) error {
	s.trigger, s.view = trigger, view
	s.deadline, _ = ctx.Deadline()
	return s.openErr
}
func (s *fakeSlack) PostEphemeral(_ context.Context, channel, user string, message slack.Message) error {
	s.channel, s.user = channel, user
	s.messages = append(s.messages, message)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixture() (*fakeStore, *fakeSlack, Options) {
	s := &fakeStore{
		integration: domain.SlackIntegration{GiveFromSlack: true, Locale: domain.SlackLocaleEN,
			Workspace: &domain.SlackWorkspace{TeamID: "T123"}},
		member:  domain.Member{ID: "sender", DisplayName: "Alice", Email: "alice@example.test"},
		program: domain.Program{MaxPointsPerPost: 120, Values: []domain.Value{{Key: "care", Label: "Care"}, {Key: "learn", Label: "Learn"}}},
		period:  domain.Period{MaxPointsPerPost: 90}, remaining: 398,
	}
	slack := &fakeSlack{}
	return s, slack, Options{Store: s, Slack: slack, SigningSecret: testSecret, PublicURL: "https://peerbonus.example.test", Now: func() time.Time { return testNow }}
}
func commandForm() url.Values {
	return url.Values{"team_id": {"T123"}, "user_id": {"U123"}, "command": {"/peerbonus"},
		"trigger_id": {"trigger-1"}, "response_url": {"https://hooks.slack.com/commands/test"}}
}
func signedRequest(path, body string, at time.Time) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(at.Unix(), 10))
	r.Header.Set("X-Slack-Signature", slackinboundtest.SignRequest(testSecret, at, []byte(body)))
	return r
}
func serve(h http.Handler, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(path, body, testNow))
	return w
}
func interactionForm(t *testing.T, payload any) string {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return url.Values{"payload": {string(body)}}.Encode()
}

type unreadBody struct{ t *testing.T }

func (b unreadBody) Read([]byte) (int, error) { b.t.Fatal("body must not be read"); return 0, io.EOF }
func (unreadBody) Close() error               { return nil }

func TestGateDoesNotReadBody(t *testing.T) {
	for _, name := range []string{"no secret", "disabled", "unverified"} {
		t.Run(name, func(t *testing.T) {
			s, _, o := fixture()
			switch name {
			case "no secret":
				o.SigningSecret = ""
				o.Store = nil
			case "disabled":
				s.integration.GiveFromSlack = false
			case "unverified":
				s.integration.Workspace = nil
			}
			for _, path := range []string{"/slack/commands", "/slack/interactions", "/slack/options"} {
				r := httptest.NewRequest(http.MethodPost, path, nil)
				r.Body, r.ContentLength = unreadBody{t}, maxBodyBytes+1
				w := httptest.NewRecorder()
				New(o).ServeHTTP(w, r)
				require.Equal(t, http.StatusNotFound, w.Code)
			}
		})
	}
}

func TestGateReadsCurrentSettings(t *testing.T) {
	s, slack, o := fixture()
	h := New(o)
	s.integration.GiveFromSlack = false
	require.Equal(t, http.StatusNotFound, serve(h, "/slack/commands", commandForm().Encode()).Code)
	s.integration.GiveFromSlack = true
	require.Equal(t, http.StatusOK, serve(h, "/slack/commands", commandForm().Encode()).Code)
	require.NotEmpty(t, slack.view)
}

func TestSignatureRejections(t *testing.T) {
	for _, name := range []string{"missing", "bad", "old", "future", "malformed timestamp", "modified body", "oversized", "chunked oversized"} {
		t.Run(name, func(t *testing.T) {
			s, slack, o := fixture()
			body := commandForm().Encode()
			at := testNow
			if name == "old" {
				at = at.Add(-5*time.Minute - time.Second)
			}
			if name == "future" {
				at = at.Add(5*time.Minute + time.Second)
			}
			if name == "oversized" || name == "chunked oversized" {
				body = strings.Repeat("x", maxBodyBytes+1)
			}
			r := signedRequest("/slack/commands", body, at)
			switch name {
			case "missing":
				r.Header.Del("X-Slack-Signature")
			case "bad":
				r.Header.Set("X-Slack-Signature", "v0="+strings.Repeat("0", 64))
			case "malformed timestamp":
				r.Header.Set("X-Slack-Request-Timestamp", "yesterday")
			case "modified body":
				r.Body = io.NopCloser(strings.NewReader(body + "&text=balance"))
			case "chunked oversized":
				r.ContentLength = -1
			}
			if name == "old" || name == "future" || name == "missing" || name == "malformed timestamp" {
				r.Body = unreadBody{t}
			}
			w := httptest.NewRecorder()
			New(o).ServeHTTP(w, r)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Empty(t, s.memberLookups)
			require.Empty(t, slack.view)
		})
	}
}

func TestSignatureWindowAndReplay(t *testing.T) {
	for _, offset := range []time.Duration{-5 * time.Minute, 5 * time.Minute} {
		t.Run(offset.String(), func(t *testing.T) {
			_, _, o := fixture()
			h := New(o)
			body := commandForm().Encode()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, signedRequest("/slack/commands", body, testNow.Add(offset)))
			require.Equal(t, http.StatusOK, w.Code)
			r := signedRequest("/slack/commands", body, testNow.Add(offset))
			r.Body = unreadBody{t}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestReplaySetPrunesAndSerializesDuplicates(t *testing.T) {
	set := replaySet{entries: map[string]time.Time{}}
	var wg sync.WaitGroup
	accepted := make(chan bool, 20)
	for range 20 {
		wg.Go(func() { accepted <- !set.seen("request", testNow, true) })
	}
	wg.Wait()
	close(accepted)
	count := 0
	for ok := range accepted {
		if ok {
			count++
		}
	}
	require.Equal(t, 1, count)
	require.True(t, set.seen("request", testNow.Add(replayTTL-time.Nanosecond), false))
	require.False(t, set.seen("request", testNow.Add(replayTTL+time.Minute), false))
	require.Empty(t, set.entries)
}

func TestWorkspaceBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, team, enterprise, fixedEnterprise string
		status                                  int
	}{
		{"wrong team", "Tother", "", "", 403},
		{"missing identity", "", "", "", 403},
		{"wrong enterprise", "", "Eother", "E123", 403},
		{"enterprise", "", "E123", "E123", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, o := fixture()
			s.integration.Workspace.EnterpriseID = tc.fixedEnterprise
			form := commandForm()
			form.Set("team_id", tc.team)
			form.Set("enterprise_id", tc.enterprise)
			require.Equal(t, tc.status, serve(New(o), "/slack/commands", form.Encode()).Code)
			if tc.status == 403 {
				require.Empty(t, s.memberLookups)
			}
		})
	}
	for _, path := range []string{"/slack/interactions", "/slack/options"} {
		s, _, o := fixture()
		body := interactionForm(t, map[string]any{"type": "view_submission", "team": map[string]string{"id": "Tother"}})
		require.Equal(t, http.StatusForbidden, serve(New(o), path, body).Code)
		require.Empty(t, s.memberLookups)
	}
}

func captureResponse(t *testing.T, o *Options) *[]ephemeralResponse {
	t.Helper()
	messages := []ephemeralResponse{}
	o.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "https://hooks.slack.com/commands/test", r.URL.String())
		require.Equal(t, "application/json; charset=utf-8", r.Header.Get("Content-Type"))
		deadline, ok := r.Context().Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), requestBudget)
		var message ephemeralResponse
		require.NoError(t, json.NewDecoder(r.Body).Decode(&message))
		messages = append(messages, message)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	return &messages
}

func TestUnknownMemberCommandGuidanceAndBalance(t *testing.T) {
	for _, unknown := range []bool{true, false} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			s, slack, o := fixture()
			if unknown {
				s.memberErr = fmt.Errorf("lookup: %w", domain.ErrNotFound)
			}
			messages := captureResponse(t, &o)
			form := commandForm()
			form.Set("text", "balance")
			require.Equal(t, http.StatusOK, serve(New(o), "/slack/commands", form.Encode()).Code)
			require.Len(t, *messages, 1)
			require.Equal(t, "ephemeral", (*messages)[0].ResponseType)
			if unknown {
				require.Contains(t, (*messages)[0].Text, "Open Peer Bonus in the browser once so we can link your account")
				require.Contains(t, (*messages)[0].Text, o.PublicURL)
				require.Empty(t, s.remainingMember)
			} else {
				require.Contains(t, (*messages)[0].Text, "398 pt")
				require.Equal(t, "sender", s.remainingMember)
				require.Equal(t, testNow, s.remainingNow)
			}
			require.Equal(t, []string{"U123"}, s.memberLookups)
			require.Empty(t, slack.view)
		})
	}
}

func TestModalShapeAndDeadline(t *testing.T) {
	for _, locale := range []domain.SlackLocale{domain.SlackLocaleEN, domain.SlackLocaleJA} {
		t.Run(string(locale), func(t *testing.T) {
			s, slack, o := fixture()
			s.integration.Locale = locale
			before := time.Now()
			require.Equal(t, http.StatusOK, serve(New(o), "/slack/commands", commandForm().Encode()).Code)
			require.Equal(t, "trigger-1", slack.trigger)
			require.WithinRange(t, slack.deadline, before, time.Now().Add(requestBudget))
			var view modal
			require.NoError(t, json.Unmarshal(slack.view, &view))
			require.Equal(t, "modal", view.Type)
			require.Equal(t, "peerbonus_give", view.CallbackID)
			require.Equal(t, "sender", view.PrivateMetadata)
			require.Equal(t, localized(locale, "贈る", "Give"), view.Submit.Text)
			require.Len(t, view.Blocks, 4)
			for i, id := range []string{"recipient", "points", "message", "values"} {
				require.Equal(t, id, view.Blocks[i].BlockID)
				require.Equal(t, id, view.Blocks[i].Element.ActionID)
			}
			recipient, points, message, values := view.Blocks[0], view.Blocks[1], view.Blocks[2], view.Blocks[3]
			require.Equal(t, "external_select", recipient.Element.Type)
			require.Equal(t, 1, *recipient.Element.MinQueryLength)
			require.Equal(t, "number_input", points.Element.Type)
			require.NotNil(t, points.Element.IsDecimalAllowed)
			require.False(t, *points.Element.IsDecimalAllowed)
			require.Equal(t, "1", points.Element.MinValue)
			require.Equal(t, "90", points.Element.MaxValue)
			require.Equal(t, "plain_text_input", message.Element.Type)
			require.True(t, message.Element.Multiline)
			require.Equal(t, 500, message.Element.MaxLength)
			require.Equal(t, "multi_static_select", values.Element.Type)
			require.True(t, values.Optional)
			require.Equal(t, 3, values.Element.MaxSelectedItems)
			require.Equal(t, []selectOption{{Text: plain("Care"), Value: "care"}, {Text: plain("Learn"), Value: "learn"}}, values.Element.Options)
		})
	}
}

func TestModalWithoutProgramValuesAndSlackFailure(t *testing.T) {
	s, slack, o := fixture()
	s.program.Values = nil
	require.Equal(t, http.StatusOK, serve(New(o), "/slack/commands", commandForm().Encode()).Code)
	var view modal
	require.NoError(t, json.Unmarshal(slack.view, &view))
	require.Len(t, view.Blocks, 3)
	slack.openErr = errors.New("test transport failure")
	require.Equal(t, http.StatusBadGateway, serve(New(o), "/slack/commands", commandForm().Encode()).Code)
}
