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
// This file contains interfaces that are shared within the peer and client API
// of the membership service provider.
// In the same folder:
//   (i) peersmp.go includes description of peer-specific MSP/Manager
//   (ii)appmsp.go includes description of an extension of PeerManager/PeerMSP
//       attempting to cover application-specific membership service provider
//       functionalities.

// Identity interface defining operations associated to a "certificate".
// That is, the public part of the identity could be thought to be a certificate,
// and offers solely signature verification capabilities. This is to be used
// at the peer side when verifying certificates that transactions are signed
// with, and verifying signatures that correspond to these certificates.///
type Identity interface {

	// Identifier returns the identifier of that identity
	Identifier() *IdentityIdentifier

	// Retrieve the provider identifier this identity belongs to
	// from the previous field
	GetMSPIdentifier() string

	// This uses the rules that govern this identity to validate it.
	// E.g., if it is a fabric TCert implemented as identity, validate
	// will check the TCert signature against the assumed root certificate
	// authority.
	Validate() (bool, error)

	// TODO: Fix this comment
	// ParticipantID would return the participant this identity is related to
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
	ParticipantID() string

	// TODO: Discuss GetOU() further.

	// Verify a signature over some message using this identity as reference
	Verify(msg []byte, sig []byte) (bool, error)

	// VerifyOpts a signature over some message using this identity as reference
	VerifyOpts(msg []byte, sig []byte, opts SignatureOpts) (bool, error)

	// VerifyAttributes verifies attributes given proofs
	VerifyAttributes(proof [][]byte, spec *AttributeProofSpec) (bool, error)

	// Serialize converts an identity to bytes
	Serialize() ([]byte, error)
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

	// NewAttributeProof creates a proof for an attribute
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
	Idp ProviderIdentifier

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

// ProviderIdentifier is a holder for an identity provider identifier
// that should be subjected to certain structure conventions.
type ProviderIdentifier struct {
	// Returns the identifier for an identity provider
	Value string
}

// IdentityIdentifier is a holder for the identifier of a specific
// identity, naturally namespaced, by its provider identifier.
type IdentityIdentifier struct {
	// The identifier of the associated membership service provider
	Mspid ProviderIdentifier
	// Returns the identifier for an identity within a provider
	Value string
}

//func toStringMemberIdentifier(mi MemberIdentifier) string {
//	return "alice"
//}

// ProviderType indicates the type of an identity provider
type ProviderType int

// The ProviderTYpe of a member relative to the member API
const (
	FABRIC ProviderType = iota // MSP is of FABRIC type
	OTHER                      // MSP is of OTHER TYPE
)

// This struct represents an Identity
// (with its MSP identifier) to be used
// to serialize it and deserialize it
type SerializedIdentity struct {
	// The identifier of the associated membership service provider
	Mspid ProviderIdentifier

	// the Identity, serialized according to the rules of its MPS
	IdBytes []byte
}
