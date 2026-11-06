package slackinbound

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
	"github.com/stretchr/testify/require"
)

func submission() interaction {
	p := interaction{Type: "view_submission", Team: slackID{ID: "T123"}, User: slackID{ID: "U123"}}
	p.View.ID, p.View.PrivateMetadata = "V123", "sender"
	p.View.State.Values = map[string]map[string]actionValue{
		"recipient": {"recipient": {SelectedOption: &selectOption{Value: "recipient"}}},
		"points":    {"points": {Value: "25"}},
		"message":   {"message": {Value: "Thanks for helping & teaching!"}},
		"values":    {"values": {SelectedOptions: []selectOption{{Value: "care"}, {Value: "learn"}}}},
	}
	return p
}

func clapPayload(t *testing.T) interaction {
	t.Helper()
	p := interaction{Type: "block_actions", Team: slackID{ID: "T123"}, User: slackID{ID: "U123"}, Channel: slackID{ID: "C123"}, TriggerID: "trigger-clap"}
	p.Actions = append(p.Actions, struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	}{ActionID: "peerbonus_clap", Value: "post-123"})
	return p
}

func TestSubmissionUsesExistingWriteAndStableIdempotency(t *testing.T) {
	s, _, o := fixture()
	body := interactionForm(t, submission())
	w := serve(New(o), "/slack/interactions", body)
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"response_action":"clear"}`, w.Body.String())
	require.Equal(t, []store.CreatePostInput{{SenderID: "sender", RecipientID: "recipient", Points: 25,
		Message: "Thanks for helping & teaching!", Values: []string{"care", "learn"}, Now: testNow, IdempotencyKey: "slack:view:V123"}}, s.posts)
	// A fresh handler has no replay cache, but passes the same durable key.
	require.Equal(t, http.StatusOK, serve(New(o), "/slack/interactions", body).Code)
	require.Equal(t, s.posts[0], s.posts[1])
}

func TestSubmissionValidationMapsToBlocks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		block string
	}{
		{"recipient", &domain.ValidationError{Field: "recipient", Msg: "invalid recipient"}, "recipient"},
		{"recipientId", &domain.ValidationError{Field: "recipientId", Msg: "invalid recipient"}, "recipient"},
		{"points", &domain.ValidationError{Field: "points", Msg: "invalid points"}, "points"},
		{"message", &domain.ValidationError{Field: "message", Msg: "invalid message"}, "message"},
		{"values", &domain.ValidationError{Field: "values", Msg: "invalid values"}, "values"},
		{"idempotency", &domain.ValidationError{Field: "idempotencyKey", Msg: "already used"}, "message"},
		{"self", domain.ErrSelfTarget, "recipient"},
		{"missing", domain.ErrRecipientNotFound, "recipient"},
		{"insufficient", &domain.InsufficientPointsError{Remaining: 4, Required: 25}, "points"},
		{"closed", domain.ErrPeriodClosed, "points"},
		{"unknown value", domain.ErrUnknownValue, "values"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, o := fixture()
			s.postErr = fmt.Errorf("write: %w", tc.err)
			w := serve(New(o), "/slack/interactions", interactionForm(t, submission()))
			require.Equal(t, http.StatusOK, w.Code)
			block, message := postError(s.postErr, domain.SlackLocaleEN)
			require.Equal(t, tc.block, block)
			require.NotEmpty(t, message)
			require.JSONEq(t, fmt.Sprintf(`{"response_action":"errors","errors":{%q:%q}}`, tc.block, message), w.Body.String())
		})
	}
}

func TestSubmissionMalformedInputsDoNotWrite(t *testing.T) {
	for _, name := range []string{"sender mismatch", "missing id", "missing recipient", "decimal points", "missing points"} {
		t.Run(name, func(t *testing.T) {
			s, _, o := fixture()
			p := submission()
			switch name {
			case "sender mismatch":
				p.View.PrivateMetadata = "other-member"
			case "missing id":
				p.View.ID = ""
			case "missing recipient":
				delete(p.View.State.Values, "recipient")
			case "decimal points":
				p.View.State.Values["points"]["points"] = actionValue{Value: "1.5"}
			case "missing points":
				delete(p.View.State.Values, "points")
			}
			w := serve(New(o), "/slack/interactions", interactionForm(t, p))
			switch name {
			case "sender mismatch":
				require.Equal(t, http.StatusForbidden, w.Code)
			case "missing id":
				require.Equal(t, http.StatusBadRequest, w.Code)
			default:
				require.Equal(t, http.StatusOK, w.Code)
				require.Contains(t, w.Body.String(), `"response_action":"errors"`)
			}
			require.Empty(t, s.posts)
		})
	}
}

func TestSubmissionLeavesDomainValidationInStore(t *testing.T) {
	for _, name := range []string{"self target", "negative points", "long message", "too many values"} {
		t.Run(name, func(t *testing.T) {
			s, _, o := fixture()
			p := submission()
			s.postErr = domain.ErrSelfTarget
			switch name {
			case "self target":
				p.View.State.Values["recipient"]["recipient"] = actionValue{SelectedOption: &selectOption{Value: "sender"}}
			case "negative points":
				p.View.State.Values["points"]["points"] = actionValue{Value: "-1"}
			case "long message":
				p.View.State.Values["message"]["message"] = actionValue{Value: strings.Repeat("あ", 501)}
			case "too many values":
				p.View.State.Values["values"]["values"] = actionValue{SelectedOptions: []selectOption{{Value: "a"}, {Value: "b"}, {Value: "c"}, {Value: "d"}}}
			}
			require.Equal(t, http.StatusOK, serve(New(o), "/slack/interactions", interactionForm(t, p)).Code)
			require.Len(t, s.posts, 1)
		})
	}
}

func TestClapCreatesThroughStoreAndReportsRemaining(t *testing.T) {
	for _, locale := range []domain.SlackLocale{domain.SlackLocaleJA, domain.SlackLocaleEN} {
		t.Run(string(locale), func(t *testing.T) {
			s, slack, o := fixture()
			s.integration.Locale = locale
			w := serve(New(o), "/slack/interactions", interactionForm(t, clapPayload(t)))
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, []store.CreateClapInput{{PostID: "post-123", MemberID: "sender", Now: testNow, IdempotencyKey: "slack:action:trigger-clap"}}, s.claps)
			require.Equal(t, "sender", s.remainingMember)
			require.Equal(t, testNow, s.remainingNow)
			require.Equal(t, "C123", slack.channel)
			require.Equal(t, "U123", slack.user)
			require.Len(t, slack.messages, 1)
			require.Equal(t, localized(locale, "拍手しました（残り 398 pt）", "Clapped (398 pt remaining)."), slack.messages[0].Text)
		})
	}
}

func TestClapDomainErrorsAreEphemeral(t *testing.T) {
	for _, tc := range []struct {
		err  error
		text string
	}{
		{domain.ErrOwnPost, "You cannot clap a post you sent or received."},
		{&domain.InsufficientPointsError{Remaining: 1, Required: 2}, "Not enough points (1 pt remaining)."},
		{domain.ErrPeriodClosed, "This period is closed; you cannot clap this post."},
		{domain.ErrNotFound, "This post is no longer available."},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			s, slack, o := fixture()
			s.clapErr = fmt.Errorf("write: %w", tc.err)
			require.Equal(t, http.StatusOK, serve(New(o), "/slack/interactions", interactionForm(t, clapPayload(t))).Code)
			require.Len(t, s.claps, 1)
			require.Len(t, slack.messages, 1)
			require.Equal(t, tc.text, slack.messages[0].Text)
			require.Empty(t, s.remainingMember)
		})
	}
}

func TestUnknownInteractionMemberGetsGuidance(t *testing.T) {
	for _, kind := range []string{"view_submission", "block_actions"} {
		t.Run(kind, func(t *testing.T) {
			s, slack, o := fixture()
			s.memberErr = domain.ErrNotFound
			p := submission()
			if kind == "block_actions" {
				p = clapPayload(t)
			}
			w := serve(New(o), "/slack/interactions", interactionForm(t, p))
			require.Equal(t, http.StatusOK, w.Code)
			if kind == "view_submission" {
				require.Contains(t, w.Body.String(), `"response_action":"errors"`)
				require.Contains(t, w.Body.String(), "Open Peer Bonus in the browser once")
			} else {
				require.Len(t, slack.messages, 1)
				require.Contains(t, slack.messages[0].Text, "Open Peer Bonus in the browser once")
			}
			require.Empty(t, s.posts)
			require.Empty(t, s.claps)
		})
	}
}

func TestOptionsSearchExcludesCaller(t *testing.T) {
	s, _, o := fixture()
	s.members = []domain.Member{{ID: "member-b", DisplayName: "Bob", Email: "bob@example.test", Pending: true}}
	p := interaction{Type: "block_suggestion", Team: slackID{ID: "T123"}, User: slackID{ID: "U123"}, Value: "Bo"}
	w := serve(New(o), "/slack/options", interactionForm(t, p))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "Bo", s.searchQ)
	require.Equal(t, "sender", s.searchExclude)
	require.Equal(t, 20, s.searchLimit)
	require.JSONEq(t, `{"options":[{"text":{"type":"plain_text","text":"Bob (bob@example.test)"},"value":"member-b"}]}`, w.Body.String())
}

func TestOptionsEmptyAndUnknownCallerDoNotListMembers(t *testing.T) {
	for _, unknown := range []bool{true, false} {
		s, _, o := fixture()
		p := interaction{Type: "block_suggestion", Team: slackID{ID: "T123"}, User: slackID{ID: "U123"}}
		if unknown {
			s.memberErr = domain.ErrNotFound
			p.Value = "Bo"
		}
		w := serve(New(o), "/slack/options", interactionForm(t, p))
		require.Equal(t, http.StatusOK, w.Code)
		require.JSONEq(t, `{"options":[]}`, w.Body.String())
		require.Empty(t, s.searchQ)
	}
}

func TestUnknownWriteErrorDoesNotLeakDetails(t *testing.T) {
	s, slack, o := fixture()
	s.postErr = errors.New("sensitive database detail")
	w := serve(New(o), "/slack/interactions", interactionForm(t, submission()))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.NotContains(t, w.Body.String(), "sensitive")
	s.clapErr = errors.New("sensitive database detail")
	w = serve(New(o), "/slack/interactions", interactionForm(t, clapPayload(t)))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.NotContains(t, w.Body.String(), "sensitive")
	require.Empty(t, slack.messages)
}
