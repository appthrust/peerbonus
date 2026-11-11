package slackinbound

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/slackinbound/slackinboundtest"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
	"github.com/stretchr/testify/require"
)

func (s *fakeStore) LinkMemberSlackUser(_ context.Context, memberID, userID string, _ time.Time) error {
	s.links = append(s.links, memberID+":"+userID)
	return s.linkErr
}
func (s *fakeStore) CreateSlackPosts(_ context.Context, in store.CreateSlackPostsInput) ([]domain.Post, bool, error) {
	s.slackPosts = append(s.slackPosts, in)
	posts := make([]domain.Post, len(in.RecipientIDs))
	for i, id := range in.RecipientIDs {
		posts[i] = domain.Post{ID: "post-" + id, Points: in.Points, Message: in.Message}
		for _, member := range s.slackMembers {
			if member.ID == id {
				posts[i].Recipient = member
			}
		}
	}
	return posts, s.replayed, s.postErr
}
func (s *fakeStore) CreateSlackClap(_ context.Context, in store.CreateSlackClapInput) (domain.Clap, domain.Post, bool, error) {
	s.slackClaps = append(s.slackClaps, in)
	return domain.Clap{ID: "clap"}, domain.Post{ID: in.PostID}, s.replayed, s.clapErr
}
func (s *fakeStore) PostIDBySlackAnnounce(_ context.Context, channel, ts string) (string, error) {
	s.announceLookups = append(s.announceLookups, channel+":"+ts)
	return s.announcePost, s.announceErr
}
func (s *fakeStore) CreateSlackPendingGive(_ context.Context, in domain.SlackPendingGive, now time.Time) (domain.SlackPendingGive, bool, error) {
	s.pendingCreates = append(s.pendingCreates, in)
	if s.pending.ID != "" {
		return s.pending, false, s.pendingCreateErr
	}
	in.ID, in.ExpiresAt = "pending-1", now.Add(domain.SlackPendingGiveTTL)
	s.pending = in
	return in, true, s.pendingCreateErr
}
func (s *fakeStore) SlackPendingGive(_ context.Context, id string, now time.Time) (domain.SlackPendingGive, error) {
	s.pendingReads = append(s.pendingReads, id)
	if s.pendingErr != nil {
		return domain.SlackPendingGive{}, s.pendingErr
	}
	if s.pending.ID != id || !now.Before(s.pending.ExpiresAt) {
		return domain.SlackPendingGive{}, domain.ErrSlackPendingGiveGone
	}
	return s.pending, nil
}
func (s *fakeStore) DeleteSlackPendingGive(_ context.Context, id, senderID string) error {
	s.pendingDeletes = append(s.pendingDeletes, id+":"+senderID)
	return s.pendingDeleteErr
}
func (s *fakeSlack) UserInfo(_ context.Context, user string) (slack.User, error) {
	s.userInfoCalls = append(s.userInfoCalls, user)
	if s.userInfoErr != nil {
		return slack.User{}, s.userInfoErr
	}
	if info, ok := s.userInfo[user]; ok {
		return info, nil
	}
	return slack.User{}, slack.ErrUserNotFound
}
func (s *fakeSlack) Permalink(_ context.Context, channel, ts string) (string, error) {
	s.permalinkCalls = append(s.permalinkCalls, channel+":"+ts)
	return s.permalink, s.permalinkErr
}
func (s *fakeSlack) PostMessage(_ context.Context, channel string, message slack.Message) (string, error) {
	s.postChannels = append(s.postChannels, channel)
	s.posted = append(s.posted, message)
	return "reply-ts", s.postErr
}

