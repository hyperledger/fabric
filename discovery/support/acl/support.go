/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package acl

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/gossip/api"
	"github.com/hyperledger/fabric/gossip/common"
	common2 "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
	"github.com/pkg/errors"
)

// ChannelConfigGetter enables to retrieve the channel config resources
type ChannelConfigGetter interface {
	// GetChannelConfig returns the resources of the channel config
	GetChannelConfig(cid string) channelconfig.Resources
}

// Verifier verifies a signature and a message
type Verifier interface {
	// VerifyByChannel checks that signature is a valid signature of message
	// under a peer's verification key, but also in the context of a specific channel.
	// If the verification succeeded, Verify returns nil meaning no error occurred.
	// If peerIdentity is nil, then the verification fails.
	VerifyByChannel(chainID common.ChainID, peerIdentity api.PeerIdentityType, signature, message []byte) error
}

// DiscoverySupport implements support that is used for service discovery
// that is related to access control
type DiscoverySupport struct {
	ChannelConfigGetter
	Verifier
}

// NewDiscoverySupport creates a new DiscoverySupport
func NewDiscoverySupport(v Verifier, chanConf ChannelConfigGetter) *DiscoverySupport {
	return &DiscoverySupport{Verifier: v, ChannelConfigGetter: chanConf}
}

// Eligible returns whether the given peer is eligible for receiving
// service from the discovery service for a given channel
func (s *DiscoverySupport) EligibleForService(channel string, data common2.SignedData) error {
	return s.VerifyByChannel(common.ChainID(channel), api.PeerIdentityType(data.Identity), data.Signature, data.Data)
}

func (s *DiscoverySupport) SatisfiesPrincipal(channel string, rawIdentity []byte, principal *msp.MSPPrincipal) error {
	conf := s.GetChannelConfig(channel)
	if conf == nil {
		return errors.Errorf("channel %s doesn't exist", channel)
	}
	mspMgr := conf.MSPManager()
	if mspMgr == nil {
		return errors.Errorf("could not find MSP manager for channel %s", channel)
	}
	identity, err := mspMgr.DeserializeIdentity(rawIdentity)
	if err != nil {
		return errors.Wrap(err, "failed deserializing identity")
	}
	return identity.SatisfiesPrincipal(principal)
}
