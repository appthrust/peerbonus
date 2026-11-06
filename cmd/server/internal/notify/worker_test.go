package notify

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	t             *testing.T
	integration   domain.SlackIntegration
	rows          []domain.SlackDelivery
	post          domain.Post
	postErr       error
	members       map[string]domain.Member
	claps         []domain.Clap
	claims        int
	lookups       int
	clapLimit     int
	onIntegration func()
}

func (s *fakeStore) SlackIntegration(context.Context) (domain.SlackIntegration, error) {
	if s.onIntegration != nil {
		s.onIntegration()
	}
	return s.integration, nil
}
func (s *fakeStore) SetSlackLastError(_ context.Context, code *string, now time.Time) error {
	s.integration.LastErrorCode, s.integration.LastErrorAt = code, &now
	return nil
}
func (s *fakeStore) ClaimSlackDeliveries(_ context.Context, now time.Time, limit int, lease time.Duration) ([]domain.SlackDelivery, error) {
	s.claims++
	require.Equal(s.t, 10, limit)
	require.Equal(s.t, 60*time.Second, lease)
	var claimed []domain.SlackDelivery
	for i := range s.rows {
		d := &s.rows[i]
		if d.State != domain.SlackDeliveryPending || d.NextAt.After(now) || (d.ClaimedUntil != nil && !d.ClaimedUntil.Before(now)) {
			continue
		}
		if d.AnnounceID != nil {
			parent, err := s.SlackDeliveryByID(context.Background(), *d.AnnounceID)
			if err != nil {
				return nil, err
			}
			if parent.State == domain.SlackDeliveryFailed || parent.State == domain.SlackDeliverySkipped {
				d.State, d.LastError = domain.SlackDeliverySkipped, new("announce_not_sent")
				continue
			}
			if parent.State != domain.SlackDeliverySent {
				continue
			}
		}
		until := now.Add(lease)
		d.ClaimedUntil = &until
		claimed = append(claimed, *d)
		if len(claimed) == limit {
			break
		}
	}
	return claimed, nil
}
func (s *fakeStore) row(id int64) *domain.SlackDelivery {
	for i := range s.rows {
		if s.rows[i].ID == id {
			return &s.rows[i]
		}
	}
	return nil
}
func (s *fakeStore) MarkSlackDeliverySent(_ context.Context, id int64, channel, ts string, now time.Time) error {
	d := s.row(id)
	d.State, d.ChannelID, d.MessageTS, d.SentAt, d.ClaimedUntil = domain.SlackDeliverySent, &channel, &ts, &now, nil
	return nil
}
func (s *fakeStore) RetrySlackDelivery(_ context.Context, id int64, at time.Time, code string) error {
	d := s.row(id)
	d.Attempts++
	d.NextAt, d.LastError, d.ClaimedUntil = at, &code, nil
	if d.Attempts >= domain.SlackMaxDeliveryAttempts {
		d.State = domain.SlackDeliveryFailed
	}
	return nil
}
func (s *fakeStore) SkipSlackDelivery(_ context.Context, id int64, reason string, _ time.Time) error {
	d := s.row(id)
	d.State, d.LastError, d.ClaimedUntil = domain.SlackDeliverySkipped, &reason, nil
	return nil
}
func (s *fakeStore) SlackDeliveryByID(_ context.Context, id int64) (domain.SlackDelivery, error) {
	if d := s.row(id); d != nil {
		return *d, nil
	}
	return domain.SlackDelivery{}, domain.ErrNotFound
}
func (s *fakeStore) PostByID(_ context.Context, id, viewerID string) (domain.Post, error) {
	require.Equal(s.t, s.post.ID, id)
	require.Empty(s.t, viewerID)
	return s.post, s.postErr
}
func (s *fakeStore) ListClaps(_ context.Context, id string, limit int) ([]domain.Clap, error) {
	require.Equal(s.t, s.post.ID, id)
	s.clapLimit = limit
	return s.claps[:min(limit, len(s.claps))], nil
}
func (s *fakeStore) MemberByID(_ context.Context, id string) (domain.Member, error) {
	m, ok := s.members[id]
	if !ok {
		return m, domain.ErrNotFound
	}
	return m, nil
}
func (s *fakeStore) SetMemberSlackUser(_ context.Context, id string, user *string, now time.Time) error {
	s.lookups++
	m := s.members[id]
	m.SlackUserID, m.SlackLookupAt = user, &now
	s.members[id] = m
	return nil
}
func (s *fakeStore) Program(context.Context) (domain.Program, error) {
	return domain.Program{Values: []domain.Value{{Key: "thanks", Label: "感謝"}}}, nil
}

