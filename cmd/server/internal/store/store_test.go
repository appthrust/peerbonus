package store

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/appthrust/appserver-go/appserver/pgstore/testutil"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func instant(s string) time.Time {
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return at
}

var october = instant("2026-10-19T10:00:00+09:00")
var november = instant("2026-11-01T00:00:00+09:00")

func fixture(t *testing.T) (*Store, []domain.Member) {
	t.Helper()
	db, _ := testutil.Database(t, Open)
	members := make([]domain.Member, 3)
	for i := range members {
		member, err := db.UpsertMember(t.Context(), MemberInput{PersonID: fmt.Sprintf("person-%d", i), Subject: new(fmt.Sprintf("subject-%d", i)), DisplayName: fmt.Sprintf("Member %d", i), Email: fmt.Sprintf("member%d@example.test", i)}, october, true)
		require.NoError(t, err)
		members[i] = member
	}
	return db, members
}
func send(t *testing.T, db *Store, sender, recipient domain.Member, points int, at time.Time) domain.Post {
	t.Helper()
	post, replayed, err := db.CreatePost(t.Context(), CreatePostInput{SenderID: sender.ID, RecipientID: recipient.ID, Points: points, Message: "  Thank you!  ", Now: at, IdempotencyKey: ulid.Make().String()})
	require.NoError(t, err)
	require.False(t, replayed)
	return post
}
func clap(t *testing.T, db *Store, post domain.Post, member domain.Member, at time.Time) domain.Clap {
	t.Helper()
	result, _, replayed, err := db.CreateClap(t.Context(), CreateClapInput{PostID: post.ID, MemberID: member.ID, Now: at, IdempotencyKey: ulid.Make().String()})
	require.NoError(t, err)
	require.False(t, replayed)
	return result
}
func count(t *testing.T, db *Store, query string, args ...any) int {
	t.Helper()
	var out int
	require.NoError(t, db.Do(t.Context(), false, func(tx *Tx) error { return tx.SQL.QueryRowContext(t.Context(), query, args...).Scan(&out) }))
	return out
}
func TestSendRemainingAndInsufficient(t *testing.T) {
	db, m := fixture(t)
	program, err := db.Program(t.Context())
	require.NoError(t, err)
	require.Equal(t, "Asia/Tokyo", program.TimeZone)
	require.Equal(t, 400, program.WeeklyAllowance)
	require.Equal(t, 120, program.MaxPointsPerPost)
	require.Equal(t, "1", program.PointUnitValue)
	require.Equal(t, "JPY", program.Currency)
	require.Equal(t, []domain.Value{
		{Key: "thanks", Label: "感謝", Description: new("助けてもらった、支えてもらった")},
		{Key: "challenge", Label: "挑戦", Description: new("新しいことに踏み出した")},
		{Key: "teamwork", Label: "協力", Description: new("チームの壁を越えて動いた")},
		{Key: "learning", Label: "学び", Description: new("知識や経験を分けてくれた")},
		{Key: "customer", Label: "顧客志向", Description: new("お客様の価値を最優先した")},
	}, program.Values)
	require.False(t, program.TimeZoneLocked)
	post := send(t, db, m[0], m[1], 120, october)
	require.Equal(t, "Thank you!", post.Message)
	require.Equal(t, "2026-W43", post.PeriodKey)
	require.Equal(t, "2026-10", post.MonthKey)
	require.True(t, post.CreatedAt.Equal(october))
	require.Equal(t, 2, count(t, db, `SELECT COUNT(*) FROM ledger`))
	stats, err := db.MyStats(t.Context(), m[0].ID, october)
	require.NoError(t, err)
	require.Equal(t, 280, stats.Period.Remaining)
	require.Equal(t, 120, stats.Period.SentPoints)
	require.Equal(t, 1, stats.Period.SentPosts)
	received, err := db.MemberStats(t.Context(), m[1].ID, october)
	require.NoError(t, err)
	require.Equal(t, 120, received.Month.ReceivedPoints)
	require.Equal(t, 1, received.Total.ReceivedPosts)
	send(t, db, m[0], m[1], 120, october)
	send(t, db, m[0], m[1], 120, october)
	_, _, err = db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 41, Message: "Too much", Now: october, IdempotencyKey: "insufficient"})
	var insufficient *domain.InsufficientPointsError
	require.ErrorAs(t, err, &insufficient)
	require.Equal(t, 40, insufficient.Remaining)
	require.Equal(t, 41, insufficient.Required)
	require.Equal(t, 6, count(t, db, `SELECT COUNT(*) FROM ledger`))
	next, err := db.MyStats(t.Context(), m[0].ID, october.AddDate(0, 0, 7))
	require.NoError(t, err)
	require.Equal(t, 400, next.Period.Remaining)
	_, _, err = db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: m[0].ID, Points: 1, Message: "Self", Now: october, IdempotencyKey: "self"})
	require.ErrorIs(t, err, domain.ErrSelfTarget)
	_, _, err = db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: "missing", Points: 1, Message: "Missing", Now: october, IdempotencyKey: "missing"})
	require.ErrorIs(t, err, domain.ErrRecipientNotFound)
}
func TestClapLedgerCountsOwnPostAndReplay(t *testing.T) {
	db, m := fixture(t)
	input := CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 50, Message: "Thanks", Now: october, IdempotencyKey: "send-key"}
	post, replayed, err := db.CreatePost(t.Context(), input)
	require.NoError(t, err)
	require.False(t, replayed)
	for _, member := range m[:2] {
		_, _, _, err := db.CreateClap(t.Context(), CreateClapInput{PostID: post.ID, MemberID: member.ID, Now: october, IdempotencyKey: "own"})
		require.ErrorIs(t, err, domain.ErrOwnPost)
	}
	ci := CreateClapInput{PostID: post.ID, MemberID: m[2].ID, Now: october, IdempotencyKey: strings.Repeat("あ", 128)}
	first, current, replayed, err := db.CreateClap(t.Context(), ci)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, 1, current.ClapCount)
	require.Equal(t, 2, current.ClapPoints)
	require.Equal(t, 1, current.ClappedByMe)
	require.Equal(t, 5, count(t, db, `SELECT COUNT(*) FROM ledger`))
	require.Equal(t, 0, count(t, db, `SELECT SUM(delta) FROM ledger WHERE clap_id=$1`, first.ID))
	clapper, err := db.MyStats(t.Context(), m[2].ID, october)
	require.NoError(t, err)
	require.Equal(t, 398, clapper.Period.Remaining)
	require.Equal(t, 1, clapper.Period.Claps)
	sender, err := db.MemberStats(t.Context(), m[0].ID, october)
	require.NoError(t, err)
	require.Equal(t, 1, sender.Total.ReceivedPoints)
	recipient, err := db.MemberStats(t.Context(), m[1].ID, october)
	require.NoError(t, err)
	require.Equal(t, 51, recipient.Total.ReceivedPoints)
	clap(t, db, post, m[2], october)
	second, current, replayed, err := db.CreateClap(t.Context(), ci)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, 2, current.ClapCount)
	require.Equal(t, 2, current.ClappedByMe)
	require.Equal(t, 8, count(t, db, `SELECT COUNT(*) FROM ledger`))
	input.Points, input.Message = -1, ""
	same, replayed, err := db.CreatePost(t.Context(), input)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, post.ID, same.ID)
	require.Equal(t, 2, same.ClapCount)
	require.Equal(t, 0, same.ClappedByMe)
	listed, err := db.ListClaps(t.Context(), post.ID, 50)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	require.NoError(t, db.DeletePost(t.Context(), post.ID, m[0].ID, october))
	_, _, _, err = db.CreateClap(t.Context(), CreateClapInput{PostID: post.ID, MemberID: m[2].ID, Now: october, IdempotencyKey: "new-after-deletion"})
	require.ErrorIs(t, err, domain.ErrNotFound)
	same, replayed, err = db.CreatePost(t.Context(), input)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, post.ID, same.ID)
}
func TestDeleteReversesEveryOriginalPeriodAndMonth(t *testing.T) {
	db, m := fixture(t)
	post := send(t, db, m[0], m[1], 100, october)
	clap(t, db, post, m[2], november)
	require.NoError(t, db.DeletePost(t.Context(), post.ID, m[0].ID, november))
	require.Equal(t, 10, count(t, db, `SELECT COUNT(*) FROM ledger`))
	require.Equal(t, 5, count(t, db, `SELECT COUNT(*) FROM ledger WHERE kind='reversal'`))
	require.Equal(t, 0, count(t, db, `SELECT COUNT(*) FROM ledger r JOIN ledger original ON original.id=r.reverses_id WHERE r.delta<>-original.delta OR r.month_key<>original.month_key OR r.period_key<>original.period_key`))
	require.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM posts WHERE id=$1 AND reversed AND deleted_by=$2`, post.ID, m[0].ID))
	require.Equal(t, 0, count(t, db, `SELECT SUM(delta) FROM ledger WHERE account='receive'`))
	sender, err := db.MyStats(t.Context(), m[0].ID, october)
	require.NoError(t, err)
	require.Equal(t, 400, sender.Period.Remaining)
	require.Zero(t, sender.Period.SentPosts)
	clapper, err := db.MyStats(t.Context(), m[2].ID, november)
	require.NoError(t, err)
	require.Equal(t, 400, clapper.Period.Remaining)
	require.Zero(t, clapper.Period.Claps)
	require.ErrorIs(t, db.DeletePost(t.Context(), post.ID, m[0].ID, november), domain.ErrNotFound)
	_, err = db.PostByID(t.Context(), post.ID, m[0].ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	posts, _, err := db.ListPosts(t.Context(), PostFilter{}, m[0].ID)
	require.NoError(t, err)
	require.Empty(t, posts)
}
func TestClosedMonthHidesWithoutReversalAndRefusesClaps(t *testing.T) {
	db, m := fixture(t)
	post := send(t, db, m[0], m[1], 100, october)
	clap(t, db, post, m[2], november)
	closed, err := db.CloseStatement(t.Context(), "2026-10", november, m[0].ID, true)
	require.NoError(t, err)
	require.Equal(t, 100, closed.TotalPoints)
	_, _, _, err = db.CreateClap(t.Context(), CreateClapInput{PostID: post.ID, MemberID: m[2].ID, Now: november, IdempotencyKey: "closed-post"})
	require.ErrorIs(t, err, domain.ErrPeriodClosed)
	_, _, err = db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 1, Message: "Closed", Now: october, IdempotencyKey: "closed-month"})
	require.ErrorIs(t, err, domain.ErrPeriodClosed)
	require.NoError(t, db.DeletePost(t.Context(), post.ID, m[0].ID, november))
	require.Equal(t, 5, count(t, db, `SELECT COUNT(*) FROM ledger`))
	require.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM posts WHERE id=$1 AND deleted_at IS NOT NULL AND NOT reversed`, post.ID))
	after, err := db.Statement(t.Context(), "2026-10", november)
	require.NoError(t, err)
	require.Equal(t, closed, after)
	stats, err := db.MemberStats(t.Context(), m[1].ID, november)
	require.NoError(t, err)
	require.Equal(t, 101, stats.Total.ReceivedPoints)
	require.Equal(t, 1, stats.Total.ReceivedPosts)
}
func TestClapRefusesClosedCurrentMonth(t *testing.T) {
	db, m := fixture(t)
	_, err := db.CloseStatement(t.Context(), "2026-11", instant("2026-12-01T00:00:00+09:00"), m[0].ID, false)
	require.NoError(t, err)
	// Backdated inputs exercise the independent current-month guard even when
	// the post month is still open; production callers always supply server now.
	post := send(t, db, m[0], m[1], 10, october)
	_, _, _, err = db.CreateClap(t.Context(), CreateClapInput{PostID: post.ID, MemberID: m[2].ID, Now: november, IdempotencyKey: "closed-current"})
	require.ErrorIs(t, err, domain.ErrPeriodClosed)
	require.Equal(t, 2, count(t, db, `SELECT COUNT(*) FROM ledger`))
}
func TestSequentialCloseAndFrozenExactAmountsAndProfiles(t *testing.T) {
	db, m := fixture(t)
	pending, err := db.UpsertMember(t.Context(), MemberInput{PersonID: "pending", DisplayName: "Before", Email: "before@example.test", Pending: true}, october, false)
	require.NoError(t, err)
	send(t, db, m[0], pending, 3, october)
	send(t, db, m[1], m[2], 7, november)
	_, err = db.CloseStatement(t.Context(), "2026-10", november.Add(-time.Nanosecond), m[0].ID, false)
	require.ErrorIs(t, err, domain.ErrMonthNotEnded)
	december := instant("2026-12-01T00:00:00+09:00")
	_, err = db.CloseStatement(t.Context(), "2026-11", december, m[0].ID, false)
	require.ErrorIs(t, err, domain.ErrPreviousMonthOpen)
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{PointUnitValue: new("0.1234567890123456789"), Currency: new("USD")}, november, m[0].ID)
	require.NoError(t, err)
	preview, err := db.Statement(t.Context(), "2026-10", november)
	require.NoError(t, err)
	require.False(t, preview.Closed)
	require.Equal(t, "0.3703703670370370367", preview.TotalAmount)
	require.False(t, preview.DirectoryRefreshed)
	refreshed, err := db.MembersWithReceiveInMonth(t.Context(), "2026-10")
	require.NoError(t, err)
	require.Len(t, refreshed, 1)
	require.Equal(t, pending.ID, refreshed[0].ID)
	require.NoError(t, db.UpdateMemberProfiles(t.Context(), []MemberInput{{PersonID: pending.PersonID, DisplayName: "At close", Email: "close@example.test", Subject: new("ignored")}}))
	closed, err := db.CloseStatement(t.Context(), "2026-10", november, m[0].ID, true)
	require.NoError(t, err)
	require.True(t, closed.Closed)
	require.True(t, closed.DirectoryRefreshed)
	require.Equal(t, 1, closed.MemberCount)
	require.Equal(t, 3, closed.TotalPoints)
	require.Equal(t, "0.3703703670370370367", closed.TotalAmount)
	require.Equal(t, "At close", closed.Lines[0].Member.DisplayName)
	require.True(t, closed.Lines[0].Member.Pending)
	_, err = db.UpsertMember(t.Context(), MemberInput{PersonID: pending.PersonID, DisplayName: "After", Email: "after@example.test", Subject: new("signed-in")}, november, true)
	require.NoError(t, err)
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{PointUnitValue: new("999"), Currency: new("JPY")}, november, m[0].ID)
	require.NoError(t, err)
	again, err := db.CloseStatement(t.Context(), "2026-10", october, m[1].ID, false)
	require.NoError(t, err)
	require.Equal(t, closed, again)
	_, err = db.CloseStatement(t.Context(), "2026-11", december, m[0].ID, false)
	require.NoError(t, err)
	summary, err := db.ListStatements(t.Context(), december)
	require.NoError(t, err)
	require.Len(t, summary, 3)
	require.Equal(t, "2026-12", summary[0].Month)
	require.False(t, summary[0].Closed)
	require.True(t, summary[1].Closed)
	require.Equal(t, "0.3703703670370370367", summary[2].TotalAmount)
}
func TestTimezoneLockAndEmptyPeriodCleanup(t *testing.T) {
	db, m := fixture(t)
	require.NoError(t, db.Do(t.Context(), true, func(tx *Tx) error {
		return tx.Exec(`INSERT INTO periods (key,starts_at,ends_at,allowance,max_points_per_post) VALUES ('2026-W42','2026-10-12T00:00:00+09:00','2026-10-19T00:00:00+09:00',400,120)`)
	}))
	program, err := db.UpdateProgram(t.Context(), domain.ProgramPatch{TimeZone: new("UTC")}, october, m[0].ID)
	require.NoError(t, err)
	require.Equal(t, "UTC", program.TimeZone)
	require.Equal(t, 0, count(t, db, `SELECT COUNT(*) FROM periods`))
	post := send(t, db, m[0], m[1], 10, october)
	program, err = db.Program(t.Context())
	require.NoError(t, err)
	require.True(t, program.TimeZoneLocked)
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{TimeZone: new("Asia/Tokyo")}, october, m[0].ID)
	require.ErrorIs(t, err, domain.ErrTimezoneLocked)
	require.NoError(t, db.DeletePost(t.Context(), post.ID, m[0].ID, october))
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{TimeZone: new("Asia/Tokyo")}, october, m[0].ID)
	require.ErrorIs(t, err, domain.ErrTimezoneLocked)
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{TimeZone: new("UTC")}, october, m[0].ID)
	require.NoError(t, err)
}
func TestPeriodFreezeSemantics(t *testing.T) {
	db, m := fixture(t)
	period, err := db.CurrentPeriod(t.Context(), october)
	require.NoError(t, err)
	require.False(t, period.Frozen)
	_, err = db.MyStats(t.Context(), m[0].ID, october)
	require.NoError(t, err)
	require.Equal(t, 0, count(t, db, `SELECT COUNT(*) FROM periods`))
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{WeeklyAllowance: new(500), MaxPointsPerPost: new(200)}, october, m[0].ID)
	require.NoError(t, err)
	period, err = db.CurrentPeriod(t.Context(), october)
	require.NoError(t, err)
	require.Equal(t, 500, period.Allowance)
	require.Equal(t, 200, period.MaxPointsPerPost)
	_, _, err = db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 201, Message: "Over limit", Now: october, IdempotencyKey: "invalid"})
	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Equal(t, 0, count(t, db, `SELECT COUNT(*) FROM periods`))
	post := send(t, db, m[0], m[1], 200, october)
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{WeeklyAllowance: new(300), MaxPointsPerPost: new(100)}, october, m[0].ID)
	require.NoError(t, err)
	period, err = db.CurrentPeriod(t.Context(), october)
	require.NoError(t, err)
	require.True(t, period.Frozen)
	require.Equal(t, 500, period.Allowance)
	require.Equal(t, 200, period.MaxPointsPerPost)
	send(t, db, m[0], m[1], 200, october)
	nextWeek := october.AddDate(0, 0, 7)
	period, err = db.CurrentPeriod(t.Context(), nextWeek)
	require.NoError(t, err)
	require.False(t, period.Frozen)
	require.Equal(t, 300, period.Allowance)
	clap(t, db, post, m[2], nextWeek)
	_, err = db.UpdateProgram(t.Context(), domain.ProgramPatch{WeeklyAllowance: new(400), MaxPointsPerPost: new(120)}, nextWeek, m[0].ID)
	require.NoError(t, err)
	period, err = db.CurrentPeriod(t.Context(), nextWeek)
	require.NoError(t, err)
	require.True(t, period.Frozen)
	require.Equal(t, 300, period.Allowance)
	require.Equal(t, 100, period.MaxPointsPerPost)
}
func TestSubjectRelinkPreservesOriginalLedger(t *testing.T) {
	db, m := fixture(t)
	post := send(t, db, m[0], m[1], 25, october)
	replacement, err := db.UpsertMember(t.Context(), MemberInput{PersonID: "new-person", Subject: m[1].Subject, DisplayName: "Replacement", Email: m[1].Email}, october, true)
	require.NoError(t, err)
	require.NotEqual(t, m[1].ID, replacement.ID)
	old, err := db.MemberByID(t.Context(), m[1].ID)
	require.NoError(t, err)
	require.Nil(t, old.Subject)
	require.True(t, old.Pending)
	require.Equal(t, m[1].DisplayName, old.DisplayName)
	current, err := db.MemberBySubject(t.Context(), *m[1].Subject)
	require.NoError(t, err)
	require.Equal(t, replacement.ID, current.ID)
	stats, err := db.MemberStats(t.Context(), old.ID, october)
	require.NoError(t, err)
	require.Equal(t, 25, stats.Total.ReceivedPoints)
	stats, err = db.MemberStats(t.Context(), replacement.ID, october)
	require.NoError(t, err)
	require.Zero(t, stats.Total.ReceivedPoints)
	updated, err := db.PostByID(t.Context(), post.ID, m[0].ID)
	require.NoError(t, err)
	require.Equal(t, old.ID, updated.Recipient.ID)
	require.True(t, updated.Recipient.Pending)
	byPerson, err := db.MemberByPersonID(t.Context(), old.PersonID)
	require.NoError(t, err)
	require.Equal(t, old.ID, byPerson.ID)
	require.Equal(t, 4, count(t, db, `SELECT COUNT(*) FROM members`))
}
func TestConcurrentSendsNeverOverspend(t *testing.T) {
	db, m := fixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, _, err := db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 50, Message: "Concurrent thanks", Now: october, IdempotencyKey: ulid.Make().String()})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	succeeded, insufficient := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		var points *domain.InsufficientPointsError
		require.True(t, errors.As(err, &points), "unexpected error: %v", err)
		require.GreaterOrEqual(t, points.Remaining, 0)
		insufficient++
	}
	require.Equal(t, 8, succeeded)
	require.Equal(t, 12, insufficient)
	stats, err := db.MyStats(t.Context(), m[0].ID, october)
	require.NoError(t, err)
	require.Zero(t, stats.Period.Remaining)
	require.Equal(t, 16, count(t, db, `SELECT COUNT(*) FROM ledger`))
}
func TestIdempotencyExpiryAndAtomicConcurrentReplay(t *testing.T) {
	db, m := fixture(t)
	input := CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 10, Message: "Once", Now: october, IdempotencyKey: "same"}
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		wg.Go(func() { _, _, err := db.CreatePost(t.Context(), input); results <- err })
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM posts`))
	input.Now = october.Add(24 * time.Hour)
	_, replayed, err := db.CreatePost(t.Context(), input)
	require.NoError(t, err)
	require.True(t, replayed)
	input.Now = input.Now.Add(time.Second)
	_, replayed, err = db.CreatePost(t.Context(), input)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, 2, count(t, db, `SELECT COUNT(*) FROM posts`))
	require.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM idempotency_keys`))
}
func TestTimelineCursorFiltersStatsAndSeed(t *testing.T) {
	db, m := fixture(t)
	_, err := db.UpdateProgram(t.Context(), domain.ProgramPatch{Values: new([]domain.Value{{Key: "care", Label: "Care"}})}, october, m[0].ID)
	require.NoError(t, err)
	for i := range 5 {
		values := []string{}
		if i%2 == 0 {
			values = []string{"care"}
		}
		_, _, err := db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 10, Message: "Thank you", Values: values, Now: october, IdempotencyKey: fmt.Sprintf("tie-%d", i)})
		require.NoError(t, err)
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		posts, next, err := db.ListPosts(t.Context(), PostFilter{Limit: 2, Cursor: cursor}, m[0].ID)
		require.NoError(t, err)
		for _, post := range posts {
			require.False(t, seen[post.ID])
			seen[post.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	require.Len(t, seen, 5)
	for _, f := range []PostFilter{{Value: "care"}, {MemberID: m[1].ID, SentBy: m[0].ID, ReceivedBy: m[1].ID, Month: "2026-10", Period: "2026-W43"}} {
		posts, _, err := db.ListPosts(t.Context(), f, m[0].ID)
		require.NoError(t, err)
		if f.Value != "" {
			require.Len(t, posts, 3)
		} else {
			require.Len(t, posts, 5)
		}
	}
	_, _, err = db.ListPosts(t.Context(), PostFilter{Cursor: "invalid"}, m[0].ID)
	require.Error(t, err)
	rows, err := db.ActivityByMember(t.Context(), "2026-10")
	require.NoError(t, err)
	require.Equal(t, m[1].ID, rows[m[1].ID].Member.ID)
	require.Equal(t, 50, rows[m[1].ID].ReceivedPoints)
	require.Equal(t, 5, rows[m[1].ID].ReceivedPosts)
	require.NoError(t, db.SeedDev(t.Context(), october))
	require.Equal(t, 3, count(t, db, `SELECT COUNT(*) FROM members`))
}
func TestSeedOnlyEmptyAndBothWeeks(t *testing.T) {
	db, _ := testutil.Database(t, Open)
	require.NoError(t, db.SeedDev(t.Context(), october))
	require.NoError(t, db.SeedDev(t.Context(), october))
	require.Equal(t, 6, count(t, db, `SELECT COUNT(*) FROM members`))
	require.Equal(t, 15, count(t, db, `SELECT COUNT(*) FROM posts`))
	require.Equal(t, 8, count(t, db, `SELECT COUNT(*) FROM claps`))
	require.Equal(t, 2, count(t, db, `SELECT COUNT(*) FROM periods`))
	require.Equal(t, 54, count(t, db, `SELECT COUNT(*) FROM ledger`))
}

func TestInboxOrderingUnreadAndMonotonicWatermark(t *testing.T) {
	db, m := fixture(t)
	received := send(t, db, m[0], m[1], 10, october)
	sent := send(t, db, m[1], m[0], 10, october.Add(time.Second))
	firstClap := clap(t, db, received, m[2], october.Add(2*time.Second))
	sentClap := clap(t, db, sent, m[2], october.Add(3*time.Second))
	tiedPost := send(t, db, m[0], m[1], 10, october.Add(3*time.Second))
	unrelated := send(t, db, m[0], m[2], 10, october.Add(4*time.Second))
	clap(t, db, unrelated, m[1], october.Add(5*time.Second))
	deleted := send(t, db, m[0], m[1], 10, october.Add(6*time.Second))
	clap(t, db, deleted, m[2], october.Add(7*time.Second))
	require.NoError(t, db.DeletePost(t.Context(), deleted.ID, m[0].ID, october.Add(8*time.Second)))

	wantIDs := []string{sentClap.ID, tiedPost.ID}
	slices.Sort(wantIDs)
	slices.Reverse(wantIDs)
	wantIDs = append(wantIDs, firstClap.ID, received.ID)
	var gotIDs []string
	cursor := ""
	for {
		items, next, seenAt, err := db.ListInbox(t.Context(), m[1].ID, 2, cursor)
		require.NoError(t, err)
		require.Nil(t, seenAt)
		for _, item := range items {
			gotIDs = append(gotIDs, item.ID)
			require.True(t, item.Unread)
			require.NotNil(t, item.Post.Clappers)
			require.Zero(t, item.Post.ClappedByMe)
			if item.Kind == domain.InboxPost {
				require.Equal(t, item.Post.ID, item.ID)
				require.Equal(t, m[0].ID, item.Actor.ID)
			} else {
				require.Equal(t, domain.InboxClap, item.Kind)
				require.Equal(t, m[2].ID, item.Actor.ID)
				require.Equal(t, m[2].ID, item.Post.Clappers[0].ID)
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	require.Equal(t, wantIDs, gotIDs)
	unread, err := db.UnreadCount(t.Context(), m[1].ID)
	require.NoError(t, err)
	require.Equal(t, 4, unread)
	seen, err := db.MarkInboxSeen(t.Context(), m[1].ID, october.Add(2*time.Second))
	require.NoError(t, err)
	items, _, seenAt, err := db.ListInbox(t.Context(), m[1].ID, 20, "")
	require.NoError(t, err)
	require.True(t, seen.Equal(*seenAt))
	require.True(t, items[0].Unread)
	require.True(t, items[1].Unread)
	require.False(t, items[2].Unread, "an event exactly at the watermark is read")
	require.False(t, items[3].Unread)
	unread, err = db.UnreadCount(t.Context(), m[1].ID)
	require.NoError(t, err)
	require.Equal(t, 2, unread)

	seen, err = db.MarkInboxSeen(t.Context(), m[1].ID, october.Add(10*time.Second))
	require.NoError(t, err)
	older, err := db.MarkInboxSeen(t.Context(), m[1].ID, october)
	require.NoError(t, err)
	require.True(t, seen.Equal(older))
	member, err := db.MemberByID(t.Context(), m[1].ID)
	require.NoError(t, err)
	require.True(t, seen.Equal(*member.InboxSeenAt))
	unread, err = db.UnreadCount(t.Context(), m[1].ID)
	require.NoError(t, err)
	require.Zero(t, unread)
	send(t, db, m[0], m[1], 10, october.Add(11*time.Second))
	unread, err = db.UnreadCount(t.Context(), m[1].ID)
	require.NoError(t, err)
	require.Equal(t, 1, unread)
	items, _, _, err = db.ListInbox(t.Context(), m[2].ID, 20, "")
	require.NoError(t, err)
	require.Len(t, items, 2, "claps on unrelated posts and the caller's own claps are excluded")
	for _, item := range items {
		require.Equal(t, unrelated.ID, item.Post.ID)
	}
	_, _, _, err = db.ListInbox(t.Context(), m[1].ID, 20, "invalid")
	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

func TestTimelineSearchEscapesWildcards(t *testing.T) {
	db, m := fixture(t)
	messages := []string{`100% Thanks`, `Care_team`, `Path\help`, `1000 careXteam Path/help`}
	posts := make([]domain.Post, 0, len(messages))
	for _, message := range messages {
		post, _, err := db.CreatePost(t.Context(), CreatePostInput{SenderID: m[0].ID, RecipientID: m[1].ID, Points: 10, Message: message, Now: october, IdempotencyKey: ulid.Make().String()})
		require.NoError(t, err)
		posts = append(posts, post)
	}
	for _, tc := range []struct {
		query string
		index int
	}{{"%", 0}, {"_", 1}, {`\`, 2}, {"tHaNkS", 0}, {"care_team", 1}, {`path\help`, 2}} {
		t.Run(tc.query, func(t *testing.T) {
			items, _, err := db.ListPosts(t.Context(), PostFilter{Q: tc.query}, m[2].ID)
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.Equal(t, posts[tc.index].ID, items[0].ID)
			require.NotNil(t, items[0].Clappers)
			require.Empty(t, items[0].Clappers)
		})
	}
	for _, query := range []string{"mEmBeR 0", "MEMBER 1"} {
		items, _, err := db.ListPosts(t.Context(), PostFilter{Q: query}, m[2].ID)
		require.NoError(t, err)
		require.Len(t, items, len(messages))
	}
	items, _, err := db.ListPosts(t.Context(), PostFilter{Q: "member 2"}, m[2].ID)
	require.NoError(t, err)
	require.Empty(t, items)
}
