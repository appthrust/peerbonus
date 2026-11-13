package store

import (
	"sync"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestSlackConversationPostsReplayAndPermalink(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	in := CreateSlackPostsInput{SenderID: members[0].ID, RecipientIDs: []string{members[1].ID, members[2].ID}, Points: 30, Message: "  Thanks!  ", Values: []string{"thanks"}, Now: october,
		Source: domain.SlackGiveSource{ChannelID: "C1", MessageTS: "123.001", Permalink: new("https://example.slack.com/archives/C1/p123001")}}
	posts, replayed, err := db.CreateSlackPosts(t.Context(), in)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Len(t, posts, 2)
	for i, post := range posts {
		require.Equal(t, in.RecipientIDs[i], post.Recipient.ID)
		require.Equal(t, 30, post.Points)
		require.Equal(t, "Thanks!", post.Message)
		require.Equal(t, []string{"thanks"}, post.Values)
		require.Equal(t, in.Source.Permalink, post.SlackPermalink)
		read, err := db.PostByID(t.Context(), post.ID, members[0].ID)
		require.NoError(t, err)
		require.Equal(t, in.Source.Permalink, read.SlackPermalink)
	}
	require.Equal(t, 4, count(t, db, `SELECT count(*) FROM ledger`))
	require.Equal(t, 4, count(t, db, `SELECT count(*) FROM slack_deliveries`))
	require.Equal(t, 2, count(t, db, `SELECT count(*) FROM slack_gives`))
	require.Zero(t, count(t, db, `SELECT count(*) FROM idempotency_keys`))
	listed, _, err := db.ListPosts(t.Context(), PostFilter{}, members[0].ID)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	for _, post := range listed {
		require.Equal(t, in.Source.Permalink, post.SlackPermalink)
	}
	inbox, _, _, err := db.ListInbox(t.Context(), members[1].ID, 20, "")
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	require.Equal(t, in.Source.Permalink, inbox[0].Post.SlackPermalink)

	in.Now = october.Add(48 * time.Hour)
	in.Points, in.Message = -1, ""
	same, replayed, err := db.CreateSlackPosts(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, posts, same)
	in.RecipientIDs = []string{members[1].ID}
	same, replayed, err = db.CreateSlackPosts(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, posts, same, "a subset must replay the entire original give")
	in.RecipientIDs = []string{"another-recipient"}
	same, replayed, err = db.CreateSlackPosts(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, posts, same, "changed recipients must never add another give")
	in.Source.Emoji = "pb-30"
	same, replayed, err = db.CreateSlackPosts(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, posts, same, "a different emoji must replay the entire original give")
	require.Equal(t, 4, count(t, db, `SELECT count(*) FROM ledger`))
	require.Equal(t, 4, count(t, db, `SELECT count(*) FROM slack_deliveries`))
}

func TestSlackConversationPostsAtomicValidation(t *testing.T) {
	for _, reason := range []string{"balance", "missing recipient", "self", "duplicate", "empty", "too many", "points", "message"} {
		t.Run(reason, func(t *testing.T) {
			db, members := fixture(t)
			connectSlack(t, db)
			_, err := db.UpdateProgram(t.Context(), domain.ProgramPatch{WeeklyAllowance: new(120)}, october, members[0].ID)
			require.NoError(t, err)
			in := CreateSlackPostsInput{SenderID: members[0].ID, RecipientIDs: []string{members[1].ID, members[2].ID}, Points: 30, Message: "Thanks", Now: october, Source: domain.SlackGiveSource{ChannelID: "C1", MessageTS: "123.001"}}
			switch reason {
			case "balance":
				in.Points = 100
			case "missing recipient":
				in.RecipientIDs[1] = "missing"
			case "self":
				in.RecipientIDs[1] = members[0].ID
			case "duplicate":
				in.RecipientIDs[1] = members[1].ID
			case "empty":
				in.RecipientIDs = nil
			case "too many":
				in.RecipientIDs = []string{"a", "b", "c", "d", "e", "f"}
			case "points":
				in.Points = 121
			case "message":
				in.Message = " "
			}
			_, replayed, err := db.CreateSlackPosts(t.Context(), in)
			require.Error(t, err)
			require.False(t, replayed)
			switch reason {
			case "balance":
				var insufficient *domain.InsufficientPointsError
				require.ErrorAs(t, err, &insufficient)
				require.Equal(t, 120, insufficient.Remaining)
				require.Equal(t, 200, insufficient.Required)
			case "missing recipient":
				require.ErrorIs(t, err, domain.ErrRecipientNotFound)
			case "self":
				require.ErrorIs(t, err, domain.ErrSelfTarget)
			default:
				var invalid *domain.ValidationError
				require.ErrorAs(t, err, &invalid)
			}
			for _, table := range []string{"posts", "ledger", "periods", "slack_gives", "slack_deliveries", "idempotency_keys"} {
				require.Zero(t, count(t, db, `SELECT count(*) FROM `+table), table)
			}
		})
	}
}

func TestSlackConversationPendingLifecycle(t *testing.T) {
	db, members := fixture(t)
	in := domain.SlackPendingGive{ChannelID: "C1", MessageTS: "123.001", ThreadTS: new("123.000"), SenderID: members[0].ID, RecipientIDs: []string{members[1].ID, members[2].ID}, Message: "Thanks"}
	pending, created, err := db.CreateSlackPendingGive(t.Context(), in, october)
	require.NoError(t, err)
	require.True(t, created)
	_, err = ulid.ParseStrict(pending.ID)
	require.NoError(t, err)
	require.True(t, pending.ExpiresAt.Equal(october.Add(domain.SlackPendingGiveTTL)))
	read, err := db.SlackPendingGive(t.Context(), pending.ID, october)
	require.NoError(t, err)
	require.Equal(t, pending, read)
	in.Message = "Changed"
	same, created, err := db.CreateSlackPendingGive(t.Context(), in, october.Add(time.Hour))
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, pending, same)
	require.ErrorIs(t, db.DeleteSlackPendingGive(t.Context(), pending.ID, members[1].ID), domain.ErrSlackPendingGiveGone)
	_, err = db.SlackPendingGive(t.Context(), pending.ID, pending.ExpiresAt)
	require.ErrorIs(t, err, domain.ErrSlackPendingGiveGone)
	in.MessageTS = "124.001"
	_, created, err = db.CreateSlackPendingGive(t.Context(), in, pending.ExpiresAt)
	require.NoError(t, err)
	require.True(t, created)
	require.Zero(t, count(t, db, `SELECT count(*) FROM slack_pending_gives WHERE id=$1`, pending.ID), "creation prunes expired rows")
	require.ErrorIs(t, db.DeleteSlackPendingGive(t.Context(), pending.ID, members[0].ID), domain.ErrSlackPendingGiveGone)
	fresh, created, err := db.CreateSlackPendingGive(t.Context(), domain.SlackPendingGive{ChannelID: "C1", MessageTS: "125.001", SenderID: members[0].ID, RecipientIDs: in.RecipientIDs, Message: "Cancel"}, pending.ExpiresAt)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, db.DeleteSlackPendingGive(t.Context(), fresh.ID, members[0].ID))
	require.ErrorIs(t, db.DeleteSlackPendingGive(t.Context(), fresh.ID, members[0].ID), domain.ErrSlackPendingGiveGone)
}

