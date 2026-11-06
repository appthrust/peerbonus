// Package notify delivers the transactional Slack outbox. A Worker runs in one
// goroutine; Kick is safe for concurrent callers. Delivery is at-least-once:
// a crash after Slack accepts a message but before marking it can duplicate it.
package notify

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
)

// Store is the worker's persistence boundary. SlackDeliveryByID resolves the
// announcement referenced by clap_update and post_delete; DMs are independent.
type Store interface {
	ClaimSlackDeliveries(context.Context, time.Time, int, time.Duration) ([]domain.SlackDelivery, error)
	MarkSlackDeliverySent(context.Context, int64, string, string, time.Time) error
	RetrySlackDelivery(context.Context, int64, time.Time, string) error
	SkipSlackDelivery(context.Context, int64, string, time.Time) error
	SlackIntegration(context.Context) (domain.SlackIntegration, error)
	SetSlackLastError(context.Context, *string, time.Time) error
	SlackDeliveryByID(context.Context, int64) (domain.SlackDelivery, error)
	PostByID(context.Context, string, string) (domain.Post, error)
	ListClaps(context.Context, string, int) ([]domain.Clap, error)
	MemberByID(context.Context, string) (domain.Member, error)
	SetMemberSlackUser(context.Context, string, *string, time.Time) error
	Program(context.Context) (domain.Program, error)
}

type Slack interface {
	AuthTest(context.Context) (slack.AuthInfo, error)
	PostMessage(context.Context, string, slack.Message) (string, error)
	UpdateMessage(context.Context, string, string, slack.Message) error
	DeleteMessage(context.Context, string, string) error
	LookupUserByEmail(context.Context, string) (string, error)
}

type Options struct {
	Store     Store
	Slack     Slack
	PublicURL string
	Now       func() time.Time
	Logger    *slog.Logger
	Interval  time.Duration
}

type Worker struct {
	store       Store
	slack       Slack
	publicURL   string
	now         func() time.Time
	logger      *slog.Logger
	interval    time.Duration
	kick        chan struct{}
	auth        slack.AuthInfo
	authExpires time.Time
	nextCall    time.Time
}

func New(o Options) *Worker {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Interval <= 0 {
		o.Interval = 2 * time.Second
	}
	return &Worker{store: o.Store, slack: o.Slack, publicURL: o.PublicURL, now: o.Now, logger: o.Logger, interval: o.Interval, kick: make(chan struct{}, 1)}
}

