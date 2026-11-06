package store

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

// Inbox events are derived from the same visible posts for both listing and counts.
const inboxEvents = `SELECT p.id,'post' AS kind,p.id AS post_id,p.sender_id AS actor_id,p.created_at
 FROM posts p WHERE p.recipient_id=$1 AND p.deleted_at IS NULL
 UNION ALL
 SELECT c.id,'clap' AS kind,p.id AS post_id,c.member_id AS actor_id,c.created_at
 FROM claps c JOIN posts p ON p.id=c.post_id
 WHERE (p.sender_id=$1 OR p.recipient_id=$1) AND c.member_id<>$1 AND p.deleted_at IS NULL`

func (s *Store) ListInbox(ctx context.Context, memberID string, limit int, cursor string) (items []domain.InboxItem, nextCursor string, seenAt *time.Time, err error) {
	items = []domain.InboxItem{}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return items, "", nil, &domain.ValidationError{Field: "limit", Msg: "must be between 1 and 50"}
	}
	var cursorAt any
	cursorID := ""
	if cursor != "" {
		at, id, err := parseCursor(cursor)
		if err != nil {
			return items, "", nil, err
		}
		cursorAt, cursorID = at, id
	}
	err = s.Do(ctx, false, func(tx *Tx) error {
		member, err := tx.member(memberID)
		if err != nil {
			return err
		}
		seenAt = member.InboxSeenAt
		rows, err := tx.SQL.QueryContext(ctx, `SELECT `+postColumns+`,
 i.id,i.kind,i.created_at,actor.id,actor.person_id,actor.subject,actor.display_name,actor.email,actor.avatar_version
 `+postJoins+`JOIN (`+inboxEvents+`) i ON i.post_id=p.id JOIN members actor ON actor.id=i.actor_id
 WHERE ($2::timestamptz IS NULL OR (i.created_at,i.id)<($2::timestamptz,$3))
 ORDER BY i.created_at DESC,i.id DESC LIMIT $4`, memberID, cursorAt, cursorID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item domain.InboxItem
			item.Post, err = scanPost(rows, &item.ID, &item.Kind, &item.CreatedAt, &item.Actor.ID, &item.Actor.PersonID, &item.Actor.Subject, &item.Actor.DisplayName, &item.Actor.Email, &item.Actor.AvatarVersion)
			if err != nil {
				return err
			}
			item.Actor.Pending = item.Actor.Subject == nil
			item.Unread = seenAt == nil || item.CreatedAt.After(*seenAt)
			items = append(items, item)
		}
		return rows.Err()
	})
	if err == nil && len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		nextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.Format(time.RFC3339Nano) + "|" + last.ID))
	}
	return items, nextCursor, seenAt, err
}

func (s *Store) UnreadCount(ctx context.Context, memberID string) (count int, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		return tx.SQL.QueryRowContext(ctx, `SELECT count(*) FROM (`+inboxEvents+`) i
 JOIN members viewer ON viewer.id=$1
 WHERE viewer.inbox_seen_at IS NULL OR i.created_at>viewer.inbox_seen_at`, memberID).Scan(&count)
	})
	return
}

func (s *Store) MarkInboxSeen(ctx context.Context, memberID string, now time.Time) (seenAt time.Time, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error {
		return tx.SQL.QueryRowContext(ctx, `UPDATE members SET inbox_seen_at=GREATEST(COALESCE(inbox_seen_at,'-infinity'::timestamptz),$2)
 WHERE id=$1 RETURNING inbox_seen_at`, memberID, now).Scan(&seenAt)
	})
	return
}