func conversationFixture() (*fakeStore, *fakeSlack, Options) {
	s, client, o := fixture()
	s.integration.GiveFromSlack = false
	s.integration.GiveInConversation = true
	s.integration.Workspace.BotUserID = "UBOT"
	s.integration.MentionPointChoices = []int{10, 30, 50}
	s.integration.ReactionRules = []domain.SlackReactionRule{{Emoji: "pb-10", Points: 10, ValueKey: new("care")}}
	s.slackMembers = map[string]domain.Member{
		"U123": {ID: "sender", SlackUserID: new("U123")},
		"U456": {ID: "recipient", SlackUserID: new("U456")},
		"U789": {ID: "recipient-2", SlackUserID: new("U789")},
	}
	client.permalink = "https://workspace.slack.com/archives/C123/p123"
	return s, client, o
}

func mentionEvent() eventEnvelope {
	return eventEnvelope{Type: "event_callback", TeamID: "T123", Event: slackEvent{
		Type: "app_mention", User: "U123", Channel: "C123", TS: "123.456", Text: "<@UBOT> <@U456> 30 Thanks for helping",
	}}
}

func reactionEvent() eventEnvelope {
	p := eventEnvelope{Type: "event_callback", TeamID: "T123", Event: slackEvent{
		Type: "reaction_added", User: "U123", Reaction: "pb-10", ItemUser: "U456",
	}}
	p.Event.Item.Type, p.Event.Item.Channel, p.Event.Item.TS = "message", "C123", "123.456"
	return p
}

func eventRequest(t *testing.T, envelope eventEnvelope) *http.Request {
	t.Helper()
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	return slackinboundtest.SignedJSONRequest(testSecret, testNow, "/slack/events", body)
}

func serveEvent(t *testing.T, h http.Handler, envelope eventEnvelope) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, eventRequest(t, envelope))
	return w
}

func assertNoConversationWork(t *testing.T, s *fakeStore, client *fakeSlack) {
	t.Helper()
	require.Empty(t, s.memberLookups)
	require.Empty(t, s.emailLookups)
	require.Empty(t, s.slackPosts)
	require.Empty(t, s.slackClaps)
	require.Empty(t, s.pendingCreates)
	require.Empty(t, s.announceLookups)
	require.Empty(t, s.remainingMember)
	require.Empty(t, client.userInfoCalls)
	require.Empty(t, client.permalinkCalls)
	require.Empty(t, client.messages)
	require.Empty(t, client.posted)
	require.Empty(t, client.view)
}

func TestConversationEventGates(t *testing.T) {
	t.Run("challenge while disabled and retried", func(t *testing.T) {
		s, client, o := conversationFixture()
		s.integration.GiveInConversation = false
		h := New(o)
		for range 2 {
			w := serveEvent(t, h, eventEnvelope{Type: "url_verification", Challenge: "verify-me"})
			require.Equal(t, http.StatusOK, w.Code)
			require.JSONEq(t, `{"challenge":"verify-me"}`, w.Body.String())
		}
		assertNoConversationWork(t, s, client)
	})
	for _, name := range []string{"disabled", "wrong team", "bot user", "bot id", "subtype", "unknown event", "unknown emoji", "self reaction", "empty recipient", "non-message reaction"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			p := mentionEvent()
			status := http.StatusOK
			switch name {
			case "disabled":
				s.integration.GiveInConversation = false
			case "wrong team":
				p.TeamID = "Tother"
				s.integration.GiveInConversation = false
				status = http.StatusForbidden
			case "bot user":
				p.Event.User = "UBOT"
			case "bot id":
				p.Event.BotID = "B123"
			case "subtype":
				p.Event.Subtype = "message_changed"
			case "unknown event":
				p.Event.Type = "message"
			case "unknown emoji":
				p = reactionEvent()
				p.Event.Reaction = "heart"
			case "self reaction":
				p = reactionEvent()
				p.Event.ItemUser = p.Event.User
			case "empty recipient":
				p = reactionEvent()
				p.Event.ItemUser = ""
			case "non-message reaction":
				p = reactionEvent()
				p.Event.Item.Type = "file"
			}
			w := serveEvent(t, New(o), p)
			require.Equal(t, status, w.Code)
			if status == http.StatusOK {
				require.Empty(t, w.Body.String())
			}
			assertNoConversationWork(t, s, client)
		})
	}
	for _, name := range []string{"secret", "store", "client", "workspace"} {
		t.Run("missing "+name, func(t *testing.T) {
			s, _, o := conversationFixture()
			switch name {
			case "secret":
				o.SigningSecret = ""
			case "store":
				o.Store = nil
			case "client":
				o.Slack = nil
			case "workspace":
				s.integration.Workspace = nil
			}
			r := eventRequest(t, mentionEvent())
			r.Body = unreadBody{t}
			w := httptest.NewRecorder()
			New(o).ServeHTTP(w, r)
			require.Equal(t, http.StatusNotFound, w.Code)
		})
	}
}

