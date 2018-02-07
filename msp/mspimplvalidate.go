/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"bytes"
	"crypto/x509"

	"time"

	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"reflect"

	"github.com/pkg/errors"
)

func (msp *bccspmsp) validateIdentity(id *identity) error {
	validationChain, err := msp.getCertificationChainForBCCSPIdentity(id)
	if err != nil {
		return errors.WithMessage(err, "could not obtain certification chain")
	}

	err = msp.validateIdentityAgainstChain(id, validationChain)
	if err != nil {
		return errors.WithMessage(err, "could not validate identity against certification chain")
	}

	err = msp.internalValidateIdentityOusFunc(id)
	if err != nil {
		return errors.WithMessage(err, "could not validate identity's OUs")
	}

	return nil
}

func (msp *bccspmsp) validateCAIdentity(id *identity) error {
	if !id.cert.IsCA {
		return errors.New("Only CA identities can be validated")
	}

	validationChain, err := msp.getUniqueValidationChain(id.cert, msp.getValidityOptsForCert(id.cert))
	if err != nil {
		return errors.WithMessage(err, "could not obtain certification chain")
	}
	if len(validationChain) == 1 {
		// validationChain[0] is the root CA certificate
		return nil
	}

	return msp.validateIdentityAgainstChain(id, validationChain)
}

func (msp *bccspmsp) validateTLSCAIdentity(cert *x509.Certificate, opts *x509.VerifyOptions) error {
	if !cert.IsCA {
		return errors.New("Only CA identities can be validated")
	}

	validationChain, err := msp.getUniqueValidationChain(cert, *opts)
	if err != nil {
		return errors.WithMessage(err, "could not obtain certification chain")
	}
	if len(validationChain) == 1 {
		// validationChain[0] is the root CA certificate
		return nil
	}

	return msp.validateCertAgainstChain(cert, validationChain)
}

func (msp *bccspmsp) validateIdentityAgainstChain(id *identity, validationChain []*x509.Certificate) error {
	return msp.validateCertAgainstChain(id.cert, validationChain)
}

func (msp *bccspmsp) validateCertAgainstChain(cert *x509.Certificate, validationChain []*x509.Certificate) error {
	// here we know that the identity is valid; now we have to check whether it has been revoked

	// identify the SKI of the CA that signed this cert
	SKI, err := getSubjectKeyIdentifierFromCert(validationChain[1])
	if err != nil {
		return errors.WithMessage(err, "could not obtain Subject Key Identifier for signer cert")
	}

	// check whether one of the CRLs we have has this cert's
	// SKI as its AuthorityKeyIdentifier
	for _, crl := range msp.CRL {
		aki, err := getAuthorityKeyIdentifierFromCrl(crl)
		if err != nil {
			return errors.WithMessage(err, "could not obtain Authority Key Identifier for crl")
		}

		// check if the SKI of the cert that signed us matches the AKI of any of the CRLs
		if bytes.Equal(aki, SKI) {
			// we have a CRL, check whether the serial number is revoked
			for _, rc := range crl.TBSCertList.RevokedCertificates {
				if rc.SerialNumber.Cmp(cert.SerialNumber) == 0 {
					// We have found a CRL whose AKI matches the SKI of
					// the CA (root or intermediate) that signed the
					// certificate that is under validation. As a
					// precaution, we verify that said CA is also the
					// signer of this CRL.
					err = validationChain[1].CheckCRLSignature(crl)
					if err != nil {
						// the CA cert that signed the certificate
						// that is under validation did not sign the
						// candidate CRL - skip
						mspLogger.Warningf("Invalid signature over the identified CRL, error %+v", err)
						continue
					}

					// A CRL also includes a time of revocation so that
					// the CA can say "this cert is to be revoked starting
					// from this time"; however here we just assume that
					// revocation applies instantaneously from the time
					// the MSP config is committed and used so we will not
					// make use of that field
					return errors.New("The certificate has been revoked")
				}
			}
		}
	}

	return nil
}

func (msp *bccspmsp) validateIdentityOUsV1(id *identity) error {
	// Check that the identity's OUs are compatible with those recognized by this MSP,
	// meaning that the intersection is not empty.
	if len(msp.ouIdentifiers) > 0 {
		found := false

		for _, OU := range id.GetOrganizationalUnits() {
			certificationIDs, exists := msp.ouIdentifiers[OU.OrganizationalUnitIdentifier]

			if exists {
				for _, certificationID := range certificationIDs {
					if bytes.Equal(certificationID, OU.CertifiersIdentifier) {
						found = true
						break
					}
				}
			}
		}

		if !found {
			if len(id.GetOrganizationalUnits()) == 0 {
				return errors.New("the identity certificate does not contain an Organizational Unit (OU)")
			}
			return errors.Errorf("none of the identity's organizational units [%v] are in MSP %s", id.GetOrganizationalUnits(), msp.name)
		}
	}

	return nil
}

