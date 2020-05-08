/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"crypto"
	"crypto/x509"
)

// KeyInfo represents a (secret) key that is either already stored
// in the bccsp/keystore or key material to be imported to the
// bccsp key-store. In later versions it may contain also a
// keystore identifier.
type KeyInfo struct {
	// Identifier of the key inside the default keystore; this for
	// the case of Software BCCSP as well as the HSM BCCSP would be
	// the SKI of the key.
	KeyIdentifier string
	// KeyMaterial (optional) for the key to be imported; this
	// must be a supported PKCS#8 private key type of either
	// *rsa.PrivateKey, *ecdsa.PrivateKey, or ed25519.PrivateKey.
	KeyMaterial crypto.PrivateKey
}

// SigningIdentityInfo represents the configuration information
// related to the signing identity the peer is to use for generating
// endorsements.
type SigningIdentityInfo struct {
	// PublicSigner carries the public information of the signing
	// identity. For an X.509 provider this would be represented by
	// an X.509 certificate.
	PublicSigner *x509.Certificate
	// PrivateSigner denotes a reference to the private key of the
	// peer's signing identity.
	PrivateSigner KeyInfo
}

// CryptoConfig contains configuration parameters
// for the cryptographic algorithms used by the MSP
// this configuration refers to.
type CryptoConfig struct {
	// SignatureHashFamily is a string representing the hash family to be used
	// during sign and verify operations.
	// Allowed values are "SHA2" and "SHA3".
	SignatureHashFamily string
	// IdentityIdentifierHashFunction is a string representing the hash function
	// to be used during the computation of the identity identifier of an MSP identity.
	// Allowed values are "SHA256", "SHA384" and "SHA3_256", "SHA3_384".
	IdentityIdentifierHashFunction string
}

// OUIdentifier represents an organizational unit and
// its related chain of trust identifier.
type OUIdentifier struct {
	// Certificate represents the second certificate in a certification chain.
	// (Notice that the first certificate in a certification chain is supposed
	// to be the certificate of an identity).
	// It must correspond to the certificate of root or intermediate CA
	// recognized by the MSP this message belongs to.
	// Starting from this certificate, a certification chain is computed
	// and bound to the OrganizationUnitIdentifier specified.
	Certificate *x509.Certificate
	// OrganizationUnitIdentifier defines the organizational unit under the
	// MSP identified with MSPIdentifier.
	OrganizationalUnitIdentifier string
}

// NodeOUs contains configuration to tell apart clients from peers from orderers
// based on OUs. If NodeOUs recognition is enabled then an msp identity
// that does not contain any of the specified OU will be considered invalid.
type NodeOUs struct {
	// If true then an msp identity that does not contain any of the specified OU will be considered invalid.
	Enable bool
	// OU Identifier of the clients.
	ClientOUIdentifier OUIdentifier
	// OU Identifier of the peers.
	PeerOUIdentifier OUIdentifier
	// OU Identifier of the admins.
	AdminOUIdentifier OUIdentifier
	// OU Identifier of the orderers.
	OrdererOUIdentifier OUIdentifier
}
