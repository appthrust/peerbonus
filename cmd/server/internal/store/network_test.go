package store

import (
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestNetworkPairsFactsAndReversal(t *testing.T) {
	db, m := fixture(t)
	idle, err := db.UpsertMember(t.Context(), MemberInput{PersonID: "idle", DisplayName: "Idle", Pending: true}, october, false)
	require.NoError(t, err)
	first := send(t, db, m[0], m[1], 10, october)
	second := send(t, db, m[1], m[2], 20, october.Add(time.Hour))
	clap(t, db, first, m[2], october.Add(2*time.Hour))
	clap(t, db, second, m[0], october.Add(3*time.Hour))
	now := october.Add(4 * time.Hour)
	facts, err := db.Network(t.Context(), "2026-10", now)
	require.NoError(t, err)
	require.Equal(t, 2, facts.Posts)
	require.Equal(t, 2, facts.Claps)
	require.True(t, now.Equal(facts.AsOf))
	require.Equal(t, "2026-10-18T15:00:00Z", facts.RecentSince.UTC().Format(time.RFC3339))
	require.Len(t, facts.Members, 4)
	require.Zero(t, facts.Members[idle.ID].Connections)
	require.Len(t, facts.Edges, 3)
	pairs := make(map[[2]string]domain.NetworkMemberEdge)
	for _, edge := range facts.Edges {
		require.Less(t, edge.From, edge.To)
		pairs[[2]string{edge.From, edge.To}] = edge
	}
	edge := func(a, b string) domain.NetworkMemberEdge { return pairs[[2]string{min(a, b), max(a, b)}] }
	require.Equal(t, domain.NetworkMemberEdge{From: min(m[0].ID, m[1].ID), To: max(m[0].ID, m[1].ID), Posts: 1, Claps: 1, Points: 10, LastAt: october.Add(3 * time.Hour).UTC()}, edge(m[0].ID, m[1].ID))
	require.Equal(t, 2, edge(m[0].ID, m[2].ID).Claps)
	require.Zero(t, edge(m[0].ID, m[2].ID).Points)
	require.Equal(t, 20, edge(m[1].ID, m[2].ID).Points)
	for _, member := range m {
		require.Equal(t, 2, facts.Members[member.ID].Connections)
	}
	require.Equal(t, 1, facts.Members[m[0].ID].SentPosts)
	require.Equal(t, 1, facts.Members[m[0].ID].ClapsGiven)
	require.Equal(t, 1, facts.Members[m[0].ID].ClapsReceived)
	require.Equal(t, 12, facts.Members[m[1].ID].ReceivedPoints, "receive ledger includes clap credits")
	require.Equal(t, 2, facts.Members[m[1].ID].ClapsReceived)
	require.Equal(t, 1, facts.Members[m[2].ID].ReceivedPosts)
	require.Zero(t, facts.Members[m[2].ID].UnclappedPosts)

	require.NoError(t, db.DeletePost(t.Context(), second.ID, m[0].ID, now))
	facts, err = db.Network(t.Context(), "2026-10", now)
	require.NoError(t, err)
	require.Equal(t, 1, facts.Posts)
	require.Equal(t, 1, facts.Claps)
	require.Len(t, facts.Edges, 3, "the remaining clap still connects its clapper to both endpoints")
	for _, edge := range facts.Edges {
		require.True(t, edge.LastAt.Before(october.Add(3*time.Hour)))
	}
	require.Equal(t, domain.NetworkActivity{Recent: 1}, facts.Members[m[0].ID].Activity)
	require.Zero(t, facts.Members[m[2].ID].ReceivedPosts)
	require.NoError(t, db.DeletePost(t.Context(), first.ID, m[0].ID, now))
	facts, err = db.Network(t.Context(), "2026-10", now)
	require.NoError(t, err)
	require.Empty(t, facts.Edges)
	require.Zero(t, facts.Posts+facts.Claps)
	for _, row := range facts.Members {
		require.Zero(t, row.Connections+row.Activity.Recent+row.ClapsReceived)
	}
}

func TestNetworkActivityWindowsAndMonthClock(t *testing.T) {
	db, m := fixture(t)
	now := instant("2026-10-20T09:00:00Z")
	recent := now.Add(-domain.DeclineWindow)
	previous := recent.Add(-domain.DeclineWindow)
	for _, at := range []time.Time{previous.Add(-time.Second), previous, recent.Add(-time.Second), recent, now, now.Add(time.Second)} {
		send(t, db, m[0], m[1], 1, at)
	}
	post := send(t, db, m[1], m[0], 2, previous)
	clap(t, db, post, m[2], recent.Add(-time.Second))
	clap(t, db, post, m[2], recent)
	clap(t, db, post, m[2], now)
	facts, err := db.Network(t.Context(), "2026-10", now)
	require.NoError(t, err)
	require.Equal(t, domain.NetworkActivity{Recent: 2, Previous: 2}, facts.Members[m[0].ID].Activity)
	require.Equal(t, domain.NetworkActivity{Previous: 1}, facts.Members[m[1].ID].Activity)
	require.Equal(t, domain.NetworkActivity{Recent: 2, Previous: 1}, facts.Members[m[2].ID].Activity)
	require.Equal(t, 4, facts.Members[m[1].ID].UnclappedPosts)
	require.Equal(t, 3, facts.Claps, "claps belong to the clap month, even on a September post")
	send(t, db, m[2], m[1], 1, instant("2026-10-01T00:00:00+09:00"))
	past, err := db.Network(t.Context(), "2026-09", now)
	require.NoError(t, err)
	require.Equal(t, "2026-09-30T14:59:59.999999999Z", past.AsOf.UTC().Format(time.RFC3339Nano))
	require.Nil(t, past.RecentSince)
	require.Equal(t, domain.NetworkActivity{Recent: 2}, past.Members[m[0].ID].Activity)
	require.Zero(t, past.Members[m[2].ID].Activity.Recent, "month-end nanoseconds must not round up to a next-month action")
	_, err = db.Network(t.Context(), "2026-13", now)
	require.ErrorIs(t, err, domain.ErrInvalidMonth)
	// Tokyo has already entered November while UTC is still in October.
	boundary, err := db.Network(t.Context(), "2026-11", instant("2026-10-31T15:00:00Z"))
	require.NoError(t, err)
	require.NotNil(t, boundary.RecentSince)
	require.Equal(t, "2026-10-25T15:00:00Z", boundary.RecentSince.UTC().Format(time.RFC3339))
}
