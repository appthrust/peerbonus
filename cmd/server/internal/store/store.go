// Package store owns Peer Bonus's Postgres schema and serialized ledger writes.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"time"

	"github.com/appthrust/appserver-go/appserver/pgstore"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ *pgstore.Store }
type Tx struct{ *pgstore.Tx }
type MemberInput struct {
	PersonID           string
	Subject            *string
	DisplayName, Email string
	Pending            bool
	AvatarVersion      *time.Time
	DirectorySyncedAt  *time.Time
}
type CreatePostInput struct {
	SenderID, RecipientID string
	Points                int
	Message               string
	Values                []string
	Now                   time.Time
	IdempotencyKey        string
}
type CreateClapInput struct {
	PostID, MemberID string
	Now              time.Time
	IdempotencyKey   string
}
type PostFilter struct {
	Limit                                              int
	Cursor                                             string
	MemberID, SentBy, ReceivedBy, Value, Period, Month string
	Q                                                  string
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	database, err := pgstore.Open(ctx, dsn, migrations, "peerbonus_schema_migrations")
	if err != nil {
		return nil, err
	}
	return &Store{Store: database}, nil
}
func (s *Store) Do(ctx context.Context, write bool, fn func(*Tx) error) error {
	err := s.Store.Do(ctx, write, func(tx *pgstore.Tx) error { return fn(&Tx{Tx: tx}) })
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

type scanner interface{ Scan(...any) error }

func (tx *Tx) monthClosed(month string) (bool, error) {
	var closed bool
	err := tx.SQL.QueryRowContext(tx.Context, `SELECT EXISTS (SELECT 1 FROM statements WHERE month_key=$1)`, month).Scan(&closed)
	return closed, err
}
func (tx *Tx) requireOpen(month string) error {
	closed, err := tx.monthClosed(month)
	if err != nil {
		return err
	}
	if closed {
		return domain.ErrPeriodClosed
	}
	return nil
}
