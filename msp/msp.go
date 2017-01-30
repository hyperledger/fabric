/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package msp

import (
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/msp"
)

// FIXME: we need better comments on the interfaces!!
// FIXME: we need better comments on the interfaces!!
// FIXME: we need better comments on the interfaces!!

//Common is implemented by both MSPManger and MSP
type Common interface {
	// DeserializeIdentity deserializes an identity.
	// Deserialization will fail if the identity is associated to
	// an msp that is different from this one that is performing
	// the deserialization.
	DeserializeIdentity(serializedIdentity []byte) (Identity, error)
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

	// Common interface needs to be implemented by MSPManager
	Common

	// Setup the MSP manager instance according to configuration information
	Setup(msps []*msp.MSPConfig) error

	// GetMSPs Provides a list of Membership Service providers
	GetMSPs() (map[string]MSP, error)
}

// MSP is the minimal Membership Service Provider Interface to be implemented
// to accommodate peer functionality
type MSP interface {

	// Common interface needs to be implemented by MSP
	Common

	// Setup the MSP instance according to configuration information
	Setup(config *msp.MSPConfig) error

	// GetType returns the provider type
	GetType() ProviderType

	// GetIdentifier returns the provider identifier
	GetIdentifier() (string, error)

	// GetSigningIdentity returns a signing identity corresponding to the provided identifier
	GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error)

	// GetDefaultSigningIdentity returns the default signing identity
	GetDefaultSigningIdentity() (SigningIdentity, error)

	// Validate checks whether the supplied identity is valid
	Validate(id Identity) error

	// SatisfiesPrincipal checks whether the identity matches
	// the description supplied in MSPPrincipal. The check may
	// involve a byte-by-byte comparison (if the principal is
	// a serialized identity) or may require MSP validation
	SatisfiesPrincipal(id Identity, principal *common.MSPPrincipal) error
}

// From this point on, there are interfaces that are shared within the peer and client API
// of the membership service provider.

// Identity interface defining operations associated to a "certificate".
// That is, the public part of the identity could be thought to be a certificate,
// and offers solely signature verification capabilities. This is to be used
// at the peer side when verifying certificates that transactions are signed
// with, and verifying signatures that correspond to these certificates.///
type Identity interface {

	// GetIdentifier returns the identifier of that identity
	GetIdentifier() *IdentityIdentifier

	// GetMSPIdentifier returns the MSP Id for this instance
	GetMSPIdentifier() string

	// Validate uses the rules that govern this identity to validate it.
	// E.g., if it is a fabric TCert implemented as identity, validate
	// will check the TCert signature against the assumed root certificate
	// authority.
	Validate() error

	// TODO: Fix this comment
	// GetOrganizationUnits returns the participant this identity is related to
	// as long as this is public information. In certain implementations
	// this could be implemented by certain attributes that are publicly
	// associated to that identity, or the identifier of the root certificate
	// authority that has provided signatures on this certificate.
	// Examples:
	//  - ParticipantID of a fabric-tcert that was signed by TCA under name
	//    "Organization 1", would be "Organization 1".
	//  - ParticipantID of an alternative implementation of tcert signed by a public
	//    CA used by organization "Organization 1", could be provided in the clear
	//    as part of that tcert structure that this call would be able to return.
	// TODO: check if we need a dedicated type for participantID properly namespaced by the associated provider identifier.
	GetOrganizationUnits() string

	// TODO: Discuss GetOU() further.

	// Verify a signature over some message using this identity as reference
	Verify(msg []byte, sig []byte) error

	// VerifyOpts a signature over some message using this identity as reference
	VerifyOpts(msg []byte, sig []byte, opts SignatureOpts) error

	// VerifyAttributes verifies attributes given a proof
	VerifyAttributes(proof []byte, spec *AttributeProofSpec) error

	// Serialize converts an identity to bytes
	Serialize() ([]byte, error)

	// SatisfiesPrincipal checks whether this instance matches
	// the description supplied in MSPPrincipal. The check may
	// involve a byte-by-byte comparison (if the principal is
	// a serialized identity) or may require MSP validation
	SatisfiesPrincipal(principal *common.MSPPrincipal) error
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

	// SignOpts the message with options
	SignOpts(msg []byte, opts SignatureOpts) ([]byte, error)

	// GetAttributeProof creates a proof for a set of attributes
	GetAttributeProof(spec *AttributeProofSpec) (proof []byte, err error)

	// GetPublicVersion returns the public parts of this identity
	GetPublicVersion() Identity

	// Renew this identity
	Renew() error
}

// ImportRequest is data required when importing a member or
//   enrollment identity that was created off-band
type ImportRequest struct {

	// IdentityProvider to enroll with
	Idp string

	// The certificate to import
	IdentityDesc []byte

	// Key reference associated to the key of the imported member
	KeyReference []string
}

// SignatureOpts are signature options
type SignatureOpts struct {
	Policy []string
	Label  string
}

// Attribute is an arbitrary name/value pair
type Attribute interface {
	Key() AttributeName
	Value() []byte
	Serialise() []byte
}

// AttributeName defines the name of an attribute assuming a
// namespace defined by the entity that certifies this attributes'
// ownership.
type AttributeName struct {
	// provider/guarantor of a certain attribute; this can be
	// expressed by the enrollment identifier of the entity that
	// issues/certifies possession of such attributes.
	provider string
	// the actual name of the attribute; should be unique for a given
	// provider
	name string
}

type AttributeProofSpec struct {
	Attributes []Attribute
	Message    []byte
}

// Structures defining the identifiers for identity providers and members
// and members that belong to them.

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

// The ProviderTYpe of a member relative to the member API
const (
	FABRIC ProviderType = iota // MSP is of FABRIC type
	OTHER                      // MSP is of OTHER TYPE
)
