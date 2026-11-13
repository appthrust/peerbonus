package slackinbound

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestConversationPendingPrompt(t *testing.T) {
	for _, name := range []string{"affordable", "per-post maximum", "multiple recipients", "no affordable choice", "existing thread", "reused pending", "Japanese"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			p := mentionEvent()
			p.Event.Text = "<@UBOT> <@U456> Thanks for helping"
			s.remaining = 40
			want := []int{10, 30}
			thread := p.Event.TS
			switch name {
			case "per-post maximum":
				s.period.MaxPointsPerPost = 20
				want = []int{10}
			case "multiple recipients":
				p.Event.Text += " <@U789>"
				want = []int{10}
			case "no affordable choice":
				s.remaining = 9
				want = nil
			case "existing thread":
				p.Event.ThreadTS = "100.000"
				thread = p.Event.ThreadTS
			case "reused pending":
				s.pending = domain.SlackPendingGive{ID: "existing-pending"}
			case "Japanese":
				s.integration.Locale = domain.SlackLocaleJA
			}
			w := serveEvent(t, New(o), p)
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Body.String())
			require.Empty(t, s.slackPosts)
			require.Empty(t, client.posted)
			require.Empty(t, client.permalinkCalls)
			require.Len(t, s.pendingCreates, 1)
			in := s.pendingCreates[0]
			require.Equal(t, "sender", in.SenderID)
			require.Equal(t, "Thanks for helping", in.Message)
			require.Equal(t, "C123", in.ChannelID)
			require.Equal(t, p.Event.TS, in.MessageTS)
			require.Equal(t, thread, *in.ThreadTS)
			if name == "multiple recipients" {
				require.Equal(t, []string{"recipient", "recipient-2"}, in.RecipientIDs)
			} else {
				require.Equal(t, []string{"recipient"}, in.RecipientIDs)
			}
			require.Len(t, client.messages, 1)
			require.Equal(t, thread, client.messages[0].ThreadTS)
			if name == "no affordable choice" {
				require.Empty(t, client.messages[0].Blocks)
				require.Contains(t, client.messages[0].Text, "9 pt remaining")
				return
			}
			var blocks []giveBlock
			require.NoError(t, json.Unmarshal(client.messages[0].Blocks, &blocks))
			require.Len(t, blocks, 2)
			require.Equal(t, "section", blocks[0].Type)
			require.Equal(t, "mrkdwn", blocks[0].Text.Type)
			require.Contains(t, blocks[0].Text.Text, "<@U456>")
			if name == "Japanese" {
				require.Contains(t, blocks[0].Text.Text, "に何 pt 贈りますか？（残り 40 pt）")
			}
			require.Equal(t, "actions", blocks[1].Type)
			require.Len(t, blocks[1].Elements, len(want)+1)
			for i, amount := range want {
				button := blocks[1].Elements[i]
				require.Equal(t, "button", button.Type)
				require.Equal(t, "plain_text", button.Text.Type)
				require.Equal(t, "peerbonus_give_points", button.ActionID)
				require.Equal(t, s.pending.ID+":"+strconv.Itoa(amount), button.Value)
			}
			cancel := blocks[1].Elements[len(want)]
			require.Equal(t, "peerbonus_give_cancel", cancel.ActionID)
			require.Equal(t, s.pending.ID, cancel.Value)
		})
	}
}

func pendingPayload(t *testing.T) interaction {
	p := clapPayload(t)
	p.Actions[0].ActionID, p.Actions[0].Value = "peerbonus_give_points", "pending-1:30"
	p.ResponseURL = "https://hooks.slack.com/commands/test"
	return p
}

func pendingFixture() domain.SlackPendingGive {
	return domain.SlackPendingGive{ID: "pending-1", ChannelID: "C123", MessageTS: "123.456", ThreadTS: new("100.000"),
		SenderID: "sender", RecipientIDs: []string{"recipient"}, Message: "Thanks", ExpiresAt: testNow.Add(time.Hour)}
}

