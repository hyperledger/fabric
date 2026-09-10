/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/utils"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (msp *bccspmsp) getCertifiersIdentifier(certRaw []byte) ([]byte, error) {
	// 1. check that certificate is registered in msp.rootCerts or msp.intermediateCerts
	cert, err := msp.getCertFromPem(certRaw)
	if err != nil {
		return nil, fmt.Errorf("Failed getting certificate for [%v]: [%s]", certRaw, err)
	}

	// 2. Sanitize it to ensure like for like comparison
	cert, err = msp.sanitizeCert(cert)
	if err != nil {
		return nil, fmt.Errorf("sanitizeCert failed %s", err)
	}

	found := false
	root := false
	// Search among root certificates
	for _, v := range msp.rootCerts {
		if v.(*identity).cert.Equal(cert) {
			found = true
			root = true
			break
		}
	}
	if !found {
		// Search among root intermediate certificates
		for _, v := range msp.intermediateCerts {
			if v.(*identity).cert.Equal(cert) {
				found = true
				break
			}
		}
	}
	if !found {
		// Certificate not valid, reject configuration
		return nil, fmt.Errorf("Failed adding OU. Certificate [%v] not in root or intermediate certs.", cert)
	}

	// 3. get the certification path for it
	var certifiersIdentifier []byte
	var chain []*x509.Certificate
	if root {
		chain = []*x509.Certificate{cert}
	} else {
		chain, err = msp.getValidationChain(cert, true)
		if err != nil {
			return nil, fmt.Errorf("Failed computing validation chain for [%v]. [%s]", cert, err)
		}
	}

	// 4. compute the hash of the certification path
	certifiersIdentifier, err = msp.getCertificationChainIdentifierFromChain(chain)
	if err != nil {
		return nil, fmt.Errorf("Failed computing Certifiers Identifier for [%v]. [%s]", certRaw, err)
	}

	return certifiersIdentifier, nil
}

func (msp *bccspmsp) setupCrypto(conf *m.FabricMSPConfig) error {
	msp.cryptoConfig = conf.GetCryptoConfig()
	if msp.cryptoConfig == nil {
		// Move to defaults
		msp.cryptoConfig = &m.FabricCryptoConfig{
			SignatureHashFamily:            bccsp.SHA2,
			IdentityIdentifierHashFunction: bccsp.SHA256,
		}
		mspLogger.Debugf("CryptoConfig was nil. Move to defaults.")
	}
	if msp.cryptoConfig.GetSignatureHashFamily() == "" {
		msp.cryptoConfig.SignatureHashFamily = bccsp.SHA2
		mspLogger.Debugf("CryptoConfig.SignatureHashFamily was nil. Move to defaults.")
	}
	if msp.cryptoConfig.GetIdentityIdentifierHashFunction() == "" {
		msp.cryptoConfig.IdentityIdentifierHashFunction = bccsp.SHA256
		mspLogger.Debugf("CryptoConfig.IdentityIdentifierHashFunction was nil. Move to defaults.")
	}

	msp.supportedPublicKeyAlgorithms = make(map[x509.PublicKeyAlgorithm]bool)
	msp.supportedPublicKeyAlgorithms[x509.ECDSA] = true

	return nil
}

