/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"crypto/ecdsa"

	"github.com/hyperledger/fabric/bccsp"
)

// IssuerPublicKey is the issuer public key
type IssuerPublicKey interface {

	// Bytes returns the byte representation of this key
	Bytes() ([]byte, error)

	// Hash returns the hash representation of this key.
	// The output is supposed to be collision-resistant
	Hash() []byte
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

	// NewPublicKeyFromBytes converts the passed bytes to an Issuer public key
	// It makes sure that the so obtained public key has the passed attributes, if specified
	NewPublicKeyFromBytes(raw []byte, attributes []string) (IssuerPublicKey, error)
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

	// NewKeyFromBytes converts the passed bytes to a User secret key
	NewKeyFromBytes(raw []byte) (Big, error)

	// MakeNym creates a new unlinkable pseudonym
	MakeNym(sk Big, key IssuerPublicKey) (Ecp, Big, error)

	// NewPublicNymFromBytes converts the passed bytes to a public nym
	NewPublicNymFromBytes(raw []byte) (Ecp, error)
}

// CredRequest is a local interface to decouple from the idemix implementation
// of the issuance of credential requests.
type CredRequest interface {
	// Sign creates a new Credential Request, the first message of the interactive credential issuance protocol
	// (from user to issuer)
	Sign(sk Big, ipk IssuerPublicKey, nonce []byte) ([]byte, error)

	// Verify verifies the credential request
	Verify(credRequest []byte, ipk IssuerPublicKey, nonce []byte) error
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

// Revocation is a local interface to decouple from the idemix implementation
// the revocation-related operations
type Revocation interface {

	// NewKey generates a long term signing key that will be used for revocation
	NewKey() (*ecdsa.PrivateKey, error)

	// Sign creates the Credential Revocation Information for a certain time period (epoch).
	// Users can use the CRI to prove that they are not revoked.
	// Note that when not using revocation (i.e., alg = ALG_NO_REVOCATION), the entered unrevokedHandles are not used,
	// and the resulting CRI can be used by any signer.
	Sign(key *ecdsa.PrivateKey, unrevokedHandles [][]byte, epoch int, alg bccsp.RevocationAlgorithm) ([]byte, error)

	// Verify verifies that the revocation PK for a certain epoch is valid,
	// by checking that it was signed with the long term revocation key.
	// Note that even if we use no revocation (i.e., alg = ALG_NO_REVOCATION), we need
	// to verify the signature to make sure the issuer indeed signed that no revocation
	// is used in this epoch.
	Verify(pk *ecdsa.PublicKey, cri []byte, epoch int, alg bccsp.RevocationAlgorithm) error
}

// SignatureScheme is a local interface to decouple from the idemix implementation
// the sign-related operations
type SignatureScheme interface {
	// Sign creates a new idemix signature (Schnorr-type signature).
	// The attributes slice steers which attributes are disclosed:
	// If attributes[i].Type == bccsp.IdemixHiddenAttribute then attribute i remains hidden and otherwise it is disclosed.
	// We require the revocation handle to remain undisclosed (i.e., attributes[rhIndex] == bccsp.IdemixHiddenAttribute).
	// Parameters are to be understood as follow:
	// cred: the serialized version of an idemix credential;
	// sk: the user secret key;
	// (Nym, RNym): Nym key-pair;
	// ipk: issuer public key;
	// attributes: as described above;
	// msg: the message to be signed;
	// rhIndex: revocation handle index relative to attributes;
	// cri: the serialized version of the Credential Revocation Information (it contains the epoch this signature
	// is created in reference to).
	Sign(cred []byte, sk Big, Nym Ecp, RNym Big, ipk IssuerPublicKey, attributes []bccsp.IdemixAttribute,
		msg []byte, rhIndex int, cri []byte) ([]byte, error)

	// Verify verifies an idemix signature.
	// The attribute slice steers which attributes it expects to be disclosed
	// If attributes[i].Type == bccsp.IdemixHiddenAttribute then attribute i remains hidden and otherwise
	// attributes[i].Value is expected to contain the disclosed attribute value.
	// In other words, this function will check that if attribute i is disclosed, the i-th attribute equals attributes[i].Value.
	// Parameters are to be understood as follow:
	// ipk: issuer public key;
	// signature: signature to verify;
	// msg: message signed;
	// attributes: as described above;
	// rhIndex: revocation handle index relative to attributes;
	// revocationPublicKey: revocation public key;
	// epoch: revocation epoch.
	Verify(ipk IssuerPublicKey, signature, msg []byte, attributes []bccsp.IdemixAttribute, rhIndex int, revocationPublicKey *ecdsa.PublicKey, epoch int) error
}

// NymSignatureScheme is a local interface to decouple from the idemix implementation
// the nym sign-related operations
type NymSignatureScheme interface {
	// Sign creates a new idemix pseudonym signature
	Sign(sk Big, Nym Ecp, RNym Big, ipk IssuerPublicKey, digest []byte) ([]byte, error)

	// Verify verifies an idemix NymSignature
	Verify(pk IssuerPublicKey, Nym Ecp, signature, digest []byte) error
}