func TestConversationEventHTTPBoundary(t *testing.T) {
	for _, name := range []string{"bad signature", "oversized", "wrong media", "invalid json", "method"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			r := eventRequest(t, mentionEvent())
			status := http.StatusUnauthorized
			switch name {
			case "bad signature":
				r.Header.Set("X-Slack-Signature", "v0="+strings.Repeat("0", 64))
			case "oversized":
				r = slackinboundtest.SignedJSONRequest(testSecret, testNow, "/slack/events", []byte(strings.Repeat("x", maxBodyBytes+1)))
			case "wrong media":
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				status = http.StatusUnsupportedMediaType
			case "invalid json":
				r = slackinboundtest.SignedJSONRequest(testSecret, testNow, "/slack/events", []byte("{"))
				status = http.StatusBadRequest
			case "method":
				r.Method = http.MethodGet
				status = http.StatusMethodNotAllowed
			}
			w := httptest.NewRecorder()
			New(o).ServeHTTP(w, r)
			require.Equal(t, status, w.Code)
			assertNoConversationWork(t, s, client)
		})
	}
}

func TestConversationMentionWritesAndReplies(t *testing.T) {
	for _, name := range []string{"new thread", "existing thread", "announce channel", "different announce channel", "announcements disabled", "permalink failure", "reply failure", "multiple recipients", "durable replay"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			p := mentionEvent()
			thread := p.Event.TS
			switch name {
			case "existing thread":
				p.Event.ThreadTS = "100.000"
				thread = p.Event.ThreadTS
			case "announce channel":
				s.integration.AnnouncePosts = true
				s.integration.AnnounceChannel = &domain.SlackChannel{ID: "C123"}
			case "different announce channel":
				s.integration.AnnouncePosts = true
				s.integration.AnnounceChannel = &domain.SlackChannel{ID: "Cother"}
			case "announcements disabled":
				s.integration.AnnounceChannel = &domain.SlackChannel{ID: "C123"}
			case "permalink failure":
				client.permalinkErr = errors.New("unavailable")
			case "reply failure":
				client.postErr = errors.New("unavailable")
			case "multiple recipients":
				p.Event.Text += " <@U789>"
			case "durable replay":
				s.replayed = true
			}
			h := New(o)
			w := serveEvent(t, h, p)
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Body.String())
			require.Len(t, s.slackPosts, 1)
			in := s.slackPosts[0]
			require.Equal(t, "sender", in.SenderID)
			require.Equal(t, 30, in.Points)
			require.Equal(t, "Thanks for helping", in.Message)
			require.Equal(t, testNow, in.Now)
			require.Equal(t, "C123", in.Source.ChannelID)
			require.Equal(t, p.Event.TS, in.Source.MessageTS)
			require.Empty(t, in.Source.Emoji)
			require.Empty(t, in.PendingID)
			if name == "permalink failure" {
				require.Nil(t, in.Source.Permalink)
			} else {
				require.Equal(t, client.permalink, *in.Source.Permalink)
			}
			if name == "multiple recipients" {
				require.Equal(t, []string{"recipient", "recipient-2"}, in.RecipientIDs)
			} else {
				require.Equal(t, []string{"recipient"}, in.RecipientIDs)
			}
			if name == "announce channel" || name == "durable replay" {
				require.Empty(t, client.posted)
			} else {
				require.Len(t, client.posted, 1)
				require.Equal(t, thread, client.posted[0].ThreadTS)
				text := "🎉 <@U123> → <@U456> 30 pt"
				if name == "multiple recipients" {
					text = "🎉 <@U123> → <@U456> <@U789> 30 pt"
				}
				require.Equal(t, text, client.posted[0].Text)
			}
			replies := len(client.posted)
			w = serveEvent(t, h, p)
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Body.String())
			require.Len(t, s.slackPosts, 1)
			require.Len(t, client.posted, replies)
			require.Empty(t, client.messages)
		})
	}
}