func TestSlackConversationPendingConsumption(t *testing.T) {
	db, members := fixture(t)
	pending, _, err := db.CreateSlackPendingGive(t.Context(), domain.SlackPendingGive{ChannelID: "C1", MessageTS: "123.001", SenderID: members[0].ID, RecipientIDs: []string{members[1].ID, members[2].ID}, Message: "Thanks"}, october)
	require.NoError(t, err)
	in := CreateSlackPostsInput{SenderID: members[0].ID, RecipientIDs: pending.RecipientIDs, Points: 30, Message: pending.Message, Now: october, PendingID: pending.ID, Source: domain.SlackGiveSource{ChannelID: pending.ChannelID, MessageTS: pending.MessageTS}}
	wrongSender := in
	wrongSender.SenderID = members[1].ID
	_, _, err = db.CreateSlackPosts(t.Context(), wrongSender)
	require.ErrorIs(t, err, domain.ErrSlackPendingGiveGone)
	expired := in
	expired.Now = pending.ExpiresAt
	_, _, err = db.CreateSlackPosts(t.Context(), expired)
	require.ErrorIs(t, err, domain.ErrSlackPendingGiveGone)
	invalid := in
	invalid.Points = 121
	_, _, err = db.CreateSlackPosts(t.Context(), invalid)
	require.Error(t, err)
	_, err = db.SlackPendingGive(t.Context(), pending.ID, october)
	require.NoError(t, err, "failed give rolls back consumption")
	require.Zero(t, count(t, db, `SELECT count(*) FROM ledger`))

	type result struct {
		posts    []domain.Post
		replayed bool
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			posts, replayed, err := db.CreateSlackPosts(t.Context(), in)
			results <- result{posts, replayed, err}
		})
	}
	wg.Wait()
	close(results)
	replays := 0
	var posts []domain.Post
	for result := range results {
		require.NoError(t, result.err)
		if posts == nil {
			posts = result.posts
		} else {
			require.Equal(t, posts, result.posts)
		}
		if result.replayed {
			replays++
		}
	}
	require.Equal(t, 1, replays)
	require.Zero(t, count(t, db, `SELECT count(*) FROM slack_pending_gives`))
	require.Equal(t, 4, count(t, db, `SELECT count(*) FROM ledger`))
	_, err = db.SlackPendingGive(t.Context(), pending.ID, october)
	require.ErrorIs(t, err, domain.ErrSlackPendingGiveGone)
	in.Source.MessageTS = "another-source"
	_, _, err = db.CreateSlackPosts(t.Context(), in)
	require.ErrorIs(t, err, domain.ErrSlackPendingGiveGone, "a used pending give cannot authorize another source")
	in.PendingID = "missing"
	_, _, err = db.CreateSlackPosts(t.Context(), in)
	require.ErrorIs(t, err, domain.ErrSlackPendingGiveGone)
}

