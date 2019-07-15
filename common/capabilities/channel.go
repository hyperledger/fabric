/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	"github.com/hyperledger/fabric/msp"
	cb "github.com/hyperledger/fabric/protos/common"
)

const (
	channelTypeName = "Channel"

	// ChannelV1_1 is the capabilities string for standard new non-backwards compatible fabric v1.1 channel capabilities.
	ChannelV1_1 = "V1_1"

	// ChannelV1_3 is the capabilities string for standard new non-backwards compatible fabric v1.3 channel capabilities.
	ChannelV1_3 = "V1_3"

	// ChannelV1_4_2 is the capabilities string for standard new non-backwards compatible fabric v1.4.2 channel capabilities.
	ChannelV1_4_2 = "V1_4_2"

	// ChannelV1_4_3 is the capabilities string for standard new non-backwards compatible fabric v1.4.3 channel capabilities.
	ChannelV1_4_3 = "V1_4_3"
)

// ChannelProvider provides capabilities information for channel level config.
type ChannelProvider struct {
	*registry
	v11  bool
	v13  bool
	v142 bool
	v143 bool
}

// NewChannelProvider creates a channel capabilities provider.
func NewChannelProvider(capabilities map[string]*cb.Capability) *ChannelProvider {
	cp := &ChannelProvider{}
	cp.registry = newRegistry(cp, capabilities)
	_, cp.v11 = capabilities[ChannelV1_1]
	_, cp.v13 = capabilities[ChannelV1_3]
	_, cp.v142 = capabilities[ChannelV1_4_2]
	_, cp.v143 = capabilities[ChannelV1_4_3]
	return cp
}

// Type returns a descriptive string for logging purposes.
func (cp *ChannelProvider) Type() string {
	return channelTypeName
}

// HasCapability returns true if the capability is supported by this binary.
func (cp *ChannelProvider) HasCapability(capability string) bool {
	switch capability {
	// Add new capability names here
	case ChannelV1_4_3:
		return true
	case ChannelV1_4_2:
		return true
	case ChannelV1_3:
		return true
	case ChannelV1_1:
		return true
	default:
		return false
	}
}

// MSPVersion returns the level of MSP support required by this channel.
func (cp *ChannelProvider) MSPVersion() msp.MSPVersion {
	switch {
	case cp.v143:
		return msp.MSPv1_4_3
	case cp.v142:
		return msp.MSPv1_3
	case cp.v13:
		return msp.MSPv1_3
	case cp.v11:
		return msp.MSPv1_1
	default:
		return msp.MSPv1_0
	}
}

// ConsensusTypeMigration return true if consensus-type migration is supported and permitted in both orderer and peer.
func (cp *ChannelProvider) ConsensusTypeMigration() bool {
	return cp.v142 || cp.v143
}

// OrgSpecificOrdererEndpoints allows for individual orderer orgs to specify their external addresses for their OSNs.
func (cp *ChannelProvider) OrgSpecificOrdererEndpoints() bool {
	return cp.v142 || cp.v143
}
