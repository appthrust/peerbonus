package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"
)

type CreateSlackPostsInput struct {
	SenderID     string
	RecipientIDs []string
	Points       int
	Message      string
	Values       []string
	Now          time.Time
	Source       domain.SlackGiveSource
	PendingID    string
}

type CreateSlackClapInput struct {
	PostID, MemberID string
	Now              time.Time
	Source           domain.SlackGiveSource
}

// CreateSlackPosts atomically gives to all recipients using the web ledger path.
// Any existing give for the source and sender replays the entire original batch,
// even if the requested recipients changed. Replay never consumes a pending give.
func (s *Store) CreateSlackPosts(ctx context.Context, in CreateSlackPostsInput) (posts []domain.Post, replayed bool, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error {
		posts, err = tx.slackGivenPosts(in.Source, in.SenderID)
		if err != nil {
			return err
		}
		if len(posts) > 0 {
			replayed = true
			return nil
		}
		if in.PendingID != "" {
			var id string
			err := tx.SQL.QueryRowContext(ctx, `DELETE FROM slack_pending_gives WHERE id=$1 AND sender_id=$2 AND expires_at>$3 RETURNING id`, in.PendingID, in.SenderID, in.Now).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrSlackPendingGiveGone
			}
			if err != nil {
				return err
			}
		}
		if len(in.RecipientIDs) < 1 || len(in.RecipientIDs) > domain.SlackMaxMentionRecipients {
			return &domain.ValidationError{Field: "recipientIds", Msg: "must contain 1 to 5 recipients"}
		}
		for i, id := range in.RecipientIDs {
			if slices.Contains(in.RecipientIDs[:i], id) {
				return &domain.ValidationError{Field: "recipientIds", Msg: "must be distinct"}
			}
		}
		ids, err := tx.writePosts(in.SenderID, in.RecipientIDs, in.Points, in.Message, in.Values, in.Now, true)
		if err != nil {
			return err
		}
		posts = make([]domain.Post, len(ids))
		for i, id := range ids {
			if err := tx.rememberSlackGive(in.Source, in.SenderID, in.RecipientIDs[i], id, nil, in.Now); err != nil {
				return err
			}
			posts[i], err = tx.post(id, in.SenderID, false)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return
}

func (tx *Tx) slackGivenPosts(source domain.SlackGiveSource, senderID string) ([]domain.Post, error) {
	rows, err := tx.SQL.QueryContext(tx.Context, postSelect+`JOIN slack_gives g ON g.post_id=p.id
 WHERE g.channel_id=$2 AND g.message_ts=$3 AND g.sender_id=$1 AND g.emoji=$4 ORDER BY g.created_at,p.id`, senderID, source.ChannelID, source.MessageTS, source.Emoji)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	posts := []domain.Post{}
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

// CreateSlackClap uses the same validation, ledger and outbox as a web clap,
// but its replay record is permanent rather than an expiring idempotency key.
func (s *Store) CreateSlackClap(ctx context.Context, in CreateSlackClapInput) (clap domain.Clap, post domain.Post, replayed bool, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error {
		var postID string
		var clapID *string
		err := tx.SQL.QueryRowContext(ctx, `SELECT post_id,clap_id FROM slack_gives
 WHERE channel_id=$1 AND message_ts=$2 AND sender_id=$3 AND emoji=$4
 AND recipient_id=(SELECT recipient_id FROM posts WHERE id=$5)`, in.Source.ChannelID, in.Source.MessageTS, in.MemberID, in.Source.Emoji, in.PostID).Scan(&postID, &clapID)
		if err == nil {
			if clapID == nil || postID != in.PostID {
				return &domain.ValidationError{Field: "source", Msg: "already used for a different action"}
			}
			clap, err = tx.clap(*clapID)
			if err != nil {
				return err
			}
			post, err = tx.post(postID, in.MemberID, true)
			replayed = true
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		clap, post, err = tx.writeClap(CreateClapInput{PostID: in.PostID, MemberID: in.MemberID, Now: in.Now})
		if err != nil {
			return err
		}
		return tx.rememberSlackGive(in.Source, in.MemberID, post.Recipient.ID, post.ID, &clap.ID, in.Now)
	})
	return
}

func (tx *Tx) rememberSlackGive(source domain.SlackGiveSource, senderID, recipientID, postID string, clapID *string, now time.Time) error {
	return tx.Exec(`INSERT INTO slack_gives (channel_id,message_ts,sender_id,recipient_id,emoji,post_id,clap_id,permalink,created_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, source.ChannelID, source.MessageTS, senderID, recipientID, source.Emoji, postID, clapID, source.Permalink, now)
}

func (s *Store) PostIDBySlackAnnounce(ctx context.Context, channelID, messageTS string) (id string, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		return tx.SQL.QueryRowContext(ctx, `SELECT post_id FROM slack_deliveries WHERE channel_id=$1 AND message_ts=$2 AND kind='post_announce' AND state='sent' ORDER BY id DESC LIMIT 1`, channelID, messageTS).Scan(&id)
	})
	return
}

const slackPendingGiveColumns = `id,channel_id,message_ts,thread_ts,sender_id,recipient_ids,message,expires_at`

func scanSlackPendingGive(row scanner) (out domain.SlackPendingGive, err error) {
	err = row.Scan(&out.ID, &out.ChannelID, &out.MessageTS, &out.ThreadTS, &out.SenderID, pq.Array(&out.RecipientIDs), &out.Message, &out.ExpiresAt)
	return
}

// CreateSlackPendingGive preserves the first unexpired mention at a channel/ts.
// It also prunes expired prompts; IDs and expiry are assigned by the store.
func (s *Store) CreateSlackPendingGive(ctx context.Context, in domain.SlackPendingGive, now time.Time) (out domain.SlackPendingGive, created bool, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error {
		if err := tx.Exec(`DELETE FROM slack_pending_gives WHERE expires_at<=$1`, now); err != nil {
			return err
		}
		out, err = scanSlackPendingGive(tx.SQL.QueryRowContext(ctx, `SELECT `+slackPendingGiveColumns+` FROM slack_pending_gives WHERE channel_id=$1 AND message_ts=$2`, in.ChannelID, in.MessageTS))
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		out, err = scanSlackPendingGive(tx.SQL.QueryRowContext(ctx, `INSERT INTO slack_pending_gives (`+slackPendingGiveColumns+`)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+slackPendingGiveColumns, ulid.Make().String(), in.ChannelID, in.MessageTS, in.ThreadTS, in.SenderID, pq.Array(in.RecipientIDs), in.Message, now.Add(domain.SlackPendingGiveTTL)))
		created = err == nil
		return err
	})
	return
}

