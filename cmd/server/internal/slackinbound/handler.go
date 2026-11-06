// Package slackinbound handles signed Slack commands, interactions, and options.
// It reuses the application's ledger writes; Slack request replay protection is
// process-local, while view/action idempotency is persisted by the store.
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
	Program(context.Context) (domain.Program, error)
	CurrentPeriod(context.Context, time.Time) (domain.Period, error)
	GiveRemaining(context.Context, string, time.Time) (int, error)
}

// Slack contains only the Web API operations needed to acknowledge input.
type Slack interface {
	OpenView(context.Context, string, json.RawMessage) error
	PostEphemeral(context.Context, string, string, slack.Message) error
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

// New serves POST /slack/{commands,interactions,options}, without OIDC. The
// feature gate runs before reading any body; enabled requests are limited to
// 64 KiB and authenticated against the exact raw form body. Every operation
// shares a 2.5 second context, including synchronous views.open.
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
	if !integration.GiveFromSlack || integration.Workspace == nil {
		http.NotFound(w, r)
		return
	}
	switch r.URL.Path {
	case "/slack/commands", "/slack/interactions", "/slack/options":
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
		h.interact(w, r, integration.Locale, payload)
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
	// Neither command user_name nor user-supplied email establishes identity.
	// The outbound worker owns the Slack-user cache; no users.info scope exists.
	return h.store.MemberBySlackUser(ctx, userID)
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
