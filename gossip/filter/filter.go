/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package filter

import (
	"math/rand"

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
	var res []*comm.RemotePeer
	rand.Seed(int64(util.RandomUInt64()))
	// Iterate over the possible candidates in random order
	for _, index := range rand.Perm(len(peerPool)) {
		// If we collected K peers, we can stop the iteration.
		if len(res) == k {
			break
		}
		peer := peerPool[index]
		// For each one, check if it is a worthy candidate to be selected
		if !filter(peer) {
			continue
		}
		p := &comm.RemotePeer{PKIID: peer.PKIid, Endpoint: peer.PreferredEndpoint()}
		res = append(res, p)
	}
	return res
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
