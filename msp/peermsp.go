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
// This file includes Membership service provider interface that covers the
// needs of a peer membership service provider interface.
// In the same folder:
//   (i) appmsp.go includes description of an extension of peerMSP and peerManager
//       covering the needs of an application/client.
//   (ii) msp.go includes interfaces considered by both peer, and application MSPs.

// MSPManager is an interface defining a manager of one or more MSPs. This
// essentially acts as a mediator to MSP calls and routes MSP related calls
// to the appropriate MSP.
type PeerMSPManager interface {

	// Setup the MSP manager instance according to configuration information
	Setup(configFile string) error

	// Process reconfiguration messages (coming e.g., from Blockchain). This
	// should take into consideration certain policies related to how, e.g.,
	// a certain certificate should be considered valid, what constitutes the
	// chain of trust, and who is authorized to change that.
	// @param reconfigMessage The message containing the reconfiguration information.
	Reconfig(reconfigMessage string) error

	// Name of the MSP manager
	Name() string

	// Policy included in the MSPManager-config.json and passed at setup time to
	// PeerMSPManager to reflect the policy in which in the given blockchain new
	// membership service providers are added.
	Policy() string

	// Provides a list of Membership Service providers
	EnlistedMSPs() (map[string]PeerMSP, error)

	// Adds an MSP to the existing list of Membership Service providers
	// by passing to the manager the configuration information for the new MSP.
	// Returns the name or identity of the MSP or error
	// @param configFile The configuration file information for this MSP
	// Todo: discuss if this is to be an internal function.
	AddMSP(configFile string) (string, error)

	// Deletes an MSP from the existing list of Membership Service providers
	// by passing the serialized identifier of the MSP to be deleted
	// Todo: discuss if this is to be an internal function.
	RemoveMSP(identifier string) (string, error)

	// ImportMember imports a member.
	// @param req The import request
	ImportSigningIdentity(req *ImportRequest) (SigningIdentity, error)

	// GetMember returns the member object corresponding to the provided identifier
	GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error)

	// DeserializeIdentity deserializes an identity
	DeserializeIdentity([]byte) (Identity, error)

	// DeleteSigningIdentity deletes a SigningIdentity corresponding to the provided identifier
	DeleteSigningIdentity(identifier string) (bool, error)

	// VerifyMSPSignature()

	// isValid checks whether the supplied identity is valid
	IsValid(Identity, *ProviderIdentifier) (bool, error)
}

// PeerMSP is the minimal Membership Service Provider Interface to be implemented
// to accommodate peer functionality
type PeerMSP interface {

	// Setup the MSP instance according to configuration information
	Setup(configFile string) error

	// Process reconfiguration messages coming from the blockchain
	// @param reconfigMessage The message containing the reconfiguration command.
	Reconfig(reconfigMessage string) error

	// Get provider type
	Type() ProviderType

	// Get provider identifier
	Identifier() (*ProviderIdentifier, error)

	// Obtain the policy to govern changes; this can be
	// having a json format.
	// Note: THIS CAN WAIT!
	Policy() string

	// ImportSigningIdentity imports a new signing identity to the MSP
	// @param req The import request
	ImportSigningIdentity(req *ImportRequest) (SigningIdentity, error)

	// GetSingingIdentity returns a signing identity corresponding to the provided identifier
	GetSigningIdentity(identifier *IdentityIdentifier) (SigningIdentity, error)

	// DeserializeIdentity deserializes an identity
	DeserializeIdentity([]byte) (Identity, error)

	// DeleteSigningIdentity deletes a specific identity object corresponding to the provided identifier
	DeleteSigningIdentity(identifier string) (bool, error)

	// isValid checks whether the supplied identity is valid
	IsValid(Identity) (bool, error)
}
