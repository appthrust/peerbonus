package api

func (a *API) inbox(s *request) (any, int, error) {
	limit, err := queryLimit(s, 20, 50)
	if err != nil {
		return nil, 0, err
	}
	if err := a.ensureMember(s); err != nil {
		return nil, 0, err
	}
	rows, cursor, seenAt, err := a.store.ListInbox(s.r.Context(), s.member.ID, limit, s.r.URL.Query().Get("cursor"))
	if err != nil {
		return nil, 0, err
	}
	items := make([]inboxItemResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, a.renderInboxItem(row))
	}
	var next *string
	if cursor != "" {
		next = &cursor
	}
	return map[string]any{"items": items, "nextCursor": next, "seenAt": seenAt}, 200, nil
}

func (a *API) markInboxSeen(s *request) (any, int, error) {
	if err := emptyBody(s); err != nil {
		return nil, 0, err
	}
	if err := a.ensureMember(s); err != nil {
		return nil, 0, err
	}
	seenAt, err := a.store.MarkInboxSeen(s.r.Context(), s.member.ID, s.now)
	return map[string]any{"seenAt": seenAt, "unread": 0}, 200, err
}
