/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gossip

import (
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	gossip2 "github.com/hyperledger/fabric/gossip/gossip"
)

// DiscoverySupport implements support that is used for service discovery
// that is obtained from gossip
type DiscoverySupport struct {
	gossip2.Gossip
}

// NewDiscoverySupport creates a new DiscoverySupport
func NewDiscoverySupport(g gossip2.Gossip) *DiscoverySupport {
	return &DiscoverySupport{g}
}

// ChannelExists returns whether a given channel exists or not
func (s *DiscoverySupport) ChannelExists(channel string) bool {
	return s.SelfChannelInfo(common.ChainID(channel)) != nil
}

// PeersOfChannel returns the NetworkMembers considered alive
// and also subscribed to the channel given
func (s *DiscoverySupport) PeersOfChannel(chain common.ChainID) discovery.Members {
	msg := s.SelfChannelInfo(chain)
	if msg == nil {
		return nil
	}
	stateInf := msg.GetStateInfo()
	selfMember := discovery.NetworkMember{
		Properties: stateInf.Properties,
		PKIid:      stateInf.PkiId,
		Envelope:   msg.Envelope,
	}
	return append(s.Gossip.PeersOfChannel(chain), selfMember)
}

// Peers returns the NetworkMembers considered alive
func (s *DiscoverySupport) Peers() discovery.Members {
	peers := s.Gossip.Peers()
	peers = append(peers, s.Gossip.SelfMembershipInfo())
	// Return only the peers that have an external endpoint.
	return discovery.Members(peers).Filter(discovery.HasExternalEndpoint)
}
