/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"github.com/hyperledger/fabric-protos-go/msp"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
)

// IdentityDeserializer converts serialized identities
// to identities.
type IdentityDeserializer interface {
	validation.Dependency
	// DeserializeIdentity deserializes an identity.
	// Deserialization will fail if the identity is associated to
	// an msp that is different from this one that is performing
	// the deserialization.
	DeserializeIdentity(serializedIdentity []byte) (Identity, error)
}

// Identity interface defining operations associated to a "certificate".
// That is, the public part of the identity could be thought to be a certificate,
// and offers solely signature verification capabilities. This is to be used
// at the peer side when verifying certificates that transactions are signed
// with, and verifying signatures that correspond to these certificates.///
type Identity interface {
	// Validate uses the rules that govern this identity to validate it.
	Validate() error

	// SatisfiesPrincipal checks whether this instance matches
	// the description supplied in MSPPrincipal. The check may
	// involve a byte-by-byte comparison (if the principal is
	// a serialized identity) or may require MSP validation
	SatisfiesPrincipal(principal *msp.MSPPrincipal) error

	// Verify a signature over some message using this identity as reference
	Verify(msg []byte, sig []byte) error

	// GetIdentityIdentifier returns the identifier of that identity
	GetIdentityIdentifier() *IdentityIdentifier

	// GetMSPIdentifier returns the MSP Id for this instance
	GetMSPIdentifier() string
}

// IdentityIdentifier is a holder for the identifier of a specific
// identity, naturally namespaced, by its provider identifier.
type IdentityIdentifier struct {

	// The identifier of the associated membership service provider
	Mspid string

	// The identifier for an identity within a provider
	Id string
}
