package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

const monthlyReceives = `SELECT member_id, SUM(delta) AS points FROM ledger WHERE account='receive' AND month_key=$1 GROUP BY member_id HAVING SUM(delta)>0`

func (s *Store) Statement(ctx context.Context, month string, now time.Time) (out domain.Statement, err error) {
	if err = domain.ValidateMonthKey(month); err != nil {
		return
	}
	err = s.Do(ctx, false, func(tx *Tx) error { out, err = tx.statement(month, now); return err })
	return
}
func (tx *Tx) statement(month string, now time.Time) (out domain.Statement, err error) {
	out.Month, out.Lines, out.CloseBlockers = month, []domain.StatementLine{}, []string{}
	var closedBy string
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT closed_at,closed_by,point_unit_value::text,currency,total_points,total_amount::text,member_count,directory_refreshed FROM statements WHERE month_key=$1`, month).Scan(&out.ClosedAt, &closedBy, &out.PointUnitValue, &out.Currency, &out.TotalPoints, &out.TotalAmount, &out.MemberCount, &out.DirectoryRefreshed)
	if errors.Is(err, sql.ErrNoRows) {
		program, err := tx.program()
		if err != nil {
			return out, err
		}
		out.PointUnitValue, out.Currency = program.PointUnitValue, program.Currency
		out.CloseBlockers, out.PreviousOpenMonth, err = tx.statementCloseBlockers(month, now, program)
		if err != nil {
			return out, err
		}
		if err := tx.SQL.QueryRowContext(tx.Context, `SELECT COALESCE(SUM(points),0), (COALESCE(SUM(points),0)*$2::numeric)::text, COUNT(*) FROM (`+monthlyReceives+`) r`, month, program.PointUnitValue).Scan(&out.TotalPoints, &out.TotalAmount, &out.MemberCount); err != nil {
			return out, err
		}
		rows, err := tx.SQL.QueryContext(tx.Context, `SELECT m.id,m.person_id,m.subject,m.display_name,m.email,m.last_seen_at,m.avatar_version,r.points,(r.points*$2::numeric)::text FROM (`+monthlyReceives+`) r JOIN members m ON m.id=r.member_id ORDER BY r.points DESC,m.id`, month, program.PointUnitValue)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			var line domain.StatementLine
			if err := rows.Scan(&line.Member.ID, &line.Member.PersonID, &line.Member.Subject, &line.Member.DisplayName, &line.Member.Email, &line.Member.LastSeenAt, &line.Member.AvatarVersion, &line.Points, &line.Amount); err != nil {
				return out, err
			}
			line.Member.Pending = line.Member.Subject == nil
			out.Lines = append(out.Lines, line)
		}
		return out, rows.Err()
	}
	if err != nil {
		return out, err
	}
	out.Closed = true
	member, err := tx.member(closedBy)
	if err != nil {
		return out, err
	}
	out.ClosedBy = &member
	rows, err := tx.SQL.QueryContext(tx.Context, `SELECT l.member_id,l.person_id,l.subject,l.display_name,l.email,m.avatar_version,l.points,l.amount::text FROM statement_lines l JOIN members m ON m.id=l.member_id WHERE l.month_key=$1 ORDER BY l.points DESC,l.member_id`, month)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var line domain.StatementLine
		if err := rows.Scan(&line.Member.ID, &line.Member.PersonID, &line.Member.Subject, &line.Member.DisplayName, &line.Member.Email, &line.Member.AvatarVersion, &line.Points, &line.Amount); err != nil {
			return out, err
		}
		line.Member.Pending = line.Member.Subject == nil
		out.Lines = append(out.Lines, line)
	}
	return out, rows.Err()
}
func (s *Store) CloseStatement(ctx context.Context, month string, now time.Time, adminMemberID string, directoryRefreshed bool) (out domain.Statement, err error) {
	if err = domain.ValidateMonthKey(month); err != nil {
		return out, err
	}
	err = s.Do(ctx, true, func(tx *Tx) error {
		closed, err := tx.monthClosed(month)
		if err != nil {
			return err
		}
		if closed {
			out, err = tx.statement(month, now)
			return err
		}
		program, err := tx.program()
		if err != nil {
			return err
		}
		blockers, _, err := tx.statementCloseBlockers(month, now, program)
		if err != nil {
			return err
		}
		if len(blockers) > 0 {
			if blockers[0] == domain.ErrMonthNotEnded.Error() {
				return domain.ErrMonthNotEnded
			}
			return domain.ErrPreviousMonthOpen
		}
		// Compute and freeze in SQL's exact numeric arithmetic. Never round-trip money
		// through float64, and snapshot identity fields including pending membership.
		if err := tx.Exec(`INSERT INTO statements (month_key,closed_at,closed_by,point_unit_value,currency,total_points,total_amount,member_count,directory_refreshed)
 SELECT $1,$2,$3,$4::numeric,$5,COALESCE(SUM(points),0),COALESCE(SUM(points),0)*$4::numeric,COUNT(*),$6 FROM (`+monthlyReceives+`) r`, month, now, adminMemberID, program.PointUnitValue, program.Currency, directoryRefreshed); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO statement_lines (month_key,member_id,person_id,subject,email,display_name,points,amount)
 SELECT $1,m.id,m.person_id,m.subject,m.email,m.display_name,r.points,r.points*$2::numeric FROM (`+monthlyReceives+`) r JOIN members m ON m.id=r.member_id`, month, program.PointUnitValue); err != nil {
			return err
		}
		out, err = tx.statement(month, now)
		return err
	})
	return out, err
}

