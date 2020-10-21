/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package support

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/discovery"
	"github.com/hyperledger/fabric/discovery/support/acl"
	"github.com/hyperledger/fabric/discovery/support/config"
	"github.com/hyperledger/fabric/msp"
)

//go:generate mockery -dir . -name GossipSupport -case underscore -output mocks/

// GossipSupport is the local interface used to generate mocks for foreign interface.
type GossipSupport interface {
	discovery.GossipSupport
}

//go:generate mockery -dir . -name ChannelPolicyManagerGetter -case underscore  -output mocks/

// ChannelPolicyManagerGetter is the local interface used to generate mocks for foreign interface.
type ChannelPolicyManagerGetter interface {
	acl.ChannelPolicyManagerGetter
}

//go:generate mockery -dir . -name PolicyManager -case underscore  -output mocks/

type PolicyManager interface {
	policies.Manager
}

//go:generate counterfeiter -o mocks/channel_config_getter.go --fake-name ChannelConfigGetter . channelConfigGetter
type channelConfigGetter interface {
	acl.ChannelConfigGetter
}

//go:generate counterfeiter -o mocks/config_getter.go --fake-name ConfigGetter . configGetter
type configGetter interface {
	config.CurrentConfigGetter
}

//go:generate counterfeiter -o mocks/configtx_validator.go --fake-name ConfigtxValidator . configtxValidator
type configtxValidator interface {
	configtx.Validator
}

//go:generate counterfeiter -o mocks/evaluator.go --fake-name Evaluator . evaluator
type evaluator interface {
	acl.Evaluator
}

//go:generate counterfeiter -o mocks/identity.go --fake-name Identity . identity
type identity interface {
	msp.Identity
}

//go:generate counterfeiter -o mocks/msp_manager.go --fake-name MSPManager . mspManager
type mspManager interface {
	msp.MSPManager
}

//go:generate counterfeiter -o mocks/resources.go --fake-name Resources . resources
type resources interface {
	channelconfig.Resources
}

//go:generate counterfeiter -o mocks/verifier.go --fake-name Verifier . verifier
type verifier interface {
	acl.Verifier
}

// DiscoverySupport aggregates all the support needed for the discovery service
type DiscoverySupport struct {
	discovery.AccessControlSupport
	discovery.GossipSupport
	discovery.EndorsementSupport
	discovery.ConfigSupport
	discovery.ConfigSequenceSupport
}

// NewDiscoverySupport returns an aggregated discovery support
func NewDiscoverySupport(
	access discovery.AccessControlSupport,
	gossip discovery.GossipSupport,
	endorsement discovery.EndorsementSupport,
	config discovery.ConfigSupport,
	sequence discovery.ConfigSequenceSupport,
) *DiscoverySupport {
	return &DiscoverySupport{
		AccessControlSupport:  access,
		GossipSupport:         gossip,
		EndorsementSupport:    endorsement,
		ConfigSupport:         config,
		ConfigSequenceSupport: sequence,
	}
}
