package slackinbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

type eventEnvelope struct {
	Type      string     `json:"type"`
	Challenge string     `json:"challenge"`
	TeamID    string     `json:"team_id"`
	Event     slackEvent `json:"event"`
}

type slackEvent struct {
	Type     string `json:"type"`
	User     string `json:"user"`
	BotID    string `json:"bot_id"`
	Subtype  string `json:"subtype"`
	Text     string `json:"text"`
	Channel  string `json:"channel"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	Reaction string `json:"reaction"`
	ItemUser string `json:"item_user"`
	Item     struct {
		Type    string `json:"type"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	} `json:"item"`
}

type eventResponse struct {
	http.ResponseWriter
	status int
}

func (w *eventResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (h *handler) events(w http.ResponseWriter, r *http.Request, integration domain.SlackIntegration, body []byte) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Expected JSON body", http.StatusUnsupportedMediaType)
		return
	}
	var envelope eventEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, "Invalid event", http.StatusBadRequest)
		return
	}
	if envelope.Type == "url_verification" {
		writeJSON(w, struct {
			Challenge string `json:"challenge"`
		}{Challenge: envelope.Challenge})
		return
	}
	if envelope.Type != "event_callback" {
		return
	}
	if envelope.TeamID == "" || envelope.TeamID != integration.Workspace.TeamID {
		http.Error(w, "Workspace mismatch", http.StatusForbidden)
		return
	}
	if !integration.GiveInConversation {
		return
	}
	event := envelope.Event
	if event.User == "" || event.User == integration.Workspace.BotUserID || event.BotID != "" || event.Subtype != "" {
		return
	}
	key := r.Header.Get("X-Slack-Request-Timestamp") + ":" + r.Header.Get("X-Slack-Signature")
	if h.replays.seen(key, h.now(), true) {
		return
	}
	switch event.Type {
	case "app_mention":
		if event.Channel != "" && event.TS != "" {
			h.mention(w, r, integration, event)
		}
	case "reaction_added":
		if event.Item.Type == "message" && event.Item.Channel != "" && event.Item.TS != "" {
			h.reaction(w, r, integration, event)
		}
	}
}

func eventThread(event slackEvent) string {
	if event.ThreadTS != "" {
		return event.ThreadTS
	}
	return event.TS
}

func (h *handler) conversationEphemeral(w http.ResponseWriter, r *http.Request, channel, user, thread string, message slack.Message, committed bool) {
	message.ThreadTS = thread
	if err := h.slack.PostEphemeral(r.Context(), channel, user, message); err != nil && !committed {
		http.Error(w, "Slack unavailable", http.StatusBadGateway)
	}
}

func mentionFormat(locale domain.SlackLocale) string {
	return localized(locale, "@Peer Bonus @相手 30 メッセージ の形式で贈ってください（相手は自分以外の 5 人まで）。", "Use @Peer Bonus @recipient 30 message (up to 5 recipients, excluding yourself).")
}

func (h *handler) mention(w http.ResponseWriter, r *http.Request, integration domain.SlackIntegration, event slackEvent) {
	locale := integration.Locale
	reply := func(text string) {
		h.conversationEphemeral(w, r, event.Channel, event.User, eventThread(event), slack.Message{Text: text}, false)
	}
	sender, err := h.member(r.Context(), event.User)
	if memberMissing(err) {
		reply(h.guidance(locale))
		return
	}
	if err != nil {
		http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
		return
	}
	recipients, points, message := parseMention(event.Text, integration.Workspace.BotUserID)
	if len(recipients) == 0 || len(recipients) > domain.SlackMaxMentionRecipients || message == "" {
		reply(mentionFormat(locale))
		return
	}
	ids := make([]string, 0, len(recipients))
	for _, user := range recipients {
		if user == event.User {
			reply(mentionFormat(locale))
			return
		}
		member, err := h.member(r.Context(), user)
		if memberMissing(err) {
			reply(fmt.Sprintf(localized(locale, "<@%s> が見つかりません。まず Web で Peer Bonus を一度開いてもらってください。", "<@%s> is not a member. Ask them to open Peer Bonus in the browser first."), user))
			return
		}
		if err != nil {
			http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
			return
		}
		if member.ID == sender.ID {
			reply(mentionFormat(locale))
			return
		}
		ids = append(ids, member.ID)
	}
	if points == nil {
		h.promptGive(w, r, integration, event, sender.ID, ids, recipients, message)
		return
	}
	_, replayed, err := h.store.CreateSlackPosts(r.Context(), store.CreateSlackPostsInput{
		SenderID: sender.ID, RecipientIDs: ids, Points: *points, Message: message, Now: h.now(),
		Source: h.giveSource(r.Context(), event.Channel, event.TS, ""),
	})
	if err != nil {
		if _, text := postError(err, locale); text != "" {
			reply(text)
		} else {
			http.Error(w, "Unable to give points", http.StatusServiceUnavailable)
		}
		return
	}
	if !replayed {
		h.successReply(r.Context(), integration, event.Channel, eventThread(event), giveReplyText(event.User, slackMentions(recipients), *points))
	}
}

