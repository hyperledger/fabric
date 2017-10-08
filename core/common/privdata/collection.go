/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"github.com/hyperledger/fabric/protos/common"
)

// Collection defines a common interface for collections
type Collection interface {
	// SetTxContext configures the tx-specific ephemeral collection info, such
	// as txid, nonce, creator -- for future use
	// SetTxContext(parameters ...interface{})

	// CollectionID returns this collection's ID
	CollectionID() string

	// GetEndorsementPolicy returns the endorsement policy for validation -- for
	// future use
	// GetEndorsementPolicy() string

	// MemberOrgs returns the collection's members as MSP IDs. This serves as
	// a human-readable way of quickly identifying who is part of a collection.
	MemberOrgs() []string
}

// CollectionAccessPolicy encapsulates functions for the access policy of a collection
type CollectionAccessPolicy interface {
	// AccessFilter returns a member filter function for a collection
	AccessFilter() Filter

	// RequiredPeerCount returns the minimum number of peers
	// required to send private data to
	RequiredPeerCount() int

	// MemberOrgs returns the collection's members as MSP IDs. This serves as
	// a human-readable way of quickly identifying who is part of a collection.
	MemberOrgs() []string
}

// Filter defines a rule that filters peers according to data signed by them.
// The Identity in the SignedData is a SerializedIdentity of a peer.
// The Data is a message the peer signed, and the Signature is the corresponding
// Signature on that Data.
// Returns: True, if the policy holds for the given signed data.
//          False otherwise
type Filter func(common.SignedData) bool

// CollectionStore retrieves stored collections based on the collection's
// properties. It works as a collection object factory and takes care of
// returning a collection object of an appropriate collection type.
type CollectionStore interface {
	// GetCollection retrieves the collection in the following way:
	// If the TxID exists in the ledger, the collection that is returned has the
	// latest configuration that was committed into the ledger before this txID
	// was committed.
	// Else - it's the latest configuration for the collection.
	RetrieveCollection(common.CollectionCriteria) Collection

	// GetCollectionAccessPolicy retrieves a collection's access policy
	RetrieveCollectionAccessPolicy(common.CollectionCriteria) CollectionAccessPolicy
}
