/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import validation "github.com/hyperledger/fabric/core/handlers/validation/api"

// Capabilities defines what capabilities the validation
// should take into account when validating a transaction
type Capabilities interface {
	validation.Dependency
	// Supported returns an error if there are unknown capabilities in this channel which are required
	Supported() error

	// ForbidDuplicateTXIdInBlock specifies whether two transactions with the same TXId are permitted
	// in the same block or whether we mark the second one as TxValidationCode_DUPLICATE_TXID
	ForbidDuplicateTXIdInBlock() bool

	// ACLs returns true if the peer supports ACLs in the channel config
	ACLs() bool

	// PrivateChannelData returns true if support for private channel data (a.k.a. collections) is enabled.
	PrivateChannelData() bool

	// CollectionUpgrade returns true if this channel is configured to allow updates to
	// existing collection or add new collections through chaincode upgrade (as introduced in v1.2)
	CollectionUpgrade() bool

	// V1_1Validation returns true is this channel is configured to perform stricter validation
	// of transactions (as introduced in v1.1).
	V1_1Validation() bool

	// V1_2Validation returns true is this channel is configured to perform stricter validation
	// of transactions (as introduced in v1.2).
	V1_2Validation() bool

	// V1_3Validation returns true if this channel supports transaction validation
	// as introduced in v1.3. This includes:
	//  - policies expressible at a ledger key granularity, as described in FAB-8812
	//  - new chaincode lifecycle, as described in FAB-11237
	V1_3Validation() bool

	// StorePvtDataOfInvalidTx returns true if the peer needs to store
	// the pvtData of invalid transactions (as introduced in v142).
	StorePvtDataOfInvalidTx() bool

	// V2_0Validation returns true if this channel supports transaction validation
	// as introduced in v2.0. This includes:
	//  - new chaincode lifecycle
	//  - implicit per-org collections
	V2_0Validation() bool

	// MetadataLifecycle is an obsolete capability but left so as not to break the interface
	MetadataLifecycle() bool

	// KeyLevelEndorsement returns true if this channel supports endorsement
	// policies expressible at a ledger key granularity, as described in FAB-8812
	KeyLevelEndorsement() bool
}
