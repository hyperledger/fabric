/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/bccsp"
	m "github.com/hyperledger/fabric/protos/msp"
	errors "github.com/pkg/errors"
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
	msp.cryptoConfig = conf.CryptoConfig
	if msp.cryptoConfig == nil {
		// Move to defaults
		msp.cryptoConfig = &m.FabricCryptoConfig{
			SignatureHashFamily:            bccsp.SHA2,
			IdentityIdentifierHashFunction: bccsp.SHA256,
		}
		mspLogger.Debugf("CryptoConfig was nil. Move to defaults.")
	}
	if msp.cryptoConfig.SignatureHashFamily == "" {
		msp.cryptoConfig.SignatureHashFamily = bccsp.SHA2
		mspLogger.Debugf("CryptoConfig.SignatureHashFamily was nil. Move to defaults.")
	}
	if msp.cryptoConfig.IdentityIdentifierHashFunction == "" {
		msp.cryptoConfig.IdentityIdentifierHashFunction = bccsp.SHA256
		mspLogger.Debugf("CryptoConfig.IdentityIdentifierHashFunction was nil. Move to defaults.")
	}

	return nil
}

func (msp *bccspmsp) setupCAs(conf *m.FabricMSPConfig) error {
	// make and fill the set of CA certs - we expect them to be there
	if len(conf.RootCerts) == 0 {
		return errors.New("expected at least one CA certificate")
	}

	// pre-create the verify options with roots and intermediates.
	// This is needed to make certificate sanitation working.
	// Recall that sanitization is applied also to root CA and intermediate
	// CA certificates. After their sanitization is done, the opts
	// will be recreated using the sanitized certs.
	msp.opts = &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()}
	for _, v := range conf.RootCerts {
		cert, err := msp.getCertFromPem(v)
		if err != nil {
			return err
		}
		msp.opts.Roots.AddCert(cert)
	}
	for _, v := range conf.IntermediateCerts {
		cert, err := msp.getCertFromPem(v)
		if err != nil {
			return err
		}
		msp.opts.Intermediates.AddCert(cert)
	}

	// Load root and intermediate CA identities
	// Recall that when an identity is created, its certificate gets sanitized
	msp.rootCerts = make([]Identity, len(conf.RootCerts))
	for i, trustedCert := range conf.RootCerts {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.rootCerts[i] = id
	}

	// make and fill the set of intermediate certs (if present)
	msp.intermediateCerts = make([]Identity, len(conf.IntermediateCerts))
	for i, trustedCert := range conf.IntermediateCerts {
		id, _, err := msp.getIdentityFromConf(trustedCert)
		if err != nil {
			return err
		}

		msp.intermediateCerts[i] = id
	}

	// root CA and intermediate CA certificates are sanitized, they can be reimported
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
	// make and fill the set of admin certs (if present)
	msp.admins = make([]Identity, len(conf.Admins))
	for i, admCert := range conf.Admins {
		id, _, err := msp.getIdentityFromConf(admCert)
		if err != nil {
			return err
		}

		msp.admins[i] = id
	}

	return nil
}

func (msp *bccspmsp) setupCRLs(conf *m.FabricMSPConfig) error {
	// setup the CRL (if present)
	msp.CRL = make([]*pkix.CertificateList, len(conf.RevocationList))
	for i, crlbytes := range conf.RevocationList {
		crl, err := x509.ParseCRL(crlbytes)
		if err != nil {
			return errors.Wrap(err, "could not parse RevocationList")
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
			return errors.WithMessage(err, fmt.Sprintf("CA Certificate problem with Subject Key Identifier extension, (SN: %x)", id.(*identity).cert.SerialNumber))
		}

		if err := msp.validateCAIdentity(id.(*identity)); err != nil {
			return errors.WithMessage(err, fmt.Sprintf("CA Certificate is not valid, (SN: %s)", id.(*identity).cert.SerialNumber))
		}
	}

	// populate certificationTreeInternalNodesMap to mark the internal nodes of the
	// certification tree
	msp.certificationTreeInternalNodesMap = make(map[string]bool)
	for _, id := range append([]Identity{}, msp.intermediateCerts...) {
		chain, err := msp.getUniqueValidationChain(id.(*identity).cert, msp.getValidityOptsForCert(id.(*identity).cert))
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("failed getting validation chain, (SN: %s)", id.(*identity).cert.SerialNumber))
		}

		// Recall chain[0] is id.(*identity).id so it does not count as a parent
		for i := 1; i < len(chain); i++ {
			msp.certificationTreeInternalNodesMap[string(chain[i].Raw)] = true
		}
	}

	return nil
}

