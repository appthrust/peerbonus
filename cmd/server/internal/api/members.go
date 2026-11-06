package api

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

func memberInput(p contract.Person, now time.Time) store.MemberInput {
	return store.MemberInput{PersonID: p.ID, Subject: p.Subject, DisplayName: p.DisplayName, Email: p.Email, Pending: p.Subject == nil, AvatarVersion: p.AvatarVersion, DirectorySyncedAt: &now}
}

// resolveSelf never invents a Person from OIDC claims. Explicit Directory
// rejections are authoritative even when a cached member exists.
func (a *API) resolveSelf(s *request) (bool, error) {
	person, err := a.directory.Me(s.r.Context(), s.bearer)
	if err != nil {
		var rejected *link.RejectedError
		if errors.As(err, &rejected) {
			return false, err
		}
		if s.member.ID == "" {
			return false, directoryUnavailable()
		}
		in := store.MemberInput{PersonID: s.member.PersonID, Subject: s.member.Subject, DisplayName: s.member.DisplayName, Email: s.member.Email, Pending: s.member.Subject == nil, AvatarVersion: s.member.AvatarVersion}
		s.member, err = a.store.UpsertMember(s.r.Context(), in, s.now, true)
		return false, err
	}
	if person.Subject == nil || *person.Subject != s.subject {
		return false, directoryUnavailable()
	}
	s.member, err = a.store.UpsertMember(s.r.Context(), memberInput(person, s.now), s.now, true)
	return err == nil, err
}
func (a *API) ensureMember(s *request) error {
	if s.member.ID != "" {
		return nil
	}
	_, err := a.resolveSelf(s)
	return err
}
func (a *API) me(s *request) (any, int, error) {
	linked, err := a.resolveSelf(s)
	if err != nil {
		return nil, 0, err
	}
	program, err := a.store.Program(s.r.Context())
	if err != nil {
		return nil, 0, err
	}
	period, err := a.store.CurrentPeriod(s.r.Context(), s.now)
	if err != nil {
		return nil, 0, err
	}
	stats, err := a.store.MyStats(s.r.Context(), s.member.ID, s.now)
	if err != nil {
		return nil, 0, err
	}
	unread, err := a.store.UnreadCount(s.r.Context(), s.member.ID)
	if err != nil {
		return nil, 0, err
	}
	result := meResponse{Member: a.renderMember(s.member), Program: renderProgram(program), Period: periodResponse{Key: period.Key, StartsAt: period.StartsAt, EndsAt: period.EndsAt, Allowance: period.Allowance, MaxPointsPerPost: period.MaxPointsPerPost}, Stats: myStats{Period: periodStats(stats.Period), Month: monthStats(stats.Month), Total: totalStats(stats.Total)}}
	result.Inbox.Unread = unread
	result.Roles.PeerbonusAdmin = s.access.IsConsoleAdmin
	result.Directory.Linked = linked
	if a.directoryPublicURL != "" {
		public := a.directoryPublicURL
		result.Directory.PublicURL = &public
	}
	if linked {
		if err := a.refreshMembers(s); err != nil {
			slog.DebugContext(s.r.Context(), "peerbonus member refresh failed", "error", err)
		}
	}
	return result, 200, nil
}

func (a *API) refreshMembers(s *request) error {
	a.memberSyncMu.Lock()
	if !a.lastMemberSync.IsZero() && s.now.Sub(a.lastMemberSync) <= time.Minute {
		a.memberSyncMu.Unlock()
		return nil
	}
	// Reserve the process-wide cooldown before any I/O, including failed
	// attempts, so concurrent /me requests do not fetch the same batch.
	a.lastMemberSync = s.now
	a.memberSyncMu.Unlock()

	members, err := a.store.StaleMembers(s.r.Context(), s.now.Add(-24*time.Hour), 100)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(members))
	requested := make(map[string]string, len(members))
	for _, member := range members {
		if member.ID == s.member.ID {
			continue
		}
		ids = append(ids, member.PersonID)
		requested[member.PersonID] = member.ID
	}
	if len(ids) == 0 {
		return nil
	}
	people, err := a.directory.PeopleByIDs(s.r.Context(), s.bearer, ids)
	if err != nil {
		return err
	}
	profiles := make([]store.MemberInput, 0, len(people))
	for _, person := range people {
		if _, ok := requested[person.ID]; ok {
			profiles = append(profiles, memberInput(person, s.now))
			delete(requested, person.ID)
		}
	}
	missing := make([]string, 0, len(requested))
	for _, memberID := range requested {
		missing = append(missing, memberID)
	}
	return a.store.RefreshMembers(s.r.Context(), profiles, missing, s.now)
}
func (a *API) program(s *request) (any, int, error) {
	program, err := a.store.Program(s.r.Context())
	return renderProgram(program), 200, err
}
func (a *API) members(s *request) (any, int, error) {
	q := strings.TrimSpace(s.r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 64 {
		return nil, 0, httpx.Invalid("Query must contain at most 64 characters")
	}
	limit, err := queryLimit(s, 10, 20)
	if err != nil {
		return nil, 0, err
	}
	people, err := a.directory.Search(s.r.Context(), s.bearer, q, limit)
	if err != nil {
		var rejected *link.RejectedError
		if errors.As(err, &rejected) {
			return nil, 0, err
		}
		return nil, 0, directoryUnavailable()
	}
	items := make([]candidate, 0, min(len(people), limit))
	for _, person := range people {
		if person.ID == s.member.PersonID || (person.Subject != nil && *person.Subject == s.subject) {
			continue
		}
		items = append(items, candidate{PersonID: person.ID, DisplayName: person.DisplayName, Email: person.Email, Pending: person.Subject == nil, Groups: []RecipientGroup{}, AvatarURL: avatarURL(person.ID, person.AvatarVersion)})
		if len(items) == limit {
			break
		}
	}
	return map[string]any{"items": items}, 200, nil
}
func (a *API) recipient(s *request, personID string) (domain.Member, error) {
	people, err := a.directory.PeopleByIDs(s.r.Context(), s.bearer, []string{personID})
	if err != nil {
		var rejected *link.RejectedError
		if errors.As(err, &rejected) {
			return domain.Member{}, err
		}
		member, lookupErr := a.store.MemberByPersonID(s.r.Context(), personID)
		if lookupErr == nil {
			return member, nil
		}
		if !errors.Is(lookupErr, domain.ErrNotFound) {
			return domain.Member{}, lookupErr
		}
		return domain.Member{}, directoryUnavailable()
	}
	for _, person := range people {
		if person.ID == personID {
			if person.Subject != nil && *person.Subject == s.subject {
				return domain.Member{}, domain.ErrSelfTarget
			}
			return a.store.UpsertMember(s.r.Context(), memberInput(person, s.now), s.now, false)
		}
	}
	return domain.Member{}, domain.ErrRecipientNotFound
}
func queryLimit(s *request, fallback, maximum int) (int, error) {
	values, ok := s.r.URL.Query()["limit"]
	if !ok {
		return fallback, nil
	}
	limit, err := strconv.Atoi(values[0])
	if err != nil || limit < 1 || limit > maximum {
		return 0, httpx.Invalid("Limit must be between 1 and " + strconv.Itoa(maximum))
	}
	return limit, nil
}
