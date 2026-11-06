package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/appserver-go/appserver/platformclient"
	"github.com/appthrust/peerbonus/cmd/server/internal/directory"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

type API struct {
	store                        *store.Store
	identity                     *identity.Authenticator
	platform                     *platformclient.Client
	directory                    *directory.Client
	directoryPublicURL           string
	mux                          *http.ServeMux
	now                          func() time.Time
	memberSyncMu                 sync.Mutex
	lastMemberSync               time.Time
	recipientsMu                 sync.Mutex
	recipientsCache              map[string]cachedRecipients
	directoryPeopleCache         map[string]cachedDirectoryPeople
	directoryProfileCache        map[directoryProfileKey]cachedDirectoryProfile
	slack                        slackAdminClient
	slackStore                   slackAdminStore
	slackSigningSecretConfigured bool
	slackChannelsMu              sync.Mutex
	slackChannels                []slack.Channel
	slackChannelsExpires         time.Time
}
type request struct {
	r       *http.Request
	w       http.ResponseWriter
	member  domain.Member
	access  platformclient.TeamAccess
	bearer  string
	subject string
	now     time.Time
}
type endpoint func(*request) (any, int, error)

func New(database *store.Store, auth *identity.Authenticator, upstream *platformclient.Client, links *link.Client, directoryPublicURL string, slackOptions SlackOptions) http.Handler {
	a := &API{store: database, identity: auth, platform: upstream, directory: directory.New(links), directoryPublicURL: directoryPublicURL, mux: http.NewServeMux(), now: time.Now, slackStore: database, slackSigningSecretConfigured: slackOptions.SigningSecretConfigured}
	if slackOptions.Client != nil {
		a.slack = slackOptions.Client
	}
	a.mux.HandleFunc("GET /api/peerbonus/v1/openapi.json", a.openapi)
	a.route("GET /api/peerbonus/v1/me", false, true, a.me)
	a.route("GET /api/peerbonus/v1/program", false, false, a.program)
	a.route("GET /api/peerbonus/v1/members", false, false, a.members)
	a.route("GET /api/peerbonus/v1/recipients", false, false, a.recipients)
	a.route("GET /api/peerbonus/v1/members/{id}", false, false, a.member)
	a.route("GET /api/peerbonus/v1/avatars/{personId}", false, false, a.avatar)
	a.route("GET /api/peerbonus/v1/posts", false, false, a.posts)
	a.route("POST /api/peerbonus/v1/posts", true, false, a.createPost)
	a.route("GET /api/peerbonus/v1/posts/{id}", false, false, a.post)
	a.route("DELETE /api/peerbonus/v1/posts/{id}", true, true, a.admin(a.deletePost))
	a.route("GET /api/peerbonus/v1/posts/{id}/claps", false, false, a.claps)
	a.route("POST /api/peerbonus/v1/posts/{id}/claps", true, false, a.createClap)
	a.route("GET /api/peerbonus/v1/inbox", false, false, a.inbox)
	a.route("POST /api/peerbonus/v1/inbox/seen", true, false, a.markInboxSeen)
	a.route("GET /api/peerbonus/v1/admin/program", false, true, a.admin(a.adminProgram))
	a.route("PATCH /api/peerbonus/v1/admin/program", true, true, a.admin(a.patchProgram))
	a.route("GET /api/peerbonus/v1/admin/statements", false, true, a.admin(a.statements))
	a.route("GET /api/peerbonus/v1/admin/statements/{month}", false, true, a.admin(a.statement))
	a.route("POST /api/peerbonus/v1/admin/statements/{month}/close", true, true, a.admin(a.closeStatement))
	a.route("GET /api/peerbonus/v1/admin/statements/{month}/export.csv", false, true, a.admin(a.exportCSV))
	a.route("GET /api/peerbonus/v1/admin/members", false, true, a.admin(a.adminMembers))
	a.route("GET /api/peerbonus/v1/admin/insights", false, true, a.admin(a.adminInsights))
	a.route("GET /api/peerbonus/v1/admin/network", false, true, a.admin(a.adminNetwork))
	a.route("GET /api/peerbonus/v1/admin/slack", false, true, a.admin(a.adminSlack))
	a.route("PATCH /api/peerbonus/v1/admin/slack", true, true, a.admin(a.patchSlack))
	a.route("POST /api/peerbonus/v1/admin/slack/verify", true, true, a.admin(a.verifySlack))
	a.route("GET /api/peerbonus/v1/admin/slack/channels", false, true, a.admin(a.slackChannelsList))
	a.route("POST /api/peerbonus/v1/admin/slack/test", true, true, a.admin(a.testSlack))
	a.route("GET /api/peerbonus/v1/admin/slack/deliveries", false, true, a.admin(a.slackDeliveries))
	a.route("/", false, false, func(*request) (any, int, error) { return nil, 0, httpx.NotFound() })
	return a
}

