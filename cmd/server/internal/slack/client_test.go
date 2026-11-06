package slack_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack/slacktest"
	"github.com/stretchr/testify/require"
)

func TestBaseURLValidation(t *testing.T) {
	_, err := slack.New("test-token")
	require.NoError(t, err)
	for _, base := range []string{"https://slack.com/api", "http://localhost", "https://localhost:443", "http://127.0.0.1:8080", "http://[::1]", "https://[::1]:8443"} {
		t.Run(base, func(t *testing.T) {
			_, err := slack.New("test-token", slack.WithBaseURL(base))
			require.NoError(t, err)
		})
	}
	for _, base := range []string{"", "https://example.com", "http://slack.com/api", "https://slack.com/api/", "https://slack.com/api?x=y", "http://127.0.0.2", "http://localhost.evil", "http://127.0.0.1/", "http://localhost/api", "http://localhost?", "http://localhost?x=y", "http://localhost#", "http://localhost#fragment", "http://user:secret@localhost", "ftp://localhost", "//localhost", "http://localhost:", "http://localhost:65536", "http://localhost:0", "http://[::1%25lo]:8080"} {
		t.Run(base, func(t *testing.T) {
			_, err := slack.New("test-token", slack.WithBaseURL(base))
			require.EqualError(t, err, "slack: base URL must be https://slack.com/api or a loopback HTTP(S) origin")
		})
	}
}

func TestAuthAndUserLookup(t *testing.T) {
	server := slacktest.New(t, slacktest.WithTeam(slack.AuthInfo{TeamID: "T456", TeamName: "Example", BotUserID: "UBOT", EnterpriseID: "E1"}))
	server.AddUser("alice@example.com", "U1")
	client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
	require.NoError(t, err)
	team, err := client.AuthTest(t.Context())
	require.NoError(t, err)
	require.Equal(t, server.Team(), team)
	calls := server.Calls("auth.test")
	require.Len(t, calls, 1)
	require.Equal(t, "Bearer test-token", calls[0].Auth)
	require.Empty(t, calls[0].Body)
	id, err := client.LookupUserByEmail(t.Context(), "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, "U1", id)
	_, err = client.LookupUserByEmail(t.Context(), "missing@example.com")
	require.ErrorIs(t, err, slack.ErrUserNotFound)
	var apiError *slack.Error
	require.ErrorAs(t, err, &apiError)
	require.Equal(t, "users_not_found", apiError.Code)
	require.Equal(t, "users.lookupByEmail", apiError.Method)
	require.Equal(t, http.StatusOK, apiError.Status)
	require.False(t, slack.IsRateLimited(err))
	server.Fail("auth.test", "invalid_auth")
	_, err = client.AuthTest(t.Context())
	require.ErrorAs(t, err, &apiError)
	require.EqualError(t, err, "slack: auth.test: invalid_auth (HTTP 200)")
	require.NotContains(t, err.Error(), "test-token")
	require.False(t, errors.Is(err, slack.ErrUserNotFound))
}

func TestRateLimitRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		header string
		want   time.Duration
	}{
		{"http", 429, `{"ok":false,"error":"ratelimited"}`, "12", 12 * time.Second},
		{"http without JSON", 429, "Too many requests", "2", 2 * time.Second},
		{"api", 200, `{"ok":false,"error":"ratelimited"}`, "4", 4 * time.Second},
		{"missing", 429, `{"ok":false}`, "", time.Second},
		{"invalid", 429, `{"ok":false}`, "invalid", time.Second},
		{"negative", 429, `{"ok":false}`, "-1", time.Second},
		{"zero", 429, `{"ok":false}`, "0", time.Second},
		{"overflow", 429, `{"ok":false}`, "9223372036854775807", time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", tc.header)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
			require.NoError(t, err)
			_, err = client.AuthTest(t.Context())
			require.True(t, slack.IsRateLimited(fmt.Errorf("wrapped: %w", err)))
			var apiError *slack.Error
			require.ErrorAs(t, err, &apiError)
			require.Equal(t, tc.want, apiError.RetryAfter)
			require.Equal(t, tc.status, apiError.Status)
		})
	}
	require.False(t, slack.IsRateLimited(nil))
	require.False(t, slack.IsRateLimited(errors.New("other")))
	require.True(t, slack.IsRateLimited(&slack.Error{Status: 429}))
}

