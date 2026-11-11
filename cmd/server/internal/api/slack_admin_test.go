package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/platformclient"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
	"github.com/stretchr/testify/require"
)

type fakeSlackAdminClient struct {
	info         slack.AuthInfo
	channels     []slack.Channel
	authErr      error
	channelsErr  error
	postErr      error
	channelCalls int
	postChannel  string
	postMessage  slack.Message
}

func (f *fakeSlackAdminClient) AuthTest(context.Context) (slack.AuthInfo, error) {
	return f.info, f.authErr
}
func (f *fakeSlackAdminClient) ListPublicChannels(context.Context) ([]slack.Channel, error) {
	f.channelCalls++
	return f.channels, f.channelsErr
}
func (f *fakeSlackAdminClient) PostMessage(_ context.Context, channel string, message slack.Message) (string, error) {
	f.postChannel, f.postMessage = channel, message
	return "123.456", f.postErr
}

type fakeSlackAdminStore struct {
	integration domain.SlackIntegration
	program     domain.Program
	programErr  error
	stats       domain.SlackDeliveryStats
	deliveries  []domain.SlackDelivery
	listState   string
	listLimit   int
	updates     int
}

func (f *fakeSlackAdminStore) SlackIntegration(context.Context) (domain.SlackIntegration, error) {
	return f.integration, nil
}
func (f *fakeSlackAdminStore) Program(context.Context) (domain.Program, error) {
	return f.program, f.programErr
}
func (f *fakeSlackAdminStore) UpdateSlackSettings(_ context.Context, patch store.SlackSettingsPatch, now time.Time) (domain.SlackIntegration, error) {
	f.updates++
	if patch.AnnounceChannel != nil {
		f.integration.AnnounceChannel = *patch.AnnounceChannel
	}
	if patch.AnnouncePosts != nil {
		f.integration.AnnouncePosts = *patch.AnnouncePosts
	}
	if patch.DMRecipientOnPost != nil {
		f.integration.DMRecipientOnPost = *patch.DMRecipientOnPost
	}
	if patch.GiveFromSlack != nil {
		f.integration.GiveFromSlack = *patch.GiveFromSlack
	}
	if patch.GiveInConversation != nil {
		f.integration.GiveInConversation = *patch.GiveInConversation
	}
	if patch.MentionPointChoices != nil {
		f.integration.MentionPointChoices = *patch.MentionPointChoices
	}
	if patch.ReactionRules != nil {
		f.integration.ReactionRules = *patch.ReactionRules
	}
	if patch.Locale != nil {
		f.integration.Locale = *patch.Locale
	}
	f.integration.UpdatedAt = now
	return f.integration, nil
}
func (f *fakeSlackAdminStore) FixSlackWorkspace(_ context.Context, ws domain.SlackWorkspace, force bool, now time.Time) (domain.SlackIntegration, error) {
	if f.integration.Workspace != nil && f.integration.Workspace.TeamID != ws.TeamID {
		if !force {
			return domain.SlackIntegration{}, domain.ErrSlackTeamChanged
		}
		f.integration.AnnounceChannel = nil
	}
	f.integration.Workspace = &ws
	f.integration.UpdatedAt = now
	return f.integration, nil
}
func (f *fakeSlackAdminStore) SetSlackLastError(_ context.Context, code *string, now time.Time) error {
	f.integration.LastErrorCode, f.integration.LastErrorAt = code, nil
	if code != nil {
		f.integration.LastErrorAt = &now
	}
	return nil
}
func (f *fakeSlackAdminStore) SlackDeliveryStats(context.Context, time.Time) (domain.SlackDeliveryStats, error) {
	return f.stats, nil
}
func (f *fakeSlackAdminStore) ListSlackDeliveries(_ context.Context, state string, limit int) ([]domain.SlackDelivery, error) {
	f.listState, f.listLimit = state, limit
	return f.deliveries, nil
}