func (msp *bccspmsp) setupCAs(conf *m.FabricMSPConfig) error {
	// make and fill the set of CA certs - we expect them to be there
	if len(conf.GetRootCerts()) == 0 {
		return errors.New("expected at least one CA certificate")
	}

	// pre-create the verify options with roots and intermediates.
	// This is needed to make certificate sanitation working.
	// Recall that sanitization is applied also to root CA and intermediate
	// CA certificates. After their sanitization is done, the opts
	// will be recreated using the sanitized certs.
	msp.opts = &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()}
	for _, v := range conf.GetRootCerts() {
		cert, err := msp.getCertFromPem(v)
		if err != nil {
			return err
		}
		msp.opts.Roots.AddCert(cert)
	}
	for _, v := range conf.GetIntermediateCerts() {
		cert, err := msp.getCertFromPem(v)
		if err != nil {
			return err
		}
		msp.opts.Intermediates.AddCert(cert)
	}

	// Load root and intermediate CA identities
	// Recall that when an identity is created, its certificate gets sanitized
	msp.rootCerts = make([]Identity, len(conf.GetRootCerts()))
	for i, trustedCert := range conf.GetRootCerts() {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.rootCerts[i] = id
	}

	// make and fill the set of intermediate certs (if present)
	msp.intermediateCerts = make([]Identity, len(conf.GetIntermediateCerts()))
	for i, trustedCert := range conf.GetIntermediateCerts() {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.intermediateCerts[i] = id
	}

	// root CA and intermediate CA certificates are sanitized, they can be re-imported
	msp.opts = &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()}
	for _, id := range msp.rootCerts {
		msp.opts.Roots.AddCert(id.(*identity).cert)
	}
	for _, id := range msp.intermediateCerts {
		msp.opts.Intermediates.AddCert(id.(*identity).cert)
	}

	return nil
}

func (msp *bccspmsp) setupAdmins(conf *m.FabricMSPConfig) error {
	return msp.internalSetupAdmin(conf)
}

func (msp *bccspmsp) setupAdminsPreV142(conf *m.FabricMSPConfig) error {
	// make and fill the set of admin certs (if present)
	msp.admins = make([]Identity, len(conf.GetAdmins()))
	for i, admCert := range conf.GetAdmins() {
		id, _, err := msp.getIdentityFromConf(admCert)
		if err != nil {
			return err
		}

		msp.admins[i] = id
	}

	return nil
}

func (msp *bccspmsp) setupAdminsV142(conf *m.FabricMSPConfig) error {
	// make and fill the set of admin certs (if present)
	if err := msp.setupAdminsPreV142(conf); err != nil {
		return err
	}

	if len(msp.admins) == 0 && (!msp.ouEnforcement || msp.adminOU == nil) {
		return errors.New("administrators must be declared when no admin ou classification is set")
	}

	return nil
}

func isECDSASignatureAlgorithm(algid asn1.ObjectIdentifier) bool {
	// This is the set of ECDSA algorithms supported by Go 1.14 for CRL
	// signatures.
	ecdsaSignaureAlgorithms := []asn1.ObjectIdentifier{
		{1, 2, 840, 10045, 4, 1},    // oidSignatureECDSAWithSHA1
		{1, 2, 840, 10045, 4, 3, 2}, // oidSignatureECDSAWithSHA256
		{1, 2, 840, 10045, 4, 3, 3}, // oidSignatureECDSAWithSHA384
		{1, 2, 840, 10045, 4, 3, 4}, // oidSignatureECDSAWithSHA512
	}
	for _, id := range ecdsaSignaureAlgorithms {
		if id.Equal(algid) {
			return true
		}
	}
	return false
}

func (msp *bccspmsp) setupCRLs(conf *m.FabricMSPConfig) error {
	// setup the CRL (if present)
	msp.CRL = make([]*pkix.CertificateList, len(conf.GetRevocationList()))
	for i, crlbytes := range conf.GetRevocationList() {
		crl, err := x509.ParseCRL(crlbytes)
		if err != nil {
			return errors.Wrap(err, "could not parse RevocationList")
		}

		// Massage the ECDSA signature values
		if isECDSASignatureAlgorithm(crl.SignatureAlgorithm.Algorithm) {
			r, s, err := utils.UnmarshalECDSASignature(crl.SignatureValue.RightAlign())
			if err != nil {
				return err
			}
			sig, err := utils.MarshalECDSASignature(r, s)
			if err != nil {
				return err
			}
			crl.SignatureValue = asn1.BitString{Bytes: sig, BitLength: 8 * len(sig)}
		}

		// TODO: pre-verify the signature on the CRL and create a map
		//       of CA certs to respective CRLs so that later upon
		//       validation we can already look up the CRL given the
		//       chain of the certificate to be validated

		msp.CRL[i] = crl
	}

	return nil
}

