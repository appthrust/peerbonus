package store

import (
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRecentRecipientsDistinctVisibleAndNewestFirst(t *testing.T) {
	db, members := fixture(t)
	pending, err := db.UpsertMember(t.Context(), MemberInput{PersonID: "pending", DisplayName: "Pending", Email: "pending@example.test", Pending: true}, october, false)
	require.NoError(t, err)
	send(t, db, members[0], members[1], 1, october)
	send(t, db, members[0], members[2], 1, october.Add(time.Minute))
	send(t, db, members[0], members[1], 1, october.Add(2*time.Minute))
	deleted := send(t, db, members[0], pending, 1, october.Add(3*time.Minute))
	send(t, db, members[2], pending, 1, october.Add(4*time.Minute))
	require.NoError(t, db.DeletePost(t.Context(), deleted.ID, members[0].ID, october.Add(5*time.Minute)))

	recent, err := db.RecentRecipients(t.Context(), members[0].ID, 8)
	require.NoError(t, err)
	require.Equal(t, []domain.Member{members[1], members[2]}, recent)
	recent, err = db.RecentRecipients(t.Context(), members[0].ID, 1)
	require.NoError(t, err)
	require.Equal(t, []domain.Member{members[1]}, recent)
	recent, err = db.RecentRecipients(t.Context(), members[1].ID, 8)
	require.NoError(t, err)
	require.Equal(t, []domain.Member{}, recent)

	// Equal timestamps retain the same descending post-ID ordering as the feed.
	first := send(t, db, members[0], pending, 1, october.Add(6*time.Minute))
	last := send(t, db, members[0], members[2], 1, october.Add(6*time.Minute))
	recent, err = db.RecentRecipients(t.Context(), members[0].ID, 8)
	require.NoError(t, err)
	want := []domain.Member{members[2], pending, members[1]}
	if first.ID > last.ID {
		want[0], want[1] = want[1], want[0]
	}
	require.Equal(t, want, recent)
	require.True(t, pending.Pending)
}