func newSlackAdminTest() (*API, *fakeSlackAdminStore, *fakeSlackAdminClient) {
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	db := &fakeSlackAdminStore{
		integration: domain.SlackIntegration{Locale: domain.SlackLocaleJA, DMRecipientOnPost: true, MentionPointChoices: []int{10, 30, 50}},
		program:     domain.Program{MaxPointsPerPost: 40, Values: []domain.Value{{Key: "care"}}},
	}
	client := &fakeSlackAdminClient{
		info:     slack.AuthInfo{TeamID: "T123", TeamName: "Acme", BotUserID: "UBOT", EnterpriseID: "E123"},
		channels: []slack.Channel{{ID: "C1", Name: "general", IsMember: true}, {ID: "C2", Name: "gifts"}, {ID: "C3", Name: "random", IsMember: true}},
	}
	return &API{slack: client, slackStore: db, now: func() time.Time { return now }}, db, client
}

func slackAdminCall(t *testing.T, a *API, fn endpoint, method, path, body string, status int) []byte {
	t.Helper()
	r := httptest.NewRequest(method, "/api/peerbonus/v1"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer admin-viewer")
	w := httptest.NewRecorder()
	s := &request{r: r, w: w, bearer: r.Header.Get("Authorization"), now: a.now(), member: domain.Member{ID: "admin"}, access: platformclient.ProjectAccess{Roles: platformclient.Roles{IsConsoleAdmin: true}}}
	result, code, err := a.admin(fn)(s)
	if err != nil {
		writeError(w, err)
	} else {
		httpx.WriteJSON(w, code, result)
	}
	require.Equal(t, status, w.Code, w.Body.String())
	return w.Body.Bytes()
}

func slackAdminErrorCode(t *testing.T, body []byte, code string) {
	t.Helper()
	var result httpx.Error
	require.NoError(t, json.Unmarshal(body, &result))
	require.Equal(t, code, result.Code)
}

func TestSlackAdminStates(t *testing.T) {
	for _, tc := range []struct {
		name      string
		client    bool
		workspace bool
		lastError *string
		want      domain.SlackState
	}{
		{"no token", false, false, nil, domain.SlackStateUnconfigured},
		{"removed token preserves settings", false, true, new("team_changed"), domain.SlackStateUnconfigured},
		{"not verified", true, false, nil, domain.SlackStateConfigured},
		{"connected", true, true, nil, domain.SlackStateConnected},
		{"changed workspace", true, true, new("team_changed"), domain.SlackStateTeamChanged},
		{"other error remains connected", true, true, new("invalid_auth"), domain.SlackStateConnected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, db, _ := newSlackAdminTest()
			if !tc.client {
				a.slack = nil
			}
			a.slackSigningSecretConfigured = true
			if tc.workspace {
				db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123", TeamName: "Acme", BotUserID: "UBOT", VerifiedAt: a.now()}
			}
			db.integration.LastErrorCode = tc.lastError
			db.stats = domain.SlackDeliveryStats{Pending: 3, Failed24h: 2, Sent24h: 9}
			body := slackAdminCall(t, a, a.adminSlack, "GET", "/admin/slack", "", 200)
			var result slackIntegrationResponse
			require.NoError(t, json.Unmarshal(body, &result))
			require.Equal(t, tc.want, result.State)
			require.Equal(t, tc.client, result.Credential.BotToken)
			require.True(t, result.Credential.SigningSecret)
			require.Equal(t, slackDeliveryStatsResponse{Pending: 3, Failed24h: 2, Sent24h: 9}, result.Deliveries)
			require.Nil(t, result.Inbound)
			require.Contains(t, string(body), `"inbound":null`)
			require.Contains(t, string(body), `"announceChannel":null`)
		})
	}
}

