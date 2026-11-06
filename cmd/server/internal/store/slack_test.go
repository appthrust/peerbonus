package store

import (
	"errors"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func connectSlack(t *testing.T, db *Store) {
	t.Helper()
	_, err := db.FixSlackWorkspace(t.Context(), domain.SlackWorkspace{TeamID: "T1", TeamName: "Acme", BotUserID: "UBOT"}, false, october)
	require.NoError(t, err)
	channel := &domain.SlackChannel{ID: "C1", Name: "thanks"}
	_, err = db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{AnnounceChannel: &channel, AnnouncePosts: new(true)}, october)
	require.NoError(t, err)
}

func TestSlackOutboxCreation(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	input := CreatePostInput{SenderID: members[0].ID, RecipientID: members[1].ID, Points: 10, Message: "Thanks", Now: october, IdempotencyKey: "post"}
	rollback := errors.New("rollback")
	require.ErrorIs(t, db.Do(t.Context(), true, func(tx *Tx) error {
		_, replayed, err := tx.createPost(input)
		require.NoError(t, err)
		require.False(t, replayed)
		var queued int
		require.NoError(t, tx.SQL.QueryRowContext(t.Context(), `SELECT count(*) FROM slack_deliveries`).Scan(&queued))
		require.Equal(t, 2, queued)
		return rollback
	}), rollback)
	require.Zero(t, count(t, db, `SELECT count(*) FROM posts`))
	require.Zero(t, count(t, db, `SELECT count(*) FROM ledger`))
	require.Zero(t, count(t, db, `SELECT count(*) FROM slack_deliveries`))

	post, replayed, err := db.CreatePost(t.Context(), input)
	require.NoError(t, err)
	require.False(t, replayed)
	_, replayed, err = db.CreatePost(t.Context(), input)
	require.NoError(t, err)
	require.True(t, replayed)
	deliveries, err := db.ListSlackDeliveries(t.Context(), "", 20)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	require.Equal(t, domain.SlackDeliveryPostDM, deliveries[0].Kind)
	require.Equal(t, &members[1].ID, deliveries[0].MemberID)
	require.Nil(t, deliveries[0].AnnounceID, "DM delivery is independent of announcements")
	require.Equal(t, domain.SlackDeliveryPostAnnounce, deliveries[1].Kind)
	require.Equal(t, new("C1"), deliveries[1].ChannelID)
	for _, delivery := range deliveries {
		require.Equal(t, post.ID, delivery.PostID)
		require.Equal(t, domain.SlackDeliveryPending, delivery.State)
		require.True(t, delivery.NextAt.Equal(october))
		require.True(t, delivery.CreatedAt.Equal(october))
	}
	claimed, err := db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 2)
	require.Less(t, claimed[0].ID, claimed[1].ID)
}

func TestSlackOutboxDeliveryFlags(t *testing.T) {
	for _, tc := range []struct {
		name      string
		verified  bool
		announce  bool
		dm        bool
		channel   bool
		wantKinds []domain.SlackDeliveryKind
	}{
		{name: "unverified", announce: true, dm: true, channel: true},
		{name: "disabled", verified: true, channel: true},
		{name: "dm only", verified: true, dm: true, wantKinds: []domain.SlackDeliveryKind{domain.SlackDeliveryPostDM}},
		{name: "announce only", verified: true, announce: true, channel: true, wantKinds: []domain.SlackDeliveryKind{domain.SlackDeliveryPostAnnounce}},
		{name: "no channel", verified: true, announce: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, members := fixture(t)
			if tc.verified {
				_, err := db.FixSlackWorkspace(t.Context(), domain.SlackWorkspace{TeamID: "T1"}, false, october)
				require.NoError(t, err)
			}
			var channel *domain.SlackChannel
			if tc.channel {
				channel = &domain.SlackChannel{ID: "C1", Name: "thanks"}
			}
			_, err := db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{AnnounceChannel: &channel, AnnouncePosts: &tc.announce, DMRecipientOnPost: &tc.dm}, october)
			require.NoError(t, err)
			post := send(t, db, members[0], members[1], 10, october)
			items, err := db.ListSlackDeliveries(t.Context(), "", 10)
			require.NoError(t, err)
			require.Len(t, items, len(tc.wantKinds))
			for i, kind := range tc.wantKinds {
				require.Equal(t, kind, items[i].Kind)
			}
			claimed, err := db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
			require.NoError(t, err)
			require.Len(t, claimed, len(tc.wantKinds))
			clap(t, db, post, members[2], october)
			require.NoError(t, db.DeletePost(t.Context(), post.ID, members[0].ID, october))
			if len(tc.wantKinds) == 0 {
				require.Zero(t, count(t, db, `SELECT count(*) FROM slack_deliveries`))
			}
		})
	}
}