// Kick coalesces wakeups without blocking a request handler, including when the
// optional worker has not been configured.
func (w *Worker) Kick() {
	if w == nil {
		return
	}
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// Run checks immediately, then on each poll or Kick. Transient errors leave work
// recoverable through retries or lease expiry. Cancellation returns ctx.Err().
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.runOnce(ctx); err != nil && ctx.Err() == nil {
			// Do not log transport or database error text, which can contain secrets.
			w.logger.WarnContext(ctx, "Slack outbox iteration failed", "code", errorCode(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-w.kick:
		}
	}
}

var (
	errPaused          = errors.New("team_changed")
	errDisabled        = errors.New("disabled")
	errPostDeleted     = errors.New("post_deleted")
	errUserNotFound    = errors.New("user_not_found")
	errAnnounceNotSent = errors.New("announce_not_sent")
	errInvalidDelivery = errors.New("invalid_delivery")
)

func (w *Worker) runOnce(ctx context.Context) error {
	integration, err := w.store.SlackIntegration(ctx)
	if err != nil || integration.Workspace == nil {
		return err
	}
	if !w.now().Before(w.authExpires) {
		if err := w.pace(ctx); err != nil {
			return err
		}
		w.nextCall = w.now().Add(time.Second)
		info, err := w.slack.AuthTest(ctx)
		if err != nil {
			code := errorCode(err)
			if saveErr := w.store.SetSlackLastError(ctx, &code, w.now()); saveErr != nil {
				return saveErr
			}
			return err
		}
		w.auth, w.authExpires = info, w.now().Add(10*time.Minute)
	}
	if err := w.checkTeam(ctx, integration); err != nil {
		if errors.Is(err, errPaused) {
			return nil
		}
		return err
	}
	if integration.LastErrorCode != nil {
		if err := w.store.SetSlackLastError(ctx, nil, w.now()); err != nil {
			return err
		}
	}
	deliveries, err := w.store.ClaimSlackDeliveries(ctx, w.now(), 10, 60*time.Second)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := 60 * time.Second
		if delivery.ClaimedUntil != nil {
			remaining = delivery.ClaimedUntil.Sub(w.now())
		}
		if remaining <= 0 {
			// Never start a Slack call with a lease another worker can reclaim.
			continue
		}
		deliveryCtx, cancel := context.WithTimeout(ctx, remaining)
		err := w.deliver(deliveryCtx, delivery)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errPaused) {
			return nil
		}
		if err == nil {
			continue
		}
		switch {
		case errors.Is(err, errDisabled), errors.Is(err, errPostDeleted), errors.Is(err, errUserNotFound), errors.Is(err, errAnnounceNotSent):
			err = w.store.SkipSlackDelivery(ctx, delivery.ID, err.Error(), w.now())
		default:
			err = w.store.RetrySlackDelivery(ctx, delivery.ID, w.now().Add(retryDelay(delivery.Attempts, err)), errorCode(err))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) checkTeam(ctx context.Context, integration domain.SlackIntegration) error {
	if integration.Workspace == nil {
		return errPaused
	}
	if integration.Workspace.TeamID != w.auth.TeamID {
		code := "team_changed"
		if err := w.store.SetSlackLastError(ctx, &code, w.now()); err != nil {
			return err
		}
		return errPaused
	}
	return nil
}

// ready checks current settings after pacing, so a queued send observes a
// disable or workspace re-verification that happened while it was waiting.
func (w *Worker) ready(ctx context.Context, delivery domain.SlackDelivery) (domain.SlackIntegration, error) {
	if err := w.pace(ctx); err != nil {
		return domain.SlackIntegration{}, err
	}
	integration, err := w.store.SlackIntegration(ctx)
	if err != nil {
		return integration, err
	}
	if err := w.checkTeam(ctx, integration); err != nil {
		return integration, err
	}
	if !integration.Delivers(delivery.Kind) {
		return integration, errDisabled
	}
	return integration, nil
}

func (w *Worker) deliver(ctx context.Context, delivery domain.SlackDelivery) error {
	integration, err := w.ready(ctx, delivery)
	if err != nil {
		return err
	}
	var channel, ts string
	if delivery.Kind == domain.SlackDeliveryClapUpdate || delivery.Kind == domain.SlackDeliveryPostDelete {
		if delivery.AnnounceID == nil {
			return errInvalidDelivery
		}
		announce, err := w.store.SlackDeliveryByID(ctx, *delivery.AnnounceID)
		if errors.Is(err, domain.ErrNotFound) {
			return errAnnounceNotSent
		}
		if err != nil {
			return err
		}
		if announce.State != domain.SlackDeliverySent || announce.ChannelID == nil || announce.MessageTS == nil {
			return errAnnounceNotSent
		}
		channel, ts = *announce.ChannelID, *announce.MessageTS
	}
	if delivery.Kind == domain.SlackDeliveryPostDelete {
		w.nextCall = w.now().Add(time.Second)
		err := w.slack.DeleteMessage(ctx, channel, ts)
		var apiErr *slack.Error
		if err != nil && !(errors.As(err, &apiErr) && apiErr.Code == "message_not_found") {
			return err
		}
		return w.store.MarkSlackDeliverySent(ctx, delivery.ID, channel, ts, w.now())
	}
	post, err := w.store.PostByID(ctx, delivery.PostID, "")
	if errors.Is(err, domain.ErrNotFound) {
		return errPostDeleted
	}
	if err != nil {
		return err
	}
	sender, err := w.store.MemberByID(ctx, post.Sender.ID)
	if err != nil {
		return err
	}
	recipientID := post.Recipient.ID
	if delivery.Kind == domain.SlackDeliveryPostDM && delivery.MemberID != nil {
		recipientID = *delivery.MemberID
	}
	recipient, err := w.store.MemberByID(ctx, recipientID)
	if err != nil {
		return err
	}
	if delivery.Kind == domain.SlackDeliveryPostDM && recipient.SlackUserID == nil {
		if recipient.SlackLookupAt != nil && w.now().Before(recipient.SlackLookupAt.Add(24*time.Hour)) {
			return errUserNotFound
		}
		w.nextCall = w.now().Add(time.Second)
		userID, err := w.slack.LookupUserByEmail(ctx, recipient.Email)
		if err != nil && !errors.Is(err, slack.ErrUserNotFound) {
			return err
		}
		// A force re-verification clears member IDs. Do not repopulate that
		// cache with a lookup from the previous workspace.
		current, integrationErr := w.store.SlackIntegration(ctx)
		if integrationErr != nil {
			return integrationErr
		}
		if teamErr := w.checkTeam(ctx, current); teamErr != nil {
			return teamErr
		}
		var resolved *string
		if err == nil && userID != "" {
			resolved = &userID
		}
		if err := w.store.SetMemberSlackUser(ctx, recipient.ID, resolved, w.now()); err != nil {
			return err
		}
		if resolved == nil {
			return errUserNotFound
		}
		recipient.SlackUserID = resolved
		integration, err = w.ready(ctx, delivery)
		if err != nil {
			return err
		}
		// Deletion during the lookup or pacing must not produce a new DM.
		if _, err := w.store.PostByID(ctx, delivery.PostID, ""); errors.Is(err, domain.ErrNotFound) {
			return errPostDeleted
		} else if err != nil {
			return err
		}
	}
	slackIDs := make(map[string]string, 2)
	if sender.SlackUserID != nil {
		slackIDs[sender.ID] = *sender.SlackUserID
	}
	if recipient.SlackUserID != nil {
		slackIDs[recipient.ID] = *recipient.SlackUserID
	}
	switch delivery.Kind {
	case domain.SlackDeliveryPostDM:
		channel = *recipient.SlackUserID
		w.nextCall = w.now().Add(time.Second)
		ts, err = w.slack.PostMessage(ctx, channel, DirectMessage(post, sender, recipient, integration.Locale, w.publicURL, slackIDs))
	case domain.SlackDeliveryPostAnnounce, domain.SlackDeliveryClapUpdate:
		program, programErr := w.store.Program(ctx)
		if programErr != nil {
			return programErr
		}
		labels := make([]string, len(post.Values))
		for i, key := range post.Values {
			labels[i] = key
			for _, value := range program.Values {
				if value.Key == key {
					labels[i] = value.Label
					break
				}
			}
		}
		post.Values = labels
		if delivery.Kind == domain.SlackDeliveryPostAnnounce {
			if delivery.ChannelID == nil {
				return errInvalidDelivery
			}
			channel = *delivery.ChannelID
			w.nextCall = w.now().Add(time.Second)
			ts, err = w.slack.PostMessage(ctx, channel, Announce(post, sender, recipient, nil, integration.Locale, w.publicURL, slackIDs))
		} else {
			claps, clapErr := w.store.ListClaps(ctx, post.ID, 3)
			if errors.Is(clapErr, domain.ErrNotFound) {
				return errPostDeleted
			}
			if clapErr != nil {
				return clapErr
			}
			w.nextCall = w.now().Add(time.Second)
			err = w.slack.UpdateMessage(ctx, channel, ts, ClapUpdate(post, sender, recipient, claps, integration.Locale, w.publicURL, slackIDs))
		}
	default:
		return errInvalidDelivery
	}
	if err != nil {
		return err
	}
	return w.store.MarkSlackDeliverySent(ctx, delivery.ID, channel, ts, w.now())
}

func (w *Worker) pace(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay := w.nextCall.Sub(w.now()); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func retryDelay(attempts int, err error) time.Duration {
	var apiErr *slack.Error
	if slack.IsRateLimited(err) && errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}
	if attempts >= 10 {
		return 15 * time.Minute
	}
	return time.Second << max(attempts, 0)
}

func errorCode(err error) string {
	var apiErr *slack.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	if errors.Is(err, errInvalidDelivery) {
		return "invalid_delivery"
	}
	return "delivery_error"
}
