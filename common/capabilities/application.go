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

	// ApplicationV1_2 is the capabilties string for standard new non-backwards compatible fabric v1.2 application capabilities.
	ApplicationV1_2 = "V1_2"

	// ApplicationV1_3 is the capabilties string for standard new non-backwards compatible fabric v1.3 application capabilities.
	ApplicationV1_3 = "V1_3"

	// ApplicationV1_4_2 is the capabilties string for standard new non-backwards compatible fabric v1.4.2 application capabilities.
	ApplicationV1_4_2 = "V1_4_2"

	// ApplicationPvtDataExperimental is the capabilties string for private data using the experimental feature of collections/sideDB.
	ApplicationPvtDataExperimental = "V1_1_PVTDATA_EXPERIMENTAL"

	// ApplicationResourcesTreeExperimental is the capabilties string for private data using the experimental feature of collections/sideDB.
	ApplicationResourcesTreeExperimental = "V1_1_RESOURCETREE_EXPERIMENTAL"
)

// ApplicationProvider provides capabilities information for application level config.
type ApplicationProvider struct {
	*registry
	v11                    bool
	v12                    bool
	v13                    bool
	v142                   bool
	v11PvtDataExperimental bool
}

// NewApplicationProvider creates a application capabilities provider.
func NewApplicationProvider(capabilities map[string]*cb.Capability) *ApplicationProvider {
	ap := &ApplicationProvider{}
	ap.registry = newRegistry(ap, capabilities)
	_, ap.v11 = capabilities[ApplicationV1_1]
	_, ap.v12 = capabilities[ApplicationV1_2]
	_, ap.v13 = capabilities[ApplicationV1_3]
	_, ap.v142 = capabilities[ApplicationV1_4_2]
	_, ap.v11PvtDataExperimental = capabilities[ApplicationPvtDataExperimental]
	return ap
}

// Type returns a descriptive string for logging purposes.
func (ap *ApplicationProvider) Type() string {
	return applicationTypeName
}

// ACLs returns whether ACLs may be specified in the channel application config
func (ap *ApplicationProvider) ACLs() bool {
	return ap.v12 || ap.v13 || ap.v142
}

// ForbidDuplicateTXIdInBlock specifies whether two transactions with the same TXId are permitted
// in the same block or whether we mark the second one as TxValidationCode_DUPLICATE_TXID
func (ap *ApplicationProvider) ForbidDuplicateTXIdInBlock() bool {
	return ap.v11 || ap.v12 || ap.v13 || ap.v142
}

// PrivateChannelData returns true if support for private channel data (a.k.a. collections) is enabled.
// In v1.1, the private channel data is experimental and has to be enabled explicitly.
// In v1.2, the private channel data is enabled by default.
func (ap *ApplicationProvider) PrivateChannelData() bool {
	return ap.v11PvtDataExperimental || ap.v12 || ap.v13 || ap.v142
}

// CollectionUpgrade returns true if this channel is configured to allow updates to
// existing collection or add new collections through chaincode upgrade (as introduced in v1.2)
func (ap ApplicationProvider) CollectionUpgrade() bool {
	return ap.v12 || ap.v13 || ap.v142
}

// V1_1Validation returns true is this channel is configured to perform stricter validation
// of transactions (as introduced in v1.1).
func (ap *ApplicationProvider) V1_1Validation() bool {
	return ap.v11 || ap.v12 || ap.v13 || ap.v142
}

// V1_2Validation returns true if this channel is configured to perform stricter validation
// of transactions (as introduced in v1.2).
func (ap *ApplicationProvider) V1_2Validation() bool {
	return ap.v12 || ap.v13 || ap.v142
}

// V1_3Validation returns true if this channel is configured to perform stricter validation
// of transactions (as introduced in v1.3).
func (ap *ApplicationProvider) V1_3Validation() bool {
	return ap.v13 || ap.v142
}

// MetadataLifecycle indicates whether the peer should use the deprecated and problematic
// v1.0/v1.1/v1.2 lifecycle, or whether it should use the newer per channel peer local chaincode
// metadata package approach planned for release with Fabric v1.3
func (ap *ApplicationProvider) MetadataLifecycle() bool {
	return false
}

// KeyLevelEndorsement returns true if this channel supports endorsement
// policies expressible at a ledger key granularity, as described in FAB-8812
func (ap *ApplicationProvider) KeyLevelEndorsement() bool {
	return ap.v13 || ap.v142
}

// There is no fabtoken support in v1.4, so always return false
func (ap *ApplicationProvider) FabToken() bool {
	return false
}

// StorePvtDataOfInvalidTx returns true if the peer needs to store
// the pvtData of invalid transactions.
func (ap *ApplicationProvider) StorePvtDataOfInvalidTx() bool {
	return ap.v142
}

// HasCapability returns true if the capability is supported by this binary.
func (ap *ApplicationProvider) HasCapability(capability string) bool {
	switch capability {
	// Add new capability names here
	case ApplicationV1_1:
		return true
	case ApplicationV1_2:
		return true
	case ApplicationV1_3:
		return true
	case ApplicationV1_4_2:
		return true
	case ApplicationPvtDataExperimental:
		return true
	case ApplicationResourcesTreeExperimental:
		return true
	default:
		return false
	}
}
