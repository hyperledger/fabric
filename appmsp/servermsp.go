package appmsp

import "github.com/hyperledger/fabric/msp"

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

// ServerEnabledMSP is an interface defining an MSP that leverages a server
// for registration and enrollment. This is an interface that can be used
// in combination to an ApplicationMSP interface or a plain MSP interface
// to add to the related MSP server-aided (online) member/node registration
// and enrollment enablement.
type ServerEnabledMSP interface {

	// Register a new member, this is only possible if the member is a registrar
	// @param req The registration request
	Register(req *RegistrationRequest) (*RegistrationResponse, error)

	// Enroll a new member
	// @param req The enrollment request
	Enroll(req *EnrollmentRequest) (msp.SigningIdentity, error)

	// RegisterAndEnroll registers and enrolls a new entity and
	// returns a long term signing identity, i.e., the enrollment identity
	// @param req The registration request
	RegisterAndEnroll(req *RegistrationRequest) (msp.SigningIdentity, error)
}

// RegistrationRequest for a new identity
type RegistrationRequest interface {
	// Name is the unique name of the identity
	GetName() string

	// Get membership service provider type/identifier this request corresponds to
	GetProviderType() msp.ProviderType

	// Group names to be associated with the identity
	GetOrganization() []string

	// Type/role of identity being registered (e.g. "peer, app, client")
	GetRole() FabricRole

	// Attributes to be associated with the identity
	GetAttributes() []string
}

// RegistrationResponse is a registration response
type RegistrationResponse struct {

	// Returns the username of the registered entity
	Username string

	// Returns the secret associated to the registered entity
	Secret []byte
}

// EnrollmentRequest is a request to enroll a member
type EnrollmentRequest struct {
	// The identity name to enroll
	Name string

	// Some information to authenticate end-user to the enrollment
	// authority
	AuthenticationInfo []byte
}

// EnrollmentResponse is a response to enrollment request
type EnrollmentResponse struct {
	// The enrollment certificate
	IdentityBytes []byte
}

/* Member roles indicates the role of the member in the fabric infrastructure */
type FabricRole int

const (
	PEER      FabricRole = iota // member is a peer
	CLIENT                      // member is a client
	REGISTRAR                   // member is a registrar (part of app)
)
