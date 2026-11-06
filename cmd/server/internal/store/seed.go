package store

import (
	"context"
	"fmt"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

// SeedDev creates the entire example only in an empty installation, under the
// same lock as real writers. Every demonstration point uses the real ledger path.
func (s *Store) SeedDev(ctx context.Context, now time.Time) error {
	return s.Do(ctx, true, func(tx *Tx) error {
		var exists bool
		if err := tx.SQL.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM members)`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		program, err := tx.program()
		if err != nil {
			return err
		}
		loc, err := time.LoadLocation(program.TimeZone)
		if err != nil {
			return err
		}
		_, start, _ := domain.PeriodFor(now, loc)
		previous := start.AddDate(0, 0, -7)
		members := make([]domain.Member, 6)
		for i, name := range []string{"Aoi Tanaka", "Ren Suzuki", "Hana Sato", "Yui Takahashi", "Sora Ito", "Kai Watanabe"} {
			subject := fmt.Sprintf("peerbonus-demo-%d", i)
			input := MemberInput{PersonID: fmt.Sprintf("peerbonus-demo-person-%d", i), Subject: &subject, DisplayName: name, Email: fmt.Sprintf("peerbonus%d@example.test", i)}
			if i == 5 {
				input.Subject = nil
				input.Pending = true
			}
			member, err := tx.upsertMember(input, now, i != 5)
			if err != nil {
				return err
			}
			members[i] = member
		}
		messages := []string{
			"Thank you for helping the team ship a thoughtful improvement.",
			"Your clear explanation made a difficult problem much easier to solve.",
			"Thanks for welcoming our new teammate and sharing what you know.",
			"The extra care you put into the customer experience really shows.",
			"Thank you for stepping in and helping us finish together.",
		}
		for i := range 15 {
			at := previous.Add(time.Duration(i) * start.Sub(previous) / 7)
			if i >= 7 {
				at = start.Add(time.Duration(i-7) * now.Sub(start) / 8)
			}
			points := 10 + (i%5)*5
			if points > program.MaxPointsPerPost {
				points = program.MaxPointsPerPost
			}
			post, _, err := tx.createPost(CreatePostInput{SenderID: members[i%6].ID, RecipientID: members[(i+1)%6].ID, Points: points, Message: messages[i%len(messages)], Now: at, IdempotencyKey: fmt.Sprintf("peerbonus-demo-post-%d", i)})
			if err != nil {
				return err
			}
			if i%2 == 0 {
				if _, _, _, err := tx.createClap(CreateClapInput{PostID: post.ID, MemberID: members[(i+2)%6].ID, Now: at, IdempotencyKey: fmt.Sprintf("peerbonus-demo-clap-%d", i)}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
