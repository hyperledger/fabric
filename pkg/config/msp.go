/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"crypto/ecdsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"

	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// MSP is the configuration information for
// a Fabric MSP.
// Here we assume a default certificate validation policy, where
// any certificate signed by any of the listed rootCA certs would
// be considered as valid under this MSP.
// This MSP may or may not come with a signing identity. If it does,
// it can also issue signing identities. If it does not, it can only
// be used to validate and verify certificates.
type MSP struct {
	// Name holds the identifier of the MSP; MSP identifier
	// is chosen by the application that governs this MSP.
	// For example, and assuming the default implementation of MSP,
	// that is X.509-based and considers a single Issuer,
	// this can refer to the Subject OU field or the Issuer OU field.
	Name string
	// List of root certificates trusted by this MSP
	// they are used upon certificate validation (see
	// comment for IntermediateCerts below).
	RootCerts []x509.Certificate
	// List of intermediate certificates trusted by this MSP;
	// they are used upon certificate validation as follows:
	// validation attempts to build a path from the certificate
	// to be validated (which is at one end of the path) and
	// one of the certs in the RootCerts field (which is at
	// the other end of the path). If the path is longer than
	// 2, certificates in the middle are searched within the
	// IntermediateCerts pool.
	IntermediateCerts []x509.Certificate
	// Identity denoting the administrator of this MSP.
	Admins []x509.Certificate
	// Identity revocation list.
	RevocationList []pkix.CertificateList
	// SigningIdentity holds information on the signing identity
	// this peer is to use, and which is to be imported by the
	// MSP defined before.
	SigningIdentity *SigningIdentityInfo
	// OrganizationalUnitIdentifiers holds one or more
	// fabric organizational unit identifiers that belong to
	// this MSP configuration.
	OrganizationalUnitIdentifiers []*OUIdentifier
	// CryptoConfig contains the configuration parameters
	// for the cryptographic algorithms used by this MSP.
	CryptoConfig *CryptoConfig
	// List of TLS root certificates trusted by this MSP.
	// They are returned by GetTLSRootCerts.
	TLSRootCerts []x509.Certificate
	// List of TLS intermediate certificates trusted by this MSP;
	// They are returned by GetTLSIntermediateCerts.
	TLSIntermediateCerts []x509.Certificate
	// fabric_node_ous contains the configuration to distinguish clients from peers from orderers
	// based on the OUs.
	NodeOus *NodeOUs
}

// SigningIdentityInfo represents the configuration information
// related to the signing identity the peer is to use for generating
// endorsements.
type SigningIdentityInfo struct {
	// PublicSigner carries the public information of the signing
	// identity. For an X.509 provider this would be represented by
	// an X.509 certificate.
	PublicSigner x509.Certificate
	// PrivateSigner denotes a reference to the private key of the
	// peer's signing identity.
	PrivateSigner *KeyInfo
}

