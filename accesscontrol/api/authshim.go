package authshim

import "github.com/hyperledger/fabric/msp"

/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

// AttributeAuthShim is an interface based on top of the chaincode shim
// to offer invocation access control based on identity attributes
// TODO: Add NewAuthShimByTransientDataKey function
// TODO: Make it later generic enough by providing as input the MSP identity
type AttributeAuthShim interface {

	// ReadAttributeValue would return the value of an attribute
	ReadAttributeValue(attName string) ([]byte, error)

	// Verify a proof of ownership of attribute atts using invocation
	// data as the message to prove possession of attributes on
	VerifyAttribute(atts []msp.Attribute)
}

// IdentityAuthShim is an interface based on top of the chaincode shim
// to offer invocation access control based on identities
// TODO: Add NewAuthShimByTransientDataKey
// TODO: Add as setup parameter also ApplicationMSP
type IdentityAuthShim interface {

	// Verify a proof of ownership of an identity using the input
	// message to prove possession of identity ownership on
	VerifyIdentityOnMessage(identity msp.Identity, message string)

	// Verify a proof of ownership of an identity using invocation
	// data as the message to prove possession of attributes on
	VerifyIdentity(identity msp.Identity)
}
