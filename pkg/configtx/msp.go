/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtx

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	mb "github.com/hyperledger/fabric-protos-go/msp"
)

// MSP is the configuration information for a Fabric MSP.
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
	RootCerts []*x509.Certificate
	// List of intermediate certificates trusted by this MSP;
	// they are used upon certificate validation as follows:
	// validation attempts to build a path from the certificate
	// to be validated (which is at one end of the path) and
	// one of the certs in the RootCerts field (which is at
	// the other end of the path). If the path is longer than
	// 2, certificates in the middle are searched within the
	// IntermediateCerts pool.
	IntermediateCerts []*x509.Certificate
	// Identity denoting the administrator of this MSP.
	Admins []*x509.Certificate
	// Identity revocation list.
	RevocationList []*pkix.CertificateList
	// SigningIdentity holds information on the signing identity
	// this peer is to use, and which is to be imported by the
	// MSP defined before.
	SigningIdentity SigningIdentityInfo
	// OrganizationalUnitIdentifiers holds one or more
	// fabric organizational unit identifiers that belong to
	// this MSP configuration.
	OrganizationalUnitIdentifiers []OUIdentifier
	// CryptoConfig contains the configuration parameters
	// for the cryptographic algorithms used by this MSP.
	CryptoConfig CryptoConfig
	// List of TLS root certificates trusted by this MSP.
	// They are returned by GetTLSRootCerts.
	TLSRootCerts []*x509.Certificate
	// List of TLS intermediate certificates trusted by this MSP;
	// They are returned by GetTLSIntermediateCerts.
	TLSIntermediateCerts []*x509.Certificate
	// fabric_node_ous contains the configuration to distinguish clients from peers from orderers
	// based on the OUs.
	NodeOus NodeOUs
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
	ClientOUIdentifier OUIdentifier
	// OU Identifier of the peers.
	PeerOUIdentifier OUIdentifier
	// OU Identifier of the admins.
	AdminOUIdentifier OUIdentifier
	// OU Identifier of the orderers.
	OrdererOUIdentifier OUIdentifier
}

// ApplicationMSP returns the MSP configuration for an existing application
// org in a config transaction.
func (c *ConfigTx) ApplicationMSP(orgName string) (MSP, error) {
	applicationOrgGroup, ok := c.base.ChannelGroup.Groups[ApplicationGroupKey].Groups[orgName]
	if !ok {
		return MSP{}, fmt.Errorf("application org %s does not exist in config", orgName)
	}

	return getMSPConfig(applicationOrgGroup)
}

// OrdererMSP returns the MSP configuration for an existing orderer org
// in a config transaction.
func (c *ConfigTx) OrdererMSP(orgName string) (MSP, error) {
	ordererOrgGroup, ok := c.base.ChannelGroup.Groups[OrdererGroupKey].Groups[orgName]
	if !ok {
		return MSP{}, fmt.Errorf("orderer org %s does not exist in config", orgName)
	}

	return getMSPConfig(ordererOrgGroup)
}

// ConsortiumMSP returns the MSP configuration for an existing consortium
// org in a config transaction.
func (c *ConfigTx) ConsortiumMSP(consortiumName, orgName string) (MSP, error) {
	consortiumGroup, ok := c.base.ChannelGroup.Groups[ConsortiumsGroupKey].Groups[consortiumName]
	if !ok {
		return MSP{}, fmt.Errorf("consortium %s does not exist in config", consortiumName)
	}

	consortiumOrgGroup, ok := consortiumGroup.Groups[orgName]
	if !ok {
		return MSP{}, fmt.Errorf("consortium org %s does not exist in config", orgName)
	}

	return getMSPConfig(consortiumOrgGroup)
}

// YEAR is a time duration for a standard 365 day year.
const YEAR = 365 * 24 * time.Hour

