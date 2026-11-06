package store

import (
	"context"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

// Network reads the whole Project's month before any Directory group filtering.
// All queries share one read transaction; no period or member is created.
func (s *Store) Network(ctx context.Context, month string, now time.Time) (out domain.NetworkFacts, err error) {
	out.Month = month
	out.Members = make(map[string]domain.NetworkMemberFacts)
	out.Edges = []domain.NetworkMemberEdge{}
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
		out.AsOf = start.AddDate(0, 1, 0).Add(-time.Nanosecond).UTC()
		if domain.MonthKeyFor(now, loc) == month {
			out.AsOf = now.UTC()
			_, weekStart, _ := domain.PeriodFor(now, loc)
			weekStart = weekStart.UTC()
			out.RecentSince = &weekStart
		}
		if err := tx.networkMembers(&out); err != nil {
			return err
		}
		return tx.networkEdges(&out)
	})
	return out, err
}

func (tx *Tx) networkMembers(out *domain.NetworkFacts) error {
	// The decline windows cross calendar months: [asOf-28d, asOf-14d)
	// and [asOf-14d, asOf]. Include an action made exactly at the request clock.
	// PostgreSQL stores microseconds and rounds timestamp parameters. Floor the
	// inclusive upper bound and ceil lower bounds, preserving comparisons to
	// the nanosecond clock (notably the last instant of a historical month).
	asOf := out.AsOf.Truncate(time.Microsecond)
	boundary := asOf
	if asOf.Before(out.AsOf) {
		boundary = boundary.Add(time.Microsecond)
	}
	rows, err := tx.SQL.QueryContext(tx.Context, `WITH month_posts AS (
 SELECT sender_id,recipient_id,clap_count FROM posts WHERE month_key=$1 AND NOT reversed
 ), month_claps AS (
 SELECT c.member_id,p.sender_id,p.recipient_id FROM claps c
 JOIN posts p ON p.id=c.post_id AND NOT p.reversed WHERE c.month_key=$1
 ), sent AS (
 SELECT sender_id,COUNT(*) AS count FROM month_posts GROUP BY sender_id
 ), received_posts AS (
 SELECT recipient_id,COUNT(*) AS count,COUNT(*) FILTER (WHERE clap_count=0) AS unclapped
 FROM month_posts GROUP BY recipient_id
 ), received AS (
 SELECT member_id,SUM(delta) AS points FROM ledger WHERE month_key=$1 AND account='receive' GROUP BY member_id
 ), clapped AS (
 SELECT member_id,COUNT(*) AS count FROM month_claps GROUP BY member_id
 ), clap_recipients AS (
 SELECT sender_id AS member_id FROM month_claps UNION ALL SELECT recipient_id FROM month_claps
 ), received_claps AS (
 SELECT member_id,COUNT(*) AS count FROM clap_recipients GROUP BY member_id
 ), actions AS (
 SELECT sender_id AS member_id,created_at FROM posts WHERE NOT reversed AND created_at >= $2 AND created_at <= $4
 UNION ALL
 SELECT c.member_id,c.created_at FROM claps c JOIN posts p ON p.id=c.post_id AND NOT p.reversed
 WHERE c.created_at >= $2 AND c.created_at <= $4
 ), activity AS (
 SELECT member_id,COUNT(*) FILTER (WHERE created_at >= $3) AS recent,
 COUNT(*) FILTER (WHERE created_at < $3) AS previous FROM actions GROUP BY member_id
 ) SELECT m.id,m.person_id,m.subject,m.display_name,m.email,m.last_seen_at,m.avatar_version,
 COALESCE(s.count,0),COALESCE(rp.count,0),COALESCE(r.points,0),COALESCE(c.count,0),COALESCE(rc.count,0),
 COALESCE(rp.unclapped,0),COALESCE(a.recent,0),COALESCE(a.previous,0)
 FROM members m LEFT JOIN sent s ON s.sender_id=m.id LEFT JOIN received_posts rp ON rp.recipient_id=m.id
 LEFT JOIN received r ON r.member_id=m.id LEFT JOIN clapped c ON c.member_id=m.id
 LEFT JOIN received_claps rc ON rc.member_id=m.id LEFT JOIN activity a ON a.member_id=m.id`,
		out.Month, boundary.Add(-2*domain.DeclineWindow), boundary.Add(-domain.DeclineWindow), asOf)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row domain.NetworkMemberFacts
		if err := rows.Scan(&row.Member.ID, &row.Member.PersonID, &row.Member.Subject, &row.Member.DisplayName, &row.Member.Email, &row.Member.LastSeenAt, &row.Member.AvatarVersion,
			&row.SentPosts, &row.ReceivedPosts, &row.ReceivedPoints, &row.ClapsGiven, &row.ClapsReceived, &row.UnclappedPosts, &row.Activity.Recent, &row.Activity.Previous); err != nil {
			return err
		}
		row.Member.Pending = row.Member.Subject == nil
		out.Members[row.Member.ID] = row
		out.Posts += row.SentPosts
		out.Claps += row.ClapsGiven
	}
	return rows.Err()
}

func (tx *Tx) networkEdges(out *domain.NetworkFacts) error {
	rows, err := tx.SQL.QueryContext(tx.Context, `WITH pairs AS (
 SELECT sender_id AS a,recipient_id AS b,1 AS posts,0 AS claps,points,created_at
 FROM posts WHERE month_key=$1 AND NOT reversed
 UNION ALL
 SELECT c.member_id,ends.member_id,0,1,0,c.created_at FROM claps c
 JOIN posts p ON p.id=c.post_id AND NOT p.reversed
 CROSS JOIN LATERAL (VALUES (p.sender_id),(p.recipient_id)) AS ends(member_id)
 WHERE c.month_key=$1
 ) SELECT LEAST(a,b),GREATEST(a,b),SUM(posts),SUM(claps),SUM(points),MAX(created_at)
 FROM pairs WHERE a<>b GROUP BY LEAST(a,b),GREATEST(a,b) ORDER BY LEAST(a,b),GREATEST(a,b)`, out.Month)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var edge domain.NetworkMemberEdge
		if err := rows.Scan(&edge.From, &edge.To, &edge.Posts, &edge.Claps, &edge.Points, &edge.LastAt); err != nil {
			return err
		}
		out.Edges = append(out.Edges, edge)
		// One aggregate row per unordered pair: each endpoint gains exactly one
		// counterpart, regardless of how many posts or claps built the pair.
		from, to := out.Members[edge.From], out.Members[edge.To]
		from.Connections++
		to.Connections++
		out.Members[edge.From], out.Members[edge.To] = from, to
	}
	return rows.Err()
}
