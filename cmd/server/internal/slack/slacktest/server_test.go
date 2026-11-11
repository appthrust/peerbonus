package slacktest_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack/slacktest"
	"github.com/stretchr/testify/require"
)

func clientFor(t *testing.T, server *slacktest.Server) *slack.Client {
	t.Helper()
	client, err := slack.New("fake-token", slack.WithBaseURL(server.URL))
	require.NoError(t, err)
	return client
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var apiError *slack.Error
	require.ErrorAs(t, err, &apiError)
	require.Equal(t, code, apiError.Code)
}

func TestMessageLifecycleViewsAndEphemeral(t *testing.T) {
	server := slacktest.New(t)
	server.AddUser("alice@example.com", "U1")
	server.AddChannel("C1", "thanks", true)
	client := clientFor(t, server)
	team, err := client.AuthTest(t.Context())
	require.NoError(t, err)
	require.Equal(t, slack.AuthInfo{TeamID: "T123", TeamName: "Acme", BotUserID: "UBOT"}, team)
	blocks := json.RawMessage(`[{"type":"section","text":{"type":"mrkdwn","text":"thanks <@U1>"}}]`)
	ts, err := client.PostMessage(t.Context(), "C1", slack.Message{Text: "thanks", Blocks: blocks, ThreadTS: "1699999999.000001"})
	require.NoError(t, err)
	require.NotEmpty(t, ts)
	messages := server.Messages()
	require.Len(t, messages, 1)
	require.Equal(t, "C1", messages[0].Channel)
	require.Equal(t, ts, messages[0].TS)
	require.Equal(t, "1699999999.000001", messages[0].ThreadTS)
	require.Equal(t, "thanks", messages[0].Text)
	require.JSONEq(t, string(blocks), string(messages[0].Blocks))
	updatedBlocks := json.RawMessage(`[{"type":"divider"}]`)
	require.NoError(t, client.UpdateMessage(t.Context(), "C1", ts, slack.Message{Text: "one clap", Blocks: updatedBlocks}))
	messages = server.Messages()
	require.Equal(t, 1, messages[0].Updates)
	require.Equal(t, "one clap", messages[0].Text)
	require.JSONEq(t, string(updatedBlocks), string(messages[0].Blocks))
	require.NoError(t, client.UpdateMessage(t.Context(), "C1", ts, slack.Message{Text: "two claps"}))
	require.NoError(t, client.DeleteMessage(t.Context(), "C1", ts))
	messages = server.Messages()
	require.True(t, messages[0].Deleted)
	require.Equal(t, 2, messages[0].Updates)
	require.Empty(t, messages[0].Blocks)
	assertCode(t, client.DeleteMessage(t.Context(), "C1", ts), "message_not_found")
	assertCode(t, client.UpdateMessage(t.Context(), "C1", ts, slack.Message{Text: "gone"}), "message_not_found")
	assertCode(t, client.UpdateMessage(t.Context(), "C1", "missing", slack.Message{Text: "missing"}), "message_not_found")
	assertCode(t, client.DeleteMessage(t.Context(), "C1", "missing"), "message_not_found")

	dmTS, err := client.PostMessage(t.Context(), "U1", slack.Message{Text: "direct thanks"})
	require.NoError(t, err)
	require.NotEqual(t, ts, dmTS)
	require.Len(t, server.Messages(), 2)
	require.NoError(t, client.PostEphemeral(t.Context(), "C1", "U1", slack.Message{Text: "clapped", ThreadTS: "1699999999.000001"}))
	require.Len(t, server.Messages(), 2, "ephemeral messages are not persistent")
	calls := server.Calls("chat.postEphemeral")
	require.Len(t, calls, 1)
	require.Equal(t, "U1", calls[0].Body["user"])
	require.Equal(t, "C1", calls[0].Body["channel"])
	require.Equal(t, "clapped", calls[0].Body["text"])
	require.Equal(t, "1699999999.000001", calls[0].Body["thread_ts"])
	view := json.RawMessage(`{"type":"modal","title":{"type":"plain_text","text":"Give"},"blocks":[]}`)
	require.NoError(t, client.OpenView(t.Context(), "trigger-1", view))
	views := server.Views()
	require.Len(t, views, 1)
	require.JSONEq(t, string(view), string(views[0]))
	calls = server.Calls("views.open")
	require.Equal(t, "trigger-1", calls[0].Body["trigger_id"])
}

