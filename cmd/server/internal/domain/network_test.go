package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNetworkThresholdsSignalsBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		node     NetworkNode
		posts    int
		activity NetworkActivity
		want     []NetworkSignal
		share    float64
		decline  bool
	}{
		{name: "isolated", want: []NetworkSignal{SignalIsolated}},
		{name: "one connection", node: NetworkNode{Connections: 1}, want: []NetworkSignal{SignalFewConnections}},
		{name: "few inclusive", node: NetworkNode{Connections: 2}, want: []NetworkSignal{SignalFewConnections}},
		{name: "above few", node: NetworkNode{Connections: 3}},
		{name: "below many", node: NetworkNode{Connections: 7}},
		{name: "many inclusive", node: NetworkNode{Connections: 8}, want: []NetworkSignal{SignalManyConnections}},
		{name: "above many", node: NetworkNode{Connections: 9}, want: []NetworkSignal{SignalManyConnections}},
		{name: "concentration post minimum", node: NetworkNode{Connections: 3, ReceivedPosts: 4}, posts: 20, share: .2},
		{name: "concentration share below", node: NetworkNode{Connections: 3, ReceivedPosts: 5}, posts: 26, share: 5.0 / 26},
		{name: "concentration inclusive", node: NetworkNode{Connections: 3, ReceivedPosts: 5}, posts: 25, share: .2, want: []NetworkSignal{SignalConcentrated}},
		{name: "concentration above", node: NetworkNode{Connections: 3, ReceivedPosts: 6}, posts: 20, share: .3, want: []NetworkSignal{SignalConcentrated}},
		{name: "zero window posts", node: NetworkNode{Connections: 3, ReceivedPosts: 5}},
		{name: "decline minimum below", node: NetworkNode{Connections: 3}, activity: NetworkActivity{Previous: 3}},
		{name: "decline inclusive", node: NetworkNode{Connections: 3}, activity: NetworkActivity{Recent: 2, Previous: 4}, want: []NetworkSignal{SignalDeclining}, decline: true},
		{name: "decline below ratio", node: NetworkNode{Connections: 3}, activity: NetworkActivity{Recent: 1, Previous: 4}, want: []NetworkSignal{SignalDeclining}, decline: true},
		{name: "decline above ratio", node: NetworkNode{Connections: 3}, activity: NetworkActivity{Recent: 3, Previous: 4}, decline: true},
		{name: "unclapped zero", node: NetworkNode{Connections: 3}},
		{name: "unclapped one", node: NetworkNode{Connections: 3, UnclappedPosts: 1}, want: []NetworkSignal{SignalUnclapped}},
		{name: "ordered combination", node: NetworkNode{Connections: 8, ReceivedPosts: 5, UnclappedPosts: 1}, posts: 25, share: .2, activity: NetworkActivity{Previous: 4}, decline: true, want: []NetworkSignal{SignalManyConnections, SignalConcentrated, SignalDeclining, SignalUnclapped}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Reusing a node must clear stale derived values.
			tc.node.Signals = []NetworkSignal{SignalUnclapped}
			tc.node.Share = 1
			tc.node.Decline = &NetworkActivity{Previous: 100}
			DefaultNetworkThresholds.Signals(&tc.node, tc.posts, tc.activity)
			require.Equal(t, tc.want, append([]NetworkSignal(nil), tc.node.Signals...))
			require.Equal(t, tc.share, tc.node.Share)
			if tc.decline {
				require.Equal(t, &tc.activity, tc.node.Decline)
			} else {
				require.Nil(t, tc.node.Decline)
			}
		})
	}
}
