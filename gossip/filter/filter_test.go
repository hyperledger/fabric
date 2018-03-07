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
	"github.com/stretchr/testify/assert"
)

func TestSelectPolicies(t *testing.T) {
	assert.True(t, SelectAllPolicy(discovery.NetworkMember{}))
	assert.False(t, SelectNonePolicy(discovery.NetworkMember{}))
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
	assert.True(t, CombineRoutingFilters(a, b)(nm))
	assert.False(t, CombineRoutingFilters(CombineRoutingFilters(a, b), SelectNonePolicy)(nm))
	assert.False(t, CombineRoutingFilters(a, b)(discovery.NetworkMember{InternalEndpoint: "b"}))
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
	assert.Len(t, matched, 2)
	assert.Contains(t, matched, peerB)
	assert.Contains(t, matched, peerC)
}

func TestFirst(t *testing.T) {
	peerA := discovery.NetworkMember{Endpoint: "a"}
	peerB := discovery.NetworkMember{Endpoint: "b"}
	peers := []discovery.NetworkMember{peerA, peerB}
	assert.Equal(t, &comm.RemotePeer{Endpoint: "a"}, First(peers, func(discovery.NetworkMember) bool {
		return true
	}))

	assert.Equal(t, &comm.RemotePeer{Endpoint: "b"}, First(peers, func(nm discovery.NetworkMember) bool {
		return nm.PreferredEndpoint() == "b"
	}))

	peerAA := discovery.NetworkMember{Endpoint: "aa"}
	peerAB := discovery.NetworkMember{Endpoint: "ab"}
	peers = append(peers, peerAA)
	peers = append(peers, peerAB)
	assert.Equal(t, &comm.RemotePeer{Endpoint: "aa"}, First(peers, func(nm discovery.NetworkMember) bool {
		return len(nm.PreferredEndpoint()) > 1
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
	assert.Len(t, SelectPeers(3, []discovery.NetworkMember{nm1, nm2, nm3}, CombineRoutingFilters(a, b)), 2)
	assert.Len(t, SelectPeers(5, []discovery.NetworkMember{nm1, nm2, nm3}, CombineRoutingFilters(a, b)), 2)
	assert.Len(t, SelectPeers(1, []discovery.NetworkMember{nm1, nm2, nm3}, CombineRoutingFilters(a, b)), 1)
}