func TestChannelMembership(t *testing.T) {
	server := slacktest.New(t)
	server.AddChannel("C1", "thanks", false)
	client := clientFor(t, server)
	_, err := client.PostMessage(t.Context(), "C1", slack.Message{Text: "thanks"})
	assertCode(t, err, "not_in_channel")
	assertCode(t, client.PostEphemeral(t.Context(), "C1", "U1", slack.Message{Text: "thanks"}), "not_in_channel")
	_, err = client.PostMessage(t.Context(), "unknown", slack.Message{Text: "thanks"})
	assertCode(t, err, "channel_not_found")
	_, err = client.PostMessage(t.Context(), "U_UNKNOWN", slack.Message{Text: "thanks"})
	assertCode(t, err, "channel_not_found")
	require.Empty(t, server.Messages())
	server.AddChannel("C1", "thanks-renamed", true)
	_, err = client.PostMessage(t.Context(), "C1", slack.Message{Text: "thanks"})
	require.NoError(t, err)
	channels, err := client.ListPublicChannels(t.Context())
	require.NoError(t, err)
	require.Equal(t, []slack.Channel{{ID: "C1", Name: "thanks-renamed", IsMember: true}}, channels)
}

func TestFailureQueueIsPerMethodAndHasNoSideEffects(t *testing.T) {
	server := slacktest.New(t)
	server.AddChannel("C1", "thanks", true)
	client := clientFor(t, server)
	server.Fail("chat.postMessage", "invalid_auth")
	server.Fail("chat.postMessage", "ratelimited")
	_, err := client.AuthTest(t.Context())
	require.NoError(t, err, "unrelated calls do not consume failures")
	_, err = client.PostMessage(t.Context(), "C1", slack.Message{Text: "first"})
	assertCode(t, err, "invalid_auth")
	_, err = client.PostMessage(t.Context(), "C1", slack.Message{Text: "second"})
	assertCode(t, err, "ratelimited")
	var apiError *slack.Error
	require.ErrorAs(t, err, &apiError)
	require.Equal(t, http.StatusTooManyRequests, apiError.Status)
	require.Equal(t, time.Second, apiError.RetryAfter)
	require.True(t, slack.IsRateLimited(err))
	require.Empty(t, server.Messages())
	ts, err := client.PostMessage(t.Context(), "C1", slack.Message{Text: "third"})
	require.NoError(t, err)
	require.Len(t, server.Calls("chat.postMessage"), 3)
	require.Len(t, server.Messages(), 1)
	server.Fail("chat.update", "internal_error")
	assertCode(t, client.UpdateMessage(t.Context(), "C1", ts, slack.Message{Text: "update"}), "internal_error")
	server.Fail("chat.delete", "internal_error")
	assertCode(t, client.DeleteMessage(t.Context(), "C1", ts), "internal_error")
	messages := server.Messages()
	require.Equal(t, "third", messages[0].Text)
	require.Zero(t, messages[0].Updates)
	require.False(t, messages[0].Deleted)
	server.Fail("views.open", "invalid_trigger")
	assertCode(t, client.OpenView(t.Context(), "trigger", json.RawMessage(`{"type":"modal"}`)), "invalid_trigger")
	require.Empty(t, server.Views())
}

func TestSnapshotsCannotMutateServerAndDumpRedactsAuthorization(t *testing.T) {
	server := slacktest.New(t)
	server.AddChannel("C1", "thanks", true)
	client := clientFor(t, server)
	_, err := client.PostMessage(t.Context(), "C1", slack.Message{Text: "thanks", Blocks: json.RawMessage(`[{"type":"divider"}]`), ThreadTS: "1699999999.000001"})
	require.NoError(t, err)
	require.NoError(t, client.OpenView(t.Context(), "trigger", json.RawMessage(`{"type":"modal"}`)))
	require.NoError(t, client.PostEphemeral(t.Context(), "C1", "U1", slack.Message{Text: "private thanks", ThreadTS: "1699999999.000001"}))
	calls := server.Calls("chat.postMessage")
	calls[0].Body["text"] = "changed"
	calls[0].Body["blocks"].([]any)[0].(map[string]any)["type"] = "changed"
	messages := server.Messages()
	messages[0].Blocks[0] = '!'
	messages[0].Deleted = true
	views := server.Views()
	views[0][0] = '!'
	require.Equal(t, "thanks", server.Calls("chat.postMessage")[0].Body["text"])
	require.Equal(t, "divider", server.Calls("chat.postMessage")[0].Body["blocks"].([]any)[0].(map[string]any)["type"])
	require.False(t, server.Messages()[0].Deleted)
	require.True(t, json.Valid(server.Messages()[0].Blocks))
	require.True(t, json.Valid(server.Views()[0]))
	resp, err := http.Get(server.URL + "/_calls")
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NotContains(t, string(data), "fake-token")
	var dump struct {
		Calls    []slacktest.Call   `json:"calls"`
		Messages []slacktest.Posted `json:"messages"`
		Views    []json.RawMessage  `json:"views"`
	}
	require.NoError(t, json.Unmarshal(data, &dump))
	require.Len(t, dump.Calls, 3)
	require.Len(t, dump.Messages, 1)
	require.Len(t, dump.Views, 1)
	require.Equal(t, "1699999999.000001", dump.Calls[0].Body["thread_ts"])
	require.Equal(t, "1699999999.000001", dump.Calls[2].Body["thread_ts"])
	require.Equal(t, "1699999999.000001", dump.Messages[0].ThreadTS)
	require.Equal(t, "Bearer [redacted]", dump.Calls[0].Auth)
	require.Equal(t, "Bearer fake-token", server.Calls("")[0].Auth)
}

