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

// AccessControlSupport checks if clients are eligible of being serviced
type AccessControlSupport interface {
	// Eligible returns whether the given peer is eligible for receiving
	// service from the discovery service for a given channel
	EligibleForService(channel string, data common2.SignedData) error
}

// GossipSupport aggregates abilities that the gossip module
// provides to the discovery service, such as knowing information about peers
type GossipSupport interface {
	// ChannelExists returns whether a given channel exists or not
	ChannelExists(channel string) bool

	// PeersOfChannel returns the NetworkMembers considered alive
	// and also subscribed to the channel given
	PeersOfChannel(common.ChainID) discovery.Members

	// Peers returns the NetworkMembers considered alive
	Peers() discovery.Members

	// IdentityInfo returns identity information about peers
	IdentityInfo() api.PeerIdentitySet
}

// EndorsementSupport provides knowledge of endorsement policy selection
// for chaincodes
type EndorsementSupport interface {
	// PeersForEndorsement returns an EndorsementDescriptor for a given set of peers, channel, and chaincode
	PeersForEndorsement(chaincode string, channel common.ChainID) (*discovery2.EndorsementDescriptor, error)
}

// ConfigSupport provides access to channel configuration
type ConfigSupport interface {
	// Config returns the channel's configuration
	Config(channel string) (*discovery2.ConfigResult, error)

	// ConfigSequence returns the configuration sequence of the a given channel
	ConfigSequence(channel string) uint64
}

// Support defines an interface that allows the discovery service
// to obtain information that other peer components have
type Support interface {
	AccessControlSupport
	GossipSupport
	EndorsementSupport
	ConfigSupport
}