func TestSlackClapsCoalesceAndReplay(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	post := send(t, db, members[0], members[1], 10, october)
	input := CreateClapInput{PostID: post.ID, MemberID: members[2].ID, IdempotencyKey: "clap", Now: october}
	rollback := errors.New("rollback")
	require.ErrorIs(t, db.Do(t.Context(), true, func(tx *Tx) error {
		_, _, _, err := tx.createClap(input)
		require.NoError(t, err)
		return rollback
	}), rollback)
	require.Zero(t, count(t, db, `SELECT count(*) FROM claps`))
	require.Zero(t, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE kind='clap_update'`))
	first, _, _, err := db.CreateClap(t.Context(), input)
	require.NoError(t, err)
	clap(t, db, post, members[2], october)
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE kind='clap_update' AND state='pending'`))
	items, err := db.ListSlackDeliveries(t.Context(), "", 10)
	require.NoError(t, err)
	require.Equal(t, &first.ID, items[0].ClapID)
	require.Equal(t, &items[2].ID, items[0].AnnounceID)
	require.NoError(t, db.MarkSlackDeliverySent(t.Context(), items[2].ID, "C1", "123.1", october))
	require.NoError(t, db.MarkSlackDeliverySent(t.Context(), items[0].ID, "C1", "123.1", october))
	_, _, replayed, err := db.CreateClap(t.Context(), input)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Zero(t, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE kind='clap_update' AND state='pending'`))
	clap(t, db, post, members[2], october)
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE kind='clap_update' AND state='pending'`))
	require.Equal(t, 2, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE kind='clap_update'`))
}

func TestSlackClapDoesNotInventAnnouncement(t *testing.T) {
	db, members := fixture(t)
	post := send(t, db, members[0], members[1], 10, october)
	connectSlack(t, db)
	clap(t, db, post, members[2], october)
	require.NoError(t, db.DeletePost(t.Context(), post.ID, members[0].ID, october))
	require.Zero(t, count(t, db, `SELECT count(*) FROM slack_deliveries`))
}

func TestSlackClaimOrderingAndLeases(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	_, err := db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{DMRecipientOnPost: new(false)}, october)
	require.NoError(t, err)
	post := send(t, db, members[0], members[1], 10, october)
	clap(t, db, post, members[2], october)
	items, err := db.ClaimSlackDeliveries(t.Context(), october.Add(-time.Second), 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, items)
	items, err = db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	announce := items[0]
	require.Equal(t, domain.SlackDeliveryPostAnnounce, announce.Kind)
	require.True(t, announce.ClaimedUntil.Equal(october.Add(time.Minute)))
	for _, at := range []time.Time{october, october.Add(time.Minute)} {
		items, err = db.ClaimSlackDeliveries(t.Context(), at, 10, time.Minute)
		require.NoError(t, err)
		require.Empty(t, items)
	}
	items, err = db.ClaimSlackDeliveries(t.Context(), october.Add(time.Minute+time.Second), 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, announce.ID, items[0].ID)
	require.NoError(t, db.MarkSlackDeliverySent(t.Context(), announce.ID, "C1", "123.1", october))
	items, err = db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, domain.SlackDeliveryClapUpdate, items[0].Kind)
	require.Equal(t, &announce.ID, items[0].AnnounceID)
	require.Nil(t, items[0].MessageTS, "claim returns the delivery, not its announcement")
}

func TestSlackClaimSkipsUnsentAnnouncementDependents(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "skipped", true: "failed"}[failed], func(t *testing.T) {
			db, members := fixture(t)
			connectSlack(t, db)
			post := send(t, db, members[0], members[1], 10, october)
			clap(t, db, post, members[2], october)
			all, err := db.ListSlackDeliveries(t.Context(), "", 10)
			require.NoError(t, err)
			announce := all[2]
			if failed {
				for range domain.SlackMaxDeliveryAttempts {
					require.NoError(t, db.RetrySlackDelivery(t.Context(), announce.ID, october, "invalid_auth"))
				}
			} else {
				require.NoError(t, db.SkipSlackDelivery(t.Context(), announce.ID, "disabled", october))
			}
			items, err := db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
			require.NoError(t, err)
			require.Len(t, items, 1, "DM remains independent of the unsent announcement")
			require.Equal(t, domain.SlackDeliveryPostDM, items[0].Kind)
			update, err := db.SlackDeliveryByID(t.Context(), all[0].ID)
			require.NoError(t, err)
			require.Equal(t, domain.SlackDeliverySkipped, update.State)
			require.Equal(t, new("announce_not_sent"), update.LastError)
			require.Nil(t, update.ClaimedUntil)
		})
	}
}

func TestSlackDeleteSkipsPendingAndEnqueuesDeletion(t *testing.T) {
	for _, sent := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending announce", true: "sent announce"}[sent], func(t *testing.T) {
			db, members := fixture(t)
			connectSlack(t, db)
			post := send(t, db, members[0], members[1], 10, october)
			clap(t, db, post, members[2], october)
			all, err := db.ListSlackDeliveries(t.Context(), "", 10)
			require.NoError(t, err)
			announce := all[2]
			if sent {
				require.NoError(t, db.MarkSlackDeliverySent(t.Context(), announce.ID, "C1", "123.1", october))
			}
			require.NoError(t, db.DeletePost(t.Context(), post.ID, members[0].ID, october))
			all, err = db.ListSlackDeliveries(t.Context(), "", 10)
			require.NoError(t, err)
			require.Len(t, all, 4)
			require.Equal(t, domain.SlackDeliveryPostDelete, all[0].Kind)
			require.Equal(t, &announce.ID, all[0].AnnounceID)
			for _, item := range all[1:] {
				if item.ID == announce.ID && sent {
					require.Equal(t, domain.SlackDeliverySent, item.State)
					continue
				}
				require.Equal(t, domain.SlackDeliverySkipped, item.State)
				require.Equal(t, new("post_deleted"), item.LastError)
				require.Nil(t, item.ClaimedUntil)
			}
			claimed, err := db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
			require.NoError(t, err)
			if sent {
				require.Len(t, claimed, 1)
				require.Equal(t, all[0].ID, claimed[0].ID)
			} else {
				require.Empty(t, claimed)
				deletion, err := db.SlackDeliveryByID(t.Context(), all[0].ID)
				require.NoError(t, err)
				require.Equal(t, domain.SlackDeliverySkipped, deletion.State)
				require.Equal(t, new("announce_not_sent"), deletion.LastError)
			}
		})
	}
}

func TestSlackDeleteRollbackAndDisabledCleanup(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	post := send(t, db, members[0], members[1], 10, october)
	clap(t, db, post, members[2], october)
	require.NoError(t, db.Do(t.Context(), true, func(tx *Tx) error {
		return tx.Exec(`CREATE FUNCTION reject_slack_delete() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'reject delivery'; END $$;
 CREATE TRIGGER reject_slack_delete BEFORE INSERT ON slack_deliveries
 FOR EACH ROW WHEN (NEW.kind='post_delete') EXECUTE FUNCTION reject_slack_delete()`)
	}))
	require.ErrorContains(t, db.DeletePost(t.Context(), post.ID, members[0].ID, october), "reject delivery")
	_, err := db.PostByID(t.Context(), post.ID, members[0].ID)
	require.NoError(t, err)
	require.Zero(t, count(t, db, `SELECT count(*) FROM ledger WHERE kind='reversal'`))
	require.Equal(t, 3, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE state='pending'`))
	_, err = db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{AnnouncePosts: new(false), DMRecipientOnPost: new(false)}, october)
	require.NoError(t, err)
	require.NoError(t, db.DeletePost(t.Context(), post.ID, members[0].ID, october))
	require.Zero(t, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE state='pending'`))
	require.Equal(t, 3, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE state='skipped' AND last_error='post_deleted'`))
}

