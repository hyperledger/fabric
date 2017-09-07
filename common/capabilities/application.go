/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

const (
	applicationTypeName = "Application"
)

// ApplicationProvider provides capabilities information for application level config.
type ApplicationProvider struct {
	*registry
}

// NewApplicationProvider creates a application capabilities provider.
func NewApplicationProvider(capabilities map[string]*cb.Capability) *ApplicationProvider {
	cp := &ApplicationProvider{}
	cp.registry = newRegistry(cp, capabilities)
	return cp
}

// Type returns a descriptive string for logging purposes.
func (cp *ApplicationProvider) Type() string {
	return applicationTypeName
}

// HasCapability returns true if the capability is supported by this binary.
func (cp *ApplicationProvider) HasCapability(capability string) bool {
	switch capability {
	// Add new capability names here
	default:
		return false
	}
}