// KeyInfo represents a (secret) key that is either already stored
// in the bccsp/keystore or key material to be imported to the
// bccsp key-store. In later versions it may contain also a
// keystore identifier.
type KeyInfo struct {
	// Identifier of the key inside the default keystore; this for
	// the case of Software BCCSP as well as the HSM BCCSP would be
	// the SKI of the key.
	KeyIdentifier string
	// KeyMaterial (optional) for the key to be imported; this is
	// properly encoded key bytes, prefixed by the type of the key.
	KeyMaterial *ecdsa.PrivateKey
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
	Certificate x509.Certificate
	// OrganizationUnitIdentifier defines the organizational unit under the
	// MSP identified with MSPIdentifier.
	OrganizationalUnitIdentifier string
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

// NodeOUs contains configuration to tell apart clients from peers from orderers
// based on OUs. If NodeOUs recognition is enabled then an msp identity
// that does not contain any of the specified OU will be considered invalid.
type NodeOUs struct {
	// If true then an msp identity that does not contain any of the specified OU will be considered invalid.
	Enable bool
	// OU Identifier of the clients.
	ClientOuIdentifier *OUIdentifier
	// OU Identifier of the peers.
	PeerOuIdentifier *OUIdentifier
	// OU Identifier of the admins.
	AdminOuIdentifier *OUIdentifier
	// OU Identifier of the orderers.
	OrdererOuIdentifier *OUIdentifier
}

// toProto converts an MSP configuration to an mb.FabricMSPConfig proto.
// It pem encodes x509 certificates and ECDSA private keys to byte slices.
func (m *MSP) toProto() (*mb.FabricMSPConfig, error) {
	var err error

	// KeyMaterial is an optional EDCSA private key
	keyMaterial := []byte{}
	if m.SigningIdentity.PrivateSigner.KeyMaterial != nil {
		keyMaterial, err = pemEncodeECDSAPrivateKey(m.SigningIdentity.PrivateSigner.KeyMaterial)
		if err != nil {
			return nil, fmt.Errorf("pem encode X.509 private key: %v", err)
		}
	}

	crl, err := buildPemEncodedCRL(m.RevocationList)
	if err != nil {
		return nil, fmt.Errorf("building pem encoded crl: %v", err)
	}

	signingIdentity := &mb.SigningIdentityInfo{
		PublicSigner: pemEncodeX509Certificate(m.SigningIdentity.PublicSigner),
		PrivateSigner: &mb.KeyInfo{
			KeyIdentifier: m.SigningIdentity.PrivateSigner.KeyIdentifier,
			KeyMaterial:   keyMaterial,
		},
	}

	ouIdentifiers := buildOUIdentifiers(m.OrganizationalUnitIdentifiers)

	fabricNodeOUs := &mb.FabricNodeOUs{
		Enable: m.NodeOus.Enable,
		ClientOuIdentifier: &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.ClientOuIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.ClientOuIdentifier.OrganizationalUnitIdentifier,
		},
		PeerOuIdentifier: &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.PeerOuIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.PeerOuIdentifier.OrganizationalUnitIdentifier,
		},
		AdminOuIdentifier: &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.AdminOuIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.AdminOuIdentifier.OrganizationalUnitIdentifier,
		},
		OrdererOuIdentifier: &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.OrdererOuIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.OrdererOuIdentifier.OrganizationalUnitIdentifier,
		},
	}

	return &mb.FabricMSPConfig{
		Name:                          m.Name,
		RootCerts:                     buildPemEncodedCertListFromX509(m.RootCerts),
		IntermediateCerts:             buildPemEncodedCertListFromX509(m.IntermediateCerts),
		Admins:                        buildPemEncodedCertListFromX509(m.Admins),
		RevocationList:                crl,
		SigningIdentity:               signingIdentity,
		OrganizationalUnitIdentifiers: ouIdentifiers,
		CryptoConfig: &mb.FabricCryptoConfig{
			SignatureHashFamily:            m.CryptoConfig.SignatureHashFamily,
			IdentityIdentifierHashFunction: m.CryptoConfig.IdentityIdentifierHashFunction,
		},
		TlsRootCerts:         buildPemEncodedCertListFromX509(m.TLSRootCerts),
		TlsIntermediateCerts: buildPemEncodedCertListFromX509(m.TLSIntermediateCerts),
		FabricNodeOus:        fabricNodeOUs,
	}, nil
}

func buildOUIdentifiers(identifiers []*OUIdentifier) []*mb.FabricOUIdentifier {
	fabricIdentifiers := []*mb.FabricOUIdentifier{}

	for _, identifier := range identifiers {
		fabricOUIdentifier := &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(identifier.Certificate),
			OrganizationalUnitIdentifier: identifier.OrganizationalUnitIdentifier,
		}

		fabricIdentifiers = append(fabricIdentifiers, fabricOUIdentifier)
	}

	return fabricIdentifiers
}

func buildPemEncodedCRL(crls []pkix.CertificateList) ([][]byte, error) {
	pemEncodedCRL := [][]byte{}

	for _, crl := range crls {
		asn1MarshalledBytes, err := asn1.Marshal(crl)
		if err != nil {
			return nil, fmt.Errorf("asn1 marshalling: %v", err)
		}

		pemEncodedCRL = append(pemEncodedCRL, pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: asn1MarshalledBytes}))
	}

	return pemEncodedCRL, nil
}

func buildPemEncodedCertListFromX509(certList []x509.Certificate) [][]byte {
	certs := [][]byte{}
	for _, cert := range certList {
		certs = append(certs, pemEncodeX509Certificate(cert))
	}

	return certs
}

func pemEncodeX509Certificate(cert x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func pemEncodeECDSAPrivateKey(priv *ecdsa.PrivateKey) ([]byte, error) {
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshalling PKCS8 private key: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}), nil
}
