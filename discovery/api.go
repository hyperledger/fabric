/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/discovery"
	common2 "github.com/hyperledger/fabric/protos/common"
	discovery2 "github.com/hyperledger/fabric/protos/discovery"
)

// support defines an interface that allows the discovery service
// to obtain information that other peer components have
type support interface {
	// IdentityInfo returns identity information about peers
	IdentityInfo() api.PeerIdentitySet

	// ChannelExists returns whether a given channel exists or not
	ChannelExists(channel string) bool

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common.ChainID) discovery.Members

	// Peers returns the NetworkMembers considered alive
	Peers() discovery.Members

	// PeersForEndorsement returns an EndorsementDescriptor for a given set of peers, channel, and chaincode
	PeersForEndorsement(chaincode string, channel common.ChainID) (*discovery2.EndorsementDescriptor, error)

	// Eligible returns whether the given peer is eligible for receiving
	// service from the discovery service for a given channel
	EligibleForService(channel string, data common2.SignedData) error

	// Config returns the channel's configuration
	Config(channel string) (*discovery2.ConfigResult, error)
}
