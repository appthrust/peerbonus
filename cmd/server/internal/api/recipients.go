package api

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"time"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

const recipientsCacheLimit = 128

type RecipientGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type recipientSelf struct {
	PersonID string           `json:"personId"`
	Groups   []RecipientGroup `json:"groups"`
}

type recipientTeam struct {
	Group   RecipientGroup `json:"group"`
	Members []candidate    `json:"members"`
}

type recipientEveryone struct {
	Total int         `json:"total"`
	Items []candidate `json:"items"`
}

type directoryRecipients struct {
	Me       recipientSelf     `json:"me"`
	Teams    []recipientTeam   `json:"teams"`
	Everyone recipientEveryone `json:"everyone"`
	groups   map[string][]RecipientGroup
}

type recipientsResponse struct {
	directoryRecipients
	Recent             []candidate `json:"recent"`
	DirectoryAvailable bool        `json:"directoryAvailable"`
}

type cachedRecipients struct {
	data    directoryRecipients
	expires time.Time
}

type cachedDirectoryPeople struct {
	people  []contract.Person
	expires time.Time
}

type directoryProfileKey struct {
	subject, personID string
}

type cachedDirectoryProfile struct {
	profile contract.PersonProfile
	expires time.Time
}

// Keep caches caller-scoped: a cached read must not widen Directory authority.
func (a *API) loadDirectoryPeople(s *request) ([]contract.Person, error) {
	a.recipientsMu.Lock()
	cached, ok := a.directoryPeopleCache[s.subject]
	a.recipientsMu.Unlock()
	if ok && s.now.Before(cached.expires) {
		return cached.people, nil
	}
	people, err := a.directory.ListAll(s.r.Context(), s.bearer, 500)
	if err != nil {
		return nil, err
	}
	a.recipientsMu.Lock()
	defer a.recipientsMu.Unlock()
	if a.directoryPeopleCache == nil {
		a.directoryPeopleCache = make(map[string]cachedDirectoryPeople)
	}
	now := a.now()
	var oldest string
	var expires time.Time
	for key, entry := range a.directoryPeopleCache {
		if !now.Before(entry.expires) {
			delete(a.directoryPeopleCache, key)
		} else if expires.IsZero() || entry.expires.Before(expires) {
			oldest, expires = key, entry.expires
		}
	}
	if _, exists := a.directoryPeopleCache[s.subject]; !exists && len(a.directoryPeopleCache) >= recipientsCacheLimit {
		delete(a.directoryPeopleCache, oldest)
	}
	a.directoryPeopleCache[s.subject] = cachedDirectoryPeople{people: people, expires: now.Add(60 * time.Second)}
	return people, nil
}

func (a *API) loadDirectoryProfile(s *request, personID string) (contract.PersonProfile, error) {
	key := directoryProfileKey{subject: s.subject, personID: personID}
	a.recipientsMu.Lock()
	cached, ok := a.directoryProfileCache[key]
	a.recipientsMu.Unlock()
	if ok && s.now.Before(cached.expires) {
		return cached.profile, nil
	}
	profile, err := a.directory.Profile(s.r.Context(), s.bearer, personID)
	if err != nil {
		return profile, err
	}
	a.recipientsMu.Lock()
	defer a.recipientsMu.Unlock()
	if a.directoryProfileCache == nil {
		a.directoryProfileCache = make(map[directoryProfileKey]cachedDirectoryProfile)
	}
	now := a.now()
	var oldest directoryProfileKey
	var expires time.Time
	for key, entry := range a.directoryProfileCache {
		if !now.Before(entry.expires) {
			delete(a.directoryProfileCache, key)
		} else if expires.IsZero() || entry.expires.Before(expires) {
			oldest, expires = key, entry.expires
		}
	}
	if _, exists := a.directoryProfileCache[key]; !exists && len(a.directoryProfileCache) >= recipientsCacheLimit {
		delete(a.directoryProfileCache, oldest)
	}
	a.directoryProfileCache[key] = cachedDirectoryProfile{profile: profile, expires: now.Add(60 * time.Second)}
	return profile, nil
}