func TestConversationPendingActions(t *testing.T) {
	for _, name := range []string{"give", "cancel", "other member", "expired", "missing", "consumed during give", "insufficient", "replayed", "bad amount", "announce channel", "cancel raced"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			s.pending = pendingFixture()
			p := pendingPayload(t)
			messages := captureResponse(t, &o)
			want := "Points given."
			switch name {
			case "cancel":
				p.Actions[0].ActionID, p.Actions[0].Value = "peerbonus_give_cancel", s.pending.ID
				want = "Cancelled."
			case "other member":
				p.User.ID = "U789"
				want = "Only the sender"
			case "expired":
				s.pending.ExpiresAt = testNow
				want = "mention Peer Bonus again"
			case "missing":
				s.pending.ID = "different"
				want = "mention Peer Bonus again"
			case "consumed during give":
				s.postErr = domain.ErrSlackPendingGiveGone
				want = "mention Peer Bonus again"
			case "insufficient":
				s.postErr = &domain.InsufficientPointsError{Remaining: 1, Required: 30}
				want = "1 pt remaining"
			case "replayed":
				s.replayed = true
			case "bad amount":
				p.Actions[0].Value = "pending-1:thirty"
				want = "mention Peer Bonus again"
			case "announce channel":
				s.integration.AnnouncePosts = true
				s.integration.AnnounceChannel = &domain.SlackChannel{ID: "C123"}
			case "cancel raced":
				p.Actions[0].ActionID, p.Actions[0].Value = "peerbonus_give_cancel", s.pending.ID
				s.pendingDeleteErr = domain.ErrSlackPendingGiveGone
				want = "mention Peer Bonus again"
			}
			w := serve(New(o), "/slack/interactions", interactionForm(t, p))
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Body.String())
			if name == "replayed" {
				require.Empty(t, *messages)
				require.Empty(t, client.posted)
				return
			}
			require.Len(t, *messages, 1)
			require.True(t, (*messages)[0].ReplaceOriginal)
			require.Equal(t, "ephemeral", (*messages)[0].ResponseType)
			require.Contains(t, (*messages)[0].Text, want)
			if name == "give" || name == "announce channel" {
				require.Len(t, s.slackPosts, 1)
				in := s.slackPosts[0]
				require.Equal(t, "pending-1", in.PendingID)
				require.Equal(t, "sender", in.SenderID)
				require.Equal(t, []string{"recipient"}, in.RecipientIDs)
				require.Equal(t, 30, in.Points)
				require.Equal(t, "Thanks", in.Message)
				require.Equal(t, "C123", in.Source.ChannelID)
				require.Equal(t, "123.456", in.Source.MessageTS)
				require.Equal(t, client.permalink, *in.Source.Permalink)
				if name == "give" {
					require.Len(t, client.posted, 1)
					require.Equal(t, "100.000", client.posted[0].ThreadTS)
					require.Equal(t, "🎉 <@U123> → <@U456> 30 pt", client.posted[0].Text)
				} else {
					require.Empty(t, client.posted)
				}
			} else {
				require.Empty(t, client.posted)
				if s.postErr == nil {
					require.Empty(t, s.slackPosts)
				}
			}
			if name == "cancel" || name == "cancel raced" {
				require.Equal(t, []string{"pending-1:sender"}, s.pendingDeletes)
			} else {
				require.Empty(t, s.pendingDeletes)
			}
		})
	}
}

func TestConversationPendingTransientAndReplyFailures(t *testing.T) {
	for _, name := range []string{"load", "write", "response after write", "response before write", "prompt store", "prompt Slack"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			s.pending = pendingFixture()
			p := pendingPayload(t)
			captureResponse(t, &o)
			switch name {
			case "load":
				s.pendingErr = errors.New("database unavailable")
			case "write":
				s.postErr = errors.New("database unavailable")
			case "response before write":
				s.pending.ExpiresAt = testNow
			case "prompt store":
				s.pendingCreateErr = errors.New("database unavailable")
			case "prompt Slack":
				client.ephemeralErr = errors.New("Slack unavailable")
			}
			if name == "response after write" || name == "response before write" {
				o.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unavailable") })}
			}
			h := New(o)
			var wCode int
			if name == "prompt store" || name == "prompt Slack" {
				e := mentionEvent()
				e.Event.Text = "<@UBOT> <@U456> Thanks"
				wCode = serveEvent(t, h, e).Code
			} else {
				wCode = serve(h, "/slack/interactions", interactionForm(t, p)).Code
			}
			if name == "response after write" {
				require.Equal(t, http.StatusOK, wCode)
				require.Len(t, s.slackPosts, 1)
				return
			}
			require.GreaterOrEqual(t, wCode, 500)
			if name == "write" {
				s.postErr = nil
				require.Equal(t, http.StatusOK, serve(h, "/slack/interactions", interactionForm(t, p)).Code)
				require.Len(t, s.slackPosts, 2)
			}
		})
	}
}

func TestConversationInteractionFlagSeparation(t *testing.T) {
	for _, name := range []string{"command", "options", "clap", "submission", "points off", "cancel off"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			path, payload, status := "/slack/interactions", clapPayload(t), http.StatusOK
			switch name {
			case "command":
				path = "/slack/commands"
				status = http.StatusNotFound
			case "options":
				path = "/slack/options"
				status = http.StatusNotFound
			case "submission":
				payload = submission()
			case "points off", "cancel off":
				s.integration.GiveFromSlack, s.integration.GiveInConversation = true, false
				payload = pendingPayload(t)
				if name == "cancel off" {
					payload.Actions[0].ActionID = "peerbonus_give_cancel"
				}
			}
			w := serve(New(o), path, interactionForm(t, payload))
			require.Equal(t, status, w.Code)
			assertNoConversationWork(t, s, client)
			require.Empty(t, s.posts)
			require.Empty(t, s.claps)
			require.Empty(t, s.pendingReads)
		})
	}
}