func TestSlackRetryStopsAtTwelveAndTerminalStatesStayTerminal(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	send(t, db, members[0], members[1], 10, october)
	items, err := db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
	require.NoError(t, err)
	id := items[0].ID
	for attempt := 1; attempt <= domain.SlackMaxDeliveryAttempts; attempt++ {
		next := october.Add(time.Duration(attempt) * time.Minute)
		require.NoError(t, db.RetrySlackDelivery(t.Context(), id, next, "ratelimited"))
		item, err := db.SlackDeliveryByID(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, attempt, item.Attempts)
		require.True(t, item.NextAt.Equal(next))
		require.Nil(t, item.ClaimedUntil)
		require.Equal(t, new("ratelimited"), item.LastError)
		if attempt < domain.SlackMaxDeliveryAttempts {
			require.Equal(t, domain.SlackDeliveryPending, item.State)
		} else {
			require.Equal(t, domain.SlackDeliveryFailed, item.State)
		}
	}
	require.NoError(t, db.RetrySlackDelivery(t.Context(), id, november, "ignored"))
	require.NoError(t, db.MarkSlackDeliverySent(t.Context(), id, "C1", "123.1", october))
	require.NoError(t, db.SkipSlackDelivery(t.Context(), id, "ignored", october))
	failed, err := db.SlackDeliveryByID(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, domain.SlackDeliveryFailed, failed.State)
	require.Equal(t, 12, failed.Attempts)
	require.Equal(t, new("ratelimited"), failed.LastError)
	require.NoError(t, db.RetrySlackDelivery(t.Context(), items[1].ID, october, "temporary"))
	require.NoError(t, db.MarkSlackDeliverySent(t.Context(), items[1].ID, "D1", "123.2", october))
	require.NoError(t, db.RetrySlackDelivery(t.Context(), items[1].ID, november, "ignored"))
	require.NoError(t, db.SkipSlackDelivery(t.Context(), items[1].ID, "ignored", october))
	sent, err := db.SlackDeliveryByID(t.Context(), items[1].ID)
	require.NoError(t, err)
	require.Equal(t, domain.SlackDeliverySent, sent.State)
	require.Equal(t, new("D1"), sent.ChannelID)
	require.Equal(t, new("123.2"), sent.MessageTS)
	require.True(t, sent.SentAt.Equal(october))
	require.Nil(t, sent.LastError)
	require.Nil(t, sent.ClaimedUntil)
	require.NoError(t, db.SkipSlackDelivery(t.Context(), -1, "missing", october))
	_, err = db.SlackDeliveryByID(t.Context(), -1)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSlackDeliveryStatsAndLists(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	send(t, db, members[0], members[1], 10, october)
	send(t, db, members[0], members[1], 10, october)
	send(t, db, members[0], members[1], 10, october)
	items, err := db.ListSlackDeliveries(t.Context(), "", 10)
	require.NoError(t, err)
	require.Len(t, items, 6)
	old := october.Add(-24*time.Hour - time.Second)
	require.NoError(t, db.MarkSlackDeliverySent(t.Context(), items[0].ID, "C1", "1", october))
	require.NoError(t, db.MarkSlackDeliverySent(t.Context(), items[1].ID, "C1", "2", old))
	for range domain.SlackMaxDeliveryAttempts {
		require.NoError(t, db.RetrySlackDelivery(t.Context(), items[2].ID, october, "failed"))
		require.NoError(t, db.RetrySlackDelivery(t.Context(), items[3].ID, old, "failed"))
	}
	require.NoError(t, db.SkipSlackDelivery(t.Context(), items[4].ID, "disabled", october))
	stats, err := db.SlackDeliveryStats(t.Context(), october)
	require.NoError(t, err)
	require.Equal(t, domain.SlackDeliveryStats{Pending: 1, Failed24h: 1, Sent24h: 1}, stats)
	for _, state := range []string{"pending", "sent", "failed", "skipped"} {
		filtered, err := db.ListSlackDeliveries(t.Context(), state, 1)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		require.Equal(t, state, string(filtered[0].State))
	}
	limited, err := db.ListSlackDeliveries(t.Context(), "", 2)
	require.NoError(t, err)
	require.Equal(t, []int64{items[0].ID, items[1].ID}, []int64{limited[0].ID, limited[1].ID})
}

func TestSlackSettingsAndWorkspaceRefix(t *testing.T) {
	db, members := fixture(t)
	initial, err := db.SlackIntegration(t.Context())
	require.NoError(t, err)
	require.Nil(t, initial.Workspace)
	require.Nil(t, initial.AnnounceChannel)
	require.False(t, initial.AnnouncePosts)
	require.True(t, initial.DMRecipientOnPost)
	require.False(t, initial.GiveFromSlack)
	require.Equal(t, domain.SlackLocaleJA, initial.Locale)
	connectSlack(t, db)
	require.NoError(t, db.SetMemberSlackUser(t.Context(), members[0].ID, new("U0"), october))
	require.NoError(t, db.SetMemberSlackUser(t.Context(), members[1].ID, nil, october))
	require.NoError(t, db.SetSlackLastError(t.Context(), new("team_changed"), october))
	withError, err := db.SlackIntegration(t.Context())
	require.NoError(t, err)
	require.Equal(t, new("team_changed"), withError.LastErrorCode)
	require.True(t, withError.LastErrorAt.Equal(october))
	require.NoError(t, db.SetSlackLastError(t.Context(), nil, october))
	cleared, err := db.SlackIntegration(t.Context())
	require.NoError(t, err)
	require.Nil(t, cleared.LastErrorCode)
	require.Nil(t, cleared.LastErrorAt)
	ws := domain.SlackWorkspace{TeamID: "T1", TeamName: "Renamed", BotUserID: "UBOT2", EnterpriseID: "E1", VerifiedBy: members[0].ID}
	later := october.Add(time.Hour)
	refreshed, err := db.FixSlackWorkspace(t.Context(), ws, true, later)
	require.NoError(t, err)
	require.Equal(t, ws.TeamName, refreshed.Workspace.TeamName)
	require.Equal(t, ws.BotUserID, refreshed.Workspace.BotUserID)
	require.Equal(t, ws.EnterpriseID, refreshed.Workspace.EnterpriseID)
	require.Equal(t, members[0].ID, refreshed.Workspace.VerifiedBy)
	require.True(t, refreshed.Workspace.VerifiedAt.Equal(later))
	require.NotNil(t, refreshed.AnnounceChannel)
	member, err := db.MemberByID(t.Context(), members[0].ID)
	require.NoError(t, err)
	require.Equal(t, new("U0"), member.SlackUserID, "same-team verification preserves the cache even with force")
	ws.TeamID = "T2"
	_, err = db.FixSlackWorkspace(t.Context(), ws, false, later)
	require.ErrorIs(t, err, domain.ErrSlackTeamChanged)
	unchanged, err := db.SlackIntegration(t.Context())
	require.NoError(t, err)
	require.Equal(t, refreshed, unchanged)
	_, err = db.FixSlackWorkspace(t.Context(), ws, true, later)
	require.NoError(t, err)
	for _, original := range members {
		member, err := db.MemberByID(t.Context(), original.ID)
		require.NoError(t, err)
		require.Nil(t, member.SlackUserID)
		require.Nil(t, member.SlackLookupAt)
	}
	forced, err := db.SlackIntegration(t.Context())
	require.NoError(t, err)
	require.Equal(t, "T2", forced.Workspace.TeamID)
	require.Nil(t, forced.AnnounceChannel)
	require.Nil(t, forced.LastErrorCode)
	channel := &domain.SlackChannel{ID: "C2", Name: "new"}
	_, err = db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{AnnounceChannel: &channel, DMRecipientOnPost: new(false), GiveFromSlack: new(true), Locale: new(domain.SlackLocaleEN)}, later)
	require.NoError(t, err)
	settings, err := db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{AnnouncePosts: new(false)}, later)
	require.NoError(t, err)
	require.Equal(t, channel, settings.AnnounceChannel)
	require.False(t, settings.AnnouncePosts)
	require.False(t, settings.DMRecipientOnPost)
	require.True(t, settings.GiveFromSlack)
	require.Equal(t, domain.SlackLocaleEN, settings.Locale)
	channel = nil
	settings, err = db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{AnnounceChannel: &channel}, later)
	require.NoError(t, err)
	require.Nil(t, settings.AnnounceChannel)
}

