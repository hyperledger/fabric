/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

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
