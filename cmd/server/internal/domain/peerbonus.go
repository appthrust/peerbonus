// Package domain defines Peer Bonus's immutable point events and settlement rules.
package domain

import (
	"errors"
	"fmt"
	"time"
)

type Program struct {
	TimeZone                          string
	TimeZoneLocked                    bool
	WeeklyAllowance, MaxPointsPerPost int
	PointUnitValue, Currency          string
	Values                            []Value
	UpdatedAt                         time.Time
	UpdatedBy                         string
}
type Value struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description *string `json:"description"`
}
type Member struct {
	ID, PersonID       string
	Subject            *string
	DisplayName, Email string
	Pending            bool
	LastSeenAt         *time.Time
	InboxSeenAt        *time.Time
	AvatarVersion      *time.Time
	DirectorySyncedAt  *time.Time
	SlackUserID        *string    // S9 cache; nil = unresolved or not found
	SlackLookupAt      *time.Time // last users.lookupByEmail attempt (24h negative cache)
}
type Period struct {
	Key                         string
	StartsAt, EndsAt            time.Time
	Allowance, MaxPointsPerPost int
	Frozen                      bool
}
type Post struct {
	ID, PeriodKey, MonthKey            string
	Sender, Recipient                  Member
	Points                             int
	Message                            string
	Values                             []string
	ClapCount, ClapPoints, ClappedByMe int
	Clappers                           []Member
	CreatedAt                          time.Time
}
type Clap struct {
	ID        string
	Member    Member
	CreatedAt time.Time
}
type InboxKind string

const (
	InboxPost InboxKind = "post"
	InboxClap InboxKind = "clap"
)

type InboxItem struct {
	ID        string
	Kind      InboxKind
	Post      Post
	Actor     Member
	CreatedAt time.Time
	Unread    bool
}
type Partner struct {
	Member        Member
	Posts, Points int
}
type ValueCount struct {
	Key           string
	Posts, Points int
}
type MonthPoint struct {
	Key                                      string
	ReceivedPoints, ReceivedPosts, SentPosts int
}
type WeekPoint struct {
	Key                  string
	StartsAt, EndsAt     time.Time
	Posts, Points, Claps int
}
type PeriodStats struct {
	Key                                                string
	StartsAt, EndsAt                                   time.Time
	Allowance, Remaining, SentPosts, SentPoints, Claps int
}
type MonthStats struct {
	Key                                      string
	ReceivedPoints, ReceivedPosts, SentPosts int
}
type TotalStats struct{ ReceivedPoints, ReceivedPosts, SentPosts int }
type MyStats struct {
	Period PeriodStats
	Month  MonthStats
	Total  TotalStats
}
type MemberStats struct {
	Month                MonthStats
	Total                TotalStats
	SentTo, ReceivedFrom []Partner
	Values               []ValueCount
	Months               []MonthPoint
}
type StatementLine struct {
	Member Member
	Points int
	Amount string
}
type Statement struct {
	Month                    string
	Closed                   bool
	ClosedAt                 *time.Time
	ClosedBy                 *Member
	PointUnitValue, Currency string
	TotalPoints              int
	TotalAmount              string
	MemberCount              int
	DirectoryRefreshed       bool
	Lines                    []StatementLine
	CloseBlockers            []string
	PreviousOpenMonth        *string
}
type StatementSummary struct {
	Month       string
	Closed      bool
	ClosedAt    *time.Time
	TotalPoints int
	TotalAmount string
	MemberCount int
}
type Insights struct {
	Month                                     string
	Posts, Points, Claps, Senders, Recipients int
	Values                                    []ValueCount
	Weeks                                     []WeekPoint
	TopRecipients, TopSenders                 []Partner
}
type Participant struct {
	PersonID, DisplayName, Email                                     string
	Pending                                                          bool
	Member                                                           *Member
	AvatarVersion                                                    *time.Time
	SentPosts, SentPoints, ReceivedPosts, ReceivedPoints, ClapsGiven int
}
type ParticipationSummary struct{ People, Senders, Recipients, Active int }
type Participation struct {
	Month              string
	DirectoryAvailable bool
	Summary            ParticipationSummary
	Items              []Participant
}
type ProgramPatch struct {
	TimeZone                          *string
	WeeklyAllowance, MaxPointsPerPost *int
	PointUnitValue, Currency          *string
	Values                            *[]Value
}

const ClapCost = 2

var (
	ErrNotFound          = errors.New("not_found")
	ErrRecipientNotFound = errors.New("recipient_not_found")
	ErrSelfTarget        = errors.New("self_target")
	ErrOwnPost           = errors.New("own_post")
	ErrUnknownValue      = errors.New("unknown_value")
	ErrPeriodClosed      = errors.New("period_closed")
	ErrMonthNotEnded     = errors.New("month_not_ended")
	ErrPreviousMonthOpen = errors.New("previous_month_open")
	ErrStatementOpen     = errors.New("statement_open")
	ErrTimezoneLocked    = errors.New("timezone_locked")
	ErrInvalidMonth      = errors.New("invalid_month")
	ErrSlackTeamChanged  = errors.New("slack team changed")
)

type ValidationError struct{ Field, Msg string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

type InsufficientPointsError struct{ Remaining, Required int }

func (e *InsufficientPointsError) Error() string {
	return fmt.Sprintf("insufficient_points: remaining %d, required %d", e.Remaining, e.Required)
}

// PeriodFor uses calendar arithmetic, so a DST transition never shifts midnight.
func PeriodFor(now time.Time, loc *time.Location) (key string, startsAt, endsAt time.Time) {
	local := now.In(loc)
	year, week := local.ISOWeek()
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	startsAt = midnight.AddDate(0, 0, -(int(local.Weekday())+6)%7)
	return fmt.Sprintf("%04d-W%02d", year, week), startsAt, startsAt.AddDate(0, 0, 7)
}
func MonthKeyFor(now time.Time, loc *time.Location) string { return now.In(loc).Format("2006-01") }
func MonthEnded(month string, now time.Time, loc *time.Location) (bool, error) {
	if err := ValidateMonthKey(month); err != nil {
		return false, err
	}
	start, err := time.ParseInLocation("2006-01", month, loc)
	if err != nil {
		return false, ErrInvalidMonth
	}
	return !now.Before(start.AddDate(0, 1, 0)), nil
}
