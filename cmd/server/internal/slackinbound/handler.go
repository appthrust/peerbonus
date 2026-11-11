// Package slackinbound handles signed Slack commands, interactions, options, and
// conversation events. Ledger writes persist event idempotency; the signature
// replay cache suppresses duplicate requests and releases failed event attempts.
package slackinbound

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

const requestBudget = 2500 * time.Millisecond

// Store is the member, program, and ledger surface used by Slack input.
// Missing members must return domain.ErrNotFound, as store.Store.Do does.
type Store interface {
	SlackIntegration(context.Context) (domain.SlackIntegration, error)
	MemberBySlackUser(context.Context, string) (domain.Member, error)
	MemberByEmail(context.Context, string) (domain.Member, error)
	MemberByID(context.Context, string) (domain.Member, error)
	SearchMembers(context.Context, string, string, int) ([]domain.Member, error)
	CreatePost(context.Context, store.CreatePostInput) (domain.Post, bool, error)
	CreateClap(context.Context, store.CreateClapInput) (domain.Clap, domain.Post, bool, error)
	CreateSlackPosts(context.Context, store.CreateSlackPostsInput) ([]domain.Post, bool, error)
	CreateSlackClap(context.Context, store.CreateSlackClapInput) (domain.Clap, domain.Post, bool, error)
	PostIDBySlackAnnounce(context.Context, string, string) (string, error)
	CreateSlackPendingGive(context.Context, domain.SlackPendingGive, time.Time) (domain.SlackPendingGive, bool, error)
	SlackPendingGive(context.Context, string, time.Time) (domain.SlackPendingGive, error)
	DeleteSlackPendingGive(context.Context, string, string) error
	LinkMemberSlackUser(context.Context, string, string, time.Time) error
	Program(context.Context) (domain.Program, error)
	CurrentPeriod(context.Context, time.Time) (domain.Period, error)
	GiveRemaining(context.Context, string, time.Time) (int, error)
}

// Slack contains only the Web API operations needed to acknowledge input.
type Slack interface {
	OpenView(context.Context, string, json.RawMessage) error
	PostEphemeral(context.Context, string, string, slack.Message) error
	PostMessage(context.Context, string, slack.Message) (string, error)
	UserInfo(context.Context, string) (slack.User, error)
	Permalink(context.Context, string, string) (string, error)
}

type Options struct {
	Store         Store
	Slack         Slack
	SigningSecret string
	PublicURL     string
	Now           func() time.Time
	// HTTPClient sends command response_url messages. Its transport can be
	// injected for tests. Redirects are disabled even on an injected client.
	HTTPClient *http.Client
}

type handler struct {
	store         Store
	slack         Slack
	signingSecret string
	publicURL     string
	now           func() time.Time
	httpClient    http.Client
	replays       replaySet
}

// New serves POST /slack/{commands,interactions,options,events}, without OIDC.
// Gates run before reading the body; requests are limited to 64 KiB and signed
// over the exact raw form or JSON body. Each request shares a 2.5 second budget.
func New(o Options) http.Handler {
	if o.Now == nil {
		o.Now = time.Now
	}
	client := http.Client{Timeout: requestBudget}
	if o.HTTPClient != nil {
		client = *o.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &handler{
		store: o.Store, slack: o.Slack, signingSecret: o.SigningSecret,
		publicURL: o.PublicURL, now: o.Now, httpClient: client,
		replays: replaySet{entries: make(map[string]time.Time)},
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.signingSecret == "" || h.store == nil || h.slack == nil {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestBudget)
	defer cancel()
	r = r.WithContext(ctx)
	integration, err := h.store.SlackIntegration(ctx)
	if err != nil {
		http.Error(w, "Slack integration unavailable", http.StatusServiceUnavailable)
		return
	}
	if integration.Workspace == nil {
		http.NotFound(w, r)
		return
	}
	switch r.URL.Path {
	case "/slack/commands", "/slack/options":
		if !integration.GiveFromSlack {
			http.NotFound(w, r)
			return
		}
	case "/slack/interactions":
		if !integration.GiveFromSlack && !integration.GiveInConversation {
			http.NotFound(w, r)
			return
		}
	case "/slack/events":
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, ok := h.verify(w, r, h.now())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/slack/events" {
		response := &eventResponse{ResponseWriter: w}
		defer func() {
			if response.status >= 500 {
				h.replays.forget(r.Header.Get("X-Slack-Request-Timestamp") + ":" + r.Header.Get("X-Slack-Signature"))
			}
		}()
		h.events(response, r, integration, body)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		http.Error(w, "Expected form body", http.StatusUnsupportedMediaType)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "Invalid form body", http.StatusBadRequest)
		return
	}
	if r.URL.Path == "/slack/commands" {
		if !workspaceMatches(integration.Workspace, form.Get("team_id"), form.Get("enterprise_id")) {
			http.Error(w, "Workspace mismatch", http.StatusForbidden)
			return
		}
		h.command(w, r, integration.Locale, form)
		return
	}
	var payload interaction
	if err := json.Unmarshal([]byte(form.Get("payload")), &payload); err != nil {
		http.Error(w, "Invalid interaction", http.StatusBadRequest)
		return
	}
	if !workspaceMatches(integration.Workspace, payload.Team.ID, payload.Enterprise.ID) {
		http.Error(w, "Workspace mismatch", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/slack/options" {
		h.options(w, r, payload)
	} else {
		h.interact(w, r, integration, payload)
	}
}

func workspaceMatches(workspace *domain.SlackWorkspace, team, enterprise string) bool {
	return (team != "" && team == workspace.TeamID) ||
		(enterprise != "" && workspace.EnterpriseID != "" && enterprise == workspace.EnterpriseID)
}

func (h *handler) member(ctx context.Context, userID string) (domain.Member, error) {
	if userID == "" {
		return domain.Member{}, domain.ErrNotFound
	}
	// Only Slack's authenticated users.info email may establish a cache miss.
	member, err := h.store.MemberBySlackUser(ctx, userID)
	if !memberMissing(err) {
		return member, err
	}
	user, err := h.slack.UserInfo(ctx, userID)
	if errors.Is(err, slack.ErrUserNotFound) || (err == nil && (user.IsBot || user.Deleted || user.Email == "")) {
		return domain.Member{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Member{}, err
	}
	member, err = h.store.MemberByEmail(ctx, user.Email)
	if err != nil {
		return domain.Member{}, err
	}
	if err := h.store.LinkMemberSlackUser(ctx, member.ID, userID, h.now()); err != nil {
		return domain.Member{}, err
	}
	return member, nil
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(body)
}

func localized(locale domain.SlackLocale, ja, en string) string {
	if locale == domain.SlackLocaleEN {
		return en
	}
	return ja
}

func (h *handler) guidance(locale domain.SlackLocale) string {
	text := localized(locale,
		"アカウントを連携するため、まずブラウザで Peer Bonus を一度開いてください。",
		"Open Peer Bonus in the browser once so we can link your account")
	if h.publicURL != "" {
		text += "\n" + h.publicURL
	}
	return text
}

func memberMissing(err error) bool { return errors.Is(err, domain.ErrNotFound) }