type slackCall struct {
	method, channel, ts, email string
	message                    slack.Message
	at                         time.Time
}
type fakeSlack struct {
	team   string
	user   string
	calls  []slackCall
	errors map[string][]error
	onCall func(string)
}

func (s *fakeSlack) call(c slackCall) error {
	c.at = time.Now()
	s.calls = append(s.calls, c)
	if s.onCall != nil {
		s.onCall(c.method)
	}
	if queue := s.errors[c.method]; len(queue) > 0 {
		s.errors[c.method] = queue[1:]
		return queue[0]
	}
	return nil
}
func (s *fakeSlack) AuthTest(context.Context) (slack.AuthInfo, error) {
	return slack.AuthInfo{TeamID: s.team}, s.call(slackCall{method: "auth"})
}
func (s *fakeSlack) PostMessage(_ context.Context, channel string, m slack.Message) (string, error) {
	return "123.456", s.call(slackCall{method: "post", channel: channel, message: m})
}
func (s *fakeSlack) UpdateMessage(_ context.Context, channel, ts string, m slack.Message) error {
	return s.call(slackCall{method: "update", channel: channel, ts: ts, message: m})
}
func (s *fakeSlack) DeleteMessage(_ context.Context, channel, ts string) error {
	return s.call(slackCall{method: "delete", channel: channel, ts: ts})
}
func (s *fakeSlack) LookupUserByEmail(_ context.Context, email string) (string, error) {
	return s.user, s.call(slackCall{method: "lookup", email: email})
}
func (s *fakeSlack) count(method string) int {
	n := 0
	for _, call := range s.calls {
		if call.method == method {
			n++
		}
	}
	return n
}

func fixture(t *testing.T) (*Worker, *fakeStore, *fakeSlack) {
	t.Helper()
	sender := domain.Member{ID: "sender", DisplayName: "Alice", Email: "alice@example.test", SlackUserID: new("UALICE")}
	recipient := domain.Member{ID: "recipient", DisplayName: "Bob", Email: "bob@example.test"}
	store := &fakeStore{t: t,
		integration: domain.SlackIntegration{Workspace: &domain.SlackWorkspace{TeamID: "T1"}, AnnounceChannel: &domain.SlackChannel{ID: "C1"}, AnnouncePosts: true, DMRecipientOnPost: true, Locale: domain.SlackLocaleJA},
		post:        domain.Post{ID: "post1", Sender: sender, Recipient: recipient, Points: 30, Message: "Thanks & <cheers>", Values: []string{"thanks"}},
		members:     map[string]domain.Member{sender.ID: sender, recipient.ID: recipient},
	}
	client := &fakeSlack{team: "T1", user: "UBOB", errors: make(map[string][]error)}
	return New(Options{Store: store, Slack: client, PublicURL: "https://bonus.example.test/"}), store, client
}
func enqueue(s *fakeStore, kind domain.SlackDeliveryKind) int64 {
	id := int64(len(s.rows) + 1)
	d := domain.SlackDelivery{ID: id, PostID: s.post.ID, Kind: kind, State: domain.SlackDeliveryPending, NextAt: time.Now()}
	if kind == domain.SlackDeliveryPostAnnounce {
		d.ChannelID = new("C1")
	}
	if kind == domain.SlackDeliveryPostDM {
		d.MemberID = new(s.post.Recipient.ID)
	}
	s.rows = append(s.rows, d)
	return id
}
func sentAnnounce(s *fakeStore) int64 {
	id := enqueue(s, domain.SlackDeliveryPostAnnounce)
	d := s.row(id)
	d.State, d.MessageTS = domain.SlackDeliverySent, new("100.200")
	return id
}
func assertPaced(t *testing.T, calls []slackCall) {
	t.Helper()
	for i := 1; i < len(calls); i++ {
		require.GreaterOrEqual(t, calls[i].at.Sub(calls[i-1].at), time.Second)
	}
}