func TestPagination(t *testing.T) {
	server := slacktest.New(t)
	want := make([]slack.Channel, 5)
	for i := range want {
		want[i] = slack.Channel{ID: fmt.Sprintf("C%d", i), Name: fmt.Sprintf("channel-%d", i), IsMember: i%2 == 0}
		server.AddChannel(want[i].ID, want[i].Name, want[i].IsMember)
	}
	client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
	require.NoError(t, err)
	channels, err := client.ListPublicChannels(t.Context())
	require.NoError(t, err)
	require.Equal(t, want, channels)
	calls := server.Calls("conversations.list")
	require.Len(t, calls, 3)
	for _, call := range calls {
		require.Equal(t, "public_channel", call.Body["types"])
		require.Equal(t, true, call.Body["exclude_archived"])
		require.Equal(t, float64(200), call.Body["limit"])
	}
	require.Empty(t, calls[0].Body["cursor"])
	require.NotEmpty(t, calls[1].Body["cursor"])
	require.NotEqual(t, calls[1].Body["cursor"], calls[2].Body["cursor"])
}

func TestPaginationCapsAtThousand(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Cursor string `json:"cursor"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if calls > 0 {
			require.Equal(t, fmt.Sprint(calls), body.Cursor)
		}
		calls++
		channels := make([]slack.Channel, 200)
		for i := range channels {
			channels[i] = slack.Channel{ID: fmt.Sprintf("C%d", (calls-1)*200+i)}
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"ok": true, "channels": channels, "response_metadata": map[string]string{"next_cursor": fmt.Sprint(calls)}}))
	}))
	t.Cleanup(server.Close)
	client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
	require.NoError(t, err)
	channels, err := client.ListPublicChannels(t.Context())
	require.NoError(t, err)
	require.Len(t, channels, 1000)
	require.Equal(t, 5, calls)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportJSONAndTimeout(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "https://slack.com/api/chat.postMessage", r.URL.String())
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.Equal(t, "application/json; charset=utf-8", r.Header.Get("Content-Type"))
		deadline, ok := r.Context().Deadline()
		require.True(t, ok)
		require.InDelta(t, 10.0, time.Until(deadline).Seconds(), 0.5)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "C1", body["channel"])
		require.Equal(t, "thanks <@U1>", body["text"])
		require.IsType(t, []any{}, body["blocks"])
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"ts":"1.000001"}`)), Header: make(http.Header)}, nil
	})}
	client, err := slack.New("test-token", slack.WithHTTPClient(httpClient))
	require.NoError(t, err)
	ts, err := client.PostMessage(t.Context(), "C1", slack.Message{Text: "thanks <@U1>", Blocks: json.RawMessage(`[{"type":"divider"}]`)})
	require.NoError(t, err)
	require.Equal(t, "1.000001", ts)
	require.Zero(t, httpClient.Timeout)
	require.Nil(t, httpClient.CheckRedirect)
}

func TestTransportHonorsCancellation(t *testing.T) {
	client, err := slack.New("test-token", slack.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, r.Context().Err()
	})}))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = client.AuthTest(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestTransportRejectsRedirects(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected = true
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	client, err := slack.New("test-token", slack.WithBaseURL(server.URL), slack.WithHTTPClient(server.Client()))
	require.NoError(t, err)
	_, err = client.AuthTest(t.Context())
	var apiError *slack.Error
	require.ErrorAs(t, err, &apiError)
	require.Equal(t, http.StatusTemporaryRedirect, apiError.Status)
	require.False(t, redirected)
}
