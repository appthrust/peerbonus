package slackinbound

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

func (h *handler) command(w http.ResponseWriter, r *http.Request, locale domain.SlackLocale, form url.Values) {
	if form.Get("command") != "/peerbonus" {
		http.Error(w, "Unknown command", http.StatusBadRequest)
		return
	}
	member, err := h.member(r.Context(), form.Get("user_id"))
	if memberMissing(err) {
		h.commandReply(w, r, form.Get("response_url"), h.guidance(locale))
		return
	}
	if err != nil {
		http.Error(w, "Member unavailable", http.StatusServiceUnavailable)
		return
	}
	switch strings.TrimSpace(form.Get("text")) {
	case "balance":
		remaining, err := h.store.GiveRemaining(r.Context(), member.ID, h.now())
		if err != nil {
			http.Error(w, "Balance unavailable", http.StatusServiceUnavailable)
			return
		}
		h.commandReply(w, r, form.Get("response_url"), fmt.Sprintf(localized(locale,
			"贈れるポイントは残り %d pt です。", "You have %d pt remaining to give."), remaining))
	case "":
		if form.Get("trigger_id") == "" {
			http.Error(w, "Missing trigger_id", http.StatusBadRequest)
			return
		}
		program, err := h.store.Program(r.Context())
		if err != nil {
			http.Error(w, "Program unavailable", http.StatusServiceUnavailable)
			return
		}
		period, err := h.store.CurrentPeriod(r.Context(), h.now())
		if err != nil {
			http.Error(w, "Period unavailable", http.StatusServiceUnavailable)
			return
		}
		view, err := json.Marshal(giveModal(member.ID, program, period, locale))
		if err != nil {
			http.Error(w, "Modal unavailable", http.StatusInternalServerError)
			return
		}
		if err := h.slack.OpenView(r.Context(), form.Get("trigger_id"), view); err != nil {
			http.Error(w, "Slack unavailable", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		h.commandReply(w, r, form.Get("response_url"), localized(locale,
			"/peerbonus または /peerbonus balance を使ってください。",
			"Use /peerbonus or /peerbonus balance."))
	}
}

type ephemeralResponse struct {
	ResponseType    string `json:"response_type"`
	Text            string `json:"text"`
	ReplaceOriginal bool   `json:"replace_original,omitempty"`
}

func (h *handler) commandReply(w http.ResponseWriter, r *http.Request, responseURL, text string) {
	if err := h.respond(r.Context(), responseURL, ephemeralResponse{ResponseType: "ephemeral", Text: text}); err != nil {
		http.Error(w, "Slack response unavailable", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) respond(ctx context.Context, rawURL string, message ephemeralResponse) error {
	endpoint, err := url.Parse(rawURL)
	// response_url is a capability supplied inside Slack's signed body, not
	// an administrator-configurable webhook. Keep its egress on Slack hosts.
	if err != nil || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.Port() != "" ||
		(endpoint.Hostname() != "hooks.slack.com" && endpoint.Hostname() != "hooks.slack-gov.com") {
		return fmt.Errorf("invalid Slack response URL")
	}
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Slack response status %d", response.StatusCode)
	}
	return nil
}

type plainText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func plain(text string) plainText { return plainText{Type: "plain_text", Text: text} }

type selectOption struct {
	Text  plainText `json:"text"`
	Value string    `json:"value"`
}

type inputElement struct {
	Type             string         `json:"type"`
	ActionID         string         `json:"action_id"`
	MinQueryLength   *int           `json:"min_query_length,omitempty"`
	IsDecimalAllowed *bool          `json:"is_decimal_allowed,omitempty"`
	MinValue         string         `json:"min_value,omitempty"`
	MaxValue         string         `json:"max_value,omitempty"`
	Multiline        bool           `json:"multiline,omitempty"`
	MaxLength        int            `json:"max_length,omitempty"`
	Options          []selectOption `json:"options,omitempty"`
	MaxSelectedItems int            `json:"max_selected_items,omitempty"`
}

type inputBlock struct {
	Type     string       `json:"type"`
	BlockID  string       `json:"block_id"`
	Label    plainText    `json:"label"`
	Element  inputElement `json:"element"`
	Optional bool         `json:"optional,omitempty"`
}

type modal struct {
	Type            string       `json:"type"`
	CallbackID      string       `json:"callback_id"`
	Title           plainText    `json:"title"`
	Submit          plainText    `json:"submit"`
	Close           plainText    `json:"close"`
	PrivateMetadata string       `json:"private_metadata"`
	Blocks          []inputBlock `json:"blocks"`
}

func giveModal(senderID string, program domain.Program, period domain.Period, locale domain.SlackLocale) modal {
	view := modal{
		Type: "modal", CallbackID: "peerbonus_give", Title: plain("Peer Bonus"),
		Submit: plain(localized(locale, "贈る", "Give")), Close: plain(localized(locale, "キャンセル", "Cancel")),
		PrivateMetadata: senderID,
		Blocks: []inputBlock{
			{Type: "input", BlockID: "recipient", Label: plain(localized(locale, "贈る相手", "Recipient")),
				Element: inputElement{Type: "external_select", ActionID: "recipient", MinQueryLength: new(1)}},
			{Type: "input", BlockID: "points", Label: plain(localized(locale, "ポイント", "Points")),
				Element: inputElement{Type: "number_input", ActionID: "points", IsDecimalAllowed: new(false), MinValue: "1", MaxValue: strconv.Itoa(period.MaxPointsPerPost)}},
			{Type: "input", BlockID: "message", Label: plain(localized(locale, "メッセージ", "Message")),
				Element: inputElement{Type: "plain_text_input", ActionID: "message", Multiline: true, MaxLength: 500}},
		},
	}
	// Program permits zero values; Slack rejects a static select with no
	// options. In that case there is no optional values field to display.
	if len(program.Values) > 0 {
		options := make([]selectOption, 0, len(program.Values))
		for _, value := range program.Values {
			options = append(options, selectOption{Text: plain(optionLabel(value.Label)), Value: value.Key})
		}
		view.Blocks = append(view.Blocks, inputBlock{
			Type: "input", BlockID: "values", Label: plain(localized(locale, "バリュー", "Values")), Optional: true,
			Element: inputElement{Type: "multi_static_select", ActionID: "values", Options: options, MaxSelectedItems: 3},
		})
	}
	return view
}

// Slack select option labels have a 75-character limit; member names and
// program labels are not constrained to that limit by the domain.
func optionLabel(text string) string {
	count := 0
	for i := range text {
		if count == 75 {
			return text[:i]
		}
		count++
	}
	return text
}