func (msp *bccspmsp) finalizeSetupCAs() error {
	// ensure that our CAs are properly formed and that they are valid
	for _, id := range append(append([]Identity{}, msp.rootCerts...), msp.intermediateCerts...) {
		if !id.(*identity).cert.IsCA {
			return errors.Errorf("CA Certificate did not have the CA attribute, (SN: %x)", id.(*identity).cert.SerialNumber)
		}
		if _, err := getSubjectKeyIdentifierFromCert(id.(*identity).cert); err != nil {
			return errors.WithMessagef(err, "CA Certificate problem with Subject Key Identifier extension, (SN: %x)", id.(*identity).cert.SerialNumber)
		}

		if err := msp.validateCAIdentity(id.(*identity)); err != nil {
			return errors.WithMessagef(err, "CA Certificate is not valid, (SN: %s)", id.(*identity).cert.SerialNumber)
		}
	}

	// populate certificationTreeInternalNodesMap to mark the internal nodes of the
	// certification tree
	msp.certificationTreeInternalNodesMap = make(map[string]bool)
	for _, id := range append([]Identity{}, msp.intermediateCerts...) {
		chain, err := msp.getUniqueValidationChain(id.(*identity).cert, msp.getValidityOptsForCert(id.(*identity).cert))
		if err != nil {
			return errors.WithMessagef(err, "failed getting validation chain, (SN: %s)", id.(*identity).cert.SerialNumber)
		}

		// Recall chain[0] is id.(*identity).id so it does not count as a parent
		for i := 1; i < len(chain); i++ {
			msp.certificationTreeInternalNodesMap[string(chain[i].Raw)] = true
		}
	}

	return nil
}

func (msp *bccspmsp) setupNodeOUs(config *m.FabricMSPConfig) error {
	if config.GetFabricNodeOus() != nil {

		msp.ouEnforcement = config.GetFabricNodeOus().GetEnable()

		if config.GetFabricNodeOus().GetClientOuIdentifier() == nil || len(config.GetFabricNodeOus().GetClientOuIdentifier().GetOrganizationalUnitIdentifier()) == 0 {
			return errors.New("Failed setting up NodeOUs. ClientOU must be different from nil.")
		}

		if config.GetFabricNodeOus().GetPeerOuIdentifier() == nil || len(config.GetFabricNodeOus().GetPeerOuIdentifier().GetOrganizationalUnitIdentifier()) == 0 {
			return errors.New("Failed setting up NodeOUs. PeerOU must be different from nil.")
		}

		// ClientOU
		msp.clientOU = &OUIdentifier{OrganizationalUnitIdentifier: config.GetFabricNodeOus().GetClientOuIdentifier().GetOrganizationalUnitIdentifier()}
		if len(config.GetFabricNodeOus().GetClientOuIdentifier().GetCertificate()) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.GetFabricNodeOus().GetClientOuIdentifier().GetCertificate())
			if err != nil {
				return err
			}
			msp.clientOU.CertifiersIdentifier = certifiersIdentifier
		}

		// PeerOU
		msp.peerOU = &OUIdentifier{OrganizationalUnitIdentifier: config.GetFabricNodeOus().GetPeerOuIdentifier().GetOrganizationalUnitIdentifier()}
		if len(config.GetFabricNodeOus().GetPeerOuIdentifier().GetCertificate()) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.GetFabricNodeOus().GetPeerOuIdentifier().GetCertificate())
			if err != nil {
				return err
			}
			msp.peerOU.CertifiersIdentifier = certifiersIdentifier
		}

	} else {
		msp.ouEnforcement = false
	}

	return nil
}

