package domain

import "time"

// Network map (design README "v3: network map"): who exchanged posts and claps
// with whom in one month, and the management signals derived per person.
// Everything here is derived from posts, claps and the Directory roster; the
// map writes nothing.

// NetworkSignal names a fact about one node that a manager acts on. New
// signals append; the UI ignores strings it does not know.
type NetworkSignal string

const (
	SignalIsolated        NetworkSignal = "isolated"
	SignalFewConnections  NetworkSignal = "fewConnections"
	SignalManyConnections NetworkSignal = "manyConnections"
	SignalConcentrated    NetworkSignal = "concentrated"
	SignalDeclining       NetworkSignal = "declining"
	SignalUnclapped       NetworkSignal = "unclapped"
)

// NetworkThresholds are release constants in v1 (invariant N4); the response
// echoes them so the UI never hardcodes a number.
type NetworkThresholds struct {
	ManyConnections    int
	FewConnections     int
	ConcentrationPosts int
	ConcentrationShare float64
	DeclineRatio       float64
	DeclineMinimum     int
}

var DefaultNetworkThresholds = NetworkThresholds{ManyConnections: 8, FewConnections: 2, ConcentrationPosts: 5, ConcentrationShare: 0.2, DeclineRatio: 0.5, DeclineMinimum: 4}

// DeclineWindow is the length of each of the two activity windows compared by
// the declining rule (the 14 days ending at asOf against the 14 days before).
const DeclineWindow = 14 * 24 * time.Hour

// NetworkActivity counts a person's own actions (sent posts + claps given)
// in the two decline windows.
type NetworkActivity struct{ Recent, Previous int }

// NetworkMemberFacts are the store's per-member facts for the window, keyed
// by member id; the API merges them with the Directory roster into nodes.
type NetworkMemberFacts struct {
	Member                                                              Member
	Connections                                                         int // distinct counterparts, Project-wide
	SentPosts, ReceivedPosts, ReceivedPoints, ClapsGiven, ClapsReceived int
	UnclappedPosts                                                      int // received posts nobody clapped
	Activity                                                            NetworkActivity
}

// NetworkMemberEdge is one undirected pair by member id (From < To).
type NetworkMemberEdge struct {
	From, To             string
	Posts, Claps, Points int
	LastAt               time.Time
}

// NetworkFacts is the store's read for one window: invariant N5, a pure
// function of the store and asOf.
type NetworkFacts struct {
	Month        string
	AsOf         time.Time
	RecentSince  *time.Time // start of the current ISO week when the month is current
	Posts, Claps int        // window totals (not reversed)
	Members      map[string]NetworkMemberFacts
	Edges        []NetworkMemberEdge
}

type NetworkGroup struct{ ID, Name, Path string }

// NetworkNode is one Directory person (or a member no longer in Directory).
type NetworkNode struct {
	PersonID                                                                            string
	MemberID                                                                            *string
	DisplayName, Email                                                                  string
	Pending                                                                             bool
	AvatarVersion                                                                       *time.Time
	Connections                                                                         int
	SentPosts, ReceivedPosts, ReceivedPoints, ClapsGiven, ClapsReceived, UnclappedPosts int
	Share                                                                               float64          // receivedPosts / window posts (0 when no posts)
	Decline                                                                             *NetworkActivity // nil unless the previous window reached DeclineMinimum
	Signals                                                                             []NetworkSignal
}

// NetworkEdge is NetworkMemberEdge keyed by person id (From < To).
type NetworkEdge struct {
	From, To             string
	Posts, Claps, Points int
	LastAt               time.Time
}

type NetworkSummary struct{ People, Connected, Isolated, Posts, Claps int }

type Network struct {
	Month              string
	AsOf               time.Time
	RecentSince        *time.Time
	DirectoryAvailable bool
	Group              *NetworkGroup
	Groups             []NetworkGroup
	Thresholds         NetworkThresholds
	Summary            NetworkSummary
	Nodes              []NetworkNode
	Edges              []NetworkEdge
}

// Signals applies the v1 rules of the design table to one node's facts.
// windowPosts is the window's post total for the concentration share.
func (t NetworkThresholds) Signals(node *NetworkNode, windowPosts int, activity NetworkActivity) {
	node.Signals = node.Signals[:0]
	switch {
	case node.Connections == 0:
		node.Signals = append(node.Signals, SignalIsolated)
	case node.Connections <= t.FewConnections:
		node.Signals = append(node.Signals, SignalFewConnections)
	case node.Connections >= t.ManyConnections:
		node.Signals = append(node.Signals, SignalManyConnections)
	}
	node.Share = 0
	if windowPosts > 0 {
		node.Share = float64(node.ReceivedPosts) / float64(windowPosts)
	}
	if node.ReceivedPosts >= t.ConcentrationPosts && node.Share >= t.ConcentrationShare {
		node.Signals = append(node.Signals, SignalConcentrated)
	}
	node.Decline = nil
	if activity.Previous >= t.DeclineMinimum {
		decline := activity
		node.Decline = &decline
		if float64(activity.Recent) <= t.DeclineRatio*float64(activity.Previous) {
			node.Signals = append(node.Signals, SignalDeclining)
		}
	}
	if node.UnclappedPosts > 0 {
		node.Signals = append(node.Signals, SignalUnclapped)
	}
}
