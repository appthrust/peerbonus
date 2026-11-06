package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"
)

const postColumns = `p.id,p.period_key,p.month_key,p.points,p.message,p.value_keys,p.clap_count,p.created_at,
 s.id,s.person_id,s.subject,s.display_name,s.email,s.last_seen_at,s.avatar_version,
 r.id,r.person_id,r.subject,r.display_name,r.email,r.last_seen_at,r.avatar_version,
 (SELECT count(*) FROM claps c WHERE c.post_id=p.id AND c.member_id=$1),
 newest_clappers.members`

const postJoins = ` FROM posts p JOIN members s ON s.id=p.sender_id JOIN members r ON r.id=p.recipient_id
 CROSS JOIN LATERAL (
   SELECT COALESCE(jsonb_agg(jsonb_build_object(
     'ID',m.id,'PersonID',m.person_id,'Subject',m.subject,
     'DisplayName',m.display_name,'Email',m.email,'AvatarVersion',m.avatar_version
   ) ORDER BY newest.created_at DESC,newest.id DESC),'[]'::jsonb) AS members
   FROM (SELECT c.id,c.member_id,c.created_at FROM claps c WHERE c.post_id=p.id ORDER BY c.created_at DESC,c.id DESC LIMIT 3) newest
   JOIN members m ON m.id=newest.member_id
 ) newest_clappers `

const postSelect = `SELECT ` + postColumns + postJoins

func scanPost(row scanner, extra ...any) (out domain.Post, err error) {
	var clappers []byte
	destinations := make([]any, 0, 24+len(extra))
	destinations = append(destinations, &out.ID, &out.PeriodKey, &out.MonthKey, &out.Points, &out.Message, pq.Array(&out.Values), &out.ClapCount, &out.CreatedAt,
		&out.Sender.ID, &out.Sender.PersonID, &out.Sender.Subject, &out.Sender.DisplayName, &out.Sender.Email, &out.Sender.LastSeenAt, &out.Sender.AvatarVersion,
		&out.Recipient.ID, &out.Recipient.PersonID, &out.Recipient.Subject, &out.Recipient.DisplayName, &out.Recipient.Email, &out.Recipient.LastSeenAt, &out.Recipient.AvatarVersion, &out.ClappedByMe, &clappers)
	err = row.Scan(append(destinations, extra...)...)
	out.Clappers = []domain.Member{}
	if err != nil {
		return
	}
	if err = json.Unmarshal(clappers, &out.Clappers); err != nil {
		return
	}
	for i := range out.Clappers {
		out.Clappers[i].Pending = out.Clappers[i].Subject == nil
	}
	out.Sender.Pending, out.Recipient.Pending = out.Sender.Subject == nil, out.Recipient.Subject == nil
	out.ClapPoints = out.ClapCount * domain.ClapCost
	return
}
func (tx *Tx) post(id, viewerID string, includeDeleted bool) (domain.Post, error) {
	return scanPost(tx.SQL.QueryRowContext(tx.Context, postSelect+`WHERE p.id=$2 AND ($3 OR p.deleted_at IS NULL)`, viewerID, id, includeDeleted))
}
func (s *Store) PostByID(ctx context.Context, id, viewerID string) (out domain.Post, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error { out, err = tx.post(id, viewerID, false); return err })
	return
}
func parseCursor(cursor string) (at time.Time, id string, err error) {
	invalid := &domain.ValidationError{Field: "cursor", Msg: "must be a valid timeline cursor"}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return at, "", invalid
	}
	timestamp, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return at, "", invalid
	}
	at, err = time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return at, "", invalid
	}
	if _, err := ulid.ParseStrict(id); err != nil {
		return at, "", invalid
	}
	return at, id, nil
}

var searchEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (s *Store) ListPosts(ctx context.Context, f PostFilter, viewerID string) (items []domain.Post, nextCursor string, err error) {
	items = []domain.Post{}
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 50 {
		return items, "", &domain.ValidationError{Field: "limit", Msg: "must be between 1 and 50"}
	}
	if f.Month != "" {
		if err := domain.ValidateMonthKey(f.Month); err != nil {
			return items, "", err
		}
	}
	if f.Period != "" {
		if err := domain.ValidatePeriodKey(f.Period); err != nil {
			return items, "", err
		}
	}
	var cursorAt any
	cursorID := ""
	if f.Cursor != "" {
		at, id, err := parseCursor(f.Cursor)
		if err != nil {
			return items, "", err
		}
		cursorAt, cursorID = at, id
	}
	search := ""
	if f.Q != "" {
		search = "%" + searchEscaper.Replace(f.Q) + "%"
	}
	err = s.Do(ctx, false, func(tx *Tx) error {
		rows, err := tx.SQL.QueryContext(ctx, postSelect+`WHERE p.deleted_at IS NULL
 AND ($2='' OR p.sender_id=$2 OR p.recipient_id=$2)
 AND ($3='' OR p.sender_id=$3) AND ($4='' OR p.recipient_id=$4)
 AND ($5='' OR $5=ANY(p.value_keys)) AND ($6='' OR p.period_key=$6) AND ($7='' OR p.month_key=$7)
 AND ($8::timestamptz IS NULL OR (p.created_at,p.id)<($8::timestamptz,$9))
 AND ($11='' OR p.message ILIKE $11 OR s.display_name ILIKE $11 OR r.display_name ILIKE $11)
 ORDER BY p.created_at DESC,p.id DESC LIMIT $10`, viewerID, f.MemberID, f.SentBy, f.ReceivedBy, f.Value, f.Period, f.Month, cursorAt, cursorID, f.Limit+1, search)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			post, err := scanPost(rows)
			if err != nil {
				return err
			}
			items = append(items, post)
		}
		return rows.Err()
	})
	if err == nil && len(items) > f.Limit {
		items = items[:f.Limit]
		last := items[len(items)-1]
		nextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.Format(time.RFC3339Nano) + "|" + last.ID))
	}
	return items, nextCursor, err
}

const clapSelect = `SELECT c.id,c.created_at,m.id,m.person_id,m.subject,m.display_name,m.email,m.last_seen_at,m.avatar_version FROM claps c JOIN members m ON m.id=c.member_id `

func scanClap(row scanner) (out domain.Clap, err error) {
	err = row.Scan(&out.ID, &out.CreatedAt, &out.Member.ID, &out.Member.PersonID, &out.Member.Subject, &out.Member.DisplayName, &out.Member.Email, &out.Member.LastSeenAt, &out.Member.AvatarVersion)
	out.Member.Pending = out.Member.Subject == nil
	return
}
func (tx *Tx) clap(id string) (domain.Clap, error) {
	return scanClap(tx.SQL.QueryRowContext(tx.Context, clapSelect+`WHERE c.id=$1`, id))
}
func (s *Store) ListClaps(ctx context.Context, postID string, limit int) (items []domain.Clap, err error) {
	items = []domain.Clap{}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 50 {
		return items, &domain.ValidationError{Field: "limit", Msg: "must be between 1 and 50"}
	}
	err = s.Do(ctx, false, func(tx *Tx) error {
		if _, err := tx.post(postID, "", false); err != nil {
			return err
		}
		rows, err := tx.SQL.QueryContext(ctx, clapSelect+`WHERE c.post_id=$1 ORDER BY c.created_at DESC,c.id DESC LIMIT $2`, postID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			clap, err := scanClap(rows)
			if err != nil {
				return err
			}
			items = append(items, clap)
		}
		return rows.Err()
	})
	return
}