func (msp *bccspmsp) setupNodeOUsV142(config *m.FabricMSPConfig) error {
	if config.GetFabricNodeOus() == nil {
		msp.ouEnforcement = false
		return nil
	}

	msp.ouEnforcement = config.GetFabricNodeOus().GetEnable()

	counter := 0
	// ClientOU
	if config.GetFabricNodeOus().GetClientOuIdentifier() != nil {
		msp.clientOU = &OUIdentifier{OrganizationalUnitIdentifier: config.GetFabricNodeOus().GetClientOuIdentifier().GetOrganizationalUnitIdentifier()}
		if len(config.GetFabricNodeOus().GetClientOuIdentifier().GetCertificate()) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.GetFabricNodeOus().GetClientOuIdentifier().GetCertificate())
			if err != nil {
				return err
			}
			msp.clientOU.CertifiersIdentifier = certifiersIdentifier
		}
		counter++
	} else {
		msp.clientOU = nil
	}

	// PeerOU
	if config.GetFabricNodeOus().GetPeerOuIdentifier() != nil {
		msp.peerOU = &OUIdentifier{OrganizationalUnitIdentifier: config.GetFabricNodeOus().GetPeerOuIdentifier().GetOrganizationalUnitIdentifier()}
		if len(config.GetFabricNodeOus().GetPeerOuIdentifier().GetCertificate()) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.GetFabricNodeOus().GetPeerOuIdentifier().GetCertificate())
			if err != nil {
				return err
			}
			msp.peerOU.CertifiersIdentifier = certifiersIdentifier
		}
		counter++
	} else {
		msp.peerOU = nil
	}

	// AdminOU
	if config.GetFabricNodeOus().GetAdminOuIdentifier() != nil {
		msp.adminOU = &OUIdentifier{OrganizationalUnitIdentifier: config.GetFabricNodeOus().GetAdminOuIdentifier().GetOrganizationalUnitIdentifier()}
		if len(config.GetFabricNodeOus().GetAdminOuIdentifier().GetCertificate()) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.GetFabricNodeOus().GetAdminOuIdentifier().GetCertificate())
			if err != nil {
				return err
			}
			msp.adminOU.CertifiersIdentifier = certifiersIdentifier
		}
		counter++
	} else {
		msp.adminOU = nil
	}

	// OrdererOU
	if config.GetFabricNodeOus().GetOrdererOuIdentifier() != nil {
		msp.ordererOU = &OUIdentifier{OrganizationalUnitIdentifier: config.GetFabricNodeOus().GetOrdererOuIdentifier().GetOrganizationalUnitIdentifier()}
		if len(config.GetFabricNodeOus().GetOrdererOuIdentifier().GetCertificate()) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.GetFabricNodeOus().GetOrdererOuIdentifier().GetCertificate())
			if err != nil {
				return err
			}
			msp.ordererOU.CertifiersIdentifier = certifiersIdentifier
		}
		counter++
	} else {
		msp.ordererOU = nil
	}

	if counter == 0 {
		// Disable NodeOU
		msp.ouEnforcement = false
	}

	return nil
}

func (msp *bccspmsp) setupSigningIdentity(conf *m.FabricMSPConfig) error {
	if conf.GetSigningIdentity() != nil {
		sid, err := msp.getSigningIdentityFromConf(conf.GetSigningIdentity())
		if err != nil {
			return err
		}

		expirationTime := sid.ExpiresAt()
		now := time.Now()
		if expirationTime.After(now) {
			mspLogger.Debug("Signing identity expires at", expirationTime)
		} else if expirationTime.IsZero() {
			mspLogger.Debug("Signing identity has no known expiration time")
		} else {
			return errors.Errorf("signing identity expired %v ago", now.Sub(expirationTime))
		}

		msp.signer = sid
	}

	return nil
}

