package slackinbound

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

type slackID struct {
	ID string `json:"id"`
}

type interaction struct {
	Type       string  `json:"type"`
	Team       slackID `json:"team"`
	Enterprise slackID `json:"enterprise"`
	User       slackID `json:"user"`
	Channel    slackID `json:"channel"`
	TriggerID  string  `json:"trigger_id"`
	Value      string  `json:"value"`
	Actions    []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
	View struct {
		ID              string `json:"id"`
		PrivateMetadata string `json:"private_metadata"`
		State           struct {
			Values map[string]map[string]actionValue `json:"values"`
		} `json:"state"`
	} `json:"view"`
}

type actionValue struct {
	Value           string         `json:"value"`
	SelectedOption  *selectOption  `json:"selected_option"`
	SelectedOptions []selectOption `json:"selected_options"`
}

func (h *handler) interact(w http.ResponseWriter, r *http.Request, locale domain.SlackLocale, payload interaction) {
	if payload.Type != "view_submission" && payload.Type != "block_actions" {
		http.Error(w, "Unsupported interaction", http.StatusBadRequest)
		return
	}
	member, err := h.member(r.Context(), payload.User.ID)
	if memberMissing(err) {
		if payload.Type == "view_submission" {
			viewError(w, "recipient", h.guidance(locale))
		} else {
			h.ephemeral(w, r, payload, h.guidance(locale))
		}
		return
	}
	if err != nil {
		http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
		return
	}
	if payload.Type == "view_submission" {
		h.submit(w, r, locale, payload, member)
	} else {
		h.clap(w, r, locale, payload, member)
	}
}

func (h *handler) submit(w http.ResponseWriter, r *http.Request, locale domain.SlackLocale, payload interaction, member domain.Member) {
	// The cached Slack identity is authoritative, not private_metadata alone.
	if payload.View.PrivateMetadata != member.ID {
		http.Error(w, "Sender mismatch", http.StatusForbidden)
		return
	}
	if payload.View.ID == "" {
		http.Error(w, "Missing view id", http.StatusBadRequest)
		return
	}
	state := payload.View.State.Values
	recipient := state["recipient"]["recipient"].SelectedOption
	if recipient == nil || recipient.Value == "" {
		viewError(w, "recipient", localized(locale, "贈る相手を選んでください。", "Select a recipient."))
		return
	}
	points, err := strconv.Atoi(state["points"]["points"].Value)
	if err != nil {
		viewError(w, "points", localized(locale, "ポイントは整数で入力してください。", "Enter a whole number of points."))
		return
	}
	selected := state["values"]["values"].SelectedOptions
	values := make([]string, len(selected))
	for i, option := range selected {
		values[i] = option.Value
	}
	_, _, err = h.store.CreatePost(r.Context(), store.CreatePostInput{
		SenderID: member.ID, RecipientID: recipient.Value, Points: points,
		Message: state["message"]["message"].Value, Values: values,
		Now: h.now(), IdempotencyKey: "slack:view:" + payload.View.ID,
	})
	if err != nil {
		block, message := postError(err, locale)
		if block == "" {
			http.Error(w, "Unable to give points", http.StatusServiceUnavailable)
			return
		}
		viewError(w, block, message)
		return
	}
	writeJSON(w, struct {
		ResponseAction string `json:"response_action"`
	}{ResponseAction: "clear"})
}

func postError(err error, locale domain.SlackLocale) (string, string) {
	var validation *domain.ValidationError
	var insufficient *domain.InsufficientPointsError
	switch {
	case errors.As(err, &validation):
		block := validation.Field
		if block == "recipientId" {
			block = "recipient"
		}
		var ja string
		switch block {
		case "recipient":
			ja = "贈る相手を確認してください。"
		case "points":
			ja = "ポイントは 1 以上、期間の上限以下で入力してください。"
		case "message":
			ja = "メッセージは 1〜500 文字で入力してください。"
		case "values":
			ja = "バリューは重複せず 3 つまで選んでください。"
		default:
			block, ja = "message", "モーダルを開き直して、もう一度お試しください。"
		}
		return block, localized(locale, ja, validation.Msg)
	case errors.Is(err, domain.ErrSelfTarget):
		return "recipient", localized(locale, "自分には贈れません。", "You cannot give points to yourself.")
	case errors.Is(err, domain.ErrRecipientNotFound):
		return "recipient", localized(locale, "相手が見つかりません。まず Web で一度開いてもらってください。", "Recipient not found. Ask them to open Peer Bonus in the browser first.")
	case errors.As(err, &insufficient):
		return "points", fmt.Sprintf(localized(locale, "ポイントが足りません（残り %d pt）。", "Not enough points (%d pt remaining)."), insufficient.Remaining)
	case errors.Is(err, domain.ErrPeriodClosed):
		return "points", localized(locale, "この期間は締められています。", "This period is closed.")
	case errors.Is(err, domain.ErrUnknownValue):
		return "values", localized(locale, "バリューが変更されました。選び直してください。", "Values have changed. Select them again.")
	default:
		return "", ""
	}
}

