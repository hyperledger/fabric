/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"testing"

	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/stretchr/testify/require"
)

func TestSelectPolicies(t *testing.T) {
	require.True(t, SelectAllPolicy(discovery.NetworkMember{}))
	require.False(t, SelectNonePolicy(discovery.NetworkMember{}))
}

func TestCombineRoutingFilters(t *testing.T) {
	nm := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
	}
	// Ensure that combine routing filter is a logical AND
	a := func(nm discovery.NetworkMember) bool {
		return nm.Endpoint == "a"
	}
	b := func(nm discovery.NetworkMember) bool {
		return nm.InternalEndpoint == "b"
	}
	require.True(t, CombineRoutingFilters(a, b)(nm))
	require.False(t, CombineRoutingFilters(CombineRoutingFilters(a, b), SelectNonePolicy)(nm))
	require.False(t, CombineRoutingFilters(a, b)(discovery.NetworkMember{InternalEndpoint: "b"}))
}

func TestAnyMatch(t *testing.T) {
	peerA := discovery.NetworkMember{Endpoint: "a"}
	peerB := discovery.NetworkMember{Endpoint: "b"}
	peerC := discovery.NetworkMember{Endpoint: "c"}
	peerD := discovery.NetworkMember{Endpoint: "d"}

	peers := []discovery.NetworkMember{peerA, peerB, peerC, peerD}

	matchB := func(nm discovery.NetworkMember) bool {
		return nm.Endpoint == "b"
	}
	matchC := func(nm discovery.NetworkMember) bool {
		return nm.Endpoint == "c"
	}

	matched := AnyMatch(peers, matchB, matchC)
	require.Len(t, matched, 2)
	require.Contains(t, matched, peerB)
	require.Contains(t, matched, peerC)
}

func TestFirst(t *testing.T) {
	var peers []discovery.NetworkMember
	// nil slice
	require.Nil(t, First(nil, SelectAllPolicy))

	// empty slice
	peers = []discovery.NetworkMember{}
	require.Nil(t, First(peers, SelectAllPolicy))

	// first in slice with any marcher
	peerA := discovery.NetworkMember{Endpoint: "a"}
	peerB := discovery.NetworkMember{Endpoint: "b"}
	peers = []discovery.NetworkMember{peerA, peerB}
	require.Equal(t, &comm.RemotePeer{Endpoint: "a"}, First(peers, SelectAllPolicy))

	// second in slice with matcher that checks for a specific peer
	require.Equal(t, &comm.RemotePeer{Endpoint: "b"}, First(peers, func(nm discovery.NetworkMember) bool {
		return nm.PreferredEndpoint() == "b"
	}))
}

func TestSelectPeers(t *testing.T) {
	a := func(nm discovery.NetworkMember) bool {
		return nm.Endpoint == "a"
	}
	b := func(nm discovery.NetworkMember) bool {
		return nm.InternalEndpoint == "b"
	}
	nm1 := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("a"),
	}
	nm2 := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("b"),
	}
	nm3 := discovery.NetworkMember{
		Endpoint:         "d",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("c"),
	}

	// individual filters
	require.Len(t, SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, a), 2)
	require.Len(t, SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, b), 3)
	// combined filters
	crf := CombineRoutingFilters(a, b)
	require.Len(t, SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, crf), 2)
	require.Len(t, SelectPeers(1, []discovery.NetworkMember{nm1, nm2, nm3}, crf), 1)
}

func BenchmarkSelectPeers(t *testing.B) {
	a := func(nm discovery.NetworkMember) bool {
		return nm.Endpoint == "a"
	}
	b := func(nm discovery.NetworkMember) bool {
		return nm.InternalEndpoint == "b"
	}
	nm1 := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("a"),
	}
	nm2 := discovery.NetworkMember{
		Endpoint:         "a",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("b"),
	}
	nm3 := discovery.NetworkMember{
		Endpoint:         "d",
		InternalEndpoint: "b",
		PKIid:            common.PKIidType("c"),
	}
	crf := CombineRoutingFilters(a, b)

	var l1, l2, l3, l4 int

	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		// individual filters
		l1 = len(SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, a))
		l2 = len(SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, b))
		// combined filters
		l3 = len(SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, crf))
		l4 = len(SelectPeers(1, []discovery.NetworkMember{nm1, nm2, nm3}, crf))
	}
	t.StopTimer()

	require.Equal(t, l1, 2)
	require.Equal(t, l2, 3)
	require.Equal(t, l3, 2)
	require.Equal(t, l4, 1)
}
