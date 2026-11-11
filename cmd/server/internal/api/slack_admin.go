package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

// SlackOptions contains process-wide credentials and platform discovery configuration.
type SlackOptions struct {
	Client                  *slack.Client
	SigningSecretConfigured bool
	PlatformAPIURL          string
}

type slackAdminClient interface {
	AuthTest(context.Context) (slack.AuthInfo, error)
	ListPublicChannels(context.Context) ([]slack.Channel, error)
	PostMessage(context.Context, string, slack.Message) (string, error)
}

type slackAdminStore interface {
	Program(context.Context) (domain.Program, error)
	SlackIntegration(context.Context) (domain.SlackIntegration, error)
	UpdateSlackSettings(context.Context, store.SlackSettingsPatch, time.Time) (domain.SlackIntegration, error)
	FixSlackWorkspace(context.Context, domain.SlackWorkspace, bool, time.Time) (domain.SlackIntegration, error)
	SetSlackLastError(context.Context, *string, time.Time) error
	ListSlackDeliveries(context.Context, string, int) ([]domain.SlackDelivery, error)
	SlackDeliveryStats(context.Context, time.Time) (domain.SlackDeliveryStats, error)
}

type slackInboundClient struct {
	url  string
	http *http.Client
}

func newSlackInboundClient(origin, projectID string, transport http.RoundTripper) *slackInboundClient {
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	return &slackInboundClient{
		url: strings.TrimRight(origin, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/inbound-endpoints/peerbonus-slack",
		http: &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (a *API) discoverSlackInbound(s *request) *slackInboundResponse {
	if a.slackInbound == nil {
		return nil
	}
	r, err := http.NewRequestWithContext(s.r.Context(), http.MethodGet, a.slackInbound.url, nil)
	if err != nil {
		return &slackInboundResponse{Reason: new("platform_unavailable")}
	}
	r.Header.Set("Authorization", s.bearer)
	r.Header.Set("Accept", "application/json")
	response, err := a.slackInbound.http.Do(r)
	if err != nil {
		return &slackInboundResponse{Reason: new("platform_unavailable")}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode != http.StatusOK {
		return &slackInboundResponse{Reason: new("platform_unavailable")}
	}
	var endpoint struct {
		URL        string `json:"url"`
		Ready      bool   `json:"ready"`
		Conditions []struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		} `json:"conditions"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&endpoint); err != nil {
		return &slackInboundResponse{Reason: new("platform_unavailable")}
	}
	result := &slackInboundResponse{Ready: endpoint.Ready}
	if endpoint.URL != "" {
		result.URL = &endpoint.URL
	}
	for _, condition := range endpoint.Conditions {
		if condition.Type == "Ready" {
			if condition.Reason != "" {
				result.Reason = &condition.Reason
			}
			break
		}
	}
	return result
}

type slackAPIError struct {
	Status  int    `json:"-"`
	Code    string `json:"error"`
	Message string `json:"message"`
	Detail  any    `json:"detail"`
}

func (e *slackAPIError) Error() string { return e.Code + ": " + e.Message }

func (a *API) slackState(integration domain.SlackIntegration) domain.SlackState {
	if a.slack == nil {
		return domain.SlackStateUnconfigured
	}
	if integration.Workspace == nil {
		return domain.SlackStateConfigured
	}
	if integration.LastErrorCode != nil && *integration.LastErrorCode == "team_changed" {
		return domain.SlackStateTeamChanged
	}
	return domain.SlackStateConnected
}

func (a *API) adminSlack(s *request) (any, int, error) {
	integration, err := a.slackStore.SlackIntegration(s.r.Context())
	if err != nil {
		return nil, 0, err
	}
	return a.slackResponse(s, integration)
}

func (a *API) slackResponse(s *request, integration domain.SlackIntegration) (any, int, error) {
	stats, err := a.slackStore.SlackDeliveryStats(s.r.Context(), s.now)
	if err != nil {
		return nil, 0, err
	}
	result := slackIntegrationResponse{State: a.slackState(integration)}
	result.Credential.BotToken = a.slack != nil
	result.Credential.SigningSecret = a.slackSigningSecretConfigured
	if ws := integration.Workspace; ws != nil {
		result.Workspace = &slackWorkspaceResponse{TeamID: ws.TeamID, TeamName: ws.TeamName, BotUserID: ws.BotUserID, VerifiedAt: ws.VerifiedAt}
	}
	if ch := integration.AnnounceChannel; ch != nil {
		result.Settings.AnnounceChannel = &slackChannelResponse{ID: ch.ID, Name: ch.Name}
	}
	result.Settings.AnnouncePosts = integration.AnnouncePosts
	result.Settings.DMRecipientOnPost = integration.DMRecipientOnPost
	result.Settings.GiveFromSlack = integration.GiveFromSlack
	result.Settings.GiveInConversation = integration.GiveInConversation
	result.Settings.MentionPointChoices = integration.MentionPointChoices
	result.Settings.ReactionRules = make([]slackReactionRule, 0, len(integration.ReactionRules))
	for _, rule := range integration.ReactionRules {
		result.Settings.ReactionRules = append(result.Settings.ReactionRules, slackReactionRule(rule))
	}
	result.Settings.Locale = integration.Locale
	if integration.LastErrorCode != nil {
		result.LastError = &slackLastErrorResponse{Code: *integration.LastErrorCode, At: integration.LastErrorAt}
	}
	result.Deliveries = slackDeliveryStatsResponse(stats)
	result.Inbound = a.discoverSlackInbound(s)
	return result, 200, nil
}

func (a *API) patchSlack(s *request) (any, int, error) {
	var input struct {
		AnnounceChannelID   json.RawMessage      `json:"announceChannelId"`
		AnnouncePosts       *bool                `json:"announcePosts"`
		DMRecipientOnPost   *bool                `json:"dmRecipientOnPost"`
		GiveFromSlack       *bool                `json:"giveFromSlack"`
		GiveInConversation  *bool                `json:"giveInConversation"`
		MentionPointChoices *[]int               `json:"mentionPointChoices"`
		ReactionRules       *[]slackReactionRule `json:"reactionRules"`
		Locale              *domain.SlackLocale  `json:"locale"`
	}
	if err := decode(s, &input); err != nil {
		return nil, 0, err
	}
	if input.Locale != nil && *input.Locale != domain.SlackLocaleJA && *input.Locale != domain.SlackLocaleEN {
		return nil, 0, httpx.Invalid("Locale must be ja or en")
	}
	integration, err := a.slackStore.SlackIntegration(s.r.Context())
	if err != nil {
		return nil, 0, err
	}
	if (input.AnnounceChannelID != nil || input.AnnouncePosts != nil || input.DMRecipientOnPost != nil || input.GiveFromSlack != nil || input.GiveInConversation != nil) && a.slackState(integration) != domain.SlackStateConnected {
		return nil, 0, slackNotConnected()
	}
	if ((input.GiveFromSlack != nil && *input.GiveFromSlack) || (input.GiveInConversation != nil && *input.GiveInConversation)) && !a.slackSigningSecretConfigured {
		return nil, 0, httpx.Fail(409, "slack_signing_secret_missing", "Configure the Slack signing secret before enabling Slack input")
	}
	patch := store.SlackSettingsPatch{AnnouncePosts: input.AnnouncePosts, DMRecipientOnPost: input.DMRecipientOnPost, GiveFromSlack: input.GiveFromSlack, GiveInConversation: input.GiveInConversation, MentionPointChoices: input.MentionPointChoices, Locale: input.Locale}
	if input.MentionPointChoices != nil || input.ReactionRules != nil {
		program, err := a.slackStore.Program(s.r.Context())
		if err != nil {
			return nil, 0, err
		}
		if input.MentionPointChoices != nil {
			if err := domain.ValidateMentionPointChoices(*input.MentionPointChoices, program); err != nil {
				return nil, 0, err
			}
		}
		if input.ReactionRules != nil {
			rules := make([]domain.SlackReactionRule, len(*input.ReactionRules))
			for i, rule := range *input.ReactionRules {
				rules[i] = domain.SlackReactionRule(rule)
				rules[i].Emoji = domain.NormalizeSlackEmoji(rule.Emoji)
			}
			if err := domain.ValidateSlackReactionRules(rules, program); err != nil {
				return nil, 0, err
			}
			patch.ReactionRules = &rules
		}
	}
	if input.AnnounceChannelID != nil {
		var id *string
		if err := json.Unmarshal(input.AnnounceChannelID, &id); err != nil || (id != nil && *id == "") {
			return nil, 0, httpx.Invalid("Announce channel must be a public channel ID or null")
		}
		var selected *domain.SlackChannel
		if id != nil {
			channels, err := a.slackPublicChannels(s)
			if err != nil {
				return nil, 0, err
			}
			for _, channel := range channels {
				if channel.ID == *id {
					selected = &domain.SlackChannel{ID: channel.ID, Name: channel.Name, IsMember: channel.IsMember}
					break
				}
			}
			if selected == nil {
				return nil, 0, httpx.Invalid("Announce channel must be a public channel returned by Slack")
			}
		}
		patch.AnnounceChannel = &selected
		integration.AnnounceChannel = selected
	}
	if input.AnnouncePosts != nil {
		integration.AnnouncePosts = *input.AnnouncePosts
	}
	if integration.AnnouncePosts && integration.AnnounceChannel == nil {
		return nil, 0, httpx.Invalid("An announce channel is required when announcePosts is true")
	}
	integration, err = a.slackStore.UpdateSlackSettings(s.r.Context(), patch, s.now)
	if err != nil {
		return nil, 0, err
	}
	return a.slackResponse(s, integration)
}

func (a *API) verifySlack(s *request) (any, int, error) {
	var input struct {
		Force bool `json:"force"`
	}
	if err := decodeBody(s, &input, true); err != nil {
		return nil, 0, err
	}
	if a.slack == nil {
		return nil, 0, httpx.Fail(409, "slack_not_configured", "Configure a Slack bot token before verifying the workspace")
	}
	info, err := a.slack.AuthTest(s.r.Context())
	if err != nil {
		return nil, 0, slackFailure(err)
	}
	ws := domain.SlackWorkspace{TeamID: info.TeamID, TeamName: info.TeamName, BotUserID: info.BotUserID, EnterpriseID: info.EnterpriseID, VerifiedAt: s.now, VerifiedBy: s.member.ID}
	integration, err := a.slackStore.FixSlackWorkspace(s.r.Context(), ws, input.Force, s.now)
	if errors.Is(err, domain.ErrSlackTeamChanged) {
		current, readErr := a.slackStore.SlackIntegration(s.r.Context())
		if readErr != nil {
			return nil, 0, readErr
		}
		if err := a.slackStore.SetSlackLastError(s.r.Context(), new("team_changed"), s.now); err != nil {
			return nil, 0, err
		}
		detail := slackTeamChangedDetail{Token: slackTeamResponse{TeamID: info.TeamID, TeamName: info.TeamName}}
		if current.Workspace != nil {
			detail.Current = slackTeamResponse{TeamID: current.Workspace.TeamID, TeamName: current.Workspace.TeamName}
		}
		return nil, 0, &slackAPIError{Status: 409, Code: "slack_team_changed", Message: "The bot token belongs to a different Slack workspace; verify with force to replace it", Detail: detail}
	}
	if err != nil {
		return nil, 0, err
	}
	if err := a.slackStore.SetSlackLastError(s.r.Context(), nil, s.now); err != nil {
		return nil, 0, err
	}
	integration.LastErrorCode, integration.LastErrorAt = nil, nil
	a.slackChannelsMu.Lock()
	a.slackChannels, a.slackChannelsExpires = nil, time.Time{}
	a.slackChannelsMu.Unlock()
	return a.slackResponse(s, integration)
}

func (a *API) slackChannelsList(s *request) (any, int, error) {
	limit, err := queryLimit(s, 20, 20)
	if err != nil {
		return nil, 0, err
	}
	integration, err := a.slackStore.SlackIntegration(s.r.Context())
	if err != nil {
		return nil, 0, err
	}
	if a.slackState(integration) != domain.SlackStateConnected {
		return nil, 0, slackNotConnected()
	}
	channels, err := a.slackPublicChannels(s)
	if err != nil {
		return nil, 0, err
	}
	q := strings.ToLower(s.r.URL.Query().Get("q"))
	items := make([]slackChannelCandidate, 0, min(limit, len(channels)))
	for _, channel := range channels {
		if strings.HasPrefix(strings.ToLower(channel.Name), q) {
			items = append(items, slackChannelCandidate{ID: channel.ID, Name: channel.Name, IsMember: channel.IsMember})
			if len(items) == limit {
				break
			}
		}
	}
	return map[string]any{"items": items}, 200, nil
}

func (a *API) slackPublicChannels(s *request) ([]slack.Channel, error) {
	a.slackChannelsMu.Lock()
	defer a.slackChannelsMu.Unlock()
	if s.now.Before(a.slackChannelsExpires) {
		return a.slackChannels, nil
	}
	channels, err := a.slack.ListPublicChannels(s.r.Context())
	if err != nil {
		return nil, slackFailure(err)
	}
	a.slackChannels, a.slackChannelsExpires = channels, s.now.Add(time.Minute)
	return channels, nil
}

func (a *API) testSlack(s *request) (any, int, error) {
	if err := emptyBody(s); err != nil {
		return nil, 0, err
	}
	integration, err := a.slackStore.SlackIntegration(s.r.Context())
	if err != nil {
		return nil, 0, err
	}
	if a.slackState(integration) != domain.SlackStateConnected || integration.AnnounceChannel == nil {
		return nil, 0, slackNotConnected()
	}
	text := "Peer Bonus の Slack 連携テストです。"
	if integration.Locale == domain.SlackLocaleEN {
		text = "This is a test message from Peer Bonus."
	}
	ts, err := a.slack.PostMessage(s.r.Context(), integration.AnnounceChannel.ID, slack.Message{Text: text})
	if err != nil {
		var slackErr *slack.Error
		if errors.As(err, &slackErr) && slackErr.Code == "not_in_channel" {
			return nil, 0, httpx.Fail(409, "slack_bot_not_in_channel", "Invite the bot to the channel with /invite @Peer Bonus")
		}
		return nil, 0, slackFailure(err)
	}
	return slackTestResponse{OK: true, TS: ts}, 200, nil
}

func (a *API) slackDeliveries(s *request) (any, int, error) {
	limit, err := queryLimit(s, 50, 50)
	if err != nil {
		return nil, 0, err
	}
	state := s.r.URL.Query().Get("state")
	switch domain.SlackDeliveryState(state) {
	case "", domain.SlackDeliveryPending, domain.SlackDeliverySent, domain.SlackDeliverySkipped, domain.SlackDeliveryFailed:
	default:
		return nil, 0, httpx.Invalid("State must be pending, sent, skipped, or failed")
	}
	rows, err := a.slackStore.ListSlackDeliveries(s.r.Context(), state, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]slackDeliveryResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, slackDeliveryResponse{ID: row.ID, Kind: row.Kind, PostID: row.PostID, State: row.State, Attempts: row.Attempts, LastError: row.LastError, CreatedAt: row.CreatedAt, SentAt: row.SentAt})
	}
	return map[string]any{"items": items}, 200, nil
}

func slackNotConnected() error {
	return httpx.Fail(409, "slack_not_connected", "Verify the Slack workspace and configure an announce channel before sending")
}

func slackFailure(err error) error {
	code := "request_failed"
	var slackErr *slack.Error
	if errors.As(err, &slackErr) && slackErr.Code != "" {
		code = slackErr.Code
	}
	return &slackAPIError{Status: 502, Code: "slack_error", Message: "Slack could not complete the request", Detail: code}
}
