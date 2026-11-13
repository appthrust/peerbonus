package slack_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
			for _, method := range []string{"auth.test", "users.info"} {
				t.Run(method, func(t *testing.T) {
					if method == "auth.test" {
						_, err = client.AuthTest(t.Context())
					} else {
						_, err = client.UserInfo(t.Context(), "U1")
					}
					require.True(t, slack.IsRateLimited(fmt.Errorf("wrapped: %w", err)))
					var apiError *slack.Error
					require.ErrorAs(t, err, &apiError)
					require.Equal(t, method, apiError.Method)
					require.Equal(t, tc.want, apiError.RetryAfter)
					require.Equal(t, tc.status, apiError.Status)
				})
			}
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
		require.Equal(t, "true", call.Body["exclude_archived"])
		require.Equal(t, "200", call.Body["limit"])
	}
	require.Empty(t, calls[0].Body["cursor"])
	require.NotEmpty(t, calls[1].Body["cursor"])
	require.NotEqual(t, calls[1].Body["cursor"], calls[2].Body["cursor"])
}

func TestPaginationCapsAtThousand(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		if calls > 0 {
			require.Equal(t, fmt.Sprint(calls), r.PostForm.Get("cursor"))
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

func TestLookupUserByEmailFormEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/users.lookupByEmail", r.URL.Path)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.Empty(t, r.URL.RawQuery)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "alice+bonus@example.com", r.PostForm.Get("email"))
		require.Empty(t, r.PostForm.Get("token"))
		_, _ = io.WriteString(w, `{"ok":true,"user":{"id":"U1"}}`)
	}))
	t.Cleanup(server.Close)
	client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
	require.NoError(t, err)
	id, err := client.LookupUserByEmail(t.Context(), "alice+bonus@example.com")
	require.NoError(t, err)
	require.Equal(t, "U1", id)
}

func TestConversationReadMethodsFormEncoding(t *testing.T) {
	for _, tc := range []struct {
		method   string
		form     url.Values
		response string
		call     func(*testing.T, *slack.Client)
	}{
		{"users.info", url.Values{"user": {"U1"}},
			`{"ok":true,"user":{"id":"U1","profile":{"email":"alice@example.com"},"is_bot":false,"deleted":false}}`,
			func(t *testing.T, client *slack.Client) {
				user, err := client.UserInfo(t.Context(), "U1")
				require.NoError(t, err)
				require.Equal(t, slack.User{ID: "U1", Email: "alice@example.com"}, user)
			}},
		{"chat.getPermalink", url.Values{"channel": {"C1"}, "message_ts": {"1700000000.000123"}},
			`{"ok":true,"permalink":"https://acme.slack.com/archives/C1/p1700000000000123"}`,
			func(t *testing.T, client *slack.Client) {
				link, err := client.Permalink(t.Context(), "C1", "1700000000.000123")
				require.NoError(t, err)
				require.Equal(t, "https://acme.slack.com/archives/C1/p1700000000000123", link)
			}},
		{"conversations.list", url.Values{"types": {"public_channel"}, "exclude_archived": {"true"}, "limit": {"200"}},
			`{"ok":true,"channels":[{"id":"C1","name":"thanks","is_member":true}],"response_metadata":{"next_cursor":""}}`,
			func(t *testing.T, client *slack.Client) {
				channels, err := client.ListPublicChannels(t.Context())
				require.NoError(t, err)
				require.Equal(t, []slack.Channel{{ID: "C1", Name: "thanks", IsMember: true}}, channels)
			}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/"+tc.method, r.URL.Path)
				require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
				require.Empty(t, r.URL.RawQuery)
				require.NoError(t, r.ParseForm())
				require.Equal(t, tc.form, r.PostForm)
				_, _ = io.WriteString(w, tc.response)
			}))
			t.Cleanup(server.Close)
			client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
			require.NoError(t, err)
			tc.call(t, client)
			require.Equal(t, 1, calls)
		})
	}
}

func TestUserInfoFlagsAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		want           slack.User
	}{
		{"bot", `{"ok":true,"user":{"id":"UBOT","is_bot":true,"profile":{}}}`, slack.User{ID: "UBOT", IsBot: true}},
		{"deleted", `{"ok":true,"user":{"id":"U1","deleted":true,"profile":{"email":"alice@example.com"}}}`, slack.User{ID: "U1", Email: "alice@example.com", Deleted: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.response)
			}))
			t.Cleanup(server.Close)
			client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
			require.NoError(t, err)
			user, err := client.UserInfo(t.Context(), tc.want.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, user)
		})
	}
	server := slacktest.New(t)
	client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
	require.NoError(t, err)
	_, err = client.UserInfo(t.Context(), "U_MISSING")
	require.ErrorIs(t, err, slack.ErrUserNotFound)
	require.EqualError(t, err, "slack: users.info: user_not_found (HTTP 200)")
	var apiError *slack.Error
	require.ErrorAs(t, err, &apiError)
	require.Equal(t, "user_not_found", apiError.Code)
	server.Fail("users.info", "invalid_auth")
	_, err = client.UserInfo(t.Context(), "U1")
	require.EqualError(t, err, "slack: users.info: invalid_auth (HTTP 200)")
	require.False(t, errors.Is(err, slack.ErrUserNotFound))
}

func TestThreadMessagesJSONEncoding(t *testing.T) {
	for _, method := range []string{"chat.postMessage", "chat.postEphemeral"} {
		for _, threadTS := range []string{"", "1700000000.000123"} {
			t.Run(method+"/"+threadTS, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, "/"+method, r.URL.Path)
					require.Equal(t, "application/json; charset=utf-8", r.Header.Get("Content-Type"))
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "C1", body["channel"])
					require.Equal(t, "thanks", body["text"])
					if method == "chat.postEphemeral" {
						require.Equal(t, "U1", body["user"])
					}
					if threadTS == "" {
						require.NotContains(t, body, "thread_ts")
					} else {
						require.Equal(t, threadTS, body["thread_ts"])
					}
					_, _ = io.WriteString(w, `{"ok":true,"ts":"1700000000.000124","message_ts":"1700000000.000124"}`)
				}))
				t.Cleanup(server.Close)
				client, err := slack.New("test-token", slack.WithBaseURL(server.URL))
				require.NoError(t, err)
				message := slack.Message{Text: "thanks", ThreadTS: threadTS}
				if method == "chat.postMessage" {
					_, err = client.PostMessage(t.Context(), "C1", message)
				} else {
					err = client.PostEphemeral(t.Context(), "C1", "U1", message)
				}
				require.NoError(t, err)
			})
		}
	}
}