func Unavailable() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, httpx.Fail(503, "peerbonus_store_unavailable", "The peerbonus database is not available"), "peerbonus")
	})
}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a.mux.ServeHTTP(w, r)
}
func (a *API) route(pattern string, mutation, needsRoles bool, fn endpoint) {
	a.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		claims, bearer, ok := a.authenticate(w, r)
		if !ok {
			return
		}
		s := &request{r: r, w: w, bearer: bearer, subject: claims.Principal.Subject, now: a.now().UTC()}
		var err error
		if needsRoles {
			s.access, err = a.platform.Access(ctx, s.subject, bearer)
			if err != nil {
				writeError(w, httpx.Fail(502, "upstream_unavailable", "The platform is temporarily unavailable"))
				return
			}
		}
		if mutation {
			// Body-less mutations (DELETE, clap, close, inbox seen) need no media type.
			if r.ContentLength != 0 {
				mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || mediaType != "application/json" {
					writeError(w, httpx.Invalid("Content-Type must be application/json"))
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		}
		s.member, err = a.store.MemberBySubject(ctx, s.subject)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			writeError(w, err)
			return
		}
		body, status, err := fn(s)
		if err != nil {
			writeError(w, err)
			return
		}
		// CSV endpoints have already written the successful response.
		if status == 0 {
			return
		}
		if status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		if raw, ok := body.(rawBody); ok {
			w.Header().Set("Content-Type", raw.contentType)
			w.Header().Set("Cache-Control", raw.cacheControl)
			w.WriteHeader(status)
			_, _ = w.Write(raw.data)
			return
		}
		httpx.WriteJSON(w, status, body)
	})
}
func (a *API) admin(fn endpoint) endpoint {
	return func(s *request) (any, int, error) {
		if !s.access.IsConsoleAdmin {
			return nil, 0, httpx.Forbidden()
		}
		return fn(s)
	}
}
func (a *API) authenticate(w http.ResponseWriter, r *http.Request) (identity.Identity, string, bool) {
	claims, _, err := a.identity.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrUnavailable):
			writeError(w, httpx.Fail(503, "identity_unavailable", "The identity provider is temporarily unavailable"))
		case errors.Is(err, identity.ErrRawKeyNotAccepted):
			writeError(w, httpx.Fail(401, "raw_key_not_accepted", "Exchange the access key for an assertion before calling this API"))
		default:
			writeError(w, httpx.Fail(401, "unauthenticated", "A valid bearer token is required"))
		}
		return identity.Identity{}, "", false
	}
	return claims, r.Header.Get("Authorization"), true
}

func decode(s *request, target any) error { return decodeBody(s, target, false) }
func emptyBody(s *request) error          { return decodeBody(s, &struct{}{}, true) }
func decodeBody(s *request, target any, allowEmpty bool) error {
	body, err := io.ReadAll(s.r.Body)
	if err != nil {
		return httpx.Invalid("Request body exceeds the size limit")
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 && allowEmpty {
		return nil
	}
	if len(body) == 0 || body[0] != '{' {
		return httpx.Invalid("Request body must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return httpx.Invalid("Request body contains invalid or unknown fields")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return httpx.Invalid("Request body must contain one JSON object")
	}
	return nil
}

func writeError(w http.ResponseWriter, err error) {
	var insufficient *domain.InsufficientPointsError
	if errors.As(err, &insufficient) {
		httpx.WriteJSON(w, 409, struct {
			Error     string `json:"error"`
			Message   string `json:"message"`
			Remaining int    `json:"remaining"`
			Required  int    `json:"required"`
		}{"insufficient_points", "Not enough points remain this week", insufficient.Remaining, insufficient.Required})
		return
	}
	var slackErr *slackAPIError
	if errors.As(err, &slackErr) {
		httpx.WriteJSON(w, slackErr.Status, slackErr)
		return
	}
	var validation *domain.ValidationError
	var rejected *link.RejectedError
	switch {
	case errors.As(err, &validation):
		err = httpx.Invalid(validation.Error())
	case errors.As(err, &rejected):
		err = httpx.Fail(rejected.Status, rejected.Code, rejected.Message)
	case errors.Is(err, domain.ErrNotFound):
		err = httpx.NotFound()
	case errors.Is(err, domain.ErrRecipientNotFound):
		err = httpx.Fail(404, "recipient_not_found", "The recipient was not found in Directory")
	case errors.Is(err, domain.ErrSelfTarget):
		err = httpx.Fail(400, "self_target", "You cannot send points to yourself")
	case errors.Is(err, domain.ErrOwnPost):
		err = httpx.Fail(403, "own_post", "You cannot clap for a post you sent or received")
	case errors.Is(err, domain.ErrUnknownValue):
		err = httpx.Fail(400, "unknown_value", "A selected value is not in the program")
	case errors.Is(err, domain.ErrInvalidMonth):
		err = httpx.Fail(400, "invalid_month", "Month must have the form YYYY-MM")
	case errors.Is(err, domain.ErrPeriodClosed):
		err = httpx.Fail(409, "period_closed", "The affected month is already closed")
	case errors.Is(err, domain.ErrMonthNotEnded):
		err = httpx.Fail(409, "month_not_ended", "The month has not ended yet")
	case errors.Is(err, domain.ErrPreviousMonthOpen):
		err = httpx.Fail(409, "previous_month_open", "Close earlier months first")
	case errors.Is(err, domain.ErrStatementOpen):
		err = httpx.Fail(409, "statement_open", "Close the statement before exporting CSV")
	case errors.Is(err, domain.ErrTimezoneLocked):
		err = httpx.Fail(409, "timezone_locked", "Time zone cannot change after points have been recorded")
	case errors.Is(err, domain.ErrSlackTeamChanged):
		err = httpx.Fail(409, "slack_team_changed", "The bot token belongs to a different Slack workspace")
	}
	httpx.WriteError(w, err, "peerbonus")
}

func directoryUnavailable() error {
	return httpx.Fail(503, "directory_unavailable", "Directory is temporarily unavailable")
}