func TestSlackAdminVerifyFixesWorkspaceAndForce(t *testing.T) {
	a, db, client := newSlackAdminTest()
	db.integration.LastErrorCode = new("invalid_auth")
	slackAdminCall(t, a, a.verifySlack, "POST", "/admin/slack/verify", `{}`, 200)
	require.Equal(t, "T123", db.integration.Workspace.TeamID)
	require.Equal(t, "E123", db.integration.Workspace.EnterpriseID)
	require.Equal(t, "admin", db.integration.Workspace.VerifiedBy)
	require.Equal(t, a.now(), db.integration.Workspace.VerifiedAt)
	require.Nil(t, db.integration.LastErrorCode)
	db.integration.AnnounceChannel = &domain.SlackChannel{ID: "C1", Name: "general"}
	slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels", "", 200)
	require.Equal(t, 1, client.channelCalls)
	client.info.TeamName = "Acme renamed"
	slackAdminCall(t, a, a.verifySlack, "POST", "/admin/slack/verify", "", 200)
	require.Equal(t, "Acme renamed", db.integration.Workspace.TeamName)
	require.NotNil(t, db.integration.AnnounceChannel)
	require.True(t, a.slackChannelsExpires.IsZero())
	client.info.TeamID, client.info.TeamName = "T456", "Other"
	body := slackAdminCall(t, a, a.verifySlack, "POST", "/admin/slack/verify", `{}`, 409)
	slackAdminErrorCode(t, body, "slack_team_changed")
	var failure struct {
		Detail slackTeamChangedDetail `json:"detail"`
	}
	require.NoError(t, json.Unmarshal(body, &failure))
	require.Equal(t, slackTeamChangedDetail{Current: slackTeamResponse{TeamID: "T123", TeamName: "Acme renamed"}, Token: slackTeamResponse{TeamID: "T456", TeamName: "Other"}}, failure.Detail)
	require.Equal(t, "T123", db.integration.Workspace.TeamID)
	require.Equal(t, new("team_changed"), db.integration.LastErrorCode)
	require.Equal(t, domain.SlackStateTeamChanged, a.slackState(db.integration))
	body = slackAdminCall(t, a, a.verifySlack, "POST", "/admin/slack/verify", `{"force":true}`, 200)
	require.Equal(t, "T456", db.integration.Workspace.TeamID)
	require.Nil(t, db.integration.AnnounceChannel)
	require.Nil(t, db.integration.LastErrorCode)
	require.Contains(t, string(body), `"state":"connected"`)
}

func TestSlackAdminVerifyFailures(t *testing.T) {
	a, db, client := newSlackAdminTest()
	a.slack = nil
	slackAdminErrorCode(t, slackAdminCall(t, a, a.verifySlack, "POST", "/admin/slack/verify", `{}`, 409), "slack_not_configured")
	a.slack = client
	client.authErr = &slack.Error{Method: "auth.test", Code: "invalid_auth"}
	body := slackAdminCall(t, a, a.verifySlack, "POST", "/admin/slack/verify", `{}`, 502)
	slackAdminErrorCode(t, body, "slack_error")
	require.Contains(t, string(body), `"detail":"invalid_auth"`)
	require.Nil(t, db.integration.Workspace)
	client.authErr = errors.New("transport failed with secret-token")
	body = slackAdminCall(t, a, a.verifySlack, "POST", "/admin/slack/verify", `{}`, 502)
	require.NotContains(t, string(body), "secret-token")
	require.Contains(t, string(body), `"detail":"request_failed"`)
	w := httptest.NewRecorder()
	writeError(w, domain.ErrSlackTeamChanged)
	require.Equal(t, 409, w.Code)
	slackAdminErrorCode(t, w.Body.Bytes(), "slack_team_changed")
}

func TestSlackAdminPatchValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, code   string
		connected, signing bool
		status             int
	}{
		{"not connected", `{"announcePosts":false}`, "slack_not_connected", false, false, 409},
		{"channel required", `{"announcePosts":true}`, "invalid_request", true, false, 400},
		{"signing required", `{"giveFromSlack":true}`, "slack_signing_secret_missing", true, false, 409},
		{"unknown channel", `{"announceChannelId":"C404"}`, "invalid_request", true, false, 400},
		{"empty channel", `{"announceChannelId":""}`, "invalid_request", true, false, 400},
		{"invalid channel type", `{"announceChannelId":42}`, "invalid_request", true, false, 400},
		{"invalid locale", `{"locale":"fr"}`, "invalid_request", true, false, 400},
		{"unknown field", `{"botToken":"secret"}`, "invalid_request", true, false, 400},
		{"empty request", ``, "invalid_request", true, false, 400},
		{"locale before verify", `{"locale":"en"}`, "", false, false, 200},
		{"valid", `{"announceChannelId":"C2","announcePosts":true,"giveFromSlack":true,"dmRecipientOnPost":false,"locale":"en"}`, "", true, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, db, _ := newSlackAdminTest()
			if tc.connected {
				db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123"}
			}
			a.slackSigningSecretConfigured = tc.signing
			body := slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", tc.body, tc.status)
			if tc.code != "" {
				slackAdminErrorCode(t, body, tc.code)
				require.Zero(t, db.updates)
			} else {
				require.Equal(t, 1, db.updates)
				require.Equal(t, domain.SlackLocaleEN, db.integration.Locale)
			}
			if tc.name == "valid" {
				require.Equal(t, &domain.SlackChannel{ID: "C2", Name: "gifts"}, db.integration.AnnounceChannel)
				require.True(t, db.integration.AnnouncePosts)
				require.True(t, db.integration.GiveFromSlack)
				require.False(t, db.integration.DMRecipientOnPost)
			}
		})
	}
	a, db, _ := newSlackAdminTest()
	db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123"}
	db.integration.AnnounceChannel = &domain.SlackChannel{ID: "C1"}
	db.integration.AnnouncePosts = true
	slackAdminErrorCode(t, slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", `{"announceChannelId":null}`, 400), "invalid_request")
	slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", `{"announceChannelId":null,"announcePosts":false}`, 200)
	require.Nil(t, db.integration.AnnounceChannel)
	require.False(t, db.integration.AnnouncePosts)
	db.integration.LastErrorCode = new("team_changed")
	slackAdminErrorCode(t, slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", `{"dmRecipientOnPost":false}`, 409), "slack_not_connected")
}

func TestSlackAdminChannelsCachePrefixAndLimits(t *testing.T) {
	a, db, client := newSlackAdminTest()
	db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123"}
	body := slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels?q=G&limit=1", "", 200)
	require.JSONEq(t, `{"items":[{"id":"C1","name":"general","isMember":true}]}`, string(body))
	now := a.now()
	a.now = func() time.Time { return now.Add(59 * time.Second) }
	slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels?q=gi", "", 200)
	slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", `{"announceChannelId":"C2"}`, 200)
	require.Equal(t, 1, client.channelCalls, "picker and patch share a 60-second cache")
	a.now = func() time.Time { return now.Add(time.Minute) }
	slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels", "", 200)
	require.Equal(t, 2, client.channelCalls)
	body = slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels?q=missing", "", 200)
	require.JSONEq(t, `{"items":[]}`, string(body))
	for _, limit := range []string{"0", "21", "oops", ""} {
		slackAdminErrorCode(t, slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels?limit="+limit, "", 400), "invalid_request")
	}
	a.now = func() time.Time { return now.Add(2 * time.Minute) }
	client.channelsErr = &slack.Error{Code: "ratelimited", Status: 429}
	slackAdminErrorCode(t, slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels", "", 502), "slack_error")
	client.channelsErr = nil
	slackAdminCall(t, a, a.slackChannelsList, "GET", "/admin/slack/channels", "", 200)
	require.Equal(t, 4, client.channelCalls, "failed channel fetches are not cached")
}

func TestSlackAdminTestMessage(t *testing.T) {
	a, db, client := newSlackAdminTest()
	slackAdminErrorCode(t, slackAdminCall(t, a, a.testSlack, "POST", "/admin/slack/test", "", 409), "slack_not_connected")
	db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123"}
	slackAdminErrorCode(t, slackAdminCall(t, a, a.testSlack, "POST", "/admin/slack/test", "", 409), "slack_not_connected")
	db.integration.AnnounceChannel = &domain.SlackChannel{ID: "C1"}
	client.postErr = &slack.Error{Method: "chat.postMessage", Code: "not_in_channel"}
	body := slackAdminCall(t, a, a.testSlack, "POST", "/admin/slack/test", "", 409)
	slackAdminErrorCode(t, body, "slack_bot_not_in_channel")
	require.Contains(t, string(body), "/invite @Peer Bonus")
	client.postErr = &slack.Error{Code: "invalid_auth"}
	slackAdminErrorCode(t, slackAdminCall(t, a, a.testSlack, "POST", "/admin/slack/test", "", 502), "slack_error")
	client.postErr = nil
	db.integration.Locale = domain.SlackLocaleEN
	body = slackAdminCall(t, a, a.testSlack, "POST", "/admin/slack/test", `{}`, 200)
	require.JSONEq(t, `{"ok":true,"ts":"123.456"}`, string(body))
	require.Equal(t, "C1", client.postChannel)
	require.Equal(t, "This is a test message from Peer Bonus.", client.postMessage.Text)
	slackAdminErrorCode(t, slackAdminCall(t, a, a.testSlack, "POST", "/admin/slack/test", `{"channel":"C2"}`, 400), "invalid_request")
}

func TestSlackAdminDeliveries(t *testing.T) {
	a, db, _ := newSlackAdminTest()
	a.slack = nil
	db.deliveries = []domain.SlackDelivery{{ID: 42, Kind: domain.SlackDeliveryPostAnnounce, PostID: "post", State: domain.SlackDeliveryFailed, Attempts: 12, LastError: new("not_in_channel"), CreatedAt: a.now()}}
	body := slackAdminCall(t, a, a.slackDeliveries, "GET", "/admin/slack/deliveries?state=failed&limit=1", "", 200)
	require.Equal(t, "failed", db.listState)
	require.Equal(t, 1, db.listLimit)
	require.JSONEq(t, `{"items":[{"id":42,"kind":"post_announce","postId":"post","state":"failed","attempts":12,"lastError":"not_in_channel","createdAt":"2026-10-20T01:00:00Z","sentAt":null}]}`, string(body))
	db.deliveries = nil
	body = slackAdminCall(t, a, a.slackDeliveries, "GET", "/admin/slack/deliveries", "", 200)
	require.JSONEq(t, `{"items":[]}`, string(body))
	require.Equal(t, 50, db.listLimit)
	require.Empty(t, db.listState)
	for _, query := range []string{"state=unknown", "limit=51", "limit=0", "limit="} {
		slackAdminErrorCode(t, slackAdminCall(t, a, a.slackDeliveries, "GET", "/admin/slack/deliveries?"+query, "", 400), "invalid_request")
	}
}

func TestSlackAdminAuthorization(t *testing.T) {
	a, _, _ := newSlackAdminTest()
	for _, fn := range []endpoint{a.adminSlack, a.patchSlack, a.verifySlack, a.slackChannelsList, a.testSlack, a.slackDeliveries} {
		_, _, err := a.admin(fn)(&request{})
		var failure *httpx.Error
		require.ErrorAs(t, err, &failure)
		require.Equal(t, http.StatusForbidden, failure.Status)
	}
}

func TestSlackAdminRoutesWithDatabase(t *testing.T) {
	h := newHarness(t)
	_, _, client := newSlackAdminTest()
	h.api.slack = client
	for _, route := range []struct{ method, path string }{
		{"GET", "/admin/slack"}, {"PATCH", "/admin/slack"}, {"POST", "/admin/slack/verify"},
		{"GET", "/admin/slack/channels"}, {"POST", "/admin/slack/test"}, {"GET", "/admin/slack/deliveries"},
	} {
		call[httpx.Error](h, "", route.method, route.path, nil, 401)
		call[httpx.Error](h, "reader", route.method, route.path, nil, 403)
	}
	initial := call[slackIntegrationResponse](h, "admin", "GET", "/admin/slack", nil, 200)
	require.Equal(t, domain.SlackStateConfigured, initial.State)
	verified := call[slackIntegrationResponse](h, "admin", "POST", "/admin/slack/verify", map[string]any{}, 200)
	require.Equal(t, "T123", verified.Workspace.TeamID)
	call[slackIntegrationResponse](h, "admin", "PATCH", "/admin/slack", map[string]any{"announceChannelId": "C1", "announcePosts": true}, 200)
	call[map[string]any](h, "admin", "GET", "/admin/slack/channels", nil, 200)
	call[slackTestResponse](h, "admin", "POST", "/admin/slack/test", nil, 200)
	post := call[postResponse](h, "writer", "POST", "/posts", postInput(10), 201, "slack-admin-outbox")
	listed := call[struct {
		Items []slackDeliveryResponse `json:"items"`
	}](h, "admin", "GET", "/admin/slack/deliveries?state=pending", nil, 200)
	require.NotEmpty(t, listed.Items)
	for _, item := range listed.Items {
		require.Equal(t, post.ID, item.PostID)
	}
}

func TestSlackConversationSettings(t *testing.T) {
	t.Run("normalizes and replaces settings", func(t *testing.T) {
		a, db, _ := newSlackAdminTest()
		db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123"}
		a.slackSigningSecretConfigured = true
		body := slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", `{"giveInConversation":true,"mentionPointChoices":[10,40],"reactionRules":[{"emoji":":PB-10:","points":10,"valueKey":null},{"emoji":":CARE:","points":40,"valueKey":"care"}]}`, 200)
		require.True(t, db.integration.GiveInConversation)
		require.Equal(t, []int{10, 40}, db.integration.MentionPointChoices)
		require.Equal(t, []domain.SlackReactionRule{{Emoji: "pb-10", Points: 10}, {Emoji: "care", Points: 40, ValueKey: new("care")}}, db.integration.ReactionRules)
		for _, response := range [][]byte{body, slackAdminCall(t, a, a.adminSlack, "GET", "/admin/slack", "", 200)} {
			var result struct {
				Settings json.RawMessage `json:"settings"`
			}
			require.NoError(t, json.Unmarshal(response, &result))
			require.JSONEq(t, `{"announceChannel":null,"announcePosts":false,"dmRecipientOnPost":true,"giveFromSlack":false,"giveInConversation":true,"mentionPointChoices":[10,40],"reactionRules":[{"emoji":"pb-10","points":10,"valueKey":null},{"emoji":"care","points":40,"valueKey":"care"}],"locale":"ja"}`, string(result.Settings))
		}
		a.slackSigningSecretConfigured = false
		body = slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", `{"giveInConversation":false,"reactionRules":[]}`, 200)
		require.False(t, db.integration.GiveInConversation)
		require.Empty(t, db.integration.ReactionRules)
		require.Contains(t, string(body), `"reactionRules":[]`)
		require.Equal(t, []int{10, 40}, db.integration.MentionPointChoices)
	})

	for _, tc := range []struct {
		name, body, code string
		connected        bool
		status           int
	}{
		{"nine rules", `{"reactionRules":[{"emoji":"a","points":1},{"emoji":"b","points":1},{"emoji":"c","points":1},{"emoji":"d","points":1},{"emoji":"e","points":1},{"emoji":"f","points":1},{"emoji":"g","points":1},{"emoji":"h","points":1},{"emoji":"i","points":1}]}`, "invalid_request", true, 400},
		{"duplicate normalized emoji", `{"reactionRules":[{"emoji":":PB-10:","points":10},{"emoji":"pb-10","points":10}]}`, "invalid_request", true, 400},
		{"rule above current maximum", `{"reactionRules":[{"emoji":"pb-50","points":41}]}`, "invalid_request", true, 400},
		{"zero rule points", `{"reactionRules":[{"emoji":"pb-0","points":0}]}`, "invalid_request", true, 400},
		{"empty emoji", `{"reactionRules":[{"emoji":"::","points":10}]}`, "invalid_request", true, 400},
		{"unknown value", `{"reactionRules":[{"emoji":"pb-10","points":10,"valueKey":"missing"}]}`, "unknown_value", true, 400},
		{"empty choices", `{"mentionPointChoices":[]}`, "invalid_request", true, 400},
		{"six choices", `{"mentionPointChoices":[1,2,3,4,5,6]}`, "invalid_request", true, 400},
		{"duplicate choices", `{"mentionPointChoices":[10,10]}`, "invalid_request", true, 400},
		{"choices above current maximum", `{"mentionPointChoices":[41]}`, "invalid_request", true, 400},
		{"zero choice", `{"mentionPointChoices":[0]}`, "invalid_request", true, 400},
		{"missing signing secret", `{"giveInConversation":true}`, "slack_signing_secret_missing", true, 409},
		{"not connected", `{"giveInConversation":true}`, "slack_not_connected", false, 409},
		{"cannot disable before connecting", `{"giveInConversation":false}`, "slack_not_connected", false, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, db, _ := newSlackAdminTest()
			if tc.connected {
				db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123"}
			}
			body := slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", tc.body, tc.status)
			slackAdminErrorCode(t, body, tc.code)
			require.Zero(t, db.updates)
		})
	}

	t.Run("defaults are non-null", func(t *testing.T) {
		a, _, _ := newSlackAdminTest()
		body := slackAdminCall(t, a, a.adminSlack, "GET", "/admin/slack", "", 200)
		require.Contains(t, string(body), `"giveInConversation":false`)
		require.Contains(t, string(body), `"mentionPointChoices":[10,30,50]`)
		require.Contains(t, string(body), `"reactionRules":[]`)
	})

	t.Run("cannot enable after workspace change", func(t *testing.T) {
		a, db, _ := newSlackAdminTest()
		db.integration.Workspace = &domain.SlackWorkspace{TeamID: "T123"}
		db.integration.LastErrorCode = new("team_changed")
		a.slackSigningSecretConfigured = true
		body := slackAdminCall(t, a, a.patchSlack, "PATCH", "/admin/slack", `{"giveInConversation":true}`, 409)
		slackAdminErrorCode(t, body, "slack_not_connected")
		require.Zero(t, db.updates)
	})

	t.Run("program failure prevents update", func(t *testing.T) {
		a, db, _ := newSlackAdminTest()
		db.programErr = errors.New("program unavailable")
		r := httptest.NewRequest("PATCH", "/admin/slack", strings.NewReader(`{"mentionPointChoices":[10]}`))
		_, _, err := a.patchSlack(&request{r: r})
		require.ErrorIs(t, err, db.programErr)
		require.Zero(t, db.updates)
	})
}

func TestSlackConversationInbound(t *testing.T) {
	for _, tc := range []struct {
		name, response, want string
		status               int
	}{
		{"missing", `{"error":"not_found"}`, `null`, 404},
		{"ready", `{"id":"peerbonus-slack","url":"https://inbound.example/slack","ready":true,"conditions":[{"type":"Bound","status":"False","reason":"Other"},{"type":"Ready","status":"True","reason":"Ready"}]}`, `{"url":"https://inbound.example/slack","ready":true,"reason":"Ready"}`, 200},
		{"not ready", `{"url":"","ready":false,"conditions":[{"type":"Ready","status":"False","reason":"TargetNotReady"}]}`, `{"url":null,"ready":false,"reason":"TargetNotReady"}`, 200},
		{"stale generation", `{"url":"https://inbound.example/slack","ready":false,"conditions":[{"type":"Ready","status":"True","reason":"Ready"}]}`, `{"url":"https://inbound.example/slack","ready":false,"reason":"Ready"}`, 200},
		{"no ready condition", `{"url":"","ready":false,"conditions":[{"type":"Bound","reason":"Other"}]}`, `{"url":null,"ready":false,"reason":null}`, 200},
		{"platform failure", `{"message":"secret-token"}`, `{"url":null,"ready":false,"reason":"platform_unavailable"}`, 503},
		{"denied", `{"error":"forbidden"}`, `{"url":null,"ready":false,"reason":"platform_unavailable"}`, 403},
		{"invalid json", `not-json`, `{"url":null,"ready":false,"reason":"platform_unavailable"}`, 200},
		{"redirect", ``, `{"url":null,"ready":false,"reason":"platform_unavailable"}`, 302},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			platform := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/projects/pj-test/inbound-endpoints/peerbonus-slack", r.URL.Path)
				require.Equal(t, "Bearer admin-viewer", r.Header.Get("Authorization"))
				require.Equal(t, "application/json", r.Header.Get("Accept"))
				if tc.status == 302 {
					w.Header().Set("Location", "/must-not-follow")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer platform.Close()
			a, _, _ := newSlackAdminTest()
			a.slackInbound = newSlackInboundClient(platform.URL+"/", "pj-test", platform.Client().Transport)
			for _, call := range []struct {
				method, body string
				fn           endpoint
			}{
				{"GET", "", a.adminSlack},
				{"PATCH", `{"locale":"en"}`, a.patchSlack},
			} {
				body := slackAdminCall(t, a, call.fn, call.method, "/admin/slack", call.body, 200)
				var result struct {
					Inbound json.RawMessage `json:"inbound"`
				}
				require.NoError(t, json.Unmarshal(body, &result))
				require.JSONEq(t, tc.want, string(result.Inbound))
				require.NotContains(t, string(body), "secret-token")
			}
			require.Equal(t, 2, calls, "each response uses its viewer; redirects are not followed")
		})
	}

	t.Run("transport failure", func(t *testing.T) {
		platform := httptest.NewServer(http.NotFoundHandler())
		a, _, _ := newSlackAdminTest()
		a.slackInbound = newSlackInboundClient(platform.URL, "pj-test", platform.Client().Transport)
		platform.Close()
		body := slackAdminCall(t, a, a.adminSlack, "GET", "/admin/slack", "", 200)
		require.Contains(t, string(body), `"inbound":{"url":null,"ready":false,"reason":"platform_unavailable"}`)
	})
}
