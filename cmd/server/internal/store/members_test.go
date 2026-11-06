package store

import (
	"testing"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestUpsertMemberAvatarVersion(t *testing.T) {
	db, _ := fixture(t)
	version := october.UTC()
	input := MemberInput{PersonID: "avatar-person", Subject: new("avatar-subject"), DisplayName: "Picture", Email: "picture@example.test", AvatarVersion: &version}
	member, err := db.UpsertMember(t.Context(), input, october, true)
	require.NoError(t, err)
	require.NotNil(t, member.AvatarVersion)
	require.True(t, version.Equal(*member.AvatarVersion))
	updated := version.Add(time.Hour)
	input.AvatarVersion = &updated
	again, err := db.UpsertMember(t.Context(), input, october, false)
	require.NoError(t, err)
	require.Equal(t, member.ID, again.ID)
	for _, lookup := range []func() (domain.Member, error){
		func() (domain.Member, error) { return db.MemberByID(t.Context(), member.ID) },
		func() (domain.Member, error) { return db.MemberByPersonID(t.Context(), input.PersonID) },
		func() (domain.Member, error) { return db.MemberBySubject(t.Context(), *input.Subject) },
	} {
		got, err := lookup()
		require.NoError(t, err)
		require.NotNil(t, got.AvatarVersion)
		require.True(t, updated.Equal(*got.AvatarVersion))
	}
	input.AvatarVersion = nil
	cleared, err := db.UpsertMember(t.Context(), input, october, false)
	require.NoError(t, err)
	require.Nil(t, cleared.AvatarVersion, "removing a Directory picture must clear the version")
	require.NoError(t, db.UpdateMemberProfiles(t.Context(), []MemberInput{{PersonID: input.PersonID, DisplayName: input.DisplayName, Email: input.Email, AvatarVersion: &version}}))
	refreshed, err := db.MemberByID(t.Context(), member.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.AvatarVersion)
	require.True(t, version.Equal(*refreshed.AvatarVersion))
	require.NoError(t, db.UpdateMemberProfiles(t.Context(), []MemberInput{{PersonID: input.PersonID, DisplayName: input.DisplayName, Email: input.Email}}))
	refreshed, err = db.MemberByID(t.Context(), member.ID)
	require.NoError(t, err)
	require.Nil(t, refreshed.AvatarVersion)
}

func TestAvatarVersionAcrossMemberProjections(t *testing.T) {
	db, members := fixture(t)
	version := october.UTC()
	for _, member := range members {
		require.NoError(t, db.UpdateMemberProfiles(t.Context(), []MemberInput{{PersonID: member.PersonID, DisplayName: member.DisplayName, Email: member.Email, AvatarVersion: &version}}))
	}
	assertPicture := func(member domain.Member) {
		t.Helper()
		require.NotNil(t, member.AvatarVersion, "person %s", member.PersonID)
		require.True(t, version.Equal(*member.AvatarVersion), "person %s", member.PersonID)
	}
	post := send(t, db, members[0], members[1], 20, october)
	assertPicture(post.Sender)
	assertPicture(post.Recipient)
	clapped := clap(t, db, post, members[2], october.Add(time.Minute))
	assertPicture(clapped.Member)
	posts, _, err := db.ListPosts(t.Context(), PostFilter{}, members[0].ID)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	assertPicture(posts[0].Sender)
	assertPicture(posts[0].Recipient)
	require.Len(t, posts[0].Clappers, 1)
	assertPicture(posts[0].Clappers[0])
	claps, err := db.ListClaps(t.Context(), post.ID, 10)
	require.NoError(t, err)
	require.Len(t, claps, 1)
	assertPicture(claps[0].Member)
	inbox, _, _, err := db.ListInbox(t.Context(), members[1].ID, 10, "")
	require.NoError(t, err)
	require.Len(t, inbox, 2)
	for _, item := range inbox {
		assertPicture(item.Actor)
		assertPicture(item.Post.Sender)
		assertPicture(item.Post.Recipient)
	}
	recent, err := db.RecentRecipients(t.Context(), members[0].ID, 8)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	assertPicture(recent[0])
	stats, err := db.MemberStats(t.Context(), members[0].ID, october)
	require.NoError(t, err)
	require.Len(t, stats.SentTo, 1)
	assertPicture(stats.SentTo[0].Member)
	activity, err := db.ActivityByMember(t.Context(), "2026-10")
	require.NoError(t, err)
	for _, row := range activity {
		assertPicture(*row.Member)
	}
	preview, err := db.Statement(t.Context(), "2026-10", november)
	require.NoError(t, err)
	for _, line := range preview.Lines {
		assertPicture(line.Member)
	}
	closed, err := db.CloseStatement(t.Context(), "2026-10", november, members[0].ID, true)
	require.NoError(t, err)
	assertPicture(*closed.ClosedBy)
	for _, line := range closed.Lines {
		assertPicture(line.Member)
	}
}

func TestUpsertMemberDirectorySyncedAt(t *testing.T) {
	db, _ := fixture(t)
	now := october.UTC()
	in := MemberInput{PersonID: "synced", Subject: new("synced-subject"), DisplayName: "Synced", Email: "synced@example.test", DirectorySyncedAt: &now}
	member, err := db.UpsertMember(t.Context(), in, now, true)
	require.NoError(t, err)
	require.Equal(t, &now, member.DirectorySyncedAt)

	later := now.Add(time.Hour)
	in.DirectorySyncedAt = &later
	member, err = db.UpsertMember(t.Context(), in, later, false)
	require.NoError(t, err)
	require.Equal(t, &later, member.DirectorySyncedAt)
	require.Equal(t, &now, member.LastSeenAt)

	// A cached /me upsert still touches last_seen_at, but must not claim a
	// fresh Directory snapshot or erase its last successful refresh time.
	in.DirectorySyncedAt = nil
	seen := later.Add(time.Hour)
	member, err = db.UpsertMember(t.Context(), in, seen, true)
	require.NoError(t, err)
	require.Equal(t, &later, member.DirectorySyncedAt)
	require.Equal(t, &seen, member.LastSeenAt)
}

func TestStaleMembersAndRefresh(t *testing.T) {
	db, members := fixture(t)
	now := october.UTC()
	recent := now.Add(-time.Hour)
	require.NoError(t, db.UpdateMemberProfiles(t.Context(), []MemberInput{{
		PersonID: members[2].PersonID, DisplayName: members[2].DisplayName, Email: members[2].Email, DirectorySyncedAt: &recent,
	}}))
	oldTime := now.Add(-25 * time.Hour)
	old, err := db.UpsertMember(t.Context(), MemberInput{PersonID: "old", Subject: new("old-subject"), DisplayName: "Old", Email: "old@example.test", AvatarVersion: &oldTime, DirectorySyncedAt: &oldTime}, now, true)
	require.NoError(t, err)
	boundary := now.Add(-24 * time.Hour)
	_, err = db.UpsertMember(t.Context(), MemberInput{PersonID: "boundary", DirectorySyncedAt: &boundary}, now, false)
	require.NoError(t, err)
	missing, err := db.UpsertMember(t.Context(), MemberInput{PersonID: members[1].PersonID, Subject: members[1].Subject, DisplayName: members[1].DisplayName, Email: members[1].Email, AvatarVersion: &oldTime}, now, true)
	require.NoError(t, err)

	stale, err := db.StaleMembers(t.Context(), boundary, 100)
	require.NoError(t, err)
	require.Len(t, stale, 3)
	require.Nil(t, stale[0].DirectorySyncedAt)
	require.Nil(t, stale[1].DirectorySyncedAt)
	require.ElementsMatch(t, []string{members[0].ID, missing.ID}, []string{stale[0].ID, stale[1].ID})
	require.Equal(t, old, stale[2], "NULL snapshots precede the oldest successful refresh")
	limited, err := db.StaleMembers(t.Context(), boundary, 1)
	require.NoError(t, err)
	require.Equal(t, stale[:1], limited)

	version := now.Add(-time.Minute)
	subject := "refreshed-subject"
	require.NoError(t, db.RefreshMembers(t.Context(), []MemberInput{
		{PersonID: members[0].PersonID, Subject: &subject, DisplayName: "Current name", Email: "current@example.test", AvatarVersion: &version},
		{PersonID: old.PersonID, DisplayName: old.DisplayName, Email: old.Email},
	}, []string{missing.ID}, now))
	refreshed, err := db.MemberByID(t.Context(), members[0].ID)
	require.NoError(t, err)
	want := members[0]
	want.Subject, want.DisplayName, want.Email = &subject, "Current name", "current@example.test"
	want.AvatarVersion, want.DirectorySyncedAt = &version, &now
	require.Equal(t, want, refreshed)
	refreshedMissing, err := db.MemberByID(t.Context(), missing.ID)
	require.NoError(t, err)
	missing.DirectorySyncedAt = &now
	require.Equal(t, missing, refreshedMissing, "missing Directory people retain every stored field")
	refreshedOld, err := db.MemberByID(t.Context(), old.ID)
	require.NoError(t, err)
	old.AvatarVersion, old.DirectorySyncedAt = nil, &now
	require.Equal(t, old, refreshedOld, "a nil Directory subject preserves the linked identity; a nil picture clears the version")
	fresh, err := db.MemberByID(t.Context(), members[2].ID)
	require.NoError(t, err)
	members[2].DirectorySyncedAt = &recent
	require.Equal(t, members[2], fresh)
	stale, err = db.StaleMembers(t.Context(), boundary, 100)
	require.NoError(t, err)
	require.Empty(t, stale, "recent and exactly-24-hour snapshots are not stale")
}

func TestStaleMembersBatchLimit(t *testing.T) {
	db, _ := fixture(t)
	require.NoError(t, db.Do(t.Context(), true, func(tx *Tx) error {
		return tx.Exec(`INSERT INTO members (id,person_id,display_name,email)
 SELECT 'batch-' || n, 'batch-person-' || n, 'Batch member', '' FROM generate_series(1,101) n`)
	}))
	stale, err := db.StaleMembers(t.Context(), october, 200)
	require.NoError(t, err)
	require.Len(t, stale, 100)
}

func TestRefreshMembersPreservesSubjectOwnership(t *testing.T) {
	db, members := fixture(t)
	now := october.UTC()
	pending, err := db.UpsertMember(t.Context(), MemberInput{PersonID: "pending-refresh", DisplayName: "Pending"}, now, false)
	require.NoError(t, err)
	require.NoError(t, db.RefreshMembers(t.Context(), []MemberInput{
		{PersonID: members[0].PersonID, Subject: members[1].Subject, DisplayName: "Updated despite conflict", Email: "updated@example.test"},
		{PersonID: pending.PersonID, Subject: new("new-subject"), DisplayName: "Now linked", Email: "linked@example.test"},
		{PersonID: "not-stored", Subject: members[1].Subject, DisplayName: "Do not insert"},
	}, nil, now))
	owner, err := db.MemberBySubject(t.Context(), *members[1].Subject)
	require.NoError(t, err)
	require.Equal(t, members[1], owner, "background refresh must not steal another member's subject")
	conflicting, err := db.MemberByID(t.Context(), members[0].ID)
	require.NoError(t, err)
	require.Equal(t, members[0].Subject, conflicting.Subject)
	require.Equal(t, "Updated despite conflict", conflicting.DisplayName)
	require.Equal(t, "updated@example.test", conflicting.Email)
	require.Equal(t, &now, conflicting.DirectorySyncedAt)
	linked, err := db.MemberByID(t.Context(), pending.ID)
	require.NoError(t, err)
	require.Equal(t, new("new-subject"), linked.Subject)
	require.False(t, linked.Pending)
	_, err = db.MemberByPersonID(t.Context(), "not-stored")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRefreshMembersIsAtomic(t *testing.T) {
	db, members := fixture(t)
	err := db.RefreshMembers(t.Context(), []MemberInput{
		{PersonID: members[0].PersonID, DisplayName: "Would be updated", Email: "updated@example.test"},
		{PersonID: members[1].PersonID, DisplayName: "Invalid", Email: "invalid\x00email"},
	}, []string{members[2].ID}, october)
	require.Error(t, err)
	for _, member := range members {
		unchanged, err := db.MemberByID(t.Context(), member.ID)
		require.NoError(t, err)
		require.Equal(t, member, unchanged, "failed refreshes must not leave partial snapshots or stamps")
	}
}
