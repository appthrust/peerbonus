package api

import (
	"net/url"
	"strings"
	"time"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
)

type slackIntegrationResponse struct {
	Credential struct {
		BotToken      bool `json:"botToken"`
		SigningSecret bool `json:"signingSecret"`
	} `json:"credential"`
	State     domain.SlackState       `json:"state"`
	Workspace *slackWorkspaceResponse `json:"workspace"`
	Settings  struct {
		AnnounceChannel   *slackChannelResponse `json:"announceChannel"`
		AnnouncePosts     bool                  `json:"announcePosts"`
		DMRecipientOnPost bool                  `json:"dmRecipientOnPost"`
		GiveFromSlack     bool                  `json:"giveFromSlack"`
		Locale            domain.SlackLocale    `json:"locale"`
	} `json:"settings"`
	Inbound    *slackInboundResponse      `json:"inbound"`
	LastError  *slackLastErrorResponse    `json:"lastError"`
	Deliveries slackDeliveryStatsResponse `json:"deliveries"`
}

type slackWorkspaceResponse struct {
	TeamID     string    `json:"teamId"`
	TeamName   string    `json:"teamName"`
	BotUserID  string    `json:"botUserId"`
	VerifiedAt time.Time `json:"verifiedAt"`
}

type slackChannelResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type slackChannelCandidate struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsMember bool   `json:"isMember"`
}

type slackInboundResponse struct {
	URL    *string `json:"url"`
	Ready  bool    `json:"ready"`
	Reason *string `json:"reason"`
}

type slackLastErrorResponse struct {
	Code string     `json:"code"`
	At   *time.Time `json:"at"`
}

type slackDeliveryStatsResponse struct {
	Pending   int `json:"pending"`
	Failed24h int `json:"failed24h"`
	Sent24h   int `json:"sent24h"`
}

type slackDeliveryResponse struct {
	ID        int64                     `json:"id"`
	Kind      domain.SlackDeliveryKind  `json:"kind"`
	PostID    string                    `json:"postId"`
	State     domain.SlackDeliveryState `json:"state"`
	Attempts  int                       `json:"attempts"`
	LastError *string                   `json:"lastError"`
	CreatedAt time.Time                 `json:"createdAt"`
	SentAt    *time.Time                `json:"sentAt"`
}

type slackTestResponse struct {
	OK bool   `json:"ok"`
	TS string `json:"ts"`
}

type slackTeamResponse struct {
	TeamID   string `json:"teamId"`
	TeamName string `json:"teamName"`
}

type slackTeamChangedDetail struct {
	Current slackTeamResponse `json:"current"`
	Token   slackTeamResponse `json:"token"`
}

