package api

import (
	"cmp"
	"slices"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

func (a *API) adminNetwork(s *request) (any, int, error) {
	month := s.r.URL.Query().Get("month")
	if err := domain.ValidateMonthKey(month); err != nil {
		return nil, 0, httpx.Invalid("Month must have the form YYYY-MM")
	}
	facts, err := a.store.Network(s.r.Context(), month, s.now)
	if err != nil {
		return nil, 0, err
	}
	groups, directoryErr := a.directory.Groups(s.r.Context(), s.bearer)
	var group *domain.NetworkGroup
	var people []contract.Person
	if directoryErr == nil {
		if id := s.r.URL.Query().Get("group"); id != "" {
			for _, item := range groups {
				if item.ID == id {
					group = &domain.NetworkGroup{ID: item.ID, Name: item.Name, Path: item.Path}
					break
				}
			}
			if group == nil {
				return nil, 0, httpx.Fail(404, "group_not_found", "The Directory group was not found")
			}
			people, directoryErr = a.directory.PeopleInGroup(s.r.Context(), s.bearer, id, 500)
		} else {
			people, directoryErr = a.loadDirectoryPeople(s)
		}
	}
	result := mergeNetwork(facts, people, groups, group, directoryErr == nil)
	return renderNetwork(result), 200, nil
}

func mergeNetwork(facts domain.NetworkFacts, people []contract.Person, groups []contract.Group, group *domain.NetworkGroup, available bool) domain.Network {
	result := domain.Network{Month: facts.Month, AsOf: facts.AsOf, RecentSince: facts.RecentSince,
		DirectoryAvailable: available, Thresholds: domain.DefaultNetworkThresholds,
		Groups: make([]domain.NetworkGroup, 0, len(groups)), Nodes: make([]domain.NetworkNode, 0, len(people)+len(facts.Members)),
		Edges: make([]domain.NetworkEdge, 0, len(facts.Edges))}
	byPerson := make(map[string]domain.NetworkMemberFacts, len(facts.Members))
	for _, row := range facts.Members {
		byPerson[row.Member.PersonID] = row
	}
	seen := make(map[string]bool, len(people)+len(facts.Members))
	appendNode := func(node domain.NetworkNode, row domain.NetworkMemberFacts) {
		node.Connections, node.SentPosts, node.ReceivedPosts = row.Connections, row.SentPosts, row.ReceivedPosts
		node.ReceivedPoints, node.ClapsGiven, node.ClapsReceived, node.UnclappedPosts = row.ReceivedPoints, row.ClapsGiven, row.ClapsReceived, row.UnclappedPosts
		if row.Member.ID != "" {
			node.MemberID = &row.Member.ID
		}
		result.Thresholds.Signals(&node, facts.Posts, row.Activity)
		result.Nodes = append(result.Nodes, node)
		seen[node.PersonID] = true
		if node.Connections > 0 {
			result.Summary.Connected++
		} else {
			result.Summary.Isolated++
		}
	}
	if available {
		result.Group = group
		for _, item := range groups {
			result.Groups = append(result.Groups, domain.NetworkGroup{ID: item.ID, Name: item.Name, Path: item.Path})
		}
		slices.SortFunc(result.Groups, func(a, b domain.NetworkGroup) int {
			return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
		})
		for _, person := range people {
			if seen[person.ID] {
				continue
			}
			appendNode(domain.NetworkNode{PersonID: person.ID, DisplayName: person.DisplayName, Email: person.Email,
				Pending: person.Pending, AvatarVersion: person.AvatarVersion}, byPerson[person.ID])
		}
	}
	// A filtered roster is authoritative: leftover members may be in other
	// groups. Without a filter (including degraded Directory), keep all members.
	if result.Group == nil {
		for personID, row := range byPerson {
			if !seen[personID] {
				appendNode(domain.NetworkNode{PersonID: personID, DisplayName: row.Member.DisplayName, Email: row.Member.Email,
					Pending: row.Member.Pending, AvatarVersion: row.Member.AvatarVersion}, row)
			}
		}
	}
	for _, edge := range facts.Edges {
		from, to := facts.Members[edge.From].Member.PersonID, facts.Members[edge.To].Member.PersonID
		if seen[from] && seen[to] {
			result.Edges = append(result.Edges, domain.NetworkEdge{From: min(from, to), To: max(from, to),
				Posts: edge.Posts, Claps: edge.Claps, Points: edge.Points, LastAt: edge.LastAt})
		}
	}
	slices.SortFunc(result.Nodes, func(a, b domain.NetworkNode) int {
		return cmp.Or(cmp.Compare(b.Connections, a.Connections), cmp.Compare(a.DisplayName, b.DisplayName), cmp.Compare(a.PersonID, b.PersonID))
	})
	slices.SortFunc(result.Edges, func(a, b domain.NetworkEdge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	result.Summary.People, result.Summary.Posts, result.Summary.Claps = len(result.Nodes), facts.Posts, facts.Claps
	return result
}