func (msp *bccspmsp) setupOUs(conf *m.FabricMSPConfig) error {
	msp.ouIdentifiers = make(map[string][][]byte)
	for _, ou := range conf.GetOrganizationalUnitIdentifiers() {

		certifiersIdentifier, err := msp.getCertifiersIdentifier(ou.GetCertificate())
		if err != nil {
			return errors.WithMessagef(err, "failed getting certificate for [%v]", ou)
		}

		// Check for duplicates
		found := false
		for _, id := range msp.ouIdentifiers[ou.GetOrganizationalUnitIdentifier()] {
			if bytes.Equal(id, certifiersIdentifier) {
				mspLogger.Warningf("Duplicate found in ou identifiers [%s, %v]", ou.GetOrganizationalUnitIdentifier(), id)
				found = true
				break
			}
		}

		if !found {
			// No duplicates found, add it
			msp.ouIdentifiers[ou.GetOrganizationalUnitIdentifier()] = append(
				msp.ouIdentifiers[ou.GetOrganizationalUnitIdentifier()],
				certifiersIdentifier,
			)
		}
	}

	return nil
}

func (msp *bccspmsp) setupTLSCAs(conf *m.FabricMSPConfig) error {
	opts := &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()}

	// Load TLS root and intermediate CA identities
	msp.tlsRootCerts = make([][]byte, len(conf.GetTlsRootCerts()))
	rootCerts := make([]*x509.Certificate, len(conf.GetTlsRootCerts()))
	for i, trustedCert := range conf.GetTlsRootCerts() {
		cert, err := msp.getCertFromPem(trustedCert)
		if err != nil {
			return err
		}

		rootCerts[i] = cert
		msp.tlsRootCerts[i] = trustedCert
		opts.Roots.AddCert(cert)
	}

	// make and fill the set of intermediate certs (if present)
	msp.tlsIntermediateCerts = make([][]byte, len(conf.GetTlsIntermediateCerts()))
	intermediateCerts := make([]*x509.Certificate, len(conf.GetTlsIntermediateCerts()))
	for i, trustedCert := range conf.GetTlsIntermediateCerts() {
		cert, err := msp.getCertFromPem(trustedCert)
		if err != nil {
			return err
		}

		intermediateCerts[i] = cert
		msp.tlsIntermediateCerts[i] = trustedCert
		opts.Intermediates.AddCert(cert)
	}

	// ensure that our CAs are properly formed and that they are valid
	for _, cert := range append(append([]*x509.Certificate{}, rootCerts...), intermediateCerts...) {
		if cert == nil {
			continue
		}

		if !cert.IsCA {
			return errors.Errorf("CA Certificate did not have the CA attribute, (SN: %x)", cert.SerialNumber)
		}
		if _, err := getSubjectKeyIdentifierFromCert(cert); err != nil {
			return errors.WithMessagef(err, "CA Certificate problem with Subject Key Identifier extension, (SN: %x)", cert.SerialNumber)
		}

		opts.CurrentTime = cert.NotBefore.Add(time.Second)
		if err := msp.validateTLSCAIdentity(cert, opts); err != nil {
			return errors.WithMessagef(err, "CA Certificate is not valid, (SN: %s)", cert.SerialNumber)
		}
	}

	return nil
}

func (msp *bccspmsp) setupV1(conf1 *m.FabricMSPConfig) error {
	err := msp.preSetupV1(conf1)
	if err != nil {
		return err
	}

	err = msp.postSetupV1(conf1)
	if err != nil {
		return err
	}

	return nil
}

func (msp *bccspmsp) preSetupV1(conf *m.FabricMSPConfig) error {
	// setup crypto config
	if err := msp.setupCrypto(conf); err != nil {
		return err
	}

	// Setup CAs
	if err := msp.setupCAs(conf); err != nil {
		return err
	}

	// Setup Admins
	if err := msp.setupAdmins(conf); err != nil {
		return err
	}

	// Setup CRLs
	if err := msp.setupCRLs(conf); err != nil {
		return err
	}

	// Finalize setup of the CAs
	if err := msp.finalizeSetupCAs(); err != nil {
		return err
	}

	// setup the signer (if present)
	if err := msp.setupSigningIdentity(conf); err != nil {
		return err
	}

	// setup TLS CAs
	if err := msp.setupTLSCAs(conf); err != nil {
		return err
	}

	// setup the OUs
	if err := msp.setupOUs(conf); err != nil {
		return err
	}

	return nil
}

