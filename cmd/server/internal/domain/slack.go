package domain

import "time"

// SlackState is the derived connection state of the Slack integration.
// unconfigured: no bot token in the environment. configured: token present,
// team not yet fixed. connected: team fixed and auth.test agrees.
// team_changed: the token now belongs to a different workspace (deliveries pause).
type SlackState string

const (
	SlackStateUnconfigured SlackState = "unconfigured"
	SlackStateConfigured   SlackState = "configured"
	SlackStateConnected    SlackState = "connected"
	SlackStateTeamChanged  SlackState = "team_changed"
)

// SlackLocale selects the language of messages the server writes to Slack.
type SlackLocale string

const (
	SlackLocaleJA SlackLocale = "ja"
	SlackLocaleEN SlackLocale = "en"
)

// SlackWorkspace is the workspace fixed by the first successful verify (S4).
type SlackWorkspace struct {
	TeamID       string
	TeamName     string
	BotUserID    string
	EnterpriseID string // empty outside Enterprise Grid
	VerifiedAt   time.Time
	VerifiedBy   string // member id
}

// SlackChannel is a public channel the bot can announce into.
type SlackChannel struct {
	ID       string
	Name     string
	IsMember bool
}

// SlackIntegration is the single-row aggregate (id = 1). Workspace is nil
// until an admin verifies the token.
type SlackIntegration struct {
	Workspace         *SlackWorkspace
	AnnounceChannel   *SlackChannel // nil = no announcements
	AnnouncePosts     bool
	DMRecipientOnPost bool
	GiveFromSlack     bool
	// GiveInConversation enables bot mentions and reactions (Events API).
	GiveInConversation  bool
	MentionPointChoices []int               // amounts offered when a mention omits points
	ReactionRules       []SlackReactionRule // ordered; empty = mentions only
	Locale              SlackLocale
	LastErrorCode       *string
	LastErrorAt         *time.Time
	UpdatedAt           time.Time
}

// Delivers reports whether a write transaction must enqueue deliveries of
// the given kind. Announcements need a fixed team and a channel.
func (i SlackIntegration) Delivers(kind SlackDeliveryKind) bool {
	if i.Workspace == nil {
		return false
	}
	switch kind {
	case SlackDeliveryPostAnnounce, SlackDeliveryClapUpdate, SlackDeliveryPostDelete:
		return i.AnnouncePosts && i.AnnounceChannel != nil
	case SlackDeliveryPostDM:
		return i.DMRecipientOnPost
	}
	return false
}

// SlackDeliveryKind names one outbox job (design §3.1, §4).
type SlackDeliveryKind string

const (
	SlackDeliveryPostAnnounce SlackDeliveryKind = "post_announce"
	SlackDeliveryPostDM       SlackDeliveryKind = "post_dm"
	SlackDeliveryClapUpdate   SlackDeliveryKind = "clap_update"
	SlackDeliveryPostDelete   SlackDeliveryKind = "post_delete"
)

// SlackDeliveryState follows design §3.3.
type SlackDeliveryState string

const (
	SlackDeliveryPending SlackDeliveryState = "pending"
	SlackDeliverySent    SlackDeliveryState = "sent"
	SlackDeliverySkipped SlackDeliveryState = "skipped"
	SlackDeliveryFailed  SlackDeliveryState = "failed"
)

// SlackMaxDeliveryAttempts is the attempt count after which a pending
// delivery becomes failed (S8).
const SlackMaxDeliveryAttempts = 12

// SlackDelivery is one outbox row. AnnounceID points at the post_announce
// row a clap_update / post_delete edits (SL3). ChannelID and MessageTS are
// set once sent.
type SlackDelivery struct {
	ID           int64
	Kind         SlackDeliveryKind
	PostID       string
	ClapID       *string
	MemberID     *string // post_dm recipient
	AnnounceID   *int64
	State        SlackDeliveryState
	Attempts     int
	NextAt       time.Time
	ClaimedUntil *time.Time
	ChannelID    *string
	MessageTS    *string
	LastError    *string
	CreatedAt    time.Time
	SentAt       *time.Time
}

// SlackDeliveryStats feeds the admin status card.
type SlackDeliveryStats struct {
	Pending   int
	Failed24h int
	Sent24h   int
}