// CreateApplicationMSPCRL creates a CRL that revokes the provided certificates
// for the specified application org signed by the provided SigningIdentity. It
// returns the CRL as the PEM-encoded bytes. If any of the provided certs were
// not signed by any of the root/intermediate CA cets in the MSP configuration,
// it will return an error.
func (c *ConfigTx) CreateApplicationMSPCRL(orgName string, signingIdentity *SigningIdentity, certs ...*x509.Certificate) (*pkix.CertificateList, error) {
	msp, err := c.ApplicationMSP(orgName)
	if err != nil {
		return nil, fmt.Errorf("retrieving application msp: %s", err)
	}

	err = msp.validateCertificates(signingIdentity.Certificate, certs...)
	if err != nil {
		return nil, err
	}

	revokeTime := time.Now().UTC()

	revokedCertificates := make([]pkix.RevokedCertificate, len(certs))
	for i, cert := range certs {
		revokedCertificates[i] = pkix.RevokedCertificate{
			SerialNumber:   cert.SerialNumber,
			RevocationTime: revokeTime,
		}
	}

	crlBytes, err := signingIdentity.Certificate.CreateCRL(rand.Reader, signingIdentity.PrivateKey, revokedCertificates, revokeTime, revokeTime.Add(YEAR))
	if err != nil {
		return nil, err
	}

	crl, err := x509.ParseCRL(crlBytes)
	if err != nil {
		return nil, err
	}

	return crl, nil
}

// validateCertificates first validates that the signing certificate is either
// a root or intermediate CA certificate for the specified application org. It
// then validates that the certificates to add to the CRL were signed by that
// signing certificate.
func (m *MSP) validateCertificates(signingCert *x509.Certificate, certs ...*x509.Certificate) error {
	err := m.isCACert(signingCert)
	if err != nil {
		return err
	}
	for _, cert := range certs {
		if err := cert.CheckSignatureFrom(signingCert); err != nil {
			return fmt.Errorf("certificate not issued by this MSP. serial number: %d", cert.SerialNumber)
		}
	}

	return nil
}

func (m *MSP) isCACert(signingCert *x509.Certificate) error {
	for _, rootCert := range m.RootCerts {
		if signingCert.Equal(rootCert) {
			return nil
		}
	}

	for _, intermediateCert := range m.IntermediateCerts {
		if signingCert.Equal(intermediateCert) {
			return nil
		}
	}
	return fmt.Errorf("signing cert is not a root/intermediate cert for this MSP: %s", m.Name)
}

