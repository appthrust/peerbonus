package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMemberStoryMonthsValuesAndPartners(t *testing.T) {
	db, m := fixture(t)
	values := []domain.Value{{Key: "a", Label: "A"}, {Key: "z", Label: "Z"}}
	_, err := db.UpdateProgram(t.Context(), domain.ProgramPatch{Values: &values}, october, m[0].ID)
	require.NoError(t, err)
	send(t, db, m[0], m[1], 7, instant("2026-04-30T14:59:59Z"))
	send(t, db, m[0], m[1], 11, instant("2026-04-30T15:00:00Z"))
	post, _, err := db.CreatePost(t.Context(), CreatePostInput{
		SenderID: m[0].ID, RecipientID: m[1].ID, Points: 20, Message: "Values", Values: []string{"a", "z"}, Now: october, IdempotencyKey: "values",
	})
	require.NoError(t, err)
	_, _, err = db.CreatePost(t.Context(), CreatePostInput{
		SenderID: m[2].ID, RecipientID: m[1].ID, Points: 40, Message: "More values", Values: []string{"z"}, Now: october, IdempotencyKey: "more-values",
	})
	require.NoError(t, err)
	clap(t, db, post, m[2], october)
	send(t, db, m[1], m[0], 3, october)
	reversed := send(t, db, m[0], m[1], 100, october)
	require.NoError(t, db.DeletePost(t.Context(), reversed.ID, m[0].ID, october))
	// Historical value keys survive removal from the current Program.
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{Values: &[]domain.Value{}}, october, m[0].ID)
	require.NoError(t, err)
	stats, err := db.MemberStats(t.Context(), m[1].ID, october)
	require.NoError(t, err)
	require.Equal(t, 79, stats.Total.ReceivedPoints)
	require.Equal(t, 4, stats.Total.ReceivedPosts)
	require.Equal(t, 61, stats.Month.ReceivedPoints)
	require.Len(t, stats.ReceivedFrom, 2)
	require.Equal(t, m[2].ID, stats.ReceivedFrom[0].Member.ID)
	require.Equal(t, 40, stats.ReceivedFrom[0].Points)
	require.Equal(t, 38, stats.ReceivedFrom[1].Points)
	require.Equal(t, 3, stats.ReceivedFrom[1].Posts)
	require.Equal(t, []domain.Partner{{Member: stats.SentTo[0].Member, Posts: 1, Points: 3}}, stats.SentTo)
	require.Equal(t, m[0].ID, stats.SentTo[0].Member.ID)
	require.Equal(t, []domain.ValueCount{{Key: "z", Posts: 2, Points: 60}, {Key: "a", Posts: 1, Points: 20}}, stats.Values)
	require.Equal(t, []domain.MonthPoint{
		{Key: "2026-05", ReceivedPoints: 11, ReceivedPosts: 1},
		{Key: "2026-06"}, {Key: "2026-07"}, {Key: "2026-08"}, {Key: "2026-09"},
		{Key: "2026-10", ReceivedPoints: 61, ReceivedPosts: 2, SentPosts: 1},
	}, stats.Months)
	atBoundary, err := db.MemberStats(t.Context(), m[1].ID, instant("2026-10-31T15:00:00Z"))
	require.NoError(t, err)
	require.Equal(t, "2026-06", atBoundary.Months[0].Key)
	require.Equal(t, domain.MonthPoint{Key: "2026-11"}, atBoundary.Months[5])
}

func TestMemberStoryPartnerRankingAndSettledHiddenPosts(t *testing.T) {
	db, m := fixture(t)
	for i, name := range []string{"Zeta", "Alpha", "Beta", "Gamma", "Delta", "Epsilon"} {
		partner, err := db.UpsertMember(t.Context(), MemberInput{PersonID: fmt.Sprintf("partner-%d", i), DisplayName: name}, october, false)
		require.NoError(t, err)
		if i == 0 {
			send(t, db, m[0], partner, 5, october)
			send(t, db, m[0], partner, 5, october)
		} else {
			send(t, db, m[0], partner, 10, october)
		}
	}
	hidden := send(t, db, m[0], m[1], 30, october)
	_, err := db.CloseStatement(t.Context(), "2026-10", november, m[0].ID, false)
	require.NoError(t, err)
	require.NoError(t, db.DeletePost(t.Context(), hidden.ID, m[0].ID, november))
	stats, err := db.MemberStats(t.Context(), m[0].ID, november)
	require.NoError(t, err)
	require.Equal(t, 8, stats.Total.SentPosts)
	require.Len(t, stats.SentTo, 5)
	for i, name := range []string{"Zeta", "Alpha", "Beta", "Delta", "Epsilon"} {
		require.Equal(t, name, stats.SentTo[i].Member.DisplayName)
	}
	insights, err := db.Insights(t.Context(), "2026-10", november)
	require.NoError(t, err)
	require.Equal(t, 8, insights.Posts)
	require.Equal(t, 90, insights.Points)
	require.Equal(t, m[1].ID, insights.TopRecipients[0].Member.ID)
	require.Equal(t, 30, insights.TopRecipients[0].Points)
	activity, err := db.ActivityByMember(t.Context(), "2026-10")
	require.NoError(t, err)
	require.Equal(t, 8, activity[m[0].ID].SentPosts)
	require.Equal(t, 30, activity[m[1].ID].ReceivedPoints)
}