// Both the preview checklist and the serialized close use these predicates.
// Keep the ended-month blocker first to preserve close's error precedence.
func (tx *Tx) statementCloseBlockers(month string, now time.Time, program domain.Program) ([]string, *string, error) {
	loc, err := time.LoadLocation(program.TimeZone)
	if err != nil {
		return nil, nil, err
	}
	ended, err := domain.MonthEnded(month, now, loc)
	if err != nil {
		return nil, nil, err
	}
	blockers := make([]string, 0, 2)
	if !ended {
		blockers = append(blockers, domain.ErrMonthNotEnded.Error())
	}
	var previous *string
	if err := tx.SQL.QueryRowContext(tx.Context, `SELECT MIN(l.month_key) FROM ledger l
 WHERE l.month_key<$1 AND NOT EXISTS (SELECT 1 FROM statements s WHERE s.month_key=l.month_key)`, month).Scan(&previous); err != nil {
		return nil, nil, err
	}
	if previous != nil {
		blockers = append(blockers, domain.ErrPreviousMonthOpen.Error())
	}
	return blockers, previous, nil
}
func (s *Store) ListStatements(ctx context.Context, now time.Time) (items []domain.StatementSummary, err error) {
	items = []domain.StatementSummary{}
	err = s.Do(ctx, false, func(tx *Tx) error {
		program, err := tx.program()
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(program.TimeZone)
		if err != nil {
			return err
		}
		currentKey := domain.MonthKeyFor(now, loc)
		var oldest string
		if err := tx.SQL.QueryRowContext(ctx, `SELECT COALESCE(MIN(month_key),$1) FROM ledger`, currentKey).Scan(&oldest); err != nil {
			return err
		}
		// One query returns all open previews and closed snapshots, including empty
		// intervening months, without fetching statement lines for the index page.
		rows, err := tx.SQL.QueryContext(ctx, `WITH months AS (
 SELECT to_char(d,'YYYY-MM') AS month FROM generate_series($1::date,$2::date,interval '1 month') d
 ), received AS (
 SELECT month_key,member_id,SUM(delta) AS points FROM ledger WHERE account='receive' GROUP BY month_key,member_id HAVING SUM(delta)>0
 ), totals AS (SELECT month_key,SUM(points) AS points,COUNT(*) AS members FROM received GROUP BY month_key)
 SELECT m.month,s.month_key IS NOT NULL,s.closed_at,COALESCE(s.total_points,t.points,0),
 COALESCE(s.total_amount,COALESCE(t.points,0)*$3::numeric)::text,COALESCE(s.member_count,t.members,0)
 FROM months m LEFT JOIN statements s ON s.month_key=m.month LEFT JOIN totals t ON t.month_key=m.month ORDER BY m.month DESC`, oldest+"-01", currentKey+"-01", program.PointUnitValue)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item domain.StatementSummary
			if err := rows.Scan(&item.Month, &item.Closed, &item.ClosedAt, &item.TotalPoints, &item.TotalAmount, &item.MemberCount); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}
func (s *Store) MembersWithReceiveInMonth(ctx context.Context, month string) (items []domain.Member, err error) {
	items = []domain.Member{}
	if err = domain.ValidateMonthKey(month); err != nil {
		return
	}
	err = s.Do(ctx, false, func(tx *Tx) error {
		rows, err := tx.SQL.QueryContext(ctx, `SELECT `+memberColumns+` FROM members WHERE id IN (SELECT member_id FROM (`+monthlyReceives+`) r) ORDER BY id`, month)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			member, err := scanMember(rows)
			if err != nil {
				return err
			}
			items = append(items, member)
		}
		return rows.Err()
	})
	return
}