// getMSPConfig parses the MSP value in a config group returns
// the configuration as an MSP type.
func getMSPConfig(configGroup *cb.ConfigGroup) (MSP, error) {
	mspValueProto := &mb.MSPConfig{}

	err := unmarshalConfigValueAtKey(configGroup, MSPKey, mspValueProto)
	if err != nil {
		return MSP{}, err
	}

	fabricMSPConfig := &mb.FabricMSPConfig{}

	err = proto.Unmarshal(mspValueProto.Config, fabricMSPConfig)
	if err != nil {
		return MSP{}, fmt.Errorf("unmarshaling fabric msp config: %v", err)
	}

	// ROOT CERTS
	rootCerts, err := parseCertificateListFromBytes(fabricMSPConfig.RootCerts)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing root certs: %v", err)
	}

	// INTERMEDIATE CERTS
	intermediateCerts, err := parseCertificateListFromBytes(fabricMSPConfig.IntermediateCerts)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing intermediate certs: %v", err)
	}

	// ADMIN CERTS
	adminCerts, err := parseCertificateListFromBytes(fabricMSPConfig.Admins)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing admin certs: %v", err)
	}

	// REVOCATION LIST
	revocationList, err := parseCRL(fabricMSPConfig.RevocationList)
	if err != nil {
		return MSP{}, err
	}

	// SIGNING IDENTITY
	publicSigner, err := parseCertificateFromBytes(fabricMSPConfig.SigningIdentity.PublicSigner)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing signing identity public signer: %v", err)
	}

	keyMaterial, err := parsePrivateKeyFromBytes(fabricMSPConfig.SigningIdentity.PrivateSigner.KeyMaterial)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing signing identity private key: %v", err)
	}

	signingIdentity := SigningIdentityInfo{
		PublicSigner: publicSigner,
		PrivateSigner: KeyInfo{
			KeyIdentifier: fabricMSPConfig.SigningIdentity.PrivateSigner.KeyIdentifier,
			KeyMaterial:   keyMaterial,
		},
	}

	// OU IDENTIFIERS
	ouIdentifiers, err := parseOUIdentifiers(fabricMSPConfig.OrganizationalUnitIdentifiers)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing ou identifiers: %v", err)
	}

	// TLS ROOT CERTS
	tlsRootCerts, err := parseCertificateListFromBytes(fabricMSPConfig.TlsRootCerts)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing tls root certs: %v", err)
	}

	// TLS INTERMEDIATE CERTS
	tlsIntermediateCerts, err := parseCertificateListFromBytes(fabricMSPConfig.TlsIntermediateCerts)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing tls intermediate certs: %v", err)
	}

	// NODE OUS
	clientOUIdentifierCert, err := parseCertificateFromBytes(fabricMSPConfig.FabricNodeOus.ClientOuIdentifier.Certificate)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing client ou identifier cert: %v", err)
	}

	peerOUIdentifierCert, err := parseCertificateFromBytes(fabricMSPConfig.FabricNodeOus.PeerOuIdentifier.Certificate)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing peer ou identifier cert: %v", err)
	}

	adminOUIdentifierCert, err := parseCertificateFromBytes(fabricMSPConfig.FabricNodeOus.AdminOuIdentifier.Certificate)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing admin ou identifier cert: %v", err)
	}

	ordererOUIdentifierCert, err := parseCertificateFromBytes(fabricMSPConfig.FabricNodeOus.OrdererOuIdentifier.Certificate)
	if err != nil {
		return MSP{}, fmt.Errorf("parsing orderer ou identifier cert: %v", err)
	}

	nodeOUs := NodeOUs{
		Enable: fabricMSPConfig.FabricNodeOus.Enable,
		ClientOUIdentifier: OUIdentifier{
			Certificate:                  clientOUIdentifierCert,
			OrganizationalUnitIdentifier: fabricMSPConfig.FabricNodeOus.ClientOuIdentifier.OrganizationalUnitIdentifier,
		},
		PeerOUIdentifier: OUIdentifier{
			Certificate:                  peerOUIdentifierCert,
			OrganizationalUnitIdentifier: fabricMSPConfig.FabricNodeOus.PeerOuIdentifier.OrganizationalUnitIdentifier,
		},
		AdminOUIdentifier: OUIdentifier{
			Certificate:                  adminOUIdentifierCert,
			OrganizationalUnitIdentifier: fabricMSPConfig.FabricNodeOus.AdminOuIdentifier.OrganizationalUnitIdentifier,
		},
		OrdererOUIdentifier: OUIdentifier{
			Certificate:                  ordererOUIdentifierCert,
			OrganizationalUnitIdentifier: fabricMSPConfig.FabricNodeOus.OrdererOuIdentifier.OrganizationalUnitIdentifier,
		},
	}

	return MSP{
		Name:                          fabricMSPConfig.Name,
		RootCerts:                     rootCerts,
		IntermediateCerts:             intermediateCerts,
		Admins:                        adminCerts,
		RevocationList:                revocationList,
		SigningIdentity:               signingIdentity,
		OrganizationalUnitIdentifiers: ouIdentifiers,
		CryptoConfig: CryptoConfig{
			SignatureHashFamily:            fabricMSPConfig.CryptoConfig.SignatureHashFamily,
			IdentityIdentifierHashFunction: fabricMSPConfig.CryptoConfig.IdentityIdentifierHashFunction,
		},
		TLSRootCerts:         tlsRootCerts,
		TLSIntermediateCerts: tlsIntermediateCerts,
		NodeOus:              nodeOUs,
	}, nil
}

func parseCertificateListFromBytes(certs [][]byte) ([]*x509.Certificate, error) {
	certificateList := []*x509.Certificate{}

	for _, cert := range certs {
		certificate, err := parseCertificateFromBytes(cert)
		if err != nil {
			return certificateList, err
		}

		certificateList = append(certificateList, certificate)
	}

	return certificateList, nil
}

