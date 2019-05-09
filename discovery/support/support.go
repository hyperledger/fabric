/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package support

import (
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/discovery"
	"github.com/hyperledger/fabric/discovery/support/acl"
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
