package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"
)

func (s *Store) CreatePost(ctx context.Context, in CreatePostInput) (out domain.Post, replayed bool, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error { out, replayed, err = tx.createPost(in); return err })
	return
}
func (tx *Tx) replay(memberID, key string, now time.Time) (postID string, clapID *string, err error) {
	if length := utf8.RuneCountInString(key); length < 1 || length > 128 {
		return "", nil, &domain.ValidationError{Field: "idempotencyKey", Msg: "must contain 1 to 128 characters"}
	}
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT post_id, clap_id FROM idempotency_keys WHERE member_id=$1 AND key=$2 AND created_at >= $3`, memberID, key, now.Add(-24*time.Hour)).Scan(&postID, &clapID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}
func (tx *Tx) remember(memberID, key, postID string, clapID *string, now time.Time) error {
	if err := tx.Exec(`DELETE FROM idempotency_keys WHERE member_id=$1 AND created_at < $2`, memberID, now.Add(-24*time.Hour)); err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO idempotency_keys (member_id,key,post_id,clap_id,created_at) VALUES ($1,$2,$3,$4,$5)`, memberID, key, postID, clapID, now)
}
func (tx *Tx) createPost(in CreatePostInput) (domain.Post, bool, error) {
	postID, clapID, err := tx.replay(in.SenderID, in.IdempotencyKey, in.Now)
	if err != nil {
		return domain.Post{}, false, err
	}
	if postID != "" {
		if clapID != nil {
			return domain.Post{}, false, &domain.ValidationError{Field: "idempotencyKey", Msg: "already used for a different action"}
		}
		post, err := tx.post(postID, in.SenderID, true)
		return post, true, err
	}
	if in.SenderID == in.RecipientID {
		return domain.Post{}, false, domain.ErrSelfTarget
	}
	if _, err := tx.member(in.SenderID); err != nil {
		return domain.Post{}, false, err
	}
	if _, err := tx.member(in.RecipientID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = domain.ErrRecipientNotFound
		}
		return domain.Post{}, false, err
	}
	program, err := tx.program()
	if err != nil {
		return domain.Post{}, false, err
	}
	period, err := tx.period(program, in.Now, true)
	if err != nil {
		return domain.Post{}, false, err
	}
	if err := domain.ValidatePoints(in.Points, period.MaxPointsPerPost); err != nil {
		return domain.Post{}, false, err
	}
	message, err := domain.ValidateMessage(in.Message)
	if err != nil {
		return domain.Post{}, false, err
	}
	if err := domain.ValidateValues(in.Values, program); err != nil {
		return domain.Post{}, false, err
	}
	loc, err := time.LoadLocation(program.TimeZone)
	if err != nil {
		return domain.Post{}, false, err
	}
	month := domain.MonthKeyFor(in.Now, loc)
	if err := tx.requireOpen(month); err != nil {
		return domain.Post{}, false, err
	}
	remaining, err := tx.remaining(in.SenderID, period)
	if err != nil {
		return domain.Post{}, false, err
	}
	if remaining < in.Points {
		return domain.Post{}, false, &domain.InsufficientPointsError{Remaining: remaining, Required: in.Points}
	}
	postID = ulid.Make().String()
	if in.Values == nil {
		in.Values = []string{}
	}
	if err := tx.Exec(`INSERT INTO posts (id,period_key,month_key,sender_id,recipient_id,points,message,value_keys,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, postID, period.Key, month, in.SenderID, in.RecipientID, in.Points, message, pq.Array(in.Values), in.Now); err != nil {
		return domain.Post{}, false, err
	}
	if err := tx.Exec(`INSERT INTO ledger (member_id,period_key,month_key,account,delta,kind,post_id,created_at) VALUES
 ($1,$3,$4,'give',-$5::integer,'send',$6,$7), ($2,$3,$4,'receive',$5::integer,'receive',$6,$7)`, in.SenderID, in.RecipientID, period.Key, month, in.Points, postID, in.Now); err != nil {
		return domain.Post{}, false, err
	}
	if err := tx.remember(in.SenderID, in.IdempotencyKey, postID, nil, in.Now); err != nil {
		return domain.Post{}, false, err
	}
	if err := tx.enqueueSlackPost(postID, in.RecipientID, in.Now); err != nil {
		return domain.Post{}, false, err
	}
	post, err := tx.post(postID, in.SenderID, false)
	return post, false, err
}
func (s *Store) CreateClap(ctx context.Context, in CreateClapInput) (clap domain.Clap, post domain.Post, replayed bool, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error { clap, post, replayed, err = tx.createClap(in); return err })
	return
}
func (tx *Tx) createClap(in CreateClapInput) (domain.Clap, domain.Post, bool, error) {
	postID, clapID, err := tx.replay(in.MemberID, in.IdempotencyKey, in.Now)
	if err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if postID != "" {
		if clapID == nil || postID != in.PostID {
			return domain.Clap{}, domain.Post{}, false, &domain.ValidationError{Field: "idempotencyKey", Msg: "already used for a different action"}
		}
		clap, err := tx.clap(*clapID)
		if err != nil {
			return domain.Clap{}, domain.Post{}, false, err
		}
		post, err := tx.post(postID, in.MemberID, true)
		return clap, post, true, err
	}
	post, err := tx.post(in.PostID, in.MemberID, false)
	if err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if post.Sender.ID == in.MemberID || post.Recipient.ID == in.MemberID {
		return domain.Clap{}, domain.Post{}, false, domain.ErrOwnPost
	}
	if _, err := tx.member(in.MemberID); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	program, err := tx.program()
	if err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	loc, err := time.LoadLocation(program.TimeZone)
	if err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	month := domain.MonthKeyFor(in.Now, loc)
	if err := tx.requireOpen(post.MonthKey); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if err := tx.requireOpen(month); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	period, err := tx.period(program, in.Now, true)
	if err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	remaining, err := tx.remaining(in.MemberID, period)
	if err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if remaining < domain.ClapCost {
		return domain.Clap{}, domain.Post{}, false, &domain.InsufficientPointsError{Remaining: remaining, Required: domain.ClapCost}
	}
	id := ulid.Make().String()
	if err := tx.Exec(`INSERT INTO claps (id,post_id,member_id,period_key,month_key,idempotency_key,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, in.PostID, in.MemberID, period.Key, month, in.IdempotencyKey, in.Now); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if err := tx.Exec(`INSERT INTO ledger (member_id,period_key,month_key,account,delta,kind,post_id,clap_id,created_at) VALUES
 ($1,$4,$5,'give',-2,'clap_give',$6,$7,$8), ($2,$4,$5,'receive',1,'clap_sender',$6,$7,$8), ($3,$4,$5,'receive',1,'clap_recipient',$6,$7,$8)`, in.MemberID, post.Sender.ID, post.Recipient.ID, period.Key, month, post.ID, id, in.Now); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if err := tx.Exec(`UPDATE posts SET clap_count=clap_count+1 WHERE id=$1`, post.ID); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if err := tx.remember(in.MemberID, in.IdempotencyKey, post.ID, &id, in.Now); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	if err := tx.enqueueSlackClap(post.ID, id, in.Now); err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	clap, err := tx.clap(id)
	if err != nil {
		return domain.Clap{}, domain.Post{}, false, err
	}
	post, err = tx.post(post.ID, in.MemberID, false)
	return clap, post, false, err
}
func (s *Store) DeletePost(ctx context.Context, id, adminMemberID string, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		post, err := tx.post(id, "", false)
		if err != nil {
			return err
		}
		closed, err := tx.monthClosed(post.MonthKey)
		if err != nil {
			return err
		}
		if !closed {
			// Sequential close makes this impossible for valid chronology. Still check
			// every affected month before appending reversals, never just the post month.
			var anyClosed bool
			if err := tx.SQL.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM ledger l JOIN statements s ON s.month_key=l.month_key WHERE l.post_id=$1)`, id).Scan(&anyClosed); err != nil {
				return err
			}
			if anyClosed {
				return domain.ErrPeriodClosed
			}
			if err := tx.Exec(`INSERT INTO ledger (member_id,period_key,month_key,account,delta,kind,post_id,clap_id,reverses_id,created_at)
 SELECT member_id,period_key,month_key,account,-delta,'reversal',post_id,clap_id,id,$2 FROM ledger WHERE post_id=$1 AND kind<>'reversal'`, id, now); err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE posts SET deleted_at=$2, deleted_by=$3, reversed=$4 WHERE id=$1`, id, now, adminMemberID, !closed); err != nil {
			return err
		}
		return tx.enqueueSlackDelete(id, now)
	})
}
