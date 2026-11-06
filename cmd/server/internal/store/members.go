package store

import (
	"context"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"
)

const memberColumns = `id, person_id, subject, display_name, email, last_seen_at, inbox_seen_at, avatar_version, directory_synced_at, slack_user_id, slack_lookup_at`

func scanMember(row scanner) (out domain.Member, err error) {
	err = row.Scan(&out.ID, &out.PersonID, &out.Subject, &out.DisplayName, &out.Email, &out.LastSeenAt, &out.InboxSeenAt, &out.AvatarVersion, &out.DirectorySyncedAt, &out.SlackUserID, &out.SlackLookupAt)
	out.Pending = out.Subject == nil
	return
}
func (s *Store) UpsertMember(ctx context.Context, in MemberInput, now time.Time, touchSeen bool) (out domain.Member, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error { out, err = tx.upsertMember(in, now, touchSeen); return err })
	return
}
func (tx *Tx) upsertMember(in MemberInput, now time.Time, touchSeen bool) (domain.Member, error) {
	if in.Subject != nil {
		if err := tx.Exec(`UPDATE members SET subject=NULL WHERE subject=$1 AND person_id<>$2`, *in.Subject, in.PersonID); err != nil {
			return domain.Member{}, err
		}
	}
	var seen *time.Time
	if touchSeen {
		seen = &now
	}
	return scanMember(tx.SQL.QueryRowContext(tx.Context, `INSERT INTO members (id, person_id, subject, display_name, email, last_seen_at, avatar_version, directory_synced_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT (person_id) DO UPDATE SET subject=EXCLUDED.subject, display_name=EXCLUDED.display_name, email=EXCLUDED.email, last_seen_at=COALESCE(EXCLUDED.last_seen_at,members.last_seen_at), avatar_version=EXCLUDED.avatar_version, directory_synced_at=COALESCE(EXCLUDED.directory_synced_at,members.directory_synced_at) RETURNING `+memberColumns,
		ulid.Make().String(), in.PersonID, in.Subject, in.DisplayName, in.Email, seen, in.AvatarVersion, in.DirectorySyncedAt))
}
func (s *Store) UpdateMemberProfiles(ctx context.Context, in []MemberInput) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		for _, member := range in {
			if err := tx.Exec(`UPDATE members SET display_name=$1, email=$2, avatar_version=$3, directory_synced_at=COALESCE($5,directory_synced_at) WHERE person_id=$4`, member.DisplayName, member.Email, member.AvatarVersion, member.PersonID, member.DirectorySyncedAt); err != nil {
				return err
			}
		}
		return nil
	})
}

// StaleMembers returns at most 100 snapshots, with never-synced members first.
func (s *Store) StaleMembers(ctx context.Context, before time.Time, limit int) (items []domain.Member, err error) {
	items = []domain.Member{}
	err = s.Do(ctx, false, func(tx *Tx) error {
		rows, err := tx.SQL.QueryContext(ctx, `SELECT `+memberColumns+` FROM members
 WHERE directory_synced_at IS NULL OR directory_synced_at < $1
 ORDER BY directory_synced_at ASC NULLS FIRST, id LIMIT $2`, before, min(limit, 100))
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

// RefreshMembers updates stored Directory snapshots without creating members or
// moving a subject from another member. Missing member IDs keep their snapshots.
func (s *Store) RefreshMembers(ctx context.Context, people []MemberInput, missing []string, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		for _, person := range people {
			if err := tx.Exec(`UPDATE members SET display_name=$1, email=$2, avatar_version=$3,
 subject=CASE WHEN $4::text IS NOT NULL AND NOT EXISTS (
   SELECT 1 FROM members existing WHERE existing.subject=$4 AND existing.id<>members.id
 ) THEN $4 ELSE members.subject END,
 directory_synced_at=$5 WHERE person_id=$6`,
				person.DisplayName, person.Email, person.AvatarVersion, person.Subject, now, person.PersonID); err != nil {
				return err
			}
		}
		if len(missing) > 0 {
			return tx.Exec(`UPDATE members SET directory_synced_at=$1 WHERE id=ANY($2)`, now, pq.Array(missing))
		}
		return nil
	})
}
func (s *Store) MemberBySubject(ctx context.Context, subject string) (out domain.Member, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		out, err = scanMember(tx.SQL.QueryRowContext(ctx, `SELECT `+memberColumns+` FROM members WHERE subject=$1`, subject))
		return err
	})
	return
}
func (s *Store) MemberByID(ctx context.Context, id string) (out domain.Member, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error { out, err = tx.member(id); return err })
	return
}
func (s *Store) MemberByPersonID(ctx context.Context, personID string) (out domain.Member, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		out, err = scanMember(tx.SQL.QueryRowContext(ctx, `SELECT `+memberColumns+` FROM members WHERE person_id=$1`, personID))
		return err
	})
	return
}
func (tx *Tx) member(id string) (domain.Member, error) {
	return scanMember(tx.SQL.QueryRowContext(tx.Context, `SELECT `+memberColumns+` FROM members WHERE id=$1`, id))
}

// RecentRecipients returns distinct recipients of the sender's visible posts,
// ordered by their latest post (including its ID to break timestamp ties).
func (s *Store) RecentRecipients(ctx context.Context, senderID string, limit int) (items []domain.Member, err error) {
	items = []domain.Member{}
	err = s.Do(ctx, false, func(tx *Tx) error {
		rows, err := tx.SQL.QueryContext(ctx, `SELECT m.id,m.person_id,m.subject,m.display_name,m.email,m.last_seen_at,m.inbox_seen_at,m.avatar_version,m.directory_synced_at,m.slack_user_id,m.slack_lookup_at
 FROM members m JOIN (
   SELECT DISTINCT ON (recipient_id) recipient_id,created_at,id
   FROM posts WHERE sender_id=$1 AND deleted_at IS NULL
   ORDER BY recipient_id,created_at DESC,id DESC
 ) recent ON recent.recipient_id=m.id
 ORDER BY recent.created_at DESC,recent.id DESC LIMIT $2`, senderID, limit)
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