func TestHappyPathAnnounceAndDM(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		enqueue(s, domain.SlackDeliveryPostAnnounce)
		enqueue(s, domain.SlackDeliveryPostDM)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 1, client.count("auth"))
		require.Equal(t, 2, client.count("post"))
		require.Equal(t, 1, client.count("lookup"))
		require.Equal(t, "C1", client.calls[1].channel)
		require.Contains(t, client.calls[1].message.Text, "#感謝")
		require.Contains(t, client.calls[1].message.Text, "<@UALICE>")
		require.Equal(t, "bob@example.test", client.calls[2].email)
		require.Equal(t, "UBOB", client.calls[3].channel)
		for _, d := range s.rows {
			require.Equal(t, domain.SlackDeliverySent, d.State)
			require.Equal(t, "123.456", *d.MessageTS)
			require.Nil(t, d.ClaimedUntil)
		}
		require.Equal(t, "UBOB", *s.members["recipient"].SlackUserID)
		assertPaced(t, client.calls)
		require.Equal(t, []string{"thanks"}, s.post.Values, "rendering must not mutate store data")
	})
}

func TestDMWithoutAnnouncement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		s.integration.AnnouncePosts, s.integration.AnnounceChannel = false, nil
		id := enqueue(s, domain.SlackDeliveryPostDM)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, domain.SlackDeliverySent, s.row(id).State)
		require.Equal(t, 1, client.count("post"))
	})
}

func TestTeamChangedPausesAndCacheExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		id := enqueue(s, domain.SlackDeliveryPostAnnounce)
		client.team = "OTHER"
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, "team_changed", *s.integration.LastErrorCode)
		require.Equal(t, 0, s.claims)
		require.Equal(t, domain.SlackDeliveryPending, s.row(id).State)
		client.team = "T1"
		time.Sleep(10*time.Minute - time.Second)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 1, client.count("auth"))
		require.Zero(t, client.count("post"))
		time.Sleep(time.Second)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 2, client.count("auth"))
		require.Nil(t, s.integration.LastErrorCode)
		require.Equal(t, domain.SlackDeliverySent, s.row(id).State)
	})
}

func TestUnverifiedAndAuthFailureDoNotClaim(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		s.integration.Workspace = nil
		require.NoError(t, w.runOnce(t.Context()))
		require.Empty(t, client.calls)
		s.integration.Workspace = &domain.SlackWorkspace{TeamID: "T1"}
		client.errors["auth"] = []error{&slack.Error{Code: "invalid_auth"}}
		require.Error(t, w.runOnce(t.Context()))
		require.Zero(t, s.claims)
		require.Equal(t, "invalid_auth", *s.integration.LastErrorCode)
	})
}

func TestRetryBackoffAndFailedAtTwelve(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		id := enqueue(s, domain.SlackDeliveryPostAnnounce)
		for range 12 {
			client.errors["post"] = append(client.errors["post"], &slack.Error{Code: "not_in_channel"})
		}
		for attempt := range 12 {
			require.NoError(t, w.runOnce(t.Context()))
			d := s.row(id)
			want := min(15*time.Minute, time.Second<<attempt)
			require.Equal(t, want, d.NextAt.Sub(time.Now()))
			require.Equal(t, attempt+1, d.Attempts)
			require.Nil(t, d.ClaimedUntil)
			require.Equal(t, "not_in_channel", *d.LastError)
			if attempt < 11 {
				require.Equal(t, domain.SlackDeliveryPending, d.State)
			}
			time.Sleep(want)
		}
		require.Equal(t, domain.SlackDeliveryFailed, s.row(id).State)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 12, client.count("post"))
	})
}

