package domain

import (
	"errors"
	"strings"
	"time"
)

// Limits of giving inside a Slack conversation (design reaction-give §2).
const (
	SlackMaxReactionRules     = 8
	SlackMaxMentionRecipients = 5
	SlackMaxPointChoices      = 5
	SlackPendingGiveTTL       = 24 * time.Hour
)

var (
	// ErrSlackPendingGiveGone: the pending give expired, was used, or belongs to someone else.
	ErrSlackPendingGiveGone = errors.New("slack_pending_give_gone")
)

// SlackReactionRule maps one emoji to the points a reaction gives (R11).
// Emoji is normalized by NormalizeSlackEmoji. ValueKey is an optional Program value.
type SlackReactionRule struct {
	Emoji    string
	Points   int
	ValueKey *string
}

// SlackGiveSource identifies the Slack message a give came from.
// Emoji is recorded for reference only, not deduplication; it is empty for a mention.
type SlackGiveSource struct {
	ChannelID, MessageTS string
	Emoji                string
	Permalink            *string
}

// SlackPendingGive is a mention without points, waiting for the sender to
// choose an amount (R8). It is usable once, only by SenderID, until ExpiresAt.
type SlackPendingGive struct {
	ID, ChannelID, MessageTS string
	ThreadTS                 *string
	SenderID                 string
	RecipientIDs             []string
	Message                  string
	ExpiresAt                time.Time
}

// NormalizeSlackEmoji returns the bare emoji name: no colons, no skin-tone
// modifier ("+1::skin-tone-3" and ":+1:" both become "+1"), lower case.
func NormalizeSlackEmoji(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.Index(name, "::"); i >= 0 {
		name = name[:i]
	}
	return strings.Trim(name, ":")
}

// ValidateSlackReactionRules expects normalized emoji names. Points are
// bounded by the Program's current maximum; value keys must exist in it.
func ValidateSlackReactionRules(rules []SlackReactionRule, program Program) error {
	if len(rules) > SlackMaxReactionRules {
		return &ValidationError{Field: "reactionRules", Msg: "must contain at most 8 rules"}
	}
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.Emoji == "" || len(rule.Emoji) > 100 || strings.ContainsAny(rule.Emoji, ": \t\n") {
			return &ValidationError{Field: "reactionRules", Msg: "emoji must be a Slack emoji name without colons"}
		}
		if seen[rule.Emoji] {
			return &ValidationError{Field: "reactionRules", Msg: "emoji must be unique"}
		}
		seen[rule.Emoji] = true
		if rule.Points < 1 || rule.Points > program.MaxPointsPerPost {
			return &ValidationError{Field: "reactionRules", Msg: "points must be between 1 and the maximum per post"}
		}
		if rule.ValueKey != nil {
			known := false
			for _, value := range program.Values {
				known = known || value.Key == *rule.ValueKey
			}
			if !known {
				return ErrUnknownValue
			}
		}
	}
	return nil
}

// ValidateMentionPointChoices bounds the amounts offered when a mention omits points.
func ValidateMentionPointChoices(choices []int, program Program) error {
	if len(choices) < 1 || len(choices) > SlackMaxPointChoices {
		return &ValidationError{Field: "mentionPointChoices", Msg: "must contain 1 to 5 amounts"}
	}
	seen := make(map[int]bool, len(choices))
	for _, points := range choices {
		if points < 1 || points > program.MaxPointsPerPost || seen[points] {
			return &ValidationError{Field: "mentionPointChoices", Msg: "amounts must be unique and between 1 and the maximum per post"}
		}
		seen[points] = true
	}
	return nil
}
