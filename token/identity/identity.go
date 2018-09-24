/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"github.com/hyperledger/fabric/msp"
)

// IssuingValidator is used to establish if the creator can issue tokens of the passed type.
type IssuingValidator interface {
	// Validate returns no error if the passed creator can issue tokens of the passed type,, an error otherwise.
	Validate(creator PublicInfo, tokenType string) error
}

// PublicInfo is used to identify token owners.
type PublicInfo interface {
	Public() []byte
}

// DeserializerManager returns instances of Deserializer
type DeserializerManager interface {
	// Deserializer returns an instance of transaction.Deserializer for the passed channel
	// if the channel exists
	Deserializer(channel string) (Deserializer, error)
}

// Deserializer
type Deserializer interface {
	// Deserialize deserializes an identity.
	// Deserialization will fail if the identity is associated to
	// an msp that is different from this one that is performing
	// the deserialization.
	DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error)
}