func (s *Store) SlackPendingGive(ctx context.Context, id string, now time.Time) (out domain.SlackPendingGive, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		out, err = scanSlackPendingGive(tx.SQL.QueryRowContext(ctx, `SELECT `+slackPendingGiveColumns+` FROM slack_pending_gives WHERE id=$1 AND expires_at>$2`, id, now))
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrSlackPendingGiveGone
		}
		return err
	})
	return
}

func (s *Store) DeleteSlackPendingGive(ctx context.Context, id, senderID string) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		var deleted string
		err := tx.SQL.QueryRowContext(ctx, `DELETE FROM slack_pending_gives WHERE id=$1 AND sender_id=$2 RETURNING id`, id, senderID).Scan(&deleted)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrSlackPendingGiveGone
		}
		return err
	})
}

func (s *Store) LinkMemberSlackUser(ctx context.Context, memberID, slackUserID string, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		if _, err := tx.member(memberID); err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE members SET slack_user_id=NULL,slack_lookup_at=NULL WHERE slack_user_id=$1 AND id<>$2`, slackUserID, memberID); err != nil {
			return err
		}
		return tx.Exec(`UPDATE members SET slack_user_id=$2,slack_lookup_at=$3 WHERE id=$1`, memberID, slackUserID, now)
	})
}