func TestRateLimitUsesRetryAfter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		id := enqueue(s, domain.SlackDeliveryPostAnnounce)
		client.errors["post"] = []error{fmt.Errorf("wrapped: %w", &slack.Error{Code: "ratelimited", Status: 429, RetryAfter: 37 * time.Second})}
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 37*time.Second, s.row(id).NextAt.Sub(time.Now()))
		time.Sleep(36 * time.Second)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 1, client.count("post"))
		time.Sleep(time.Second)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, domain.SlackDeliverySent, s.row(id).State)
	})
}

func TestMissingUserNegativeCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		client.errors["lookup"] = []error{fmt.Errorf("wrapped: %w", slack.ErrUserNotFound)}
		id := enqueue(s, domain.SlackDeliveryPostDM)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, "user_not_found", *s.row(id).LastError)
		require.Equal(t, domain.SlackDeliverySkipped, s.row(id).State)
		cached := *s.members["recipient"].SlackLookupAt
		require.Nil(t, s.members["recipient"].SlackUserID)
		id = enqueue(s, domain.SlackDeliveryPostDM)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, domain.SlackDeliverySkipped, s.row(id).State)
		require.Equal(t, 1, client.count("lookup"))
		require.Equal(t, 1, s.lookups)
		time.Sleep(cached.Add(24 * time.Hour).Sub(time.Now()))
		id = enqueue(s, domain.SlackDeliveryPostDM)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 2, client.count("lookup"))
		require.Equal(t, domain.SlackDeliverySent, s.row(id).State)
	})
}

func TestLookupFailureDoesNotCacheMissingUser(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		id := enqueue(s, domain.SlackDeliveryPostDM)
		client.errors["lookup"] = []error{&slack.Error{Code: "ratelimited", Status: 429, RetryAfter: time.Minute}}
		require.NoError(t, w.runOnce(t.Context()))
		require.Nil(t, s.members["recipient"].SlackLookupAt)
		require.Equal(t, domain.SlackDeliveryPending, s.row(id).State)
		require.Equal(t, time.Minute, s.row(id).NextAt.Sub(time.Now()))
	})
}

func TestClapUpdateUsesCurrentCountAndThreeNames(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		parent := sentAnnounce(s)
		id := enqueue(s, domain.SlackDeliveryClapUpdate)
		s.row(id).AnnounceID = &parent
		s.post.ClapCount = 9
		for _, name := range []string{"One", "Two", "Three", "Four"} {
			s.claps = append(s.claps, domain.Clap{Member: domain.Member{DisplayName: name}})
		}
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 3, s.clapLimit)
		require.Equal(t, 1, client.count("update"))
		require.Zero(t, client.count("post"))
		call := client.calls[1]
		require.Equal(t, "C1", call.channel)
		require.Equal(t, "100.200", call.ts)
		require.Contains(t, call.message.Text, "拍手 (9): One, Two, Three")
		require.NotContains(t, call.message.Text, "Four")
		require.Equal(t, domain.SlackDeliverySent, s.row(id).State)
	})
}

func TestDeleteMissingMessageCountsAsSent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		parent := sentAnnounce(s)
		id := enqueue(s, domain.SlackDeliveryPostDelete)
		s.row(id).AnnounceID = &parent
		s.postErr = domain.ErrNotFound
		client.errors["delete"] = []error{&slack.Error{Code: "message_not_found"}}
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 1, client.count("delete"))
		require.Equal(t, "C1", client.calls[1].channel)
		require.Equal(t, "100.200", client.calls[1].ts)
		require.Equal(t, domain.SlackDeliverySent, s.row(id).State)
	})
}

func TestDisabledAndDeletedPostsSkip(t *testing.T) {
	for _, kind := range []domain.SlackDeliveryKind{domain.SlackDeliveryPostAnnounce, domain.SlackDeliveryPostDM} {
		for _, reason := range []string{"disabled", "post_deleted"} {
			t.Run(string(kind)+"/"+reason, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					w, s, client := fixture(t)
					id := enqueue(s, kind)
					if reason == "disabled" {
						s.integration.AnnouncePosts, s.integration.DMRecipientOnPost = false, false
					} else {
						s.postErr = fmt.Errorf("wrapped: %w", domain.ErrNotFound)
					}
					require.NoError(t, w.runOnce(t.Context()))
					require.Equal(t, domain.SlackDeliverySkipped, s.row(id).State)
					require.Equal(t, reason, *s.row(id).LastError)
					require.Zero(t, client.count("post"))
					require.Zero(t, client.count("lookup"))
				})
			})
		}
	}
}

