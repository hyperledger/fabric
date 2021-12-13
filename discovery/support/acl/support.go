/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package acl

import (
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("discovery.acl")

// ChannelConfigGetter enables to retrieve the channel config resources
type ChannelConfigGetter interface {
	// GetChannelConfig returns the resources of the channel config
	GetChannelConfig(cid string) channelconfig.Resources
}

// ChannelConfigGetterFunc returns the resources of the channel config
type ChannelConfigGetterFunc func(cid string) channelconfig.Resources

// GetChannelConfig returns the resources of the channel config
func (f ChannelConfigGetterFunc) GetChannelConfig(cid string) channelconfig.Resources {
	return f(cid)
}

// Verifier verifies a signature and a message
type Verifier interface {
	// VerifyByChannel checks that signature is a valid signature of message
	// under a peer's verification key, but also in the context of a specific channel.
	// If the verification succeeded, Verify returns nil meaning no error occurred.
	// If peerIdentity is nil, then the verification fails.
	VerifyByChannel(channel string, sd *protoutil.SignedData) error
}

// Evaluator evaluates signatures.
// It is used to evaluate signatures for the local MSP
type Evaluator interface {
	policies.Policy
}

// DiscoverySupport implements support that is used for service discovery
// that is related to access control
type DiscoverySupport struct {
	ChannelConfigGetter
	Verifier
	Evaluator
}

// NewDiscoverySupport creates a new DiscoverySupport
func NewDiscoverySupport(v Verifier, e Evaluator, chanConf ChannelConfigGetter) *DiscoverySupport {
	return &DiscoverySupport{Verifier: v, Evaluator: e, ChannelConfigGetter: chanConf}
}

// EligibleForService returns whether the given peer is eligible for receiving
// service from the discovery service for a given channel
func (s *DiscoverySupport) EligibleForService(channel string, data protoutil.SignedData) error {
	if channel == "" {
		return s.EvaluateSignedData([]*protoutil.SignedData{&data})
	}
	return s.VerifyByChannel(channel, &data)
}

// ConfigSequence returns the configuration sequence of the given channel
func (s *DiscoverySupport) ConfigSequence(channel string) uint64 {
	// No sequence if the channel is empty
	if channel == "" {
		return 0
	}
	conf := s.GetChannelConfig(channel)
	if conf == nil {
		logger.Panic("Failed obtaining channel config for channel", channel)
	}
	v := conf.ConfigtxValidator()
	if v == nil {
		logger.Panic("ConfigtxValidator for channel", channel, "is nil")
	}
	return v.Sequence()
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
		logger.Warnw("failed deserializing identity", "error", err, "identity", protoutil.LogMessageForSerializedIdentity(rawIdentity))
		return errors.Wrap(err, "failed deserializing identity")
	}
	return identity.SatisfiesPrincipal(principal)
}

// ChannelPolicyManagerGetter is a support interface
// to get access to the policy manager of a given channel
type ChannelPolicyManagerGetter interface {
	// Returns the policy manager associated to the passed channel
	// and true if it was the manager requested, or false if it is the default manager
	Manager(channelID string) policies.Manager
}

// NewChannelVerifier returns a new channel verifier from the given policy and policy manager getter
func NewChannelVerifier(policy string, polMgr policies.ChannelPolicyManagerGetter) *ChannelVerifier {
	return &ChannelVerifier{
		Policy:                     policy,
		ChannelPolicyManagerGetter: polMgr,
	}
}

// ChannelVerifier verifies a signature and a message on the context of a channel
type ChannelVerifier struct {
	policies.ChannelPolicyManagerGetter
	Policy string
}

// VerifyByChannel checks that signature is a valid signature of message
// under a peer's verification key, but also in the context of a specific channel.
// If the verification succeeded, Verify returns nil meaning no error occurred.
// If peerIdentity is nil, then the verification fails.
func (cv *ChannelVerifier) VerifyByChannel(channel string, sd *protoutil.SignedData) error {
	mgr := cv.Manager(channel)
	if mgr == nil {
		return errors.Errorf("policy manager for channel %s doesn't exist", channel)
	}
	pol, _ := mgr.GetPolicy(cv.Policy)
	if pol == nil {
		return errors.New("failed obtaining channel application writers policy")
	}
	return pol.EvaluateSignedData([]*protoutil.SignedData{sd})
}
