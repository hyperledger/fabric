/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

const (
	channelTypeName = "Channel"

	// ChannelV1_1 is the capabilties string for standard new non-backwards compatible fabric v1.1 channel capabilities.
	ChannelV1_1 = "V1.1"
)

// MSPVersion is used to communicate the level of the Channel MSP to the MSP implementations.
type MSPVersion int

const (
	// MSPv1_0 is the version of the MSP framework shipped with v1.0.x of fabric.
	MSPv1_0 = iota

	// MSPv1_1 is the version of the MSP framework enabled in v1.1.0 of fabric.
	MSPv1_1
)

// ChannelProvider provides capabilities information for channel level config.
type ChannelProvider struct {
	*registry
	v11 bool
}

// NewChannelProvider creates a channel capabilities provider.
func NewChannelProvider(capabilities map[string]*cb.Capability) *ChannelProvider {
	cp := &ChannelProvider{}
	cp.registry = newRegistry(cp, capabilities)
	_, cp.v11 = capabilities[ChannelV1_1]
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
	case ChannelV1_1:
		return true
	default:
		return false
	}
}

// MSPVersion returns the level of MSP support required by this channel.
func (cp *ChannelProvider) MSPVersion() MSPVersion {
	switch {
	case cp.v11:
		return MSPv1_1
	default:
		return MSPv1_0
	}
}