func TestInsightsClipsWeeksAndUsesReceiveLedger(t *testing.T) {
	db, m := fixture(t)
	previous := send(t, db, m[0], m[1], 50, instant("2026-09-30T14:59:59Z"))
	clap(t, db, previous, m[2], instant("2026-09-30T15:00:00Z"))
	post := send(t, db, m[0], m[1], 20, october)
	clap(t, db, post, m[2], october)
	clap(t, db, post, m[2], november)
	reversed := send(t, db, m[0], m[1], 30, october)
	clap(t, db, reversed, m[2], october)
	require.NoError(t, db.DeletePost(t.Context(), reversed.ID, m[0].ID, november))
	insights, err := db.Insights(t.Context(), "2026-10", november)
	require.NoError(t, err)
	require.Equal(t, 1, insights.Posts)
	require.Equal(t, 24, insights.Points)
	require.Equal(t, 2, insights.Claps)
	require.Equal(t, 1, insights.Senders)
	require.Equal(t, 1, insights.Recipients)
	require.Equal(t, 20, insights.TopRecipients[0].Points)
	require.Equal(t, 20, insights.TopSenders[0].Points)
	require.Len(t, insights.Weeks, 5)
	require.Equal(t, "2026-W40", insights.Weeks[0].Key)
	require.Equal(t, 2, insights.Weeks[0].Points)
	require.Zero(t, insights.Weeks[0].Posts)
	require.Equal(t, 22, insights.Weeks[3].Points)
	require.Equal(t, 1, insights.Weeks[3].Posts)
	require.Zero(t, insights.Weeks[4].Points)
	var posts, points, claps int
	for _, week := range insights.Weeks {
		posts += week.Posts
		points += week.Points
		claps += week.Claps
	}
	require.Equal(t, insights.Posts, posts)
	require.Equal(t, insights.Points, points)
	require.Equal(t, insights.Claps, claps)
	activity, err := db.ActivityByMember(t.Context(), "2026-10")
	require.NoError(t, err)
	require.Equal(t, 2, activity[m[2].ID].ClapsGiven)
	require.Equal(t, 22, activity[m[1].ID].ReceivedPoints)
}

func TestMonthWeeksCalendarBoundaries(t *testing.T) {
	for _, tc := range []struct {
		month, zone, first, last string
		count                    int
	}{
		{"2021-02", "Asia/Tokyo", "2021-W05", "2021-W08", 4},
		{"2021-05", "Asia/Tokyo", "2021-W17", "2021-W22", 6},
		{"2021-01", "Asia/Tokyo", "2020-W53", "2021-W04", 5},
		{"2026-03", "America/New_York", "2026-W09", "2026-W14", 6},
	} {
		t.Run(tc.month+tc.zone, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			require.NoError(t, err)
			start, err := time.ParseInLocation("2006-01", tc.month, loc)
			require.NoError(t, err)
			weeks := monthWeeks(start, loc)
			require.Len(t, weeks, tc.count)
			require.Equal(t, tc.first, weeks[0].Key)
			require.Equal(t, tc.last, weeks[len(weeks)-1].Key)
			require.False(t, weeks[0].StartsAt.After(start))
			require.False(t, weeks[len(weeks)-1].EndsAt.Before(start.AddDate(0, 1, 0)))
			for i, week := range weeks {
				require.Equal(t, time.Monday, week.StartsAt.Weekday())
				require.Zero(t, week.StartsAt.Hour())
				require.True(t, week.StartsAt.AddDate(0, 0, 7).Equal(week.EndsAt))
				if i > 0 {
					require.True(t, weeks[i-1].EndsAt.Equal(week.StartsAt))
				}
				require.Zero(t, week.Posts+week.Points+week.Claps)
			}
		})
	}
}
