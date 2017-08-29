/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

// SerializedPolicy defines a persisted policy
type SerializedPolicy interface {
	// Channel returns the channel this SerializedPolicy corresponds to
	Channel() string
	// Raw returns the policy in its raw form
	Raw() []byte
}

// PolicyStore defines an object that retrieves stored SerializedPolicies
// based on the collection's properties
type PolicyStore interface {
	// GetPolicy retrieves the collection policy from in the following way:
	// If the TxID exists in the ledger, the policy that is returned is the latest policy
	// which was committed into the ledger before this txID was committed.
	// Else - it's the latest policy for the collection.
	CollectionPolicy(rwset.CollectionCriteria) SerializedPolicy
}

// Filter defines a rule that filters peers according to data signed by them.
// The Identity in the SignedData is a SerializedIdentity of a peer.
// The Data is a message the peer signed, and the Signature is the corresponding
// Signature on that Data.
// Returns: True, if the policy holds for the given signed data.
//          False otherwise
type Filter func(common.SignedData) bool

// PolicyParser parses SerializedPolicies and returns a Filter
type PolicyParser interface {
	// Parse parses a given SerializedPolicy and returns a Filter
	// that is derived from it
	Parse(SerializedPolicy) Filter
}
