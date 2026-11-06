package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

// SlackSettingsPatch distinguishes an unchanged channel from an explicitly cleared one.
type SlackSettingsPatch struct {
	AnnounceChannel                                 **domain.SlackChannel
	AnnouncePosts, DMRecipientOnPost, GiveFromSlack *bool
	Locale                                          *domain.SlackLocale
}

func (s *Store) SlackIntegration(ctx context.Context) (out domain.SlackIntegration, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error { out, err = tx.slackIntegration(); return err })
	return
}

func (tx *Tx) slackIntegration() (out domain.SlackIntegration, err error) {
	var teamID, teamName, botUserID, enterpriseID, verifiedBy, channelID, channelName sql.NullString
	var verifiedAt sql.NullTime
	err = tx.SQL.QueryRowContext(tx.Context, `SELECT team_id,team_name,bot_user_id,enterprise_id,verified_at,verified_by,
 announce_channel_id,announce_channel_name,announce_posts,dm_recipient_on_post,give_from_slack,locale,
 last_error_code,last_error_at,updated_at FROM slack_integration WHERE id=1`).Scan(
		&teamID, &teamName, &botUserID, &enterpriseID, &verifiedAt, &verifiedBy,
		&channelID, &channelName, &out.AnnouncePosts, &out.DMRecipientOnPost, &out.GiveFromSlack, &out.Locale,
		&out.LastErrorCode, &out.LastErrorAt, &out.UpdatedAt)
	if err != nil {
		return
	}
	if teamID.Valid {
		out.Workspace = &domain.SlackWorkspace{TeamID: teamID.String, TeamName: teamName.String, BotUserID: botUserID.String, EnterpriseID: enterpriseID.String, VerifiedAt: verifiedAt.Time, VerifiedBy: verifiedBy.String}
	}
	if channelID.Valid {
		out.AnnounceChannel = &domain.SlackChannel{ID: channelID.String, Name: channelName.String}
	}
	return
}

func (s *Store) UpdateSlackSettings(ctx context.Context, patch SlackSettingsPatch, now time.Time) (out domain.SlackIntegration, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error {
		current, err := tx.slackIntegration()
		if err != nil {
			return err
		}
		if patch.AnnounceChannel != nil {
			current.AnnounceChannel = *patch.AnnounceChannel
		}
		if patch.AnnouncePosts != nil {
			current.AnnouncePosts = *patch.AnnouncePosts
		}
		if patch.DMRecipientOnPost != nil {
			current.DMRecipientOnPost = *patch.DMRecipientOnPost
		}
		if patch.GiveFromSlack != nil {
			current.GiveFromSlack = *patch.GiveFromSlack
		}
		if patch.Locale != nil {
			current.Locale = *patch.Locale
		}
		var channelID, channelName *string
		if current.AnnounceChannel != nil {
			channelID, channelName = &current.AnnounceChannel.ID, &current.AnnounceChannel.Name
		}
		if err := tx.Exec(`UPDATE slack_integration SET announce_channel_id=$1,announce_channel_name=$2,
 announce_posts=$3,dm_recipient_on_post=$4,give_from_slack=$5,locale=$6,updated_at=$7 WHERE id=1`,
			channelID, channelName, current.AnnouncePosts, current.DMRecipientOnPost, current.GiveFromSlack, current.Locale, now); err != nil {
			return err
		}
		out, err = tx.slackIntegration()
		return err
	})
	return out, err
}

func (s *Store) FixSlackWorkspace(ctx context.Context, ws domain.SlackWorkspace, force bool, now time.Time) (out domain.SlackIntegration, err error) {
	err = s.Do(ctx, true, func(tx *Tx) error {
		current, err := tx.slackIntegration()
		if err != nil {
			return err
		}
		if current.Workspace != nil && current.Workspace.TeamID != ws.TeamID {
			if !force {
				return domain.ErrSlackTeamChanged
			}
			if err := tx.Exec(`UPDATE members SET slack_user_id=NULL,slack_lookup_at=NULL`); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE slack_integration SET announce_channel_id=NULL,announce_channel_name=NULL WHERE id=1`); err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE slack_integration SET team_id=$1,team_name=$2,bot_user_id=$3,enterprise_id=NULLIF($4,''),
 verified_at=$5,verified_by=NULLIF($6,''),last_error_code=NULL,last_error_at=NULL,updated_at=$5 WHERE id=1`,
			ws.TeamID, ws.TeamName, ws.BotUserID, ws.EnterpriseID, now, ws.VerifiedBy); err != nil {
			return err
		}
		out, err = tx.slackIntegration()
		return err
	})
	return
}

func (s *Store) SetSlackLastError(ctx context.Context, code *string, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		var at *time.Time
		if code != nil {
			at = &now
		}
		return tx.Exec(`UPDATE slack_integration SET last_error_code=$1,last_error_at=$2,updated_at=$3 WHERE id=1`, code, at, now)
	})
}