func (msp *bccspmsp) validateIdentityOUsV11(id *identity) error {
	// Run the same checks as per V1
	err := msp.validateIdentityOUsV1(id)
	if err != nil {
		return err
	}

	// Perform V1_1 additional checks:
	//
	// -- Check for OU enforcement
	if !msp.ouEnforcement {
		// No enforcement required
		return nil
	}

	// Make sure that the identity has only one of the special OUs
	// used to tell apart clients, peers and orderers.
	counter := 0
	for _, OU := range id.GetOrganizationalUnits() {
		// Is OU.OrganizationalUnitIdentifier one of the special OUs?
		var nodeOU *OUIdentifier
		switch OU.OrganizationalUnitIdentifier {
		case msp.clientOU.OrganizationalUnitIdentifier:
			nodeOU = msp.clientOU
		case msp.peerOU.OrganizationalUnitIdentifier:
			nodeOU = msp.peerOU
		default:
			continue
		}

		// Yes. Then, enforce the certifiers identifier is this is specified.
		// It is not specified, it means that any certification path is fine.
		if len(nodeOU.CertifiersIdentifier) != 0 && !bytes.Equal(nodeOU.CertifiersIdentifier, OU.CertifiersIdentifier) {
			return errors.Errorf("certifiersIdentifier does not match: [%v], MSP: [%s]", id.GetOrganizationalUnits(), msp.name)
		}
		counter++
		if counter > 1 {
			break
		}
	}
	if counter != 1 {
		return errors.Errorf("the identity must be a client, a peer or an orderer identity to be valid, not a combination of them. OUs: [%v], MSP: [%s]", id.GetOrganizationalUnits(), msp.name)
	}

	return nil
}

func (msp *bccspmsp) getValidityOptsForCert(cert *x509.Certificate) x509.VerifyOptions {
	// First copy the opts to override the CurrentTime field
	// in order to make the certificate passing the expiration test
	// independently from the real local current time.
	// This is a temporary workaround for FAB-3678

	var tempOpts x509.VerifyOptions
	tempOpts.Roots = msp.opts.Roots
	tempOpts.DNSName = msp.opts.DNSName
	tempOpts.Intermediates = msp.opts.Intermediates
	tempOpts.KeyUsages = msp.opts.KeyUsages
	tempOpts.CurrentTime = cert.NotBefore.Add(time.Second)

	return tempOpts
}

/*
   This is the definition of the ASN.1 marshalling of AuthorityKeyIdentifier
   from https://www.ietf.org/rfc/rfc5280.txt

   AuthorityKeyIdentifier ::= SEQUENCE {
      keyIdentifier             [0] KeyIdentifier           OPTIONAL,
      authorityCertIssuer       [1] GeneralNames            OPTIONAL,
      authorityCertSerialNumber [2] CertificateSerialNumber OPTIONAL  }

   KeyIdentifier ::= OCTET STRING

   CertificateSerialNumber  ::=  INTEGER

*/

type authorityKeyIdentifier struct {
	KeyIdentifier             []byte  `asn1:"optional,tag:0"`
	AuthorityCertIssuer       []byte  `asn1:"optional,tag:1"`
	AuthorityCertSerialNumber big.Int `asn1:"optional,tag:2"`
}

// getAuthorityKeyIdentifierFromCrl returns the Authority Key Identifier
// for the supplied CRL. The authority key identifier can be used to identify
// the public key corresponding to the private key which was used to sign the CRL.
func getAuthorityKeyIdentifierFromCrl(crl *pkix.CertificateList) ([]byte, error) {
	aki := authorityKeyIdentifier{}

	for _, ext := range crl.TBSCertList.Extensions {
		// Authority Key Identifier is identified by the following ASN.1 tag
		// authorityKeyIdentifier (2 5 29 35) (see https://tools.ietf.org/html/rfc3280.html)
		if reflect.DeepEqual(ext.Id, asn1.ObjectIdentifier{2, 5, 29, 35}) {
			_, err := asn1.Unmarshal(ext.Value, &aki)
			if err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal AKI")
			}

			return aki.KeyIdentifier, nil
		}
	}

	return nil, errors.New("authorityKeyIdentifier not found in certificate")
}

// getSubjectKeyIdentifierFromCert returns the Subject Key Identifier for the supplied certificate
// Subject Key Identifier is an identifier of the public key of this certificate
func getSubjectKeyIdentifierFromCert(cert *x509.Certificate) ([]byte, error) {
	var SKI []byte

	for _, ext := range cert.Extensions {
		// Subject Key Identifier is identified by the following ASN.1 tag
		// subjectKeyIdentifier (2 5 29 14) (see https://tools.ietf.org/html/rfc3280.html)
		if reflect.DeepEqual(ext.Id, asn1.ObjectIdentifier{2, 5, 29, 14}) {
			_, err := asn1.Unmarshal(ext.Value, &SKI)
			if err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal Subject Key Identifier")
			}

			return SKI, nil
		}
	}

	return nil, errors.New("subjectKeyIdentifier not found in certificate")
}

// isCACert does a few checks on the certificate,
// assuming it's a CA; it returns true if all looks good
// and false otherwise
func isCACert(cert *x509.Certificate) bool {
	_, err := getSubjectKeyIdentifierFromCert(cert)
	if err != nil {
		return false
	}

	if !cert.IsCA {
		return false
	}

	return true
}
