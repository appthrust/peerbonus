package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
)

type textObject struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type button struct {
	Type     string     `json:"type"`
	Text     textObject `json:"text"`
	ActionID string     `json:"action_id,omitempty"`
	Value    string     `json:"value,omitempty"`
	URL      string     `json:"url,omitempty"`
}

type block struct {
	Type     string       `json:"type"`
	Text     *textObject  `json:"text,omitempty"`
	Elements []textObject `json:"elements,omitempty"`
}

// Separate action blocks keep their elements typed without custom JSON encoding.
type actions struct {
	Type     string   `json:"type"`
	Elements []button `json:"elements"`
}

var mrkdwnEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func memberName(member domain.Member, slackIDs map[string]string) string {
	if id := slackIDs[member.ID]; id != "" {
		return "<@" + mrkdwnEscaper.Replace(id) + ">"
	}
	return mrkdwnEscaper.Replace(member.DisplayName)
}

func postURL(publicURL, id string) string {
	return strings.TrimRight(publicURL, "/") + "/posts/" + url.PathEscape(id)
}

func openButton(locale domain.SlackLocale, target string) button {
	label := "Peer Bonus で開く"
	if locale == domain.SlackLocaleEN {
		label = "Open in Peer Bonus"
	}
	return button{Type: "button", Text: textObject{Type: "plain_text", Text: label}, URL: target}
}

func section(text string) block {
	return block{Type: "section", Text: &textObject{Type: "mrkdwn", Text: text}}
}

// Announce renders a post. Values must contain display labels, resolved by the
// caller from Program.Values; rendering does not read mutable application state.
func Announce(post domain.Post, sender, recipient domain.Member, claps []domain.Clap, locale domain.SlackLocale, publicURL string, slackIDs map[string]string) slack.Message {
	header := fmt.Sprintf("🎉 %s → %s に %d pt", memberName(sender, slackIDs), memberName(recipient, slackIDs), post.Points)
	clapLabel := fmt.Sprintf("拍手 (%d)", post.ClapCount)
	if locale == domain.SlackLocaleEN {
		header = fmt.Sprintf("🎉 %s → %s · %d pt", memberName(sender, slackIDs), memberName(recipient, slackIDs), post.Points)
		clapLabel = fmt.Sprintf("Clap (%d)", post.ClapCount)
	}
	message := mrkdwnEscaper.Replace(post.Message)
	blocks := make([]any, 0, 5)
	blocks = append(blocks, section(header), section(message))
	fallback := []string{header, message}
	if len(post.Values) > 0 {
		labels := make([]string, len(post.Values))
		for i, label := range post.Values {
			labels[i] = "#" + mrkdwnEscaper.Replace(label)
		}
		text := strings.Join(labels, " ")
		blocks = append(blocks, block{Type: "context", Elements: []textObject{{Type: "mrkdwn", Text: text}}})
		fallback = append(fallback, text)
	}
	clapText := clapLabel
	if len(claps) > 0 {
		names := make([]string, min(len(claps), 3))
		for i := range names {
			names[i] = mrkdwnEscaper.Replace(claps[i].Member.DisplayName)
		}
		clapText += ": " + strings.Join(names, ", ")
		blocks = append(blocks, block{Type: "context", Elements: []textObject{{Type: "mrkdwn", Text: clapText}}})
	}
	target := postURL(publicURL, post.ID)
	blocks = append(blocks, actions{Type: "actions", Elements: []button{
		{Type: "button", Text: textObject{Type: "plain_text", Text: clapLabel}, ActionID: "peerbonus_clap", Value: post.ID},
		openButton(locale, target),
	}})
	fallback = append(fallback, clapText, target)
	// These blocks contain only strings and slices of concrete JSON structs.
	encoded, _ := json.Marshal(blocks)
	return slack.Message{Text: strings.Join(fallback, "\n"), Blocks: encoded}
}

// DirectMessage renders the recipient's private notification with its post link.
func DirectMessage(post domain.Post, sender, recipient domain.Member, locale domain.SlackLocale, publicURL string, slackIDs map[string]string) slack.Message {
	header := fmt.Sprintf("%s から %d pt とメッセージが届きました", memberName(sender, slackIDs), post.Points)
	if locale == domain.SlackLocaleEN {
		header = fmt.Sprintf("You received %d pt and a message from %s", post.Points, memberName(sender, slackIDs))
	}
	text := header + "\n\n" + mrkdwnEscaper.Replace(post.Message)
	target := postURL(publicURL, post.ID)
	encoded, _ := json.Marshal([]any{section(text), actions{Type: "actions", Elements: []button{openButton(locale, target)}}})
	return slack.Message{Text: text + "\n" + target, Blocks: encoded}
}

// ClapUpdate replaces the announcement with the current count and latest names.
func ClapUpdate(post domain.Post, sender, recipient domain.Member, claps []domain.Clap, locale domain.SlackLocale, publicURL string, slackIDs map[string]string) slack.Message {
	return Announce(post, sender, recipient, claps, locale, publicURL, slackIDs)
}
