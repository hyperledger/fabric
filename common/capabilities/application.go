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

	// ApplicationV1_1 is the capabilties string for standard new non-backwards compatible fabric v1.1 application capabilities.
	ApplicationV1_1 = "V1_1"
)

// ApplicationProvider provides capabilities information for application level config.
type ApplicationProvider struct {
	*registry
	v11 bool
}

// NewApplicationProvider creates a application capabilities provider.
func NewApplicationProvider(capabilities map[string]*cb.Capability) *ApplicationProvider {
	ap := &ApplicationProvider{}
	ap.registry = newRegistry(ap, capabilities)
	_, ap.v11 = capabilities[ApplicationV1_1]
	return ap
}

// Type returns a descriptive string for logging purposes.
func (ap *ApplicationProvider) Type() string {
	return applicationTypeName
}

// HasCapability returns true if the capability is supported by this binary.
func (ap *ApplicationProvider) HasCapability(capability string) bool {
	switch capability {
	// Add new capability names here
	case ApplicationV1_1:
		return true
	default:
		return false
	}
}

// LifecycleViaConfig returns true if chaincode lifecycle should be managed via the resources config
// tree rather than via the deprecated v1.0 endorser tx mechanism.
func (ap *ApplicationProvider) LifecycleViaConfig() bool {
	return ap.v11
}

// ForbidDuplicateTXIdInBlock specifies whether two transactions with the same TXId are permitted
// in the same block or whether we mark the second one as TxValidationCode_DUPLICATE_TXID
func (ap *ApplicationProvider) ForbidDuplicateTXIdInBlock() bool {
	return ap.v11
}
