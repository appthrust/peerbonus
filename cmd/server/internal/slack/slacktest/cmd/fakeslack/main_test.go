package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack/slacktest"
	"github.com/stretchr/testify/require"
)

type startupOutput chan string

func (s startupOutput) Write(p []byte) (int, error) { s <- string(p); return len(p), nil }

func TestRunSeedsFixturesAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	output := make(startupOutput, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"--listen", "127.0.0.1:0", "--user", "alice@example.com=U1", "--user", "bob@example.com=U2", "--bot", "bot@example.com=UBOT", "--channel", "C1=thanks:member", "--channel", "C2=general", "--channel", "C3=team:true"}, output, io.Discard)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("fake server did not stop")
		}
	})
	var line string
	select {
	case line = <-output:
	case <-time.After(5 * time.Second):
		t.Fatal("fake server did not announce its URL")
	}
	require.True(t, strings.HasPrefix(line, "URL=http://127.0.0.1:"))
	base := strings.TrimSpace(strings.TrimPrefix(line, "URL="))
	client, err := slack.New("fake-token", slack.WithBaseURL(base))
	require.NoError(t, err)
	for email, want := range map[string]string{"alice@example.com": "U1", "bob@example.com": "U2"} {
		id, err := client.LookupUserByEmail(t.Context(), email)
		require.NoError(t, err)
		require.Equal(t, want, id)
		user, err := client.UserInfo(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, slack.User{ID: id, Email: email}, user)
	}
	bot, err := client.UserInfo(t.Context(), "UBOT")
	require.NoError(t, err)
	require.Equal(t, slack.User{ID: "UBOT", Email: "bot@example.com", IsBot: true}, bot)
	channels, err := client.ListPublicChannels(t.Context())
	require.NoError(t, err)
	require.Equal(t, []slack.Channel{{ID: "C1", Name: "thanks", IsMember: true}, {ID: "C2", Name: "general", IsMember: false}, {ID: "C3", Name: "team", IsMember: true}}, channels)
	ts, err := client.PostMessage(t.Context(), "C1", slack.Message{Text: "thanks", ThreadTS: "1699999999.000001"})
	require.NoError(t, err)
	link, err := client.Permalink(t.Context(), "C1", ts)
	require.NoError(t, err)
	require.Equal(t, "https://example.slack.com/archives/C1/p"+strings.ReplaceAll(ts, ".", ""), link)
	require.NoError(t, client.PostEphemeral(t.Context(), "C1", "U1", slack.Message{Text: "thanks privately", ThreadTS: "1699999999.000001"}))
	require.NoError(t, client.UpdateMessage(t.Context(), "C1", ts, slack.Message{Text: "updated"}))
	require.NoError(t, client.DeleteMessage(t.Context(), "C1", ts))
	resp, err := http.Get(base + "/_calls")
	require.NoError(t, err)
	defer resp.Body.Close()
	var dump struct {
		Calls    []slacktest.Call   `json:"calls"`
		Messages []slacktest.Posted `json:"messages"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&dump))
	require.Len(t, dump.Calls, 12)
	for _, call := range dump.Calls {
		if call.Method == "chat.postMessage" || call.Method == "chat.postEphemeral" {
			require.Equal(t, "1699999999.000001", call.Body["thread_ts"])
		}
	}
	require.Len(t, dump.Messages, 1)
	require.Equal(t, "1699999999.000001", dump.Messages[0].ThreadTS)
	require.True(t, dump.Messages[0].Deleted)
	require.Equal(t, 1, dump.Messages[0].Updates)
	require.Equal(t, "updated", dump.Messages[0].Text)
}

func TestInvalidFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--user", "alice"}, {"--user", "=U1"}, {"--user", "alice="},
		{"--bot", "bot"}, {"--bot", "=UBOT"}, {"--bot", "bot="},
		{"--channel", "C1"}, {"--channel", "=thanks"}, {"--channel", "C1="},
		{"--channel", "C1=:member"}, {"--channel", "C1=thanks:maybe"},
		{"unexpected"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			require.Error(t, run(t.Context(), args, io.Discard, io.Discard))
		})
	}
	require.NoError(t, run(t.Context(), []string{"--help"}, io.Discard, io.Discard))
}