const slackDeliveryColumns = `id,kind,post_id,clap_id,member_id,announce_id,state,attempts,next_at,claimed_until,channel_id,message_ts,last_error,created_at,sent_at`

func scanSlackDelivery(row scanner) (out domain.SlackDelivery, err error) {
	err = row.Scan(&out.ID, &out.Kind, &out.PostID, &out.ClapID, &out.MemberID, &out.AnnounceID, &out.State, &out.Attempts, &out.NextAt, &out.ClaimedUntil, &out.ChannelID, &out.MessageTS, &out.LastError, &out.CreatedAt, &out.SentAt)
	return
}

func readSlackDeliveries(rows *sql.Rows) ([]domain.SlackDelivery, error) {
	defer rows.Close()
	items := []domain.SlackDelivery{}
	for rows.Next() {
		item, err := scanSlackDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ClaimSlackDeliveries serializes leases with all other writers. Only edits and
// deletions depend on the announcement; recipient DMs are independent deliveries.
func (s *Store) ClaimSlackDeliveries(ctx context.Context, now time.Time, limit int, lease time.Duration) (items []domain.SlackDelivery, err error) {
	items = []domain.SlackDelivery{}
	err = s.Do(ctx, true, func(tx *Tx) error {
		if err := tx.Exec(`UPDATE slack_deliveries d SET state='skipped',last_error='announce_not_sent',claimed_until=NULL
 WHERE d.state='pending' AND d.kind IN ('clap_update','post_delete')
 AND EXISTS (SELECT 1 FROM slack_deliveries a WHERE a.id=d.announce_id AND a.state IN ('skipped','failed'))`); err != nil {
			return err
		}
		rows, err := tx.SQL.QueryContext(ctx, `WITH claimed AS (
 UPDATE slack_deliveries SET claimed_until=$2 WHERE id IN (
 SELECT d.id FROM slack_deliveries d WHERE d.state='pending' AND d.next_at<=$1
 AND (d.claimed_until IS NULL OR d.claimed_until<$1)
 AND (d.kind IN ('post_announce','post_dm') OR EXISTS (
 SELECT 1 FROM slack_deliveries a WHERE a.id=d.announce_id AND a.state='sent' AND a.message_ts IS NOT NULL))
 ORDER BY d.id LIMIT $3) RETURNING `+slackDeliveryColumns+`)
 SELECT `+slackDeliveryColumns+` FROM claimed ORDER BY id`, now, now.Add(lease), limit)
		if err != nil {
			return err
		}
		items, err = readSlackDeliveries(rows)
		return err
	})
	return
}

func (s *Store) MarkSlackDeliverySent(ctx context.Context, id int64, channelID, messageTS string, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		return tx.Exec(`UPDATE slack_deliveries SET state='sent',channel_id=NULLIF($2,''),message_ts=NULLIF($3,''),
 sent_at=$4,claimed_until=NULL,last_error=NULL WHERE id=$1 AND state='pending'`, id, channelID, messageTS, now)
	})
}

func (s *Store) RetrySlackDelivery(ctx context.Context, id int64, nextAt time.Time, lastError string) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		return tx.Exec(`UPDATE slack_deliveries SET attempts=attempts+1,next_at=$2,last_error=$3,claimed_until=NULL,
 state=CASE WHEN attempts+1 >= $4 THEN 'failed' ELSE 'pending' END WHERE id=$1 AND state='pending'`, id, nextAt, lastError, domain.SlackMaxDeliveryAttempts)
	})
}

func (s *Store) SkipSlackDelivery(ctx context.Context, id int64, reason string, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		return tx.Exec(`UPDATE slack_deliveries SET state='skipped',last_error=$2,claimed_until=NULL WHERE id=$1 AND state='pending'`, id, reason)
	})
}

func (s *Store) ListSlackDeliveries(ctx context.Context, state string, limit int) (items []domain.SlackDelivery, err error) {
	items = []domain.SlackDelivery{}
	err = s.Do(ctx, false, func(tx *Tx) error {
		rows, err := tx.SQL.QueryContext(ctx, `SELECT `+slackDeliveryColumns+` FROM slack_deliveries WHERE ($1='' OR state=$1) ORDER BY id DESC LIMIT $2`, state, limit)
		if err != nil {
			return err
		}
		items, err = readSlackDeliveries(rows)
		return err
	})
	return
}

