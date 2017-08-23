/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import "github.com/hyperledger/fabric/protos/ledger/rwset"

// SerializedPolicy defines a persisted policy
type SerializedPolicy interface {
	// Channel returns the channel this SerializedPolicy corresponds to
	Channel() string
	// Raw returns the policy in its raw form
	Raw() []byte
}

// SerializedIdentity defines an identity of a network participant
type SerializedIdentity []byte

// PolicyStore defines an object that retrieves stored SerializedPolicies
// based on the collection's properties
type PolicyStore interface {
	// GetPolicy retrieves the collection policy from in the following way:
	// If the TxID exists in the ledger, the policy that is returned is the latest policy
	// which was committed into the ledger before this txID was committed.
	// Else - it's the latest policy for the collection.
	CollectionPolicy(rwset.CollectionCriteria) SerializedPolicy
}

// Filter defines a rule that filters out SerializedIdentities
// that the policy doesn't hold for them.
// Returns: True, if the policy holds for the given SerializedIdentity,
//          False otherwise
type Filter func(SerializedIdentity) bool

// PolicyParser parses SerializedPolicies and returns a Filter
type PolicyParser interface {
	// Parse parses a given SerializedPolicy and returns a Filter
	// that is derived from it
	Parse(SerializedPolicy) Filter
}