func TestConversationMentionBusinessErrors(t *testing.T) {
	for _, name := range []string{"sender missing", "no recipient", "no message", "too many recipients", "self", "recipient missing", "insufficient", "closed", "validation"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			p := mentionEvent()
			want := "Use @Peer Bonus"
			switch name {
			case "sender missing":
				delete(s.slackMembers, "U123")
				want = "Open Peer Bonus"
			case "no recipient":
				p.Event.Text = "<@UBOT> 30 Thanks"
			case "no message":
				p.Event.Text = "<@UBOT> <@U456> 30"
			case "too many recipients":
				p.Event.Text += " <@U1> <@U2> <@U3> <@U4> <@U5>"
			case "self":
				p.Event.Text = "<@UBOT> <@U123> 30 Thanks"
			case "recipient missing":
				delete(s.slackMembers, "U456")
				want = "<@U456> is not a member"
			case "insufficient":
				s.postErr = &domain.InsufficientPointsError{Remaining: 2, Required: 30}
				want = "2 pt remaining"
			case "closed":
				s.postErr = domain.ErrPeriodClosed
				want = "period is closed"
			case "validation":
				s.postErr = &domain.ValidationError{Field: "points", Msg: "Too many points"}
				want = "Too many points"
			}
			w := serveEvent(t, New(o), p)
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Body.String())
			require.Len(t, client.messages, 1)
			require.Contains(t, client.messages[0].Text, want)
			require.Equal(t, p.Event.TS, client.messages[0].ThreadTS)
			require.Equal(t, p.Event.User, client.user)
			require.Empty(t, client.posted)
			if s.postErr == nil {
				require.Empty(t, s.slackPosts)
			}
		})
	}
}

func TestConversationMemberResolution(t *testing.T) {
	for _, name := range []string{"linked", "bot", "deleted", "empty email", "missing member", "slack error", "link error"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			delete(s.slackMembers, "U123")
			user := slack.User{ID: "U123", Email: "verified@example.test"}
			s.emailMember = domain.Member{ID: "sender"}
			status := http.StatusOK
			switch name {
			case "bot":
				user.IsBot = true
			case "deleted":
				user.Deleted = true
			case "empty email":
				user.Email = ""
			case "missing member":
				s.emailErr = domain.ErrNotFound
			case "slack error":
				client.userInfoErr = errors.New("unavailable")
				status = http.StatusServiceUnavailable
			case "link error":
				s.linkErr = errors.New("unavailable")
				status = http.StatusServiceUnavailable
			}
			client.userInfo = map[string]slack.User{"U123": user}
			w := serveEvent(t, New(o), mentionEvent())
			require.Equal(t, status, w.Code)
			require.Equal(t, []string{"U123"}, client.userInfoCalls)
			if name == "linked" {
				require.Equal(t, []string{"verified@example.test"}, s.emailLookups)
				require.Equal(t, []string{"sender:U123"}, s.links)
				require.Len(t, s.slackPosts, 1)
			} else {
				require.Empty(t, s.slackPosts)
			}
			if name == "bot" || name == "deleted" || name == "empty email" {
				require.Empty(t, s.emailLookups)
				require.Empty(t, s.links)
			}
		})
	}
}