func parseCertificateFromBytes(cert []byte) (*x509.Certificate, error) {
	pemBlock, _ := pem.Decode(cert)

	certificate, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		return &x509.Certificate{}, err
	}

	return certificate, nil
}

func parseCRL(crls [][]byte) ([]*pkix.CertificateList, error) {
	certificateLists := []*pkix.CertificateList{}

	for _, crl := range crls {
		pemBlock, _ := pem.Decode(crl)

		certificateList, err := x509.ParseCRL(pemBlock.Bytes)
		if err != nil {
			return certificateLists, fmt.Errorf("parsing crl: %v", err)
		}

		certificateLists = append(certificateLists, certificateList)
	}

	return certificateLists, nil
}

func parsePrivateKeyFromBytes(priv []byte) (crypto.PrivateKey, error) {
	if len(priv) == 0 {
		return nil, nil
	}

	pemBlock, _ := pem.Decode(priv)

	privateKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed parsing PKCS#8 private key: %v", err)
	}

	return privateKey, nil
}

func parseOUIdentifiers(identifiers []*mb.FabricOUIdentifier) ([]OUIdentifier, error) {
	fabricIdentifiers := []OUIdentifier{}

	for _, identifier := range identifiers {
		cert, err := parseCertificateFromBytes(identifier.Certificate)
		if err != nil {
			return fabricIdentifiers, err
		}

		fabricOUIdentifier := OUIdentifier{
			Certificate:                  cert,
			OrganizationalUnitIdentifier: identifier.OrganizationalUnitIdentifier,
		}

		fabricIdentifiers = append(fabricIdentifiers, fabricOUIdentifier)
	}

	return fabricIdentifiers, nil
}

// toProto converts an MSP configuration to an mb.FabricMSPConfig proto.
// It pem encodes x509 certificates and ECDSA private keys to byte slices.
func (m *MSP) toProto() (*mb.FabricMSPConfig, error) {
	var err error

	// KeyMaterial is an optional EDCSA private key
	keyMaterial := []byte{}
	if m.SigningIdentity.PrivateSigner.KeyMaterial != nil {
		keyMaterial, err = pemEncodePKCS8PrivateKey(m.SigningIdentity.PrivateSigner.KeyMaterial)
		if err != nil {
			return nil, fmt.Errorf("pem encode PKCS#8 private key: %v", err)
		}
	}

	revocationList, err := buildPemEncodedRevocationList(m.RevocationList)
	if err != nil {
		return nil, fmt.Errorf("building pem encoded revocation list: %v", err)
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
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.ClientOUIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.ClientOUIdentifier.OrganizationalUnitIdentifier,
		},
		PeerOuIdentifier: &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.PeerOUIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.PeerOUIdentifier.OrganizationalUnitIdentifier,
		},
		AdminOuIdentifier: &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.AdminOUIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.AdminOUIdentifier.OrganizationalUnitIdentifier,
		},
		OrdererOuIdentifier: &mb.FabricOUIdentifier{
			Certificate:                  pemEncodeX509Certificate(m.NodeOus.OrdererOUIdentifier.Certificate),
			OrganizationalUnitIdentifier: m.NodeOus.OrdererOUIdentifier.OrganizationalUnitIdentifier,
		},
	}

	return &mb.FabricMSPConfig{
		Name:                          m.Name,
		RootCerts:                     buildPemEncodedCertListFromX509(m.RootCerts),
		IntermediateCerts:             buildPemEncodedCertListFromX509(m.IntermediateCerts),
		Admins:                        buildPemEncodedCertListFromX509(m.Admins),
		RevocationList:                revocationList,
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

func buildOUIdentifiers(identifiers []OUIdentifier) []*mb.FabricOUIdentifier {
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

// buildPemEncodedRevocationList returns a byte slice of the pem-encoded
// CRLs for a revocation list.
func buildPemEncodedRevocationList(crls []*pkix.CertificateList) ([][]byte, error) {
	pemEncodedRevocationList := [][]byte{}

	for _, crl := range crls {
		// asn1MarshalledBytes, err := asn1.Marshal(*crl)
		pemCRL, err := pemEncodeCRL(crl)
		if err != nil {
			return nil, err
		}

		pemEncodedRevocationList = append(pemEncodedRevocationList, pemCRL)
	}

	return pemEncodedRevocationList, nil
}

func pemEncodeCRL(crl *pkix.CertificateList) ([]byte, error) {
	asn1MarshalledBytes, err := asn1.Marshal(*crl)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: asn1MarshalledBytes}), nil
}

