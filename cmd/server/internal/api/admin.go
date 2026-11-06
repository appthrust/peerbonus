package api

import (
	"cmp"
	"encoding/csv"
	"log"
	"slices"
	"strconv"

	contract "github.com/appthrust/appserver-go/appserver/contract/directory"
	"github.com/appthrust/appserver-go/appserver/httpx"
	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/store"
)

func (a *API) adminProgram(s *request) (any, int, error) {
	program, err := a.store.Program(s.r.Context())
	return renderProgramAdmin(program), 200, err
}
func (a *API) patchProgram(s *request) (any, int, error) {
	var input struct {
		TimeZone         *string         `json:"timeZone"`
		WeeklyAllowance  *int            `json:"weeklyAllowance"`
		MaxPointsPerPost *int            `json:"maxPointsPerPost"`
		PointUnitValue   *string         `json:"pointUnitValue"`
		Currency         *string         `json:"currency"`
		Values           *[]domain.Value `json:"values"`
	}
	if err := decode(s, &input); err != nil {
		return nil, 0, err
	}
	if err := a.ensureMember(s); err != nil {
		return nil, 0, err
	}
	program, err := a.store.UpdateProgram(s.r.Context(), domain.ProgramPatch(input), s.now, s.member.ID)
	return renderProgramAdmin(program), 200, err
}
func (a *API) statements(s *request) (any, int, error) {
	rows, err := a.store.ListStatements(s.r.Context(), s.now)
	items := make([]statementSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, statementSummary(row))
	}
	return map[string]any{"items": items}, 200, err
}
func statementMonth(s *request) (string, error) {
	month := s.r.PathValue("month")
	if err := domain.ValidateMonthKey(month); err != nil {
		return "", domain.ErrInvalidMonth
	}
	return month, nil
}
func (a *API) statement(s *request) (any, int, error) {
	month, err := statementMonth(s)
	if err != nil {
		return nil, 0, err
	}
	statement, err := a.store.Statement(s.r.Context(), month, s.now)
	return a.renderStatement(statement), 200, err
}
func (a *API) closeStatement(s *request) (any, int, error) {
	if err := emptyBody(s); err != nil {
		return nil, 0, err
	}
	month, err := statementMonth(s)
	if err != nil {
		return nil, 0, err
	}
	statement, err := a.store.Statement(s.r.Context(), month, s.now)
	if err != nil {
		return nil, 0, err
	}
	if statement.Closed {
		return a.renderStatement(statement), 200, nil
	}
	if len(statement.CloseBlockers) > 0 {
		if statement.CloseBlockers[0] == domain.ErrMonthNotEnded.Error() {
			return nil, 0, domain.ErrMonthNotEnded
		}
		return nil, 0, domain.ErrPreviousMonthOpen
	}
	if err := a.ensureMember(s); err != nil {
		return nil, 0, err
	}
	members, err := a.store.MembersWithReceiveInMonth(s.r.Context(), month)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(members))
	requested := make(map[string]bool, len(members))
	for _, member := range members {
		ids = append(ids, member.PersonID)
		requested[member.PersonID] = true
	}
	people, refreshErr := a.directory.PeopleByIDs(s.r.Context(), s.bearer, ids)
	profiles := make([]store.MemberInput, 0, len(people))
	for _, person := range people {
		if requested[person.ID] {
			profiles = append(profiles, memberInput(person, s.now))
			delete(requested, person.ID)
		}
	}
	if len(profiles) > 0 {
		if err := a.store.UpdateMemberProfiles(s.r.Context(), profiles); err != nil {
			return nil, 0, err
		}
	}
	statement, err = a.store.CloseStatement(s.r.Context(), month, s.now, s.member.ID, refreshErr == nil && len(requested) == 0)
	return a.renderStatement(statement), 200, err
}
func (a *API) exportCSV(s *request) (any, int, error) {
	month, err := statementMonth(s)
	if err != nil {
		return nil, 0, err
	}
	statement, err := a.store.Statement(s.r.Context(), month, s.now)
	if err != nil {
		return nil, 0, err
	}
	if !statement.Closed {
		return nil, 0, domain.ErrStatementOpen
	}
	s.w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	s.w.Header().Set("Content-Disposition", `attachment; filename="peerbonus-`+month+`.csv"`)
	_, _ = s.w.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(s.w)
	_ = writer.Write([]string{"month", "person_id", "email", "display_name", "pending", "points", "unit_value", "currency", "amount"})
	for _, line := range statement.Lines {
		if err := writer.Write([]string{month, line.Member.PersonID, line.Member.Email, line.Member.DisplayName, strconv.FormatBool(line.Member.Subject == nil), strconv.Itoa(line.Points), statement.PointUnitValue, statement.Currency, line.Amount}); err != nil {
			break
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Printf("peerbonus CSV response failed: %v", err)
	}
	return nil, 0, nil
}
func (a *API) adminMembers(s *request) (any, int, error) {
	month := s.r.URL.Query().Get("month")
	if err := domain.ValidateMonthKey(month); err != nil {
		return nil, 0, httpx.Invalid("Month must have the form YYYY-MM")
	}
	activity, err := a.store.ActivityByMember(s.r.Context(), month)
	if err != nil {
		return nil, 0, err
	}
	people, directoryErr := a.loadDirectoryPeople(s)
	result := mergeParticipation(month, activity, people, directoryErr == nil)
	return a.renderParticipation(result), 200, nil
}

func (a *API) adminInsights(s *request) (any, int, error) {
	month := s.r.URL.Query().Get("month")
	if err := domain.ValidateMonthKey(month); err != nil {
		return nil, 0, err
	}
	result, err := a.store.Insights(s.r.Context(), month, s.now)
	return a.renderInsights(result), 200, err
}

func mergeParticipation(month string, activity map[string]domain.Participant, people []contract.Person, available bool) domain.Participation {
	result := domain.Participation{Month: month, DirectoryAvailable: available, Items: make([]domain.Participant, 0, len(people)+len(activity))}
	byPerson := make(map[string]domain.Participant, len(activity))
	for _, row := range activity {
		row.PersonID, row.DisplayName, row.Email = row.Member.PersonID, row.Member.DisplayName, row.Member.Email
		row.Pending = row.Member.Pending
		row.AvatarVersion = row.Member.AvatarVersion
		byPerson[row.PersonID] = row
	}
	if available {
		for _, person := range people {
			row := byPerson[person.ID]
			row.PersonID, row.DisplayName, row.Email, row.Pending = person.ID, person.DisplayName, person.Email, person.Pending
			row.AvatarVersion = person.AvatarVersion
			result.Items = append(result.Items, row)
			delete(byPerson, person.ID)
		}
	}
	slices.SortFunc(result.Items, compareParticipants)
	// People no longer returned by Directory remain visible for accounting,
	// after the current directory roster.
	rosterSize := len(result.Items)
	for _, row := range byPerson {
		result.Items = append(result.Items, row)
	}
	slices.SortFunc(result.Items[rosterSize:], compareParticipants)
	result.Summary.People = len(result.Items)
	for _, row := range result.Items {
		if row.SentPosts > 0 {
			result.Summary.Senders++
		}
		if row.ReceivedPosts > 0 {
			result.Summary.Recipients++
		}
		if row.SentPosts > 0 || row.ReceivedPosts > 0 || row.ReceivedPoints > 0 || row.ClapsGiven > 0 {
			result.Summary.Active++
		}
	}
	return result
}

func compareParticipants(a, b domain.Participant) int {
	if points := cmp.Compare(b.ReceivedPoints, a.ReceivedPoints); points != 0 {
		return points
	}
	if points := cmp.Compare(b.SentPoints, a.SentPoints); points != 0 {
		return points
	}
	if name := cmp.Compare(a.DisplayName, b.DisplayName); name != 0 {
		return name
	}
	return cmp.Compare(a.PersonID, b.PersonID)
}
