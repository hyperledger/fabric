/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"time"

	"github.com/hyperledger/fabric-protos-go/msp"
)

// IdentityDeserializer is implemented by both MSPManger and MSP
type IdentityDeserializer interface {
	// DeserializeIdentity deserializes an identity.
	// Deserialization will fail if the identity is associated to
	// an msp that is different from this one that is performing
	// the deserialization.
	DeserializeIdentity(serializedIdentity []byte) (Identity, error)

	// IsWellFormed checks if the given identity can be deserialized into its provider-specific form
	IsWellFormed(identity *msp.SerializedIdentity) error
}

// Membership service provider APIs for Hyperledger Fabric:
//
// By "membership service provider" we refer to an abstract component of the
// system that would provide (anonymous) credentials to clients, and peers for
// them to participate in Hyperledger/fabric network. Clients use these
// credentials to authenticate their transactions, and peers use these credentials
// to authenticate transaction processing results (endorsements). While
// strongly connected to the transaction processing components of the systems,
// this interface aims to have membership services components defined, in such
// a way such that alternate implementations of this can be smoothly plugged in
// without modifying the core of transaction processing components of the system.
//
// This file includes Membership service provider interface that covers the
// needs of a peer membership service provider interface.

// MSPManager is an interface defining a manager of one or more MSPs. This
// essentially acts as a mediator to MSP calls and routes MSP related calls
// to the appropriate MSP.
// This object is immutable, it is initialized once and never changed.
type MSPManager interface {

	// IdentityDeserializer interface needs to be implemented by MSPManager
	IdentityDeserializer

	// Setup the MSP manager instance according to configuration information
	Setup(msps []MSP) error

	// GetMSPs Provides a list of Membership Service providers
	GetMSPs() (map[string]MSP, error)
}

// MSP is the minimal Membership Service Provider Interface to be implemented
// to accommodate peer functionality
type MSP interface {

	// IdentityDeserializer interface needs to be implemented by MSP
	IdentityDeserializer

	// Setup the MSP instance according to configuration information
	Setup(config *msp.MSPConfig) error

	// GetVersion returns the version of this MSP
	GetVersion() MSPVersion

	// GetType returns the provider type
	GetType() ProviderType

	// GetIdentifier returns the provider identifier
	GetIdentifier() (string, error)

	// GetDefaultSigningIdentity returns the default signing identity
	GetDefaultSigningIdentity() (SigningIdentity, error)

	// GetTLSRootCerts returns the TLS root certificates for this MSP
	GetTLSRootCerts() [][]byte

	// GetTLSIntermediateCerts returns the TLS intermediate root certificates for this MSP
	GetTLSIntermediateCerts() [][]byte

	// Validate checks whether the supplied identity is valid
	Validate(id Identity) error

	// SatisfiesPrincipal checks whether the identity matches
	// the description supplied in MSPPrincipal. The check may
	// involve a byte-by-byte comparison (if the principal is
	// a serialized identity) or may require MSP validation
	SatisfiesPrincipal(id Identity, principal *msp.MSPPrincipal) error
}

// OUIdentifier represents an organizational unit and
// its related chain of trust identifier.
type OUIdentifier struct {
	// CertifiersIdentifier is the hash of certificates chain of trust
	// related to this organizational unit
	CertifiersIdentifier []byte
	// OrganizationUnitIdentifier defines the organizational unit under the
	// MSP identified with MSPIdentifier
	OrganizationalUnitIdentifier string
}

// From this point on, there are interfaces that are shared within the peer and client API
// of the membership service provider.

// Identity interface defining operations associated to a "certificate".
// That is, the public part of the identity could be thought to be a certificate,
// and offers solely signature verification capabilities. This is to be used
// at the peer side when verifying certificates that transactions are signed
// with, and verifying signatures that correspond to these certificates.///
type Identity interface {

	// ExpiresAt returns the time at which the Identity expires.
	// If the returned time is the zero value, it implies
	// the Identity does not expire, or that its expiration
	// time is unknown
	ExpiresAt() time.Time

	// GetIdentifier returns the identifier of that identity
	GetIdentifier() *IdentityIdentifier

	// GetMSPIdentifier returns the MSP Id for this instance
	GetMSPIdentifier() string

	// Validate uses the rules that govern this identity to validate it.
	// E.g., if it is a fabric TCert implemented as identity, validate
	// will check the TCert signature against the assumed root certificate
	// authority.
	Validate() error

	// GetOrganizationalUnits returns zero or more organization units or
	// divisions this identity is related to as long as this is public
	// information. Certain MSP implementations may use attributes
	// that are publicly associated to this identity, or the identifier of
	// the root certificate authority that has provided signatures on this
	// certificate.
	// Examples:
	//  - if the identity is an x.509 certificate, this function returns one
	//    or more string which is encoded in the Subject's Distinguished Name
	//    of the type OU
	// TODO: For X.509 based identities, check if we need a dedicated type
	//       for OU where the Certificate OU is properly namespaced by the
	//       signer's identity
	GetOrganizationalUnits() []*OUIdentifier

	// Anonymous returns true if this is an anonymous identity, false otherwise
	Anonymous() bool

	// Verify a signature over some message using this identity as reference
	Verify(msg []byte, sig []byte) error

	// Serialize converts an identity to bytes
	Serialize() ([]byte, error)

	// SatisfiesPrincipal checks whether this instance matches
	// the description supplied in MSPPrincipal. The check may
	// involve a byte-by-byte comparison (if the principal is
	// a serialized identity) or may require MSP validation
	SatisfiesPrincipal(principal *msp.MSPPrincipal) error
}

// SigningIdentity is an extension of Identity to cover signing capabilities.
// E.g., signing identity should be requested in the case of a client who wishes
// to sign transactions, or fabric endorser who wishes to sign proposal
// processing outcomes.
type SigningIdentity interface {

	// Extends Identity
	Identity

	// Sign the message
	Sign(msg []byte) ([]byte, error)

	// GetPublicVersion returns the public parts of this identity
	GetPublicVersion() Identity
}

// IdentityIdentifier is a holder for the identifier of a specific
// identity, naturally namespaced, by its provider identifier.
type IdentityIdentifier struct {

	// The identifier of the associated membership service provider
	Mspid string

	// The identifier for an identity within a provider
	Id string
}

// ProviderType indicates the type of an identity provider
type ProviderType int

// The ProviderType of a member relative to the member API
const (
	FABRIC ProviderType = iota // MSP is of FABRIC type
	IDEMIX                     // MSP is of IDEMIX type
	OTHER                      // MSP is of OTHER TYPE

	// NOTE: as new types are added to this set,
	// the mspTypes map below must be extended
)

var mspTypeStrings = map[ProviderType]string{
	FABRIC: "bccsp",
	IDEMIX: "idemix",
}

// ProviderTypeToString returns a string that represents the ProviderType integer
func ProviderTypeToString(id ProviderType) string {
	if res, found := mspTypeStrings[id]; found {
		return res
	}

	return ""
}