func (s *Store) SlackDeliveryStats(ctx context.Context, now time.Time) (out domain.SlackDeliveryStats, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		// The schema retains the last retry schedule, not a separate failure timestamp.
		return tx.SQL.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE state='pending'),
 count(*) FILTER (WHERE state='failed' AND next_at >= $1),
 count(*) FILTER (WHERE state='sent' AND sent_at >= $1) FROM slack_deliveries`, now.Add(-24*time.Hour)).Scan(&out.Pending, &out.Failed24h, &out.Sent24h)
	})
	return
}

func (s *Store) SlackDeliveryByID(ctx context.Context, id int64) (out domain.SlackDelivery, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		out, err = scanSlackDelivery(tx.SQL.QueryRowContext(ctx, `SELECT `+slackDeliveryColumns+` FROM slack_deliveries WHERE id=$1`, id))
		return err
	})
	return
}

func (s *Store) SetMemberSlackUser(ctx context.Context, memberID string, slackUserID *string, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		return tx.Exec(`UPDATE members SET slack_user_id=$2,slack_lookup_at=$3 WHERE id=$1`, memberID, slackUserID, now)
	})
}

func (s *Store) MemberBySlackUser(ctx context.Context, slackUserID string) (out domain.Member, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		out, err = scanMember(tx.SQL.QueryRowContext(ctx, `SELECT `+memberColumns+` FROM members WHERE slack_user_id=$1`, slackUserID))
		return err
	})
	return
}

func (s *Store) MemberByEmail(ctx context.Context, email string) (out domain.Member, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		out, err = scanMember(tx.SQL.QueryRowContext(ctx, `SELECT `+memberColumns+` FROM members WHERE lower(email)=lower($1) ORDER BY (subject IS NOT NULL) DESC,id LIMIT 1`, email))
		return err
	})
	return
}

var slackMemberPrefixEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (s *Store) SearchMembers(ctx context.Context, q string, excludeID string, limit int) (items []domain.Member, err error) {
	items = []domain.Member{}
	// Treat LIKE metacharacters as literal prefix characters, not wildcards.
	prefix := slackMemberPrefixEscaper.Replace(q) + "%"
	err = s.Do(ctx, false, func(tx *Tx) error {
		rows, err := tx.SQL.QueryContext(ctx, `SELECT `+memberColumns+` FROM members
 WHERE id<>$2 AND (display_name ILIKE $1 OR email ILIKE $1) ORDER BY lower(display_name),id LIMIT $3`, prefix, excludeID, limit)
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

func (s *Store) GiveRemaining(ctx context.Context, memberID string, now time.Time) (out int, err error) {
	err = s.Do(ctx, false, func(tx *Tx) error {
		program, err := tx.program()
		if err != nil {
			return err
		}
		period, err := tx.period(program, now, false)
		if err != nil {
			return err
		}
		out, err = tx.remaining(memberID, period)
		return err
	})
	return
}

func (tx *Tx) enqueueSlackPost(postID, recipientID string, now time.Time) error {
	integration, err := tx.slackIntegration()
	if err != nil {
		return err
	}
	if integration.Delivers(domain.SlackDeliveryPostAnnounce) {
		if err := tx.Exec(`INSERT INTO slack_deliveries (kind,post_id,channel_id,next_at,created_at) VALUES ('post_announce',$1,$2,$3,$3)`, postID, integration.AnnounceChannel.ID, now); err != nil {
			return err
		}
	}
	if integration.Delivers(domain.SlackDeliveryPostDM) {
		return tx.Exec(`INSERT INTO slack_deliveries (kind,post_id,member_id,next_at,created_at) VALUES ('post_dm',$1,$2,$3,$3)`, postID, recipientID, now)
	}
	return nil
}

func (tx *Tx) enqueueSlackClap(postID, clapID string, now time.Time) error {
	integration, err := tx.slackIntegration()
	if err != nil {
		return err
	}
	if !integration.Delivers(domain.SlackDeliveryClapUpdate) {
		return nil
	}
	return tx.Exec(`INSERT INTO slack_deliveries (kind,post_id,clap_id,announce_id,next_at,created_at)
 SELECT 'clap_update',$1,$2,id,$3,$3 FROM slack_deliveries
 WHERE post_id=$1 AND kind='post_announce' AND state IN ('pending','sent') ORDER BY id DESC LIMIT 1
 ON CONFLICT DO NOTHING`, postID, clapID, now)
}

func (tx *Tx) enqueueSlackDelete(postID string, now time.Time) error {
	integration, err := tx.slackIntegration()
	if err != nil {
		return err
	}
	if integration.Delivers(domain.SlackDeliveryPostDelete) {
		if err := tx.Exec(`INSERT INTO slack_deliveries (kind,post_id,announce_id,next_at,created_at)
 SELECT 'post_delete',$1,id,$2,$2 FROM slack_deliveries
 WHERE post_id=$1 AND kind='post_announce' AND state IN ('pending','sent') ORDER BY id DESC LIMIT 1`, postID, now); err != nil {
			return err
		}
	}
	return tx.Exec(`UPDATE slack_deliveries SET state='skipped',last_error='post_deleted',claimed_until=NULL
 WHERE post_id=$1 AND state='pending' AND kind IN ('post_announce','post_dm','clap_update')`, postID)
}