func (msp *bccspmsp) setupNodeOUs(config *m.FabricMSPConfig) error {
	if config.FabricNodeOus != nil {

		msp.ouEnforcement = config.FabricNodeOus.Enable

		// ClientOU
		msp.clientOU = &OUIdentifier{OrganizationalUnitIdentifier: config.FabricNodeOus.ClientOuIdentifier.OrganizationalUnitIdentifier}
		if len(config.FabricNodeOus.ClientOuIdentifier.Certificate) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.FabricNodeOus.ClientOuIdentifier.Certificate)
			if err != nil {
				return err
			}
			msp.clientOU.CertifiersIdentifier = certifiersIdentifier
		}

		// PeerOU
		msp.peerOU = &OUIdentifier{OrganizationalUnitIdentifier: config.FabricNodeOus.PeerOuIdentifier.OrganizationalUnitIdentifier}
		if len(config.FabricNodeOus.PeerOuIdentifier.Certificate) != 0 {
			certifiersIdentifier, err := msp.getCertifiersIdentifier(config.FabricNodeOus.PeerOuIdentifier.Certificate)
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

func (msp *bccspmsp) setupSigningIdentity(conf *m.FabricMSPConfig) error {
	if conf.SigningIdentity != nil {
		sid, err := msp.getSigningIdentityFromConf(conf.SigningIdentity)
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
	for _, ou := range conf.OrganizationalUnitIdentifiers {

		certifiersIdentifier, err := msp.getCertifiersIdentifier(ou.Certificate)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("failed getting certificate for [%v]", ou))
		}

		// Check for duplicates
		found := false
		for _, id := range msp.ouIdentifiers[ou.OrganizationalUnitIdentifier] {
			if bytes.Equal(id, certifiersIdentifier) {
				mspLogger.Warningf("Duplicate found in ou identifiers [%s, %v]", ou.OrganizationalUnitIdentifier, id)
				found = true
				break
			}
		}

		if !found {
			// No duplicates found, add it
			msp.ouIdentifiers[ou.OrganizationalUnitIdentifier] = append(
				msp.ouIdentifiers[ou.OrganizationalUnitIdentifier],
				certifiersIdentifier,
			)
		}
	}

	return nil
}

func (msp *bccspmsp) setupTLSCAs(conf *m.FabricMSPConfig) error {

	opts := &x509.VerifyOptions{Roots: x509.NewCertPool(), Intermediates: x509.NewCertPool()}

	// Load TLS root and intermediate CA identities
	msp.tlsRootCerts = make([][]byte, len(conf.TlsRootCerts))
	rootCerts := make([]*x509.Certificate, len(conf.TlsRootCerts))
	for i, trustedCert := range conf.TlsRootCerts {
		cert, err := msp.getCertFromPem(trustedCert)
		if err != nil {
			return err
		}

		rootCerts[i] = cert
		msp.tlsRootCerts[i] = trustedCert
		opts.Roots.AddCert(cert)
	}

	// make and fill the set of intermediate certs (if present)
	msp.tlsIntermediateCerts = make([][]byte, len(conf.TlsIntermediateCerts))
	intermediateCerts := make([]*x509.Certificate, len(conf.TlsIntermediateCerts))
	for i, trustedCert := range conf.TlsIntermediateCerts {
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
			return errors.WithMessage(err, fmt.Sprintf("CA Certificate problem with Subject Key Identifier extension, (SN: %x)", cert.SerialNumber))
		}

		if err := msp.validateTLSCAIdentity(cert, opts); err != nil {
			return errors.WithMessage(err, fmt.Sprintf("CA Certificate is not valid, (SN: %s)", cert.SerialNumber))
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

func (msp *bccspmsp) postSetupV1(conf *m.FabricMSPConfig) error {
	// make sure that admins are valid members as well
	// this way, when we validate an admin MSP principal
	// we can simply check for exact match of certs
	for i, admin := range msp.admins {
		err := admin.Validate()
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("admin %d is invalid", i))
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
		Principal:               principalBytes}
	for i, admin := range msp.admins {
		err = admin.SatisfiesPrincipal(principal)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("admin %d is invalid", i))
		}
	}

	return nil
}