func (msp *bccspmsp) preSetupV142(conf *m.FabricMSPConfig) error {
	// setup crypto config
	if err := msp.setupCrypto(conf); err != nil {
		return err
	}

	// Setup CAs
	if err := msp.setupCAs(conf); err != nil {
		return err
	}

	// Setup CRLs
	if err := msp.setupCRLs(conf); err != nil {
		return err
	}

	// Finalize setup of the CAs
	if err := msp.finalizeSetupCAs(); err != nil {
		return err
	}

	// setup the signer (if present)
	if err := msp.setupSigningIdentity(conf); err != nil {
		return err
	}

	// setup TLS CAs
	if err := msp.setupTLSCAs(conf); err != nil {
		return err
	}

	// setup the OUs
	if err := msp.setupOUs(conf); err != nil {
		return err
	}

	// setup NodeOUs
	if err := msp.setupNodeOUsV142(conf); err != nil {
		return err
	}

	// Setup Admins
	if err := msp.setupAdmins(conf); err != nil {
		return err
	}

	return nil
}

func (msp *bccspmsp) postSetupV1(conf *m.FabricMSPConfig) error {
	// make sure that admins are valid members as well
	// this way, when we validate an admin MSP principal
	// we can simply check for exact match of certs
	for i, admin := range msp.admins {
		err := admin.Validate()
		if err != nil {
			return errors.WithMessagef(err, "admin %d is invalid", i)
		}
	}

	return nil
}

func (msp *bccspmsp) setupV11(conf *m.FabricMSPConfig) error {
	err := msp.preSetupV1(conf)
	if err != nil {
		return err
	}

	// setup NodeOUs
	if err := msp.setupNodeOUs(conf); err != nil {
		return err
	}

	err = msp.postSetupV11(conf)
	if err != nil {
		return err
	}

	return nil
}

func (msp *bccspmsp) setupV142(conf *m.FabricMSPConfig) error {
	err := msp.preSetupV142(conf)
	if err != nil {
		return err
	}

	err = msp.postSetupV142(conf)
	if err != nil {
		return err
	}

	return nil
}

func (msp *bccspmsp) setupV3(conf *m.FabricMSPConfig) error {
	err := msp.preSetupV142(conf)
	if err != nil {
		return err
	}

	msp.supportedPublicKeyAlgorithms[x509.Ed25519] = true

	err = msp.postSetupV142(conf)
	if err != nil {
		return err
	}

	return nil
}

func (msp *bccspmsp) postSetupV11(conf *m.FabricMSPConfig) error {
	// Check for OU enforcement
	if !msp.ouEnforcement {
		// No enforcement required. Call post setup as per V1
		return msp.postSetupV1(conf)
	}

	// Check that admins are clients
	principalBytes, err := proto.Marshal(&m.MSPRole{Role: m.MSPRole_CLIENT, MspIdentifier: msp.name})
	if err != nil {
		return errors.Wrapf(err, "failed creating MSPRole_CLIENT")
	}
	principal := &m.MSPPrincipal{
		PrincipalClassification: m.MSPPrincipal_ROLE,
		Principal:               principalBytes,
	}
	for i, admin := range msp.admins {
		err = admin.SatisfiesPrincipal(principal)
		if err != nil {
			return errors.WithMessagef(err, "admin %d is invalid", i)
		}
	}

	return nil
}

func (msp *bccspmsp) postSetupV142(conf *m.FabricMSPConfig) error {
	// Check for OU enforcement
	if !msp.ouEnforcement {
		// No enforcement required. Call post setup as per V1
		return msp.postSetupV1(conf)
	}

	// Check that admins are clients or admins
	for i, admin := range msp.admins {
		err1 := msp.hasOURole(admin, m.MSPRole_CLIENT)
		err2 := msp.hasOURole(admin, m.MSPRole_ADMIN)
		if err1 != nil && err2 != nil {
			return errors.Errorf("admin %d is invalid [%s,%s]", i, err1, err2)
		}
	}

	return nil
}