type memberResponse struct {
	ID           string  `json:"id"`
	PersonID     string  `json:"personId"`
	DisplayName  string  `json:"displayName"`
	Email        string  `json:"email"`
	Pending      bool    `json:"pending"`
	DirectoryURL *string `json:"directoryUrl"`
	AvatarURL    *string `json:"avatarUrl"`
}
type candidate struct {
	PersonID    string           `json:"personId"`
	DisplayName string           `json:"displayName"`
	Email       string           `json:"email"`
	Pending     bool             `json:"pending"`
	Groups      []RecipientGroup `json:"groups"`
	AvatarURL   *string          `json:"avatarUrl"`
}
type periodResponse struct {
	Key              string    `json:"key"`
	StartsAt         time.Time `json:"startsAt"`
	EndsAt           time.Time `json:"endsAt"`
	Allowance        int       `json:"allowance"`
	MaxPointsPerPost int       `json:"maxPointsPerPost"`
}
type programResponse struct {
	TimeZone         string         `json:"timeZone"`
	TimeZoneLocked   bool           `json:"timeZoneLocked"`
	WeeklyAllowance  int            `json:"weeklyAllowance"`
	MaxPointsPerPost int            `json:"maxPointsPerPost"`
	ClapCost         int            `json:"clapCost"`
	Values           []domain.Value `json:"values"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}
type programAdminResponse struct {
	programResponse
	PointUnitValue string `json:"pointUnitValue"`
	Currency       string `json:"currency"`
}
type postResponse struct {
	ID          string           `json:"id"`
	Sender      memberResponse   `json:"sender"`
	Recipient   memberResponse   `json:"recipient"`
	Points      int              `json:"points"`
	Message     string           `json:"message"`
	Values      []string         `json:"values"`
	ClapCount   int              `json:"clapCount"`
	ClapPoints  int              `json:"clapPoints"`
	ClappedByMe int              `json:"clappedByMe"`
	Clappers    []memberResponse `json:"clappers"`
	Period      string           `json:"period"`
	Month       string           `json:"month"`
	CreatedAt   time.Time        `json:"createdAt"`
}
type clapResponse struct {
	ID        string         `json:"id"`
	Member    memberResponse `json:"member"`
	CreatedAt time.Time      `json:"createdAt"`
}
type inboxItemResponse struct {
	ID        string           `json:"id"`
	Kind      domain.InboxKind `json:"kind"`
	Post      postResponse     `json:"post"`
	Actor     memberResponse   `json:"actor"`
	CreatedAt time.Time        `json:"createdAt"`
	Unread    bool             `json:"unread"`
}
type partnerResponse struct {
	Member memberResponse `json:"member"`
	Posts  int            `json:"posts"`
	Points int            `json:"points"`
}
type valueCountResponse struct {
	Key    string `json:"key"`
	Posts  int    `json:"posts"`
	Points int    `json:"points"`
}
type monthPointResponse struct {
	Key            string `json:"key"`
	ReceivedPoints int    `json:"receivedPoints"`
	ReceivedPosts  int    `json:"receivedPosts"`
	SentPosts      int    `json:"sentPosts"`
}
type weekPointResponse struct {
	Key      string    `json:"key"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
	Posts    int       `json:"posts"`
	Points   int       `json:"points"`
	Claps    int       `json:"claps"`
}
type insightsResponse struct {
	Month         string               `json:"month"`
	Posts         int                  `json:"posts"`
	Points        int                  `json:"points"`
	Claps         int                  `json:"claps"`
	Senders       int                  `json:"senders"`
	Recipients    int                  `json:"recipients"`
	Values        []valueCountResponse `json:"values"`
	Weeks         []weekPointResponse  `json:"weeks"`
	TopRecipients []partnerResponse    `json:"topRecipients"`
	TopSenders    []partnerResponse    `json:"topSenders"`
}
type periodStats struct {
	Key        string    `json:"key"`
	StartsAt   time.Time `json:"startsAt"`
	EndsAt     time.Time `json:"endsAt"`
	Allowance  int       `json:"allowance"`
	Remaining  int       `json:"remaining"`
	SentPosts  int       `json:"sentPosts"`
	SentPoints int       `json:"sentPoints"`
	Claps      int       `json:"claps"`
}
type monthStats struct {
	Key            string `json:"key"`
	ReceivedPoints int    `json:"receivedPoints"`
	ReceivedPosts  int    `json:"receivedPosts"`
	SentPosts      int    `json:"sentPosts"`
}
type totalStats struct {
	ReceivedPoints int `json:"receivedPoints"`
	ReceivedPosts  int `json:"receivedPosts"`
	SentPosts      int `json:"sentPosts"`
}
type myStats struct {
	Period periodStats `json:"period"`
	Month  monthStats  `json:"month"`
	Total  totalStats  `json:"total"`
}
type memberStats struct {
	Month    monthStats `json:"month"`
	Total    totalStats `json:"total"`
	Partners struct {
		SentTo       []partnerResponse `json:"sentTo"`
		ReceivedFrom []partnerResponse `json:"receivedFrom"`
	} `json:"partners"`
	Values []valueCountResponse `json:"values"`
	Months []monthPointResponse `json:"months"`
}
type meResponse struct {
	Member memberResponse `json:"member"`
	Roles  struct {
		PeerbonusAdmin bool `json:"peerbonusAdmin"`
	} `json:"roles"`
	Program programResponse `json:"program"`
	Stats   myStats         `json:"stats"`
	Period  periodResponse  `json:"period"`
	Inbox   struct {
		Unread int `json:"unread"`
	} `json:"inbox"`
	Directory struct {
		Linked    bool    `json:"linked"`
		PublicURL *string `json:"publicUrl"`
	} `json:"directory"`
}
type statementLine struct {
	Member memberResponse `json:"member"`
	Points int            `json:"points"`
	Amount string         `json:"amount"`
}
type statementResponse struct {
	Month              string          `json:"month"`
	Closed             bool            `json:"closed"`
	ClosedAt           *time.Time      `json:"closedAt"`
	ClosedBy           *memberResponse `json:"closedBy"`
	PointUnitValue     string          `json:"pointUnitValue"`
	Currency           string          `json:"currency"`
	TotalPoints        int             `json:"totalPoints"`
	TotalAmount        string          `json:"totalAmount"`
	MemberCount        int             `json:"memberCount"`
	DirectoryRefreshed bool            `json:"directoryRefreshed"`
	Lines              []statementLine `json:"lines"`
	CloseBlockers      []string        `json:"closeBlockers"`
	PreviousOpenMonth  *string         `json:"previousOpenMonth"`
}
type statementSummary struct {
	Month       string     `json:"month"`
	Closed      bool       `json:"closed"`
	ClosedAt    *time.Time `json:"closedAt"`
	TotalPoints int        `json:"totalPoints"`
	TotalAmount string     `json:"totalAmount"`
	MemberCount int        `json:"memberCount"`
}
type participantResponse struct {
	PersonID       string          `json:"personId"`
	DisplayName    string          `json:"displayName"`
	Email          string          `json:"email"`
	Pending        bool            `json:"pending"`
	AvatarURL      *string         `json:"avatarUrl"`
	Member         *memberResponse `json:"member"`
	SentPosts      int             `json:"sentPosts"`
	SentPoints     int             `json:"sentPoints"`
	ReceivedPosts  int             `json:"receivedPosts"`
	ReceivedPoints int             `json:"receivedPoints"`
	ClapsGiven     int             `json:"clapsGiven"`
}
type participationResponse struct {
	Month              string `json:"month"`
	DirectoryAvailable bool   `json:"directoryAvailable"`
	Summary            struct {
		People     int `json:"people"`
		Senders    int `json:"senders"`
		Recipients int `json:"recipients"`
		Active     int `json:"active"`
	} `json:"summary"`
	Items []participantResponse `json:"items"`
}

type networkGroupResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}
type networkThresholdsResponse struct {
	ManyConnections    int     `json:"manyConnections"`
	FewConnections     int     `json:"fewConnections"`
	ConcentrationPosts int     `json:"concentrationPosts"`
	ConcentrationShare float64 `json:"concentrationShare"`
	DeclineRatio       float64 `json:"declineRatio"`
	DeclineMinimum     int     `json:"declineMinimum"`
}
type networkActivityResponse struct {
	Recent   int `json:"recent"`
	Previous int `json:"previous"`
}
type networkNodeResponse struct {
	PersonID       string                   `json:"personId"`
	MemberID       *string                  `json:"memberId"`
	DisplayName    string                   `json:"displayName"`
	Email          string                   `json:"email"`
	Pending        bool                     `json:"pending"`
	AvatarURL      *string                  `json:"avatarUrl"`
	Connections    int                      `json:"connections"`
	SentPosts      int                      `json:"sentPosts"`
	ReceivedPosts  int                      `json:"receivedPosts"`
	ReceivedPoints int                      `json:"receivedPoints"`
	ClapsGiven     int                      `json:"clapsGiven"`
	ClapsReceived  int                      `json:"clapsReceived"`
	UnclappedPosts int                      `json:"unclappedPosts"`
	Share          float64                  `json:"share"`
	Decline        *networkActivityResponse `json:"decline"`
	Signals        []domain.NetworkSignal   `json:"signals"`
}
type networkEdgeResponse struct {
	From   string    `json:"from"`
	To     string    `json:"to"`
	Posts  int       `json:"posts"`
	Claps  int       `json:"claps"`
	Points int       `json:"points"`
	LastAt time.Time `json:"lastAt"`
}
type networkSummaryResponse struct {
	People    int `json:"people"`
	Connected int `json:"connected"`
	Isolated  int `json:"isolated"`
	Posts     int `json:"posts"`
	Claps     int `json:"claps"`
}
type networkResponse struct {
	Month              string                    `json:"month"`
	AsOf               time.Time                 `json:"asOf"`
	RecentSince        *time.Time                `json:"recentSince"`
	DirectoryAvailable bool                      `json:"directoryAvailable"`
	Group              *networkGroupResponse     `json:"group"`
	Groups             []networkGroupResponse    `json:"groups"`
	Thresholds         networkThresholdsResponse `json:"thresholds"`
	Summary            networkSummaryResponse    `json:"summary"`
	Nodes              []networkNodeResponse     `json:"nodes"`
	Edges              []networkEdgeResponse     `json:"edges"`
}

