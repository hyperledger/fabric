/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"github.com/hyperledger/fabric/gossip/comm"
	"github.com/hyperledger/fabric/gossip/discovery"
	"github.com/hyperledger/fabric/gossip/util"
)

// RoutingFilter defines a predicate on a NetworkMember
// It is used to assert whether a given NetworkMember should be
// selected for be given a message
type RoutingFilter func(discovery.NetworkMember) bool

// SelectNonePolicy selects an empty set of members
var SelectNonePolicy = func(discovery.NetworkMember) bool {
	return false
}

// SelectAllPolicy selects all members given
var SelectAllPolicy = func(discovery.NetworkMember) bool {
	return true
}

// CombineRoutingFilters returns the logical AND of given routing filters
func CombineRoutingFilters(filters ...RoutingFilter) RoutingFilter {
	return func(member discovery.NetworkMember) bool {
		for _, filter := range filters {
			if !filter(member) {
				return false
			}
		}
		return true
	}
}

// SelectPeers returns a slice of peers that match the routing filter
func SelectPeers(k int, peerPool []discovery.NetworkMember, filter RoutingFilter) []*comm.RemotePeer {
	var filteredPeers []*comm.RemotePeer
	for _, peer := range peerPool {
		if filter(peer) {
			filteredPeers = append(filteredPeers, &comm.RemotePeer{PKIID: peer.PKIid, Endpoint: peer.PreferredEndpoint()})
		}
	}

	var indices []int
	if len(filteredPeers) <= k {
		indices = make([]int, len(filteredPeers))
		for i := 0; i < len(filteredPeers); i++ {
			indices[i] = i
		}
	} else {
		indices = util.GetRandomIndices(k, len(filteredPeers)-1)
	}

	var remotePeers []*comm.RemotePeer
	for _, index := range indices {
		remotePeers = append(remotePeers, filteredPeers[index])
	}

	return remotePeers
}

// First returns the first peer that matches the given filter
func First(peerPool []discovery.NetworkMember, filter RoutingFilter) *comm.RemotePeer {
	for _, p := range peerPool {
		if filter(p) {
			return &comm.RemotePeer{PKIID: p.PKIid, Endpoint: p.PreferredEndpoint()}
		}
	}
	return nil
}

// AnyMatch filters out peers that don't match any of the given filters
func AnyMatch(peerPool []discovery.NetworkMember, filters ...RoutingFilter) []discovery.NetworkMember {
	var res []discovery.NetworkMember
	for _, peer := range peerPool {
		for _, matches := range filters {
			if matches(peer) {
				res = append(res, peer)
				break
			}
		}
	}
	return res
}