func (h *handler) giveSource(ctx context.Context, channel, ts, emoji string) domain.SlackGiveSource {
	source := domain.SlackGiveSource{ChannelID: channel, MessageTS: ts, Emoji: emoji}
	if link, err := h.slack.Permalink(ctx, channel, ts); err == nil && link != "" {
		source.Permalink = &link
	}
	return source
}

func slackMentions(users []string) string {
	mentions := make([]string, len(users))
	for i, user := range users {
		mentions[i] = "<@" + user + ">"
	}
	return strings.Join(mentions, " ")
}

func giveReplyText(sender, recipients string, points int) string {
	return fmt.Sprintf("🎉 <@%s> → %s %d pt", sender, recipients, points)
}

func (h *handler) successReply(ctx context.Context, integration domain.SlackIntegration, channel, thread, text string) {
	if integration.AnnouncePosts && integration.AnnounceChannel != nil && integration.AnnounceChannel.ID == channel {
		return
	}
	// The ledger has committed. A reply failure must not retry the give.
	_, _ = h.slack.PostMessage(ctx, channel, slack.Message{Text: text, ThreadTS: thread})
}

func (h *handler) reaction(w http.ResponseWriter, r *http.Request, integration domain.SlackIntegration, event slackEvent) {
	emoji := domain.NormalizeSlackEmoji(event.Reaction)
	var rule *domain.SlackReactionRule
	for i := range integration.ReactionRules {
		if integration.ReactionRules[i].Emoji == emoji {
			rule = &integration.ReactionRules[i]
			break
		}
	}
	if rule == nil || event.ItemUser == "" || event.ItemUser == event.User {
		return
	}
	channel, thread := event.Item.Channel, event.Item.TS
	locale := integration.Locale
	reply := func(text string, committed bool) {
		h.conversationEphemeral(w, r, channel, event.User, thread, slack.Message{Text: text}, committed)
	}
	var postID string
	if event.ItemUser == integration.Workspace.BotUserID {
		var err error
		postID, err = h.store.PostIDBySlackAnnounce(r.Context(), channel, thread)
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		if err != nil {
			http.Error(w, "Post unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	sender, err := h.member(r.Context(), event.User)
	if memberMissing(err) {
		reply(h.guidance(locale), false)
		return
	}
	if err != nil {
		http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
		return
	}
	if postID != "" {
		_, _, replayed, err := h.store.CreateSlackClap(r.Context(), store.CreateSlackClapInput{
			PostID: postID, MemberID: sender.ID, Now: h.now(),
			Source: domain.SlackGiveSource{ChannelID: channel, MessageTS: thread, Emoji: rule.Emoji},
		})
		if err != nil {
			if text := clapError(err, locale); text != "" {
				reply(text, false)
			} else {
				http.Error(w, "Unable to clap", http.StatusServiceUnavailable)
			}
			return
		}
		if !replayed {
			if remaining, err := h.store.GiveRemaining(r.Context(), sender.ID, h.now()); err == nil {
				reply(fmt.Sprintf(localized(locale, "拍手しました（残り %d pt）", "Clapped (%d pt remaining)."), remaining), true)
			}
		}
		return
	}
	recipient, err := h.member(r.Context(), event.ItemUser)
	if memberMissing(err) {
		reply(fmt.Sprintf(localized(locale, "<@%s> が見つかりません。まず Web で Peer Bonus を一度開いてもらってください。", "<@%s> is not a member. Ask them to open Peer Bonus in the browser first."), event.ItemUser), false)
		return
	}
	if err != nil {
		http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
		return
	}
	if recipient.ID == sender.ID {
		return
	}
	var values []string
	if rule.ValueKey != nil {
		values = []string{*rule.ValueKey}
	}
	_, replayed, err := h.store.CreateSlackPosts(r.Context(), store.CreateSlackPostsInput{
		SenderID: sender.ID, RecipientIDs: []string{recipient.ID}, Points: rule.Points, Values: values, Now: h.now(),
		Message: fmt.Sprintf(localized(locale, ":%s: Slack のメッセージへのリアクション", ":%s: Reaction to a Slack message"), rule.Emoji),
		Source:  h.giveSource(r.Context(), channel, thread, rule.Emoji),
	})
	if err != nil {
		if _, text := postError(err, locale); text != "" {
			reply(text, false)
		} else {
			http.Error(w, "Unable to give points", http.StatusServiceUnavailable)
		}
		return
	}
	if !replayed {
		h.successReply(r.Context(), integration, channel, thread, giveReplyText(event.User, slackMentions([]string{event.ItemUser}), rule.Points))
	}
}
