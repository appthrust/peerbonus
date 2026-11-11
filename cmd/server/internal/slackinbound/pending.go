package slackinbound

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

type giveButton struct {
	Type     string    `json:"type"`
	Text     plainText `json:"text"`
	ActionID string    `json:"action_id"`
	Value    string    `json:"value"`
}

type giveBlock struct {
	Type     string       `json:"type"`
	Text     *plainText   `json:"text,omitempty"`
	Elements []giveButton `json:"elements,omitempty"`
}

func (h *handler) promptGive(w http.ResponseWriter, r *http.Request, integration domain.SlackIntegration, event slackEvent, senderID string, recipientIDs, recipients []string, message string) {
	remaining, err := h.store.GiveRemaining(r.Context(), senderID, h.now())
	if err != nil {
		http.Error(w, "Balance unavailable", http.StatusServiceUnavailable)
		return
	}
	period, err := h.store.CurrentPeriod(r.Context(), h.now())
	if err != nil {
		http.Error(w, "Period unavailable", http.StatusServiceUnavailable)
		return
	}
	thread := eventThread(event)
	pending, _, err := h.store.CreateSlackPendingGive(r.Context(), domain.SlackPendingGive{
		ChannelID: event.Channel, MessageTS: event.TS, ThreadTS: &thread,
		SenderID: senderID, RecipientIDs: recipientIDs, Message: message,
	}, h.now())
	if err != nil {
		http.Error(w, "Pending give unavailable", http.StatusServiceUnavailable)
		return
	}
	buttons := make([]giveButton, 0, len(integration.MentionPointChoices)+1)
	for _, points := range integration.MentionPointChoices {
		if points > 0 && points <= period.MaxPointsPerPost && points <= remaining/len(recipientIDs) {
			buttons = append(buttons, giveButton{Type: "button", Text: plain(fmt.Sprintf("%d pt", points)),
				ActionID: "peerbonus_give_points", Value: pending.ID + ":" + strconv.Itoa(points)})
		}
	}
	locale := integration.Locale
	if len(buttons) == 0 {
		h.conversationEphemeral(w, r, event.Channel, event.User, thread, slack.Message{Text: fmt.Sprintf(localized(locale,
			"贈れる候補がありません（残り %d pt）。", "No available point choices (%d pt remaining)."), remaining)}, false)
		return
	}
	text := fmt.Sprintf(localized(locale, "%s に何 pt 贈りますか？（残り %d pt）", "How many points would you like to give %s? (%d pt remaining)"), slackMentions(recipients), remaining)
	buttons = append(buttons, giveButton{Type: "button", Text: plain(localized(locale, "やめる", "Cancel")),
		ActionID: "peerbonus_give_cancel", Value: pending.ID})
	blocks, _ := json.Marshal([]giveBlock{
		{Type: "section", Text: &plainText{Type: "mrkdwn", Text: text}},
		{Type: "actions", Elements: buttons},
	})
	h.conversationEphemeral(w, r, event.Channel, event.User, thread, slack.Message{Text: text, Blocks: blocks}, false)
}

func pendingGone(locale domain.SlackLocale) string {
	return localized(locale, "もう一度メンションしてください。", "Please mention Peer Bonus again.")
}

func (h *handler) pendingAction(w http.ResponseWriter, r *http.Request, integration domain.SlackIntegration, payload interaction, actionID, value string) {
	response := &eventResponse{ResponseWriter: w}
	w = response
	defer func() {
		if response.status >= 500 {
			h.replays.forget(r.Header.Get("X-Slack-Request-Timestamp") + ":" + r.Header.Get("X-Slack-Signature"))
		}
	}()
	reply := func(text string, committed bool) {
		err := h.respond(r.Context(), payload.ResponseURL, ephemeralResponse{ResponseType: "ephemeral", Text: text, ReplaceOriginal: true})
		if err != nil && !committed {
			http.Error(w, "Slack response unavailable", http.StatusBadGateway)
		}
	}
	locale := integration.Locale
	member, err := h.member(r.Context(), payload.User.ID)
	if memberMissing(err) {
		reply(h.guidance(locale), false)
		return
	}
	if err != nil {
		http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
		return
	}
	pendingID := value
	points := 0
	if actionID == "peerbonus_give_points" {
		var amount string
		pendingID, amount, _ = strings.Cut(value, ":")
		points, err = strconv.Atoi(amount)
		if err != nil || points <= 0 {
			reply(pendingGone(locale), false)
			return
		}
	}
	pending, err := h.store.SlackPendingGive(r.Context(), pendingID, h.now())
	if errors.Is(err, domain.ErrSlackPendingGiveGone) {
		reply(pendingGone(locale), false)
		return
	}
	if err != nil {
		http.Error(w, "Pending give unavailable", http.StatusServiceUnavailable)
		return
	}
	if pending.SenderID != member.ID {
		reply(localized(locale, "贈る本人だけが操作できます。", "Only the sender can choose or cancel this give."), false)
		return
	}
	if actionID == "peerbonus_give_cancel" {
		err := h.store.DeleteSlackPendingGive(r.Context(), pending.ID, member.ID)
		if errors.Is(err, domain.ErrSlackPendingGiveGone) {
			reply(pendingGone(locale), false)
		} else if err != nil {
			http.Error(w, "Pending give unavailable", http.StatusServiceUnavailable)
		} else {
			reply(localized(locale, "キャンセルしました。", "Cancelled."), true)
		}
		return
	}
	posts, replayed, err := h.store.CreateSlackPosts(r.Context(), store.CreateSlackPostsInput{
		SenderID: member.ID, RecipientIDs: pending.RecipientIDs, Points: points, Message: pending.Message,
		Now: h.now(), PendingID: pending.ID, Source: h.giveSource(r.Context(), pending.ChannelID, pending.MessageTS, ""),
	})
	if errors.Is(err, domain.ErrSlackPendingGiveGone) {
		reply(pendingGone(locale), false)
		return
	}
	if err != nil {
		if _, text := postError(err, locale); text != "" {
			reply(text, false)
		} else {
			http.Error(w, "Unable to give points", http.StatusServiceUnavailable)
		}
		return
	}
	if replayed {
		return
	}
	thread := pending.MessageTS
	if pending.ThreadTS != nil && *pending.ThreadTS != "" {
		thread = *pending.ThreadTS
	}
	recipients := make([]string, len(posts))
	for i, post := range posts {
		if post.Recipient.SlackUserID != nil {
			recipients[i] = "<@" + *post.Recipient.SlackUserID + ">"
		} else {
			recipients[i] = mrkdwnEscaper.Replace(post.Recipient.DisplayName)
		}
	}
	h.successReply(r.Context(), integration, pending.ChannelID, thread, giveReplyText(payload.User.ID, strings.Join(recipients, " "), points))
	reply(localized(locale, "ポイントを贈りました。", "Points given."), true)
}
