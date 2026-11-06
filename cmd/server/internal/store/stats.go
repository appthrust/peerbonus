package store

import (
	"context"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

func (tx *Tx) memberStats(memberID, month string) (out domain.MemberStats, err error) {
	out.Month.Key = month
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT
 COALESCE(SUM(delta) FILTER (WHERE month_key=$2),0),COALESCE(SUM(delta),0)
 FROM ledger WHERE member_id=$1 AND account='receive'`, memberID, month).Scan(&out.Month.ReceivedPoints, &out.Total.ReceivedPoints)
	if err != nil {
		return
	}
	// Reversed posts cease contributing to counts; hidden, settled posts retain
	// their financial effect and therefore their historical statistics.
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT
 COUNT(*) FILTER (WHERE recipient_id=$1 AND month_key=$2), COUNT(*) FILTER (WHERE sender_id=$1 AND month_key=$2),
 COUNT(*) FILTER (WHERE recipient_id=$1), COUNT(*) FILTER (WHERE sender_id=$1)
 FROM posts WHERE NOT reversed AND (sender_id=$1 OR recipient_id=$1)`, memberID, month).Scan(&out.Month.ReceivedPosts, &out.Month.SentPosts, &out.Total.ReceivedPosts, &out.Total.SentPosts)
	return
}
func (s *Store) MemberStats(ctx context.Context, memberID string, now time.Time) (out domain.MemberStats, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		if _, err := tx.member(memberID); err != nil {
			return err
		}
		program, err := tx.program()
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(program.TimeZone)
		if err != nil {
			return err
		}
		month := domain.MonthKeyFor(now, loc)
		out, err = tx.memberStats(memberID, month)
		if err != nil {
			return err
		}
		out.SentTo, err = tx.partners("recipient_id", "p.sender_id=$1 AND p.deleted_at IS NULL", memberID)
		if err != nil {
			return err
		}
		out.ReceivedFrom, err = tx.partners("sender_id", "p.recipient_id=$1 AND p.deleted_at IS NULL", memberID)
		if err != nil {
			return err
		}
		out.Values, err = tx.valueCounts("p.recipient_id=$1 AND p.deleted_at IS NULL", memberID)
		if err != nil {
			return err
		}
		out.Months, err = tx.memberMonths(memberID, month)
		return err
	})
	return out, err
}
func (s *Store) MyStats(ctx context.Context, memberID string, now time.Time) (out domain.MyStats, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		if _, err := tx.member(memberID); err != nil {
			return err
		}
		program, err := tx.program()
		if err != nil {
			return err
		}
		period, err := tx.period(program, now, false)
		if err != nil {
			return err
		}
		out.Period.Key, out.Period.StartsAt, out.Period.EndsAt, out.Period.Allowance = period.Key, period.StartsAt, period.EndsAt, period.Allowance
		out.Period.Remaining, err = tx.remaining(memberID, period)
		if err != nil {
			return err
		}
		if err := tx.SQL.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(points),0) FROM posts WHERE sender_id=$1 AND period_key=$2 AND NOT reversed`, memberID, period.Key).Scan(&out.Period.SentPosts, &out.Period.SentPoints); err != nil {
			return err
		}
		if err := tx.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM claps c JOIN posts p ON p.id=c.post_id WHERE c.member_id=$1 AND c.period_key=$2 AND NOT p.reversed`, memberID, period.Key).Scan(&out.Period.Claps); err != nil {
			return err
		}
		loc, err := time.LoadLocation(program.TimeZone)
		if err != nil {
			return err
		}
		stats, err := tx.memberStats(memberID, domain.MonthKeyFor(now, loc))
		out.Month, out.Total = stats.Month, stats.Total
		return err
	})
	return out, err
}
func (s *Store) ActivityByMember(ctx context.Context, month string) (items map[string]domain.Participant, err error) {
	items = make(map[string]domain.Participant)
	if err = domain.ValidateMonthKey(month); err != nil {
		return items, err
	}
	err = s.Do(ctx, false, func(tx *Tx) error {
		rows, err := tx.SQL.QueryContext(ctx, `WITH sent AS (
 SELECT sender_id,COUNT(*) AS count,SUM(points) AS points FROM posts WHERE month_key=$1 AND NOT reversed GROUP BY sender_id
 ), received_posts AS (
 SELECT recipient_id,COUNT(*) AS count FROM posts WHERE month_key=$1 AND NOT reversed GROUP BY recipient_id
 ), received AS (
 SELECT member_id,SUM(delta) AS points FROM ledger WHERE month_key=$1 AND account='receive' GROUP BY member_id
 ), clapped AS (
 SELECT c.member_id,COUNT(*) AS count FROM claps c JOIN posts p ON p.id=c.post_id WHERE c.month_key=$1 AND NOT p.reversed GROUP BY c.member_id
 ) SELECT m.id,m.person_id,m.subject,m.display_name,m.email,m.last_seen_at,m.avatar_version,
 COALESCE(s.count,0),COALESCE(s.points,0),COALESCE(rp.count,0),COALESCE(r.points,0),COALESCE(c.count,0)
 FROM members m LEFT JOIN sent s ON s.sender_id=m.id LEFT JOIN received_posts rp ON rp.recipient_id=m.id
 LEFT JOIN received r ON r.member_id=m.id LEFT JOIN clapped c ON c.member_id=m.id`, month)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			row := domain.Participant{Member: &domain.Member{}}
			if err := rows.Scan(&row.Member.ID, &row.Member.PersonID, &row.Member.Subject, &row.Member.DisplayName, &row.Member.Email, &row.Member.LastSeenAt, &row.Member.AvatarVersion, &row.SentPosts, &row.SentPoints, &row.ReceivedPosts, &row.ReceivedPoints, &row.ClapsGiven); err != nil {
				return err
			}
			row.Member.Pending = row.Member.Subject == nil
			items[row.Member.ID] = row
		}
		return rows.Err()
	})
	return items, err
}

