package appmsp

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

import "github.com/hyperledger/fabric/msp"
import mspconfig "github.com/hyperledger/fabric/protos/msp"

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
// needs of an application. This is based on an extension of peerMSP and peerManager
//  covering the needs of an application/client.
//

// ApplicationMSPManager is an interface defining a manager of one or more membership
// service providers (MSPs). This essentially acts as a mediator to MSP calls and routes
// MSP related calls to the appropriate MSP.
type ApplicationMSPManager interface {

	// Extends PeerMSPManager
	msp.MSPManager

	// GetMember returns the member object corresponding to the provided identifier
	GetMember(identifier MemberIdentifier) (Member, error)

	// DeleteMember deletes a specific member object corresponding to the provided
	// identifiers
	DeleteMember(identifier MemberIdentifier) error

	// ImportMember imports a member.
	// @param req The import request
	ImportMember(req *MemberImportRequest) (Member, error)
}

// ApplicationMSP is the membership service provider interface for application use
type ApplicationMSP interface {

	// Extends MSP
	msp.MSP

	// GetMember returns an already enrolled member,
	// or nil if an enrolled member with this name was not found.
	// @param name The enrollment name
	GetMember(identifier MemberIdentifier) (Member, error)

	// DeleteMember deletes a specific member object corresponding to the provided identifier
	DeleteMember(identifier MemberIdentifier) (bool, error)

	// ImportMember imports a member.
	// @param req The import request
	ImportMember(req *MemberImportRequest) (Member, error)
}

type MemberImportRequest struct {
	// MSPIdentifier to enroll with
	MSPIdentifier string

	// MemberSigningIdentity includes the long term enrollment identity
	// of  a member
	MemberSigningIdentity *mspconfig.SigningIdentityInfo
}

// Member represents an enrolled entity within an identity provider
type Member interface {

	// GetIdentifier returns the member identifier; this naturally includes the
	// identifier of the provider this member is a member of.
	GetIdentifier() MemberIdentifier

	// GetOrganizationUnits returns the organization this member belongs to. In certain
	// implementations this could be implemented by certain attributes that
	// are publicly associated to that member, or that member's identity.
	// E.g., Groups here could refer to the authorities that have signed a
	// member's enrollment certificate or registered tse user.
	GetOrganizationUnits() string

	// GetFabricRole returns the role in the fabric of this member. E.g., if the
	// member is a peer, or a client.
	// Note: Do we have to have distinction here you think?
	GetFabricRole() FabricRole

	// GetEnrollmentIdentity returns the enrollment identity of this member.
	// The assumption here is that there is only one enrollment identity per
	// member.
	GetEndrollmentIdentity() msp.SigningIdentity

	// GetIdentities returns other identities for use by this member, that comply
	// with the specifications passed as arguments.
	// @param specs The specifications of the identities that are to be retrieved.
	GetIdentities(count int, specs *IdentitySpec) ([]msp.SigningIdentity, error)

	// GetAttributes returns all attributes associated with this member
	GetAttributes() []string

	// DeleteIdentities deletes all identities complying with the specifications passed as parameter
	// in this function. This function aims to serve clean up operations, i.e.,
	// removing expired identities, or revoked ones(?).
	// @param specs The identity specs of the identities to be deleted.
	DeleteIdentities(specs *IdentitySpec) error

	// RevokeIdentities revokes all identities that comply with the specifications passed as parameter
	// in this function.
	// @param specs The identity specs of the identities to be deleted.
	RevokeIdentities(specs *IdentitySpec) error
}

// IdentitySpec is the interface describing the specifications of the
// identity one wants to recover
type IdentitySpec interface {
	// Identity of the identity
	GetName() string

	// Type indicates whether it is a signing or an Identity
	GetType()

	// IsAnonymous returns a boolean indicating if the identity is anonymous
	IsAnonymous() bool

	// A list of attributes this identity includes
	GetAttributeList() []msp.Attribute

	// Identifier of the identity to recover
	GetIdentifier() string
}

// MemberIdentifier uniquely identifies a member naturally
// inheriting the namespace of the associated identity provider.
type MemberIdentifier struct {
	// The identifier of the associated identity provider
	MSPIdentifier string
	// Returns the identifier for a member within a provider
	MemberIdentifier string
}