func TestSlackConversationSettingsAndRemovedValue(t *testing.T) {
	db, members := fixture(t)
	initial, err := db.SlackIntegration(t.Context())
	require.NoError(t, err)
	require.False(t, initial.GiveInConversation)
	require.Equal(t, []int{10, 30, 50}, initial.MentionPointChoices)
	require.Equal(t, []domain.SlackReactionRule{}, initial.ReactionRules)
	rules := []domain.SlackReactionRule{{Emoji: "z-thanks", Points: 30, ValueKey: new("thanks")}, {Emoji: "a-care", Points: 10}}
	choices := []int{50, 10, 30}
	settings, err := db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{GiveInConversation: new(true), MentionPointChoices: &choices, ReactionRules: &rules}, october)
	require.NoError(t, err)
	require.True(t, settings.GiveInConversation)
	require.Equal(t, choices, settings.MentionPointChoices)
	require.Equal(t, rules, settings.ReactionRules)
	loaded, err := db.SlackIntegration(t.Context())
	require.NoError(t, err)
	require.Equal(t, settings, loaded)
	unchanged, err := db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{}, october)
	require.NoError(t, err)
	require.Equal(t, settings, unchanged)

	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{Values: new([]domain.Value{{Key: "teamwork", Label: "Teamwork"}})}, october, members[0].ID)
	require.NoError(t, err)
	posts, _, err := db.CreateSlackPosts(t.Context(), CreateSlackPostsInput{SenderID: members[0].ID, RecipientIDs: []string{members[1].ID}, Points: 30, Message: "Reaction", Values: []string{*rules[0].ValueKey, "teamwork"}, Now: october, Source: domain.SlackGiveSource{ChannelID: "C1", MessageTS: "123.001", Emoji: rules[0].Emoji}})
	require.NoError(t, err)
	require.Equal(t, []string{"teamwork"}, posts[0].Values)
	_, _, err = db.CreatePost(t.Context(), CreatePostInput{SenderID: members[0].ID, RecipientID: members[1].ID, Points: 30, Message: "Web remains strict", Values: []string{"thanks"}, Now: october, IdempotencyKey: "web"})
	require.ErrorIs(t, err, domain.ErrUnknownValue)
	cleared, err := db.UpdateSlackSettings(t.Context(), SlackSettingsPatch{ReactionRules: new([]domain.SlackReactionRule{}), GiveInConversation: new(false)}, october)
	require.NoError(t, err)
	require.Empty(t, cleared.ReactionRules)
	require.False(t, cleared.GiveInConversation)
	require.Equal(t, choices, cleared.MentionPointChoices)
}

