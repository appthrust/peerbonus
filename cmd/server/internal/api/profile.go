package api

func (a *API) member(s *request) (any, int, error) {
	member, err := a.store.MemberByID(s.r.Context(), s.r.PathValue("id"))
	if err != nil {
		return nil, 0, err
	}
	stats, err := a.store.MemberStats(s.r.Context(), member.ID, s.now)
	if err != nil {
		return nil, 0, err
	}
	groups := []RecipientGroup{}
	if profile, err := a.loadDirectoryProfile(s, member.PersonID); err == nil {
		groups = directGroups(profile)
	}
	return map[string]any{"member": a.renderMember(member), "groups": groups, "stats": a.renderMemberStats(stats)}, 200, nil
}
