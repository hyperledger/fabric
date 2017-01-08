/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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

// SelectPeers returns a slice of peers that match a list of routing filters
func SelectPeers(k int, peerPool []discovery.NetworkMember, filters ...RoutingFilter) []*comm.RemotePeer {
	var indices []int
	if len(peerPool) <= k {
		indices = make([]int, len(peerPool))
		for i := 0; i < len(peerPool); i++ {
			indices[i] = i
		}
	} else {
		indices = util.GetRandomIndices(k, len(peerPool)-1)
	}

	var remotePeers []*comm.RemotePeer
	for _, index := range indices {
		peer := peerPool[index]
		if CombineRoutingFilters(filters ...)(peer) {
			remotePeers = append(remotePeers, &comm.RemotePeer{PKIID: peer.PKIid, Endpoint: peer.Endpoint})
		}

	}
	return remotePeers
}