func TestSettingsAndDeletionRecheckedAfterLookup(t *testing.T) {
	for _, reason := range []string{"disabled", "post_deleted", "team_changed"} {
		t.Run(reason, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w, s, client := fixture(t)
				id := enqueue(s, domain.SlackDeliveryPostDM)
				client.onCall = func(method string) {
					if method != "lookup" {
						return
					}
					switch reason {
					case "disabled":
						s.integration.DMRecipientOnPost = false
					case "post_deleted":
						s.postErr = domain.ErrNotFound
					case "team_changed":
						s.integration.Workspace = &domain.SlackWorkspace{TeamID: "T2"}
					}
				}
				require.NoError(t, w.runOnce(t.Context()))
				require.Zero(t, client.count("post"))
				if reason == "team_changed" {
					require.Equal(t, domain.SlackDeliveryPending, s.row(id).State)
					require.Equal(t, "team_changed", *s.integration.LastErrorCode)
				} else {
					require.Equal(t, domain.SlackDeliverySkipped, s.row(id).State)
					require.Equal(t, reason, *s.row(id).LastError)
				}
			})
		})
	}
}

func TestLeaseAndBatchLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		for range 12 {
			enqueue(s, domain.SlackDeliveryPostAnnounce)
		}
		until := time.Now().Add(time.Minute)
		s.row(1).ClaimedUntil = &until
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 10, client.count("post"))
		require.Equal(t, domain.SlackDeliveryPending, s.row(1).State)
		require.Equal(t, domain.SlackDeliveryPending, s.row(12).State)
		time.Sleep(until.Sub(time.Now()) + time.Nanosecond)
		require.NoError(t, w.runOnce(t.Context()))
		require.Equal(t, 12, client.count("post"))
		assertPaced(t, client.calls)
	})
}

func TestExpiredBatchLeasesAreNotSent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		for range 10 {
			enqueue(s, domain.SlackDeliveryPostAnnounce)
		}
		client.onCall = func(method string) {
			if method == "post" {
				time.Sleep(10 * time.Second)
			}
		}
		claimedAt := time.Now()
		require.NoError(t, w.runOnce(t.Context()))
		require.Less(t, client.count("post"), 10)
		for _, call := range client.calls {
			if call.method == "post" {
				require.True(t, call.at.Before(claimedAt.Add(60*time.Second)))
			}
		}
		require.Equal(t, domain.SlackDeliveryPending, s.row(10).State)
	})
}

func TestRunKickTickAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, _ := fixture(t)
		require.Equal(t, 2*time.Second, w.interval)
		var absent *Worker
		absent.Kick()
		ctx, cancel := context.WithCancel(t.Context())
		var result error
		go func() { result = w.Run(ctx) }()
		synctest.Wait()
		require.Equal(t, 1, s.claims)
		w.Kick()
		w.Kick()
		synctest.Wait()
		require.GreaterOrEqual(t, s.claims, 2)
		before := s.claims
		time.Sleep(2 * time.Second)
		synctest.Wait()
		require.Greater(t, s.claims, before)
		cancel()
		synctest.Wait()
		require.ErrorIs(t, result, context.Canceled)
	})
}

func TestCancellationInterruptsPacing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, s, client := fixture(t)
		id := enqueue(s, domain.SlackDeliveryPostAnnounce)
		ctx, cancel := context.WithCancel(t.Context())
		client.onCall = func(method string) {
			if method == "auth" {
				cancel()
			}
		}
		err := w.runOnce(ctx)
		require.True(t, errors.Is(err, context.Canceled))
		require.Zero(t, client.count("post"))
		require.Zero(t, s.row(id).Attempts)
	})
}