func TestSlackMemberLookupAndSearch(t *testing.T) {
	db, members := fixture(t)
	require.NoError(t, db.SetMemberSlackUser(t.Context(), members[0].ID, new("U0"), october))
	found, err := db.MemberBySlackUser(t.Context(), "U0")
	require.NoError(t, err)
	require.Equal(t, members[0].ID, found.ID)
	require.Equal(t, new("U0"), found.SlackUserID)
	require.True(t, found.SlackLookupAt.Equal(october))
	_, err = db.MemberBySlackUser(t.Context(), "missing")
	require.ErrorIs(t, err, domain.ErrNotFound)
	pending, err := db.UpsertMember(t.Context(), MemberInput{PersonID: "pending", DisplayName: "Pending", Email: "MEMBER0@EXAMPLE.TEST"}, october, false)
	require.NoError(t, err)
	found, err = db.MemberByEmail(t.Context(), "MeMbEr0@eXaMpLe.TeSt")
	require.NoError(t, err)
	require.Equal(t, members[0].ID, found.ID, "linked member wins over a pending email match")
	require.False(t, found.Pending)
	require.Equal(t, new("U0"), found.SlackUserID)
	_, err = db.MemberByEmail(t.Context(), "missing@example.test")
	require.ErrorIs(t, err, domain.ErrNotFound)
	search, err := db.SearchMembers(t.Context(), "mEMber", members[0].ID, 20)
	require.NoError(t, err)
	require.Len(t, search, 3)
	require.Equal(t, pending.ID, search[2].ID, "pending members are searchable by email")
	search, err = db.SearchMembers(t.Context(), "MEMBER1@", "", 20)
	require.NoError(t, err)
	require.Len(t, search, 1)
	require.Equal(t, members[1].ID, search[0].ID)
	search, err = db.SearchMembers(t.Context(), "Member", "", 1)
	require.NoError(t, err)
	require.Len(t, search, 1)
	for _, prefix := range []string{"%", "_", `\\`, "ember", "example.test"} {
		search, err = db.SearchMembers(t.Context(), prefix, "", 20)
		require.NoError(t, err)
		require.Empty(t, search, "search is a literal prefix, not a wildcard or substring: %s", prefix)
	}
	require.NoError(t, db.SetMemberSlackUser(t.Context(), members[0].ID, nil, november))
	found, err = db.MemberByID(t.Context(), members[0].ID)
	require.NoError(t, err)
	require.Nil(t, found.SlackUserID)
	require.True(t, found.SlackLookupAt.Equal(november))
	require.NoError(t, db.SetMemberSlackUser(t.Context(), members[1].ID, new("U1"), october))
	send(t, db, members[0], members[1], 10, october)
	recent, err := db.RecentRecipients(t.Context(), members[0].ID, 10)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	require.Equal(t, new("U1"), recent[0].SlackUserID)
	require.True(t, recent[0].SlackLookupAt.Equal(october))
}

func TestGiveRemainingUsesCurrentFrozenPeriodWithoutWriting(t *testing.T) {
	db, members := fixture(t)
	remaining, err := db.GiveRemaining(t.Context(), members[0].ID, october)
	require.NoError(t, err)
	require.Equal(t, 400, remaining)
	require.Zero(t, count(t, db, `SELECT count(*) FROM periods`))
	post := send(t, db, members[0], members[1], 10, october)
	clap(t, db, post, members[2], october)
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{WeeklyAllowance: new(500)}, october, members[0].ID)
	require.NoError(t, err)
	for i, expected := range []int{390, 400, 398} {
		remaining, err := db.GiveRemaining(t.Context(), members[i].ID, october)
		require.NoError(t, err)
		require.Equal(t, expected, remaining)
	}
	remaining, err = db.GiveRemaining(t.Context(), members[0].ID, october.AddDate(0, 0, 7))
	require.NoError(t, err)
	require.Equal(t, 500, remaining)
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM periods`))
	require.NoError(t, db.DeletePost(t.Context(), post.ID, members[0].ID, october))
	remaining, err = db.GiveRemaining(t.Context(), members[0].ID, october)
	require.NoError(t, err)
	require.Equal(t, 400, remaining)
}
