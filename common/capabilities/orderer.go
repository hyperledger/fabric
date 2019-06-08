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

	// OrdererV1_1 is the capabilities string for standard new non-backwards compatible Fabric v1.1 orderer capabilities.
	OrdererV1_1 = "V1_1"

	// OrdererV1_4_2 is the capabilities string for standard new non-backwards compatible Fabric v1.4.2 orderer capabilities.
	OrdererV1_4_2 = "V1_4_2"
)

// OrdererProvider provides capabilities information for orderer level config.
type OrdererProvider struct {
	*registry
	v11BugFixes bool
	v142        bool
}

// NewOrdererProvider creates an orderer capabilities provider.
func NewOrdererProvider(capabilities map[string]*cb.Capability) *OrdererProvider {
	cp := &OrdererProvider{}
	cp.registry = newRegistry(cp, capabilities)
	_, cp.v11BugFixes = capabilities[OrdererV1_1]
	_, cp.v142 = capabilities[OrdererV1_4_2]
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
	case OrdererV1_1:
		return true
	case OrdererV1_4_2:
		return true
	default:
		return false
	}
}

// PredictableChannelTemplate specifies whether the v1.0 undesirable behavior of setting the /Channel
// group's mod_policy to "" and copying versions from the channel config should be fixed or not.
func (cp *OrdererProvider) PredictableChannelTemplate() bool {
	return cp.v11BugFixes || cp.v142
}

// Resubmission specifies whether the v1.0 non-deterministic commitment of tx should be fixed by re-submitting
// the re-validated tx.
func (cp *OrdererProvider) Resubmission() bool {
	return cp.v11BugFixes || cp.v142
}

// ExpirationCheck specifies whether the orderer checks for identity expiration checks
// when validating messages
func (cp *OrdererProvider) ExpirationCheck() bool {
	return cp.v11BugFixes || cp.v142
}

// ConsensusTypeMigration checks whether the orderer permits a consensus-type migration.
//
// A Kafka-based Ordering Service Node requires these capabilities in order to receive and process a config update
// with consensus-type migration change. Migration is supported from Kafka to Raft only.
// If not present, these config updates will be rejected.
func (cp *OrdererProvider) ConsensusTypeMigration() bool {
	return cp.v142
}