// The column and predicate are internal SQL fragments, never user input.
func (tx *Tx) partners(column, predicate string, args ...any) ([]domain.Partner, error) {
	rows, err := tx.SQL.QueryContext(tx.Context, `SELECT
 m.id,m.person_id,m.subject,m.display_name,m.email,m.last_seen_at,m.avatar_version,COUNT(*),SUM(p.points)
 FROM posts p JOIN members m ON m.id=p.`+column+`
 WHERE NOT p.reversed AND `+predicate+`
 GROUP BY m.id ORDER BY SUM(p.points) DESC,COUNT(*) DESC,m.display_name,m.id LIMIT 5`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Partner, 0, 5)
	for rows.Next() {
		var item domain.Partner
		if err := rows.Scan(&item.Member.ID, &item.Member.PersonID, &item.Member.Subject, &item.Member.DisplayName, &item.Member.Email, &item.Member.LastSeenAt, &item.Member.AvatarVersion, &item.Posts, &item.Points); err != nil {
			return nil, err
		}
		item.Member.Pending = item.Member.Subject == nil
		items = append(items, item)
	}
	return items, rows.Err()
}

func (tx *Tx) valueCounts(predicate string, args ...any) ([]domain.ValueCount, error) {
	rows, err := tx.SQL.QueryContext(tx.Context, `SELECT v.key,COUNT(*),SUM(p.points)
 FROM posts p CROSS JOIN LATERAL unnest(p.value_keys) AS v(key)
 WHERE NOT p.reversed AND `+predicate+` GROUP BY v.key ORDER BY COUNT(*) DESC,v.key`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ValueCount{}
	for rows.Next() {
		var item domain.ValueCount
		if err := rows.Scan(&item.Key, &item.Posts, &item.Points); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (tx *Tx) memberMonths(memberID, month string) ([]domain.MonthPoint, error) {
	rows, err := tx.SQL.QueryContext(tx.Context, `WITH months AS (
 SELECT to_char(d,'YYYY-MM') AS key
 FROM generate_series($2::timestamp-interval '5 months',$2::timestamp,interval '1 month') d
 ), received AS (
 SELECT month_key,SUM(delta) AS points FROM ledger
 WHERE member_id=$1 AND account='receive' AND month_key IN (SELECT key FROM months) GROUP BY month_key
 ), counts AS (
 SELECT month_key,COUNT(*) FILTER (WHERE recipient_id=$1) AS received,COUNT(*) FILTER (WHERE sender_id=$1) AS sent
 FROM posts WHERE NOT reversed AND (sender_id=$1 OR recipient_id=$1)
 AND month_key IN (SELECT key FROM months) GROUP BY month_key
 ) SELECT m.key,COALESCE(r.points,0),COALESCE(c.received,0),COALESCE(c.sent,0)
 FROM months m LEFT JOIN received r ON r.month_key=m.key LEFT JOIN counts c ON c.month_key=m.key ORDER BY m.key`, memberID, month+"-01")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.MonthPoint, 0, 6)
	for rows.Next() {
		var item domain.MonthPoint
		if err := rows.Scan(&item.Key, &item.ReceivedPoints, &item.ReceivedPosts, &item.SentPosts); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Insights(ctx context.Context, month string, now time.Time) (out domain.Insights, err error) {
	out.Month = month
	if err = domain.ValidateMonthKey(month); err != nil {
		return out, err
	}
	err = s.Do(ctx, false, func(tx *Tx) error {
		program, err := tx.program()
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(program.TimeZone)
		if err != nil {
			return err
		}
		start, err := time.ParseInLocation("2006-01", month, loc)
		if err != nil {
			return domain.ErrInvalidMonth
		}
		out.Weeks = monthWeeks(start, loc)
		if err := tx.SQL.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(DISTINCT sender_id),COUNT(DISTINCT recipient_id)
 FROM posts WHERE month_key=$1 AND NOT reversed`, month).Scan(&out.Posts, &out.Senders, &out.Recipients); err != nil {
			return err
		}
		if err := tx.SQL.QueryRowContext(ctx, `SELECT COALESCE(SUM(delta),0) FROM ledger
 WHERE month_key=$1 AND account='receive'`, month).Scan(&out.Points); err != nil {
			return err
		}
		if err := tx.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM claps c JOIN posts p ON p.id=c.post_id
 WHERE c.month_key=$1 AND NOT p.reversed`, month).Scan(&out.Claps); err != nil {
			return err
		}
		out.Values, err = tx.valueCounts("p.month_key=$1", month)
		if err != nil {
			return err
		}
		out.TopRecipients, err = tx.partners("recipient_id", "p.month_key=$1", month)
		if err != nil {
			return err
		}
		out.TopSenders, err = tx.partners("sender_id", "p.month_key=$1", month)
		if err != nil {
			return err
		}
		// Clip activity to the month even when its first or last ISO week crosses
		// a month boundary. Receive ledger rows include clap credits and reversals.
		rows, err := tx.SQL.QueryContext(ctx, `SELECT period_key,SUM(posts),SUM(points),SUM(claps) FROM (
 SELECT period_key,COUNT(*) AS posts,0 AS points,0 AS claps FROM posts
 WHERE month_key=$1 AND NOT reversed GROUP BY period_key
 UNION ALL
 SELECT period_key,0,SUM(delta),0 FROM ledger WHERE month_key=$1 AND account='receive' GROUP BY period_key
 UNION ALL
 SELECT c.period_key,0,0,COUNT(*) FROM claps c JOIN posts p ON p.id=c.post_id
 WHERE c.month_key=$1 AND NOT p.reversed GROUP BY c.period_key
 ) activity GROUP BY period_key`, month)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var point domain.WeekPoint
			if err := rows.Scan(&point.Key, &point.Posts, &point.Points, &point.Claps); err != nil {
				return err
			}
			for i := range out.Weeks {
				if out.Weeks[i].Key == point.Key {
					out.Weeks[i].Posts, out.Weeks[i].Points, out.Weeks[i].Claps = point.Posts, point.Points, point.Claps
					break
				}
			}
		}
		return rows.Err()
	})
	return out, err
}

func monthWeeks(start time.Time, loc *time.Location) []domain.WeekPoint {
	items := make([]domain.WeekPoint, 0, 6)
	end := start.AddDate(0, 1, 0)
	for at := start; at.Before(end); {
		key, startsAt, endsAt := domain.PeriodFor(at, loc)
		items = append(items, domain.WeekPoint{Key: key, StartsAt: startsAt, EndsAt: endsAt})
		at = endsAt
	}
	return items
}
