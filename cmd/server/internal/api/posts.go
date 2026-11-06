package api

import (
	"strings"
	"unicode/utf8"

	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

func (a *API) posts(s *request) (any, int, error) {
	limit, err := queryLimit(s, 20, 50)
	if err != nil {
		return nil, 0, err
	}
	q := s.r.URL.Query()
	search := strings.TrimSpace(q.Get("q"))
	if utf8.RuneCountInString(search) > 100 {
		return nil, 0, httpx.Invalid("Query must contain at most 100 characters")
	}
	if q.Has("period") {
		if err := domain.ValidatePeriodKey(q.Get("period")); err != nil {
			return nil, 0, httpx.Invalid("Period must have the form YYYY-Www")
		}
	}
	if q.Has("month") {
		if err := domain.ValidateMonthKey(q.Get("month")); err != nil {
			return nil, 0, httpx.Invalid("Month must have the form YYYY-MM")
		}
	}
	rows, cursor, err := a.store.ListPosts(s.r.Context(), store.PostFilter{Limit: limit, Cursor: q.Get("cursor"), MemberID: q.Get("member"), SentBy: q.Get("sent"), ReceivedBy: q.Get("received"), Value: q.Get("value"), Period: q.Get("period"), Month: q.Get("month"), Q: search}, s.member.ID)
	if err != nil {
		return nil, 0, err
	}
	items := make([]postResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, a.renderPost(row))
	}
	var next *string
	if cursor != "" {
		next = &cursor
	}
	return map[string]any{"items": items, "nextCursor": next}, 200, nil
}
func (a *API) post(s *request) (any, int, error) {
	post, err := a.store.PostByID(s.r.Context(), s.r.PathValue("id"), s.member.ID)
	return a.renderPost(post), 200, err
}
func idempotencyKey(s *request) (string, error) {
	key := s.r.Header.Get("Idempotency-Key")
	if n := utf8.RuneCountInString(key); n < 1 || n > 128 {
		return "", httpx.Invalid("Idempotency-Key must contain 1 to 128 characters")
	}
	return key, nil
}
func (a *API) createPost(s *request) (any, int, error) {
	key, err := idempotencyKey(s)
	if err != nil {
		return nil, 0, err
	}
	var input struct {
		RecipientPersonID string   `json:"recipientPersonId"`
		Points            int      `json:"points"`
		Message           string   `json:"message"`
		Values            []string `json:"values"`
	}
	if err := decode(s, &input); err != nil {
		return nil, 0, err
	}
	if n := utf8.RuneCountInString(input.RecipientPersonID); n < 1 || n > 128 {
		return nil, 0, httpx.Invalid("Recipient person ID must contain 1 to 128 characters")
	}
	if err := a.ensureMember(s); err != nil {
		return nil, 0, err
	}
	recipient, err := a.recipient(s, input.RecipientPersonID)
	if err != nil {
		return nil, 0, err
	}
	post, _, err := a.store.CreatePost(s.r.Context(), store.CreatePostInput{SenderID: s.member.ID, RecipientID: recipient.ID, Points: input.Points, Message: input.Message, Values: input.Values, Now: s.now, IdempotencyKey: key})
	return a.renderPost(post), 201, err
}
func (a *API) deletePost(s *request) (any, int, error) {
	if err := emptyBody(s); err != nil {
		return nil, 0, err
	}
	if err := a.ensureMember(s); err != nil {
		return nil, 0, err
	}
	return nil, 204, a.store.DeletePost(s.r.Context(), s.r.PathValue("id"), s.member.ID, s.now)
}
func (a *API) claps(s *request) (any, int, error) {
	rows, err := a.store.ListClaps(s.r.Context(), s.r.PathValue("id"), 50)
	items := make([]clapResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, a.renderClap(row))
	}
	return map[string]any{"items": items}, 200, err
}
func (a *API) createClap(s *request) (any, int, error) {
	key, err := idempotencyKey(s)
	if err != nil {
		return nil, 0, err
	}
	if err := emptyBody(s); err != nil {
		return nil, 0, err
	}
	if err := a.ensureMember(s); err != nil {
		return nil, 0, err
	}
	clap, post, _, err := a.store.CreateClap(s.r.Context(), store.CreateClapInput{PostID: s.r.PathValue("id"), MemberID: s.member.ID, Now: s.now, IdempotencyKey: key})
	return map[string]any{"clap": a.renderClap(clap), "post": a.renderPost(post)}, 201, err
}