func TestConversationTransientFailureCanRetry(t *testing.T) {
	for _, name := range []string{"store", "ephemeral"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			p := mentionEvent()
			if name == "store" {
				s.postErr = errors.New("sensitive database detail")
			} else {
				p.Event.Text = "<@UBOT>"
				client.ephemeralErr = errors.New("Slack unavailable")
			}
			h := New(o)
			w := serveEvent(t, h, p)
			require.GreaterOrEqual(t, w.Code, 500)
			require.NotContains(t, w.Body.String(), "sensitive")
			s.postErr, client.ephemeralErr = nil, nil
			w = serveEvent(t, h, p)
			require.Equal(t, http.StatusOK, w.Code)
			if name == "store" {
				require.Len(t, s.slackPosts, 2)
				require.Len(t, client.posted, 1)
			} else {
				require.Len(t, client.messages, 2)
			}
		})
	}
}

func TestConversationReactionGiveAndAnnounceClap(t *testing.T) {
	for _, name := range []string{"give", "normalized", "announce", "unrelated bot message", "clap replay", "clap domain error", "clap transient", "clap reply failure", "clap balance failure"} {
		t.Run(name, func(t *testing.T) {
			s, client, o := conversationFixture()
			p := reactionEvent()
			if strings.HasPrefix(name, "clap") || name == "announce" || name == "unrelated bot message" {
				p.Event.ItemUser = "UBOT"
				s.announcePost = "announced-post"
			}
			switch name {
			case "normalized":
				p.Event.Reaction = ":PB-10::skin-tone-3:"
			case "unrelated bot message":
				s.announceErr = domain.ErrNotFound
			case "clap replay":
				s.replayed = true
			case "clap domain error":
				s.clapErr = domain.ErrOwnPost
			case "clap transient":
				s.clapErr = errors.New("unavailable")
			case "clap reply failure":
				client.ephemeralErr = errors.New("unavailable")
			case "clap balance failure":
				s.remainingErr = errors.New("unavailable")
			}
			w := serveEvent(t, New(o), p)
			if name == "clap transient" {
				require.Equal(t, http.StatusServiceUnavailable, w.Code)
				return
			}
			require.Equal(t, http.StatusOK, w.Code)
			require.Empty(t, w.Body.String())
			if name == "give" || name == "normalized" {
				require.Len(t, s.slackPosts, 1)
				in := s.slackPosts[0]
				require.Equal(t, 10, in.Points)
				require.Equal(t, []string{"recipient"}, in.RecipientIDs)
				require.Equal(t, []string{"care"}, in.Values)
				require.Equal(t, ":pb-10: Reaction to a Slack message", in.Message)
				require.Equal(t, domain.SlackGiveSource{ChannelID: "C123", MessageTS: "123.456", Emoji: "pb-10", Permalink: &client.permalink}, in.Source)
				require.Len(t, client.posted, 1)
				require.Equal(t, "123.456", client.posted[0].ThreadTS)
			} else {
				require.Empty(t, s.slackPosts)
				require.Empty(t, client.posted)
				if name == "unrelated bot message" {
					require.Empty(t, s.memberLookups)
					require.Empty(t, s.slackClaps)
					require.Empty(t, client.messages)
					return
				}
				require.Equal(t, []store.CreateSlackClapInput{{PostID: "announced-post", MemberID: "sender", Now: testNow, Source: domain.SlackGiveSource{ChannelID: "C123", MessageTS: "123.456", Emoji: "pb-10"}}}, s.slackClaps)
				if name == "clap replay" || name == "clap balance failure" {
					require.Empty(t, client.messages)
				} else {
					require.Len(t, client.messages, 1)
				}
				if name == "announce" {
					require.Equal(t, "Clapped (398 pt remaining).", client.messages[0].Text)
				}
				if name == "clap domain error" {
					require.Contains(t, client.messages[0].Text, "cannot clap")
				}
			}
		})
	}
}
