package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

func (s *Store) Program(ctx context.Context) (out domain.Program, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error { out, err = tx.program(); return err })
	return
}
func (tx *Tx) program() (out domain.Program, err error) {
	var values []byte
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT time_zone, weekly_allowance, max_points_per_post, point_unit_value::text, currency, values, updated_at, updated_by, EXISTS(SELECT 1 FROM ledger) FROM program WHERE id=1`).Scan(&out.TimeZone, &out.WeeklyAllowance, &out.MaxPointsPerPost, &out.PointUnitValue, &out.Currency, &values, &out.UpdatedAt, &out.UpdatedBy, &out.TimeZoneLocked)
	if err != nil {
		return
	}
	err = json.Unmarshal(values, &out.Values)
	return
}
func (s *Store) UpdateProgram(ctx context.Context, patch domain.ProgramPatch, now time.Time, byMemberID string) (out domain.Program, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error {
		current, err := tx.program()
		if err != nil {
			return err
		}
		if err := domain.ValidateProgramPatch(patch, current); err != nil {
			return err
		}
		if patch.TimeZone != nil && *patch.TimeZone != current.TimeZone {
			if current.TimeZoneLocked {
				return domain.ErrTimezoneLocked
			}
			// Empty pre-ledger periods must not retain boundaries in the old timezone.
			if err := tx.Exec(`DELETE FROM periods WHERE NOT EXISTS (SELECT 1 FROM ledger WHERE period_key=periods.key)`); err != nil {
				return err
			}
			current.TimeZone = *patch.TimeZone
		}
		if patch.WeeklyAllowance != nil {
			current.WeeklyAllowance = *patch.WeeklyAllowance
		}
		if patch.MaxPointsPerPost != nil {
			current.MaxPointsPerPost = *patch.MaxPointsPerPost
		}
		if patch.PointUnitValue != nil {
			current.PointUnitValue = *patch.PointUnitValue
		}
		if patch.Currency != nil {
			current.Currency = *patch.Currency
		}
		if patch.Values != nil {
			current.Values = *patch.Values
		}
		if current.Values == nil {
			current.Values = []domain.Value{}
		}
		values, err := json.Marshal(current.Values)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE program SET time_zone=$1, weekly_allowance=$2, max_points_per_post=$3, point_unit_value=$4::numeric, currency=$5, values=$6::jsonb, updated_at=$7, updated_by=$8 WHERE id=1`, current.TimeZone, current.WeeklyAllowance, current.MaxPointsPerPost, current.PointUnitValue, current.Currency, string(values), now, byMemberID); err != nil {
			return err
		}
		out, err = tx.program()
		return err
	})
	return out, err
}
func (s *Store) CurrentPeriod(ctx context.Context, now time.Time) (out domain.Period, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		program, err := tx.program()
		if err != nil {
			return err
		}
		out, err = tx.period(program, now, false)
		return err
	})
	return
}

// Only CreatePost/CreateClap call period with freeze=true, inside their write tx.
func (tx *Tx) period(program domain.Program, now time.Time, freeze bool) (out domain.Period, err error) {
	loc, err := time.LoadLocation(program.TimeZone)
	if err != nil {
		return out, err
	}
	out.Key, out.StartsAt, out.EndsAt = domain.PeriodFor(now, loc)
	out.Allowance, out.MaxPointsPerPost = program.WeeklyAllowance, program.MaxPointsPerPost
	if freeze {
		if err := tx.Exec(`INSERT INTO periods (key, starts_at, ends_at, allowance, max_points_per_post) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (key) DO NOTHING`, out.Key, out.StartsAt, out.EndsAt, out.Allowance, out.MaxPointsPerPost); err != nil {
			return out, err
		}
	}
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT starts_at, ends_at, allowance, max_points_per_post FROM periods WHERE key=$1`, out.Key).Scan(&out.StartsAt, &out.EndsAt, &out.Allowance, &out.MaxPointsPerPost)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	out.Frozen = err == nil
	return
}
func (tx *Tx) remaining(memberID string, period domain.Period) (remaining int, err error) {
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT $3::bigint + COALESCE(SUM(delta),0) FROM ledger WHERE member_id=$1 AND account='give' AND period_key=$2`, memberID, period.Key, period.Allowance).Scan(&remaining)
	return
}