func directGroups(profile contract.PersonProfile) []RecipientGroup {
	groups := make([]RecipientGroup, 0, len(profile.Memberships))
	for _, membership := range profile.Memberships {
		if membership.InheritedFrom == nil {
			group := RecipientGroup(membership.Group)
			if !slices.Contains(groups, group) {
				groups = append(groups, group)
			}
		}
	}
	slices.SortFunc(groups, func(a, b RecipientGroup) int {
		if depth := cmp.Compare(strings.Count(b.Path, "/"), strings.Count(a.Path, "/")); depth != 0 {
			return depth
		}
		if name := cmp.Compare(a.Name, b.Name); name != 0 {
			return name
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return groups
}

func (a *API) recipients(s *request) (any, int, error) {
	data, err := a.loadDirectoryRecipients(s)
	available := err == nil
	if err != nil {
		var denied *link.DeniedError
		if !errors.Is(err, link.ErrUnavailable) && !errors.Is(err, link.ErrAbsent) && !errors.As(err, &denied) {
			return nil, 0, err
		}
		personID := data.Me.PersonID
		if personID == "" {
			personID = s.member.PersonID
		}
		data = directoryRecipients{
			Me:       recipientSelf{PersonID: personID, Groups: []RecipientGroup{}},
			Teams:    []recipientTeam{},
			Everyone: recipientEveryone{Items: []candidate{}},
		}
	}
	// Resolve the existing sender by authoritative person ID without creating a
	// Member merely because the picker was opened.
	member := s.member
	if data.Me.PersonID != "" && data.Me.PersonID != member.PersonID {
		member, err = a.store.MemberByPersonID(s.r.Context(), data.Me.PersonID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, 0, err
		}
	}
	result := recipientsResponse{directoryRecipients: data, Recent: []candidate{}, DirectoryAvailable: available}
	if member.ID != "" {
		recent, err := a.store.RecentRecipients(s.r.Context(), member.ID, 8)
		if err != nil {
			return nil, 0, err
		}
		for _, person := range recent {
			if person.PersonID == data.Me.PersonID || (person.Subject != nil && *person.Subject == s.subject) {
				continue
			}
			result.Recent = append(result.Recent, candidate{
				PersonID: person.PersonID, DisplayName: person.DisplayName, Email: person.Email,
				Pending: person.Pending, Groups: data.personGroups(person.PersonID),
				AvatarURL: avatarURL(person.PersonID, person.AvatarVersion),
			})
		}
	}
	return result, 200, nil
}

func (a *API) loadDirectoryRecipients(s *request) (directoryRecipients, error) {
	a.recipientsMu.Lock()
	cached, ok := a.recipientsCache[s.subject]
	a.recipientsMu.Unlock()
	if ok && s.now.Before(cached.expires) {
		return cached.data, nil
	}

	data := directoryRecipients{
		Me:       recipientSelf{Groups: []RecipientGroup{}},
		Teams:    []recipientTeam{},
		Everyone: recipientEveryone{Items: []candidate{}},
		groups:   make(map[string][]RecipientGroup),
	}
	person, err := a.directory.Me(s.r.Context(), s.bearer)
	if err != nil {
		return data, err
	}
	if person.Subject == nil || *person.Subject != s.subject {
		return data, link.ErrUnavailable
	}
	data.Me.PersonID = person.ID
	profile, err := a.loadDirectoryProfile(s, person.ID)
	if err != nil {
		return data, err
	}
	data.Me.Groups = directGroups(profile)
	for _, group := range data.Me.Groups[:min(len(data.Me.Groups), 5)] {
		people, err := a.directory.PeopleInGroup(s.r.Context(), s.bearer, group.ID, 100)
		if err != nil {
			return data, err
		}
		members := recipientCandidates(people, person.ID, s.subject, 100)
		for _, member := range members {
			data.groups[member.PersonID] = append(data.groups[member.PersonID], group)
		}
		data.Teams = append(data.Teams, recipientTeam{Group: group, Members: members})
	}
	// Teams with nobody else to give to go last; otherwise keep the
	// depth-then-name order so the default tab is predictable.
	slices.SortStableFunc(data.Teams, func(a, b recipientTeam) int {
		if depth := cmp.Compare(strings.Count(b.Group.Path, "/"), strings.Count(a.Group.Path, "/")); depth != 0 {
			return depth
		}
		return cmp.Compare(boolInt(len(a.Members) == 0), boolInt(len(b.Members) == 0))
	})
	for i, team := range data.Teams {
		data.Me.Groups[i] = team.Group
	}
	// Directory has no total or pagination. Fetch its maximum of 500 so total
	// is exact below that cap, and a lower bound when the response is full.
	people, err := a.loadDirectoryPeople(s)
	if err != nil {
		return data, err
	}
	for _, person := range people {
		if person.ID != data.Me.PersonID && (person.Subject == nil || *person.Subject != s.subject) {
			data.Everyone.Total++
		}
	}
	data.Everyone.Items = recipientCandidates(people, person.ID, s.subject, 100)
	for i := range data.Teams {
		data.setGroups(data.Teams[i].Members)
	}
	data.setGroups(data.Everyone.Items)
	a.cacheRecipients(s.subject, data)
	return data, nil
}

func recipientCandidates(people []contract.Person, personID, subject string, limit int) []candidate {
	// Cached Directory slices are immutable and shared by concurrent reads.
	people = slices.Clone(people)
	slices.SortFunc(people, func(a, b contract.Person) int {
		if name := cmp.Compare(a.DisplayName, b.DisplayName); name != 0 {
			return name
		}
		return cmp.Compare(a.ID, b.ID)
	})
	items := make([]candidate, 0, min(len(people), limit))
	for _, person := range people {
		if person.ID == personID || (person.Subject != nil && *person.Subject == subject) {
			continue
		}
		items = append(items, candidate{PersonID: person.ID, DisplayName: person.DisplayName, Email: person.Email, Pending: person.Subject == nil, Groups: []RecipientGroup{}, AvatarURL: avatarURL(person.ID, person.AvatarVersion)})
		if len(items) == limit {
			break
		}
	}
	return items
}

func (data directoryRecipients) personGroups(personID string) []RecipientGroup {
	if groups := data.groups[personID]; groups != nil {
		return groups
	}
	return []RecipientGroup{}
}

func (data directoryRecipients) setGroups(items []candidate) {
	for i := range items {
		items[i].Groups = data.personGroups(items[i].PersonID)
	}
}

func (a *API) cacheRecipients(subject string, data directoryRecipients) {
	a.recipientsMu.Lock()
	defer a.recipientsMu.Unlock()
	if a.recipientsCache == nil {
		a.recipientsCache = make(map[string]cachedRecipients)
	}
	now := a.now()
	oldest := ""
	var expires time.Time
	for key, cached := range a.recipientsCache {
		if !now.Before(cached.expires) {
			delete(a.recipientsCache, key)
			continue
		}
		if expires.IsZero() || cached.expires.Before(expires) {
			oldest, expires = key, cached.expires
		}
	}
	if _, exists := a.recipientsCache[subject]; !exists && len(a.recipientsCache) >= recipientsCacheLimit {
		delete(a.recipientsCache, oldest)
	}
	a.recipientsCache[subject] = cachedRecipients{data: data, expires: now.Add(60 * time.Second)}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
