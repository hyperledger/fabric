/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

import (
	"crypto/ecdsa"

	"github.com/hyperledger/fabric/bccsp"
)

// IssuerPublicKey is the issuer public key
type IssuerPublicKey interface {

	// Bytes returns the byte representation of this key
	Bytes() ([]byte, error)
}

// IssuerPublicKey is the issuer secret key
type IssuerSecretKey interface {

	// Bytes returns the byte representation of this key
	Bytes() ([]byte, error)

	// Public returns the corresponding public key
	Public() IssuerPublicKey
}

// Issuer is a local interface to decouple from the idemix implementation
type Issuer interface {
	// NewKey generates a new idemix issuer key w.r.t the passed attribute names.
	NewKey(AttributeNames []string) (IssuerSecretKey, error)
}

// Big represent a big integer
type Big interface {
	// Bytes returns the byte representation of this key
	Bytes() ([]byte, error)
}

// Ecp represents an elliptic curve point
type Ecp interface {
	// Bytes returns the byte representation of this key
	Bytes() ([]byte, error)
}

// User is a local interface to decouple from the idemix implementation
type User interface {
	// NewKey generates a new User secret key
	NewKey() (Big, error)

	// MakeNym creates a new unlinkable pseudonym
	MakeNym(sk Big, key IssuerPublicKey) (Ecp, Big, error)
}

// CredRequest is a local interface to decouple from the idemix implementation
// of the issuance of credential requests.
type CredRequest interface {
	// Sign creates a new Credential Request, the first message of the interactive credential issuance protocol
	// (from user to issuer)
	Sign(sk Big, ipk IssuerPublicKey) ([]byte, error)

	// Verify verifies the credential request
	Verify(credRequest []byte, ipk IssuerPublicKey) error
}

// CredRequest is a local interface to decouple from the idemix implementation
// of the issuance of credentials.
type Credential interface {

	// Sign issues a new credential, which is the last step of the interactive issuance protocol
	// All attribute values are added by the issuer at this step and then signed together with a commitment to
	// the user's secret key from a credential request
	Sign(key IssuerSecretKey, credentialRequest []byte, attributes []bccsp.IdemixAttribute) ([]byte, error)

	// Verify cryptographically verifies the credential by verifying the signature
	// on the attribute values and user's secret key
	Verify(sk Big, ipk IssuerPublicKey, credential []byte, attributes []bccsp.IdemixAttribute) error
}

// Revocation handles idemix revocation-related operations
type Revocation interface {

	// NewKey generates a long term signing key that will be used for revocation
	NewKey() (*ecdsa.PrivateKey, error)
}