func buildPemEncodedCertListFromX509(certList []*x509.Certificate) [][]byte {
	certs := [][]byte{}
	for _, cert := range certList {
		certs = append(certs, pemEncodeX509Certificate(cert))
	}

	return certs
}

func pemEncodeX509Certificate(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func pemEncodePKCS8PrivateKey(priv crypto.PrivateKey) ([]byte, error) {
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshaling PKCS#8 private key: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}), nil
}

// UpdateApplicationMSP updates the MSP config for the specified application
// org group.
func (c *ConfigTx) UpdateApplicationMSP(updatedMSP MSP, orgName string) error {
	currentMSP, err := c.ApplicationMSP(orgName)
	if err != nil {
		return fmt.Errorf("retrieving msp: %v", err)
	}

	if currentMSP.Name != updatedMSP.Name {
		return errors.New("MSP name cannot be changed")
	}

	err = updatedMSP.validateCACerts()
	if err != nil {
		return err
	}

	err = setMSPConfigForOrg(c.Updated(), updatedMSP, orgName)
	if err != nil {
		return err
	}

	return nil
}

func setMSPConfigForOrg(config *cb.Config, updatedMSP MSP, orgName string) error {
	fabricMSPConfig, err := updatedMSP.toProto()
	if err != nil {
		return err
	}

	conf, err := proto.Marshal(fabricMSPConfig)
	if err != nil {
		return fmt.Errorf("marshaling msp config: %v", err)
	}

	mspConfig := &mb.MSPConfig{
		Config: conf,
	}

	orgGroup, err := getApplicationOrg(config, orgName)
	if err != nil {
		return err
	}

	err = setValue(orgGroup, mspValue(mspConfig), AdminsPolicyKey)
	if err != nil {
		return err
	}

	return nil
}

func (m *MSP) validateCACerts() error {
	err := validateCACerts(m.RootCerts)
	if err != nil {
		return fmt.Errorf("invalid root cert: %v", err)
	}

	err = validateCACerts(m.IntermediateCerts)
	if err != nil {
		return fmt.Errorf("invalid intermediate cert: %v", err)
	}
	//TODO: follow the workaround that msp code use to incorporate cert.Verify()
	for _, ic := range m.IntermediateCerts {
		validIntermediateCert := false
		for _, rc := range m.RootCerts {
			err := ic.CheckSignatureFrom(rc)
			if err == nil {
				validIntermediateCert = true
				break
			}
		}
		if !validIntermediateCert {
			return fmt.Errorf("intermediate cert not signed by any root certs of this MSP. serial number: %d", ic.SerialNumber)
		}
	}

	err = validateCACerts(m.TLSRootCerts)
	if err != nil {
		return fmt.Errorf("invalid tls root cert: %v", err)
	}

	err = validateCACerts(m.TLSIntermediateCerts)
	if err != nil {
		return fmt.Errorf("invalid tls intermediate cert: %v", err)
	}

	return nil
}

func validateCACerts(caCerts []*x509.Certificate) error {
	for _, caCert := range caCerts {
		if (caCert.KeyUsage & x509.KeyUsageCertSign) == 0 {
			return fmt.Errorf("KeyUsage must be x509.KeyUsageCertSign. serial number: %d", caCert.SerialNumber)
		}

		if !caCert.IsCA {
			return fmt.Errorf("must be a CA certificate. serial number: %d", caCert.SerialNumber)
		}
	}

	return nil
}
