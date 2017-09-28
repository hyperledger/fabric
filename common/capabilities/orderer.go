/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package capabilities

import (
	cb "github.com/hyperledger/fabric/protos/common"
)

const (
	ordererTypeName = "Orderer"
)

// OrdererProvider provides capabilities information for orderer level config.
type OrdererProvider struct {
	*registry
}

// NewOrdererProvider creates an orderer capabilities provider.
func NewOrdererProvider(capabilities map[string]*cb.Capability) *OrdererProvider {
	cp := &OrdererProvider{}
	cp.registry = newRegistry(cp, capabilities)
	return cp
}

// Type returns a descriptive string for logging purposes.
func (cp *OrdererProvider) Type() string {
	return ordererTypeName
}

// HasCapability returns true if the capability is supported by this binary.
func (cp *OrdererProvider) HasCapability(capability string) bool {
	switch capability {
	// Add new capability names here
	default:
		return false
	}
}