func (a *API) renderMember(m domain.Member) memberResponse {
	result := memberResponse{ID: m.ID, PersonID: m.PersonID, DisplayName: m.DisplayName, Email: m.Email, Pending: m.Subject == nil, AvatarURL: avatarURL(m.PersonID, m.AvatarVersion)}
	if a.directoryPublicURL != "" {
		public := strings.TrimRight(a.directoryPublicURL, "/") + "/people/" + url.PathEscape(m.PersonID)
		result.DirectoryURL = &public
	}
	return result
}
func avatarURL(personID string, version *time.Time) *string {
	if version == nil {
		return nil
	}
	path := "/api/peerbonus/v1/avatars/" + url.PathEscape(personID) + "?v=" + url.QueryEscape(version.Format(time.RFC3339))
	return &path
}

func renderProgram(p domain.Program) programResponse {
	values := p.Values
	if values == nil {
		values = []domain.Value{}
	}
	return programResponse{TimeZone: p.TimeZone, TimeZoneLocked: p.TimeZoneLocked, WeeklyAllowance: p.WeeklyAllowance, MaxPointsPerPost: p.MaxPointsPerPost, ClapCost: domain.ClapCost, Values: values, UpdatedAt: p.UpdatedAt}
}
func renderProgramAdmin(p domain.Program) programAdminResponse {
	return programAdminResponse{programResponse: renderProgram(p), PointUnitValue: p.PointUnitValue, Currency: p.Currency}
}
func (a *API) renderPost(p domain.Post) postResponse {
	values := p.Values
	if values == nil {
		values = []string{}
	}
	clappers := make([]memberResponse, 0, len(p.Clappers))
	for _, member := range p.Clappers {
		clappers = append(clappers, a.renderMember(member))
	}
	return postResponse{ID: p.ID, Sender: a.renderMember(p.Sender), Recipient: a.renderMember(p.Recipient), Points: p.Points, Message: p.Message, Values: values, ClapCount: p.ClapCount, ClapPoints: p.ClapPoints, ClappedByMe: p.ClappedByMe, Clappers: clappers, Period: p.PeriodKey, Month: p.MonthKey, CreatedAt: p.CreatedAt}
}
func (a *API) renderClap(c domain.Clap) clapResponse {
	return clapResponse{ID: c.ID, Member: a.renderMember(c.Member), CreatedAt: c.CreatedAt}
}
func (a *API) renderStatement(s domain.Statement) statementResponse {
	blockers := s.CloseBlockers
	if blockers == nil {
		blockers = []string{}
	}
	result := statementResponse{Month: s.Month, Closed: s.Closed, ClosedAt: s.ClosedAt, PointUnitValue: s.PointUnitValue, Currency: s.Currency, TotalPoints: s.TotalPoints, TotalAmount: s.TotalAmount, MemberCount: s.MemberCount, DirectoryRefreshed: s.DirectoryRefreshed, Lines: make([]statementLine, 0, len(s.Lines)), CloseBlockers: blockers, PreviousOpenMonth: s.PreviousOpenMonth}
	if s.ClosedBy != nil {
		member := a.renderMember(*s.ClosedBy)
		result.ClosedBy = &member
	}
	for _, line := range s.Lines {
		result.Lines = append(result.Lines, statementLine{Member: a.renderMember(line.Member), Points: line.Points, Amount: line.Amount})
	}
	return result
}

func (a *API) renderInboxItem(item domain.InboxItem) inboxItemResponse {
	return inboxItemResponse{ID: item.ID, Kind: item.Kind, Post: a.renderPost(item.Post), Actor: a.renderMember(item.Actor), CreatedAt: item.CreatedAt, Unread: item.Unread}
}

func (a *API) renderPartner(partner domain.Partner) partnerResponse {
	return partnerResponse{Member: a.renderMember(partner.Member), Posts: partner.Posts, Points: partner.Points}
}

func (a *API) renderPartners(partners []domain.Partner) []partnerResponse {
	result := make([]partnerResponse, 0, len(partners))
	for _, partner := range partners {
		result = append(result, a.renderPartner(partner))
	}
	return result
}