func TestConcurrentPosts(t *testing.T) {
	server := slacktest.New(t)
	server.AddChannel("C1", "thanks", true)
	client := clientFor(t, server)
	errors := make(chan error, 12)
	for i := range 12 {
		go func() {
			_, err := client.PostMessage(t.Context(), "C1", slack.Message{Text: fmt.Sprint(i)})
			errors <- err
		}()
	}
	for range 12 {
		require.NoError(t, <-errors)
	}
	require.Len(t, server.Messages(), 12)
	seen := make(map[string]bool)
	for _, message := range server.Messages() {
		require.False(t, seen[message.TS])
		seen[message.TS] = true
	}
}

func TestReadMethodsAcceptJSONAndForm(t *testing.T) {
	for _, tc := range []struct {
		method string
		body   map[string]string
		want   string
	}{
		{"users.lookupByEmail", map[string]string{"email": "alice+bonus@example.com"}, `{"ok":true,"user":{"id":"U1"}}`},
		{"users.info", map[string]string{"user": "U1"}, `{"ok":true,"user":{"id":"U1","is_bot":false,"deleted":false,"profile":{"email":"alice+bonus@example.com"}}}`},
		{"users.info", map[string]string{"user": "UBOT"}, `{"ok":true,"user":{"id":"UBOT","is_bot":true,"deleted":false,"profile":{"email":"bot@example.com"}}}`},
		{"chat.getPermalink", map[string]string{"channel": "C1", "message_ts": "1700000000.000123"}, `{"ok":true,"channel":"C1","permalink":"https://example.slack.com/archives/C1/p1700000000000123"}`},
		{"conversations.list", map[string]string{"types": "public_channel", "exclude_archived": "true", "limit": "200"}, `{"ok":true,"channels":[{"id":"C1","name":"thanks","is_member":true}],"response_metadata":{"next_cursor":""}}`},
	} {
		for _, encoding := range []string{"application/json", "application/x-www-form-urlencoded"} {
			t.Run(tc.method+"/"+tc.body["user"]+"/"+encoding, func(t *testing.T) {
				server := slacktest.New(t)
				server.AddUser("alice+bonus@example.com", "U1")
				server.AddBot("bot@example.com", "UBOT")
				server.AddChannel("C1", "thanks", true)
				var body string
				if encoding == "application/json" {
					data, err := json.Marshal(tc.body)
					require.NoError(t, err)
					body = string(data)
				} else {
					form := url.Values{}
					for key, value := range tc.body {
						form.Set(key, value)
					}
					body = form.Encode()
				}
				resp, err := http.Post(server.URL+"/"+tc.method, encoding+"; charset=utf-8", strings.NewReader(body))
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)
				data, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.JSONEq(t, tc.want, string(data))
				calls := server.Calls(tc.method)
				require.Len(t, calls, 1)
				for key, value := range tc.body {
					require.Equal(t, value, calls[0].Body[key])
				}
			})
		}
	}
}

func TestPermalinkForIncomingMessages(t *testing.T) {
	server := slacktest.New(t)
	server.AddChannel("C1", "thanks", true)
	client := clientFor(t, server)
	link, err := client.Permalink(t.Context(), "C1", "1700000000.000123")
	require.NoError(t, err)
	require.Equal(t, "https://example.slack.com/archives/C1/p1700000000000123", link)
	require.Empty(t, server.Messages())
	_, err = client.Permalink(t.Context(), "missing", "1700000000.000123")
	assertCode(t, err, "channel_not_found")
	_, err = client.Permalink(t.Context(), "C1", "")
	assertCode(t, err, "message_not_found")
	server.Fail("chat.getPermalink", "ratelimited")
	_, err = client.Permalink(t.Context(), "C1", "1700000000.000123")
	require.True(t, slack.IsRateLimited(err))
}

func TestFormBlocksAndViews(t *testing.T) {
	server := slacktest.New(t)
	server.AddChannel("C1", "thanks", true)
	blocks := `[{"type":"section","text":{"type":"plain_text","text":"thanks & cheers"}}]`
	form := url.Values{"channel": {"C1"}, "text": {"thanks"}, "thread_ts": {"1699999999.000001"}, "blocks": {blocks}}
	resp, err := http.PostForm(server.URL+"/chat.postMessage", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	messages := server.Messages()
	require.Len(t, messages, 1)
	require.JSONEq(t, blocks, string(messages[0].Blocks))
	require.Equal(t, "1699999999.000001", messages[0].ThreadTS)
	view := `{"type":"modal","blocks":[]}`
	resp, err = http.PostForm(server.URL+"/views.open", url.Values{"trigger_id": {"T1"}, "view": {view}})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, server.Views(), 1)
	require.JSONEq(t, view, string(server.Views()[0]))
}