func TestSlackConversationLinkAndWorkspaceChange(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	require.NoError(t, db.SetMemberSlackUser(t.Context(), members[0].ID, new("U1"), october))
	later := october.Add(time.Hour)
	require.NoError(t, db.LinkMemberSlackUser(t.Context(), members[1].ID, "U1", later))
	old, err := db.MemberByID(t.Context(), members[0].ID)
	require.NoError(t, err)
	require.Nil(t, old.SlackUserID)
	require.Nil(t, old.SlackLookupAt)
	linked, err := db.MemberBySlackUser(t.Context(), "U1")
	require.NoError(t, err)
	require.Equal(t, members[1].ID, linked.ID)
	require.True(t, linked.SlackLookupAt.Equal(later))
	require.ErrorIs(t, db.LinkMemberSlackUser(t.Context(), "missing", "U1", later), domain.ErrNotFound)
	linked, err = db.MemberBySlackUser(t.Context(), "U1")
	require.NoError(t, err)
	require.Equal(t, members[1].ID, linked.ID, "a missing target must not clear an existing link")
	pending, _, err := db.CreateSlackPendingGive(t.Context(), domain.SlackPendingGive{ChannelID: "C1", MessageTS: "123.001", SenderID: members[0].ID, RecipientIDs: []string{members[1].ID}, Message: "Thanks"}, october)
	require.NoError(t, err)
	_, err = db.FixSlackWorkspace(t.Context(), domain.SlackWorkspace{TeamID: "T1", BotUserID: "UBOT"}, true, later)
	require.NoError(t, err)
	_, err = db.SlackPendingGive(t.Context(), pending.ID, later)
	require.NoError(t, err)
	_, err = db.FixSlackWorkspace(t.Context(), domain.SlackWorkspace{TeamID: "T2", BotUserID: "UBOT2"}, false, later)
	require.ErrorIs(t, err, domain.ErrSlackTeamChanged)
	_, err = db.SlackPendingGive(t.Context(), pending.ID, later)
	require.NoError(t, err)
	_, err = db.FixSlackWorkspace(t.Context(), domain.SlackWorkspace{TeamID: "T2", BotUserID: "UBOT2"}, true, later)
	require.NoError(t, err)
	_, err = db.SlackPendingGive(t.Context(), pending.ID, later)
	require.ErrorIs(t, err, domain.ErrSlackPendingGiveGone)
}

func TestSlackConversationClapAndAnnounce(t *testing.T) {
	db, members := fixture(t)
	connectSlack(t, db)
	post := send(t, db, members[0], members[1], 30, october)
	_, err := db.PostIDBySlackAnnounce(t.Context(), "C1", "123.001")
	require.ErrorIs(t, err, domain.ErrNotFound)
	deliveries, err := db.ClaimSlackDeliveries(t.Context(), october, 10, time.Minute)
	require.NoError(t, err)
	for _, delivery := range deliveries {
		if delivery.Kind == domain.SlackDeliveryPostAnnounce {
			require.NoError(t, db.MarkSlackDeliverySent(t.Context(), delivery.ID, "C1", "123.001", october))
		} else {
			require.NoError(t, db.MarkSlackDeliverySent(t.Context(), delivery.ID, "D1", "124.001", october))
		}
	}
	id, err := db.PostIDBySlackAnnounce(t.Context(), "C1", "123.001")
	require.NoError(t, err)
	require.Equal(t, post.ID, id)
	_, err = db.PostIDBySlackAnnounce(t.Context(), "D1", "124.001")
	require.ErrorIs(t, err, domain.ErrNotFound)
	in := CreateSlackClapInput{PostID: post.ID, MemberID: members[2].ID, Now: october, Source: domain.SlackGiveSource{ChannelID: "C1", MessageTS: "123.001", Emoji: "pb-10", Permalink: new("https://example.slack.com/clap")}}
	first, updated, replayed, err := db.CreateSlackClap(t.Context(), in)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, 1, updated.ClapCount)
	require.Nil(t, updated.SlackPermalink, "clap source is not the original give's permalink")
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM slack_gives WHERE clap_id=$1 AND recipient_id=$2`, first.ID, members[1].ID))
	require.Equal(t, 5, count(t, db, `SELECT count(*) FROM ledger`))
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM idempotency_keys`), "only the web post has an idempotency key")
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE kind='clap_update'`))
	in.Now = october.Add(48 * time.Hour)
	same, updated, replayed, err := db.CreateSlackClap(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, first.ID, same.ID)
	require.Equal(t, 1, updated.ClapCount)
	require.Equal(t, 5, count(t, db, `SELECT count(*) FROM ledger`))
	in.Source.Emoji = "pb-30"
	same, updated, replayed, err = db.CreateSlackClap(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, first.ID, same.ID)
	require.Equal(t, 1, updated.ClapCount)
	require.Equal(t, 5, count(t, db, `SELECT count(*) FROM ledger`))
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM slack_gives`))
	require.Equal(t, 1, count(t, db, `SELECT count(*) FROM slack_deliveries WHERE kind='clap_update'`))
	in.MemberID = members[0].ID
	_, _, _, err = db.CreateSlackClap(t.Context(), in)
	require.ErrorIs(t, err, domain.ErrOwnPost)
	in.MemberID = members[2].ID
	require.NoError(t, db.DeletePost(t.Context(), post.ID, members[0].ID, october))
	_, _, replayed, err = db.CreateSlackClap(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	in.Source.Emoji = "pb-10"
	same, _, replayed, err = db.CreateSlackClap(t.Context(), in)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, first.ID, same.ID)
}