func renderValueCounts(values []domain.ValueCount) []valueCountResponse {
	result := make([]valueCountResponse, 0, len(values))
	for _, value := range values {
		result = append(result, valueCountResponse(value))
	}
	return result
}

func (a *API) renderMemberStats(stats domain.MemberStats) memberStats {
	result := memberStats{Month: monthStats(stats.Month), Total: totalStats(stats.Total), Values: renderValueCounts(stats.Values), Months: make([]monthPointResponse, 0, len(stats.Months))}
	result.Partners.SentTo = a.renderPartners(stats.SentTo)
	result.Partners.ReceivedFrom = a.renderPartners(stats.ReceivedFrom)
	for _, month := range stats.Months {
		result.Months = append(result.Months, monthPointResponse(month))
	}
	return result
}

func (a *API) renderInsights(insights domain.Insights) insightsResponse {
	result := insightsResponse{Month: insights.Month, Posts: insights.Posts, Points: insights.Points, Claps: insights.Claps, Senders: insights.Senders, Recipients: insights.Recipients, Values: renderValueCounts(insights.Values), Weeks: make([]weekPointResponse, 0, len(insights.Weeks)), TopRecipients: a.renderPartners(insights.TopRecipients), TopSenders: a.renderPartners(insights.TopSenders)}
	for _, week := range insights.Weeks {
		result.Weeks = append(result.Weeks, weekPointResponse(week))
	}
	return result
}

func (a *API) renderParticipation(participation domain.Participation) participationResponse {
	result := participationResponse{Month: participation.Month, DirectoryAvailable: participation.DirectoryAvailable, Items: make([]participantResponse, 0, len(participation.Items))}
	result.Summary.People = participation.Summary.People
	result.Summary.Senders = participation.Summary.Senders
	result.Summary.Recipients = participation.Summary.Recipients
	result.Summary.Active = participation.Summary.Active
	for _, participant := range participation.Items {
		item := participantResponse{PersonID: participant.PersonID, DisplayName: participant.DisplayName, Email: participant.Email, Pending: participant.Pending, SentPosts: participant.SentPosts, SentPoints: participant.SentPoints, ReceivedPosts: participant.ReceivedPosts, ReceivedPoints: participant.ReceivedPoints, ClapsGiven: participant.ClapsGiven}
		item.AvatarURL = avatarURL(participant.PersonID, participant.AvatarVersion)
		if participant.Member != nil {
			member := a.renderMember(*participant.Member)
			item.Member = &member
		}
		result.Items = append(result.Items, item)
	}
	return result
}

func renderNetwork(network domain.Network) networkResponse {
	result := networkResponse{Month: network.Month, AsOf: network.AsOf, RecentSince: network.RecentSince,
		DirectoryAvailable: network.DirectoryAvailable, Thresholds: networkThresholdsResponse(network.Thresholds),
		Summary: networkSummaryResponse(network.Summary), Groups: make([]networkGroupResponse, 0, len(network.Groups)),
		Nodes: make([]networkNodeResponse, 0, len(network.Nodes)), Edges: make([]networkEdgeResponse, 0, len(network.Edges))}
	if network.Group != nil {
		group := networkGroupResponse(*network.Group)
		result.Group = &group
	}
	for _, group := range network.Groups {
		result.Groups = append(result.Groups, networkGroupResponse(group))
	}
	for _, node := range network.Nodes {
		item := networkNodeResponse{PersonID: node.PersonID, MemberID: node.MemberID, DisplayName: node.DisplayName, Email: node.Email,
			Pending: node.Pending, AvatarURL: avatarURL(node.PersonID, node.AvatarVersion), Connections: node.Connections,
			SentPosts: node.SentPosts, ReceivedPosts: node.ReceivedPosts, ReceivedPoints: node.ReceivedPoints,
			ClapsGiven: node.ClapsGiven, ClapsReceived: node.ClapsReceived, UnclappedPosts: node.UnclappedPosts, Share: node.Share, Signals: node.Signals}
		if item.Signals == nil {
			item.Signals = []domain.NetworkSignal{}
		}
		if node.Decline != nil {
			decline := networkActivityResponse(*node.Decline)
			item.Decline = &decline
		}
		result.Nodes = append(result.Nodes, item)
	}
	for _, edge := range network.Edges {
		result.Edges = append(result.Edges, networkEdgeResponse(edge))
	}
	return result
}
