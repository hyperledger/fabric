// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cid

import "crypto/x509"

// ChaincodeStubInterface is used by deployable chaincode apps to get identity
// of the  agent (or user) submitting the transaction.
type ChaincodeStubInterface interface {
	// GetCreator returns `SignatureHeader.Creator` (e.g. an identity)
	// of the `SignedProposal`. This is the identity of the agent (or user)
	// submitting the transaction.
	GetCreator() ([]byte, error)
}

// ClientIdentity represents information about the identity that submitted the
// transaction
type ClientIdentity interface {

	// GetID returns the ID associated with the invoking identity.  This ID
	// is guaranteed to be unique within the MSP.
	GetID() (string, error)

	// Return the MSP ID of the client
	GetMSPID() (string, error)

	// GetAttributeValue returns the value of the client's attribute named `attrName`.
	// If the client possesses the attribute, `found` is true and `value` equals the
	// value of the attribute.
	// If the client does not possess the attribute, `found` is false and `value`
	// equals "".
	GetAttributeValue(attrName string) (value string, found bool, err error)

	// AssertAttributeValue verifies that the client has the attribute named `attrName`
	// with a value of `attrValue`; otherwise, an error is returned.
	AssertAttributeValue(attrName, attrValue string) error

	// GetX509Certificate returns the X509 certificate associated with the client,
	// or nil if it was not identified by an X509 certificate.
	GetX509Certificate() (*x509.Certificate, error)
}