func viewError(w http.ResponseWriter, block, message string) {
	writeJSON(w, struct {
		ResponseAction string            `json:"response_action"`
		Errors         map[string]string `json:"errors"`
	}{ResponseAction: "errors", Errors: map[string]string{block: message}})
}

func (h *handler) clap(w http.ResponseWriter, r *http.Request, locale domain.SlackLocale, payload interaction, member domain.Member) {
	for _, action := range payload.Actions {
		if action.ActionID != "peerbonus_clap" {
			continue
		}
		if payload.TriggerID == "" || action.Value == "" || payload.Channel.ID == "" {
			http.Error(w, "Incomplete clap action", http.StatusBadRequest)
			return
		}
		_, _, _, err := h.store.CreateClap(r.Context(), store.CreateClapInput{
			PostID: action.Value, MemberID: member.ID, Now: h.now(),
			IdempotencyKey: "slack:action:" + payload.TriggerID,
		})
		if err != nil {
			var insufficient *domain.InsufficientPointsError
			var text string
			switch {
			case errors.Is(err, domain.ErrOwnPost):
				text = localized(locale, "自分が贈った・受け取った投稿には拍手できません。", "You cannot clap a post you sent or received.")
			case errors.As(err, &insufficient):
				text = fmt.Sprintf(localized(locale, "ポイントが足りません（残り %d pt）。", "Not enough points (%d pt remaining)."), insufficient.Remaining)
			case errors.Is(err, domain.ErrPeriodClosed):
				text = localized(locale, "この期間は締められているため拍手できません。", "This period is closed; you cannot clap this post.")
			case errors.Is(err, domain.ErrNotFound):
				text = localized(locale, "投稿が見つかりません。", "This post is no longer available.")
			default:
				http.Error(w, "Unable to clap", http.StatusServiceUnavailable)
				return
			}
			h.ephemeral(w, r, payload, text)
			return
		}
		remaining, err := h.store.GiveRemaining(r.Context(), member.ID, h.now())
		if err != nil {
			http.Error(w, "Balance unavailable", http.StatusServiceUnavailable)
			return
		}
		h.ephemeral(w, r, payload, fmt.Sprintf(localized(locale, "拍手しました（残り %d pt）", "Clapped (%d pt remaining)."), remaining))
		return
	}
	// Link buttons and other non-mutating actions need only an acknowledgement.
	w.WriteHeader(http.StatusOK)
}

func (h *handler) ephemeral(w http.ResponseWriter, r *http.Request, payload interaction, text string) {
	if payload.Channel.ID == "" || payload.User.ID == "" {
		http.Error(w, "Missing ephemeral recipient", http.StatusBadRequest)
		return
	}
	if err := h.slack.PostEphemeral(r.Context(), payload.Channel.ID, payload.User.ID, slack.Message{Text: text}); err != nil {
		http.Error(w, "Slack unavailable", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) options(w http.ResponseWriter, r *http.Request, payload interaction) {
	if payload.Type != "block_suggestion" {
		http.Error(w, "Unsupported options request", http.StatusBadRequest)
		return
	}
	result := struct {
		Options []selectOption `json:"options"`
	}{Options: []selectOption{}}
	member, err := h.member(r.Context(), payload.User.ID)
	if memberMissing(err) {
		writeJSON(w, result)
		return
	}
	if err != nil {
		http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
		return
	}
	q := strings.TrimSpace(payload.Value)
	if q == "" {
		writeJSON(w, result)
		return
	}
	members, err := h.store.SearchMembers(r.Context(), q, member.ID, 20)
	if err != nil {
		http.Error(w, "Members unavailable", http.StatusServiceUnavailable)
		return
	}
	result.Options = make([]selectOption, 0, len(members))
	for _, member := range members {
		result.Options = append(result.Options, selectOption{
			Text: plain(optionLabel(member.DisplayName + " (" + member.Email + ")")), Value: member.ID,
		})
	}
	writeJSON(w, result)
}
