/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"

	"github.com/pkg/errors"
)

type IssuedCRL struct {
	// IssuerCertificate is a PEM encoded certificate
	//of the issuer of the CRL
	IssuerCertificate []byte

	// RevocationList is a PEM encoded CRL
	RevocationList []byte
}

type revoked struct {
	sn        string
	rawIssuer string
	aki       string
}

// RevokedCertificates tells whether a given certificate is revoked or not
type RevokedCertificates struct {
	m map[revoked]struct{}
}

// NewRevokedCertificates builds an in-memory mapping of the given pairs of issuers and CRLs
// and aggregates it into RevokedCertificates
func NewRevokedCertificates(issuedCRLs []IssuedCRL) (*RevokedCertificates, error) {
	rc := &RevokedCertificates{
		m: make(map[revoked]struct{}),
	}

	for i, issuedCRL := range issuedCRLs {
		bl, _ := pem.Decode(issuedCRL.IssuerCertificate)
		if bl == nil {
			return nil, errors.Errorf("issuer %d is not a valid PEM encoded certificate", i)
		}

		issuerDER := bl.Bytes

		bl, _ = pem.Decode(issuedCRL.RevocationList)
		if bl == nil {
			return nil, errors.Errorf("revocation list %d is not PEM encoded", i)
		}

		crlDER := bl.Bytes

		issuer, err := x509.ParseCertificate(issuerDER)
		if bl == nil {
			return nil, errors.Wrapf(err, "issuer %d is not a valid certificate", i)
		}

		crl, err := x509.ParseCRL(crlDER)
		if err != nil {
			return nil, errors.Wrapf(err, "revocation list %d is not a valid CRL", i)
		}

		if err := issuer.CheckCRLSignature(crl); err != nil {
			return nil, errors.Wrapf(err, "revocation list %d is not signed by its corresponding issuer", i)
		}

		for _, revokedCert := range crl.TBSCertList.RevokedCertificates {
			sn := revokedCert.SerialNumber.String()
			revokedEntry := revoked{
				rawIssuer: string(issuer.RawSubject),
				sn:        sn,
			}

			rc.m[revokedEntry] = struct{}{}

			// From rfc5280# 5.2.1:
			// The identification can be based on either the key
			//   identifier (the subject key identifier in the CRL signer's
			//   certificate) or the issuer name and serial number.
			if hasAuthorityKeyIdExtension(crl.TBSCertList.Extensions) {
				revokedEntry := revoked{
					aki: string(issuer.SubjectKeyId),
					sn:  sn,
				}
				rc.m[revokedEntry] = struct{}{}
			}
		} // Iterate over all revoked certificates in CRL
	} // Iterate over all pairs of issuer and corresponding revocation list

	return rc, nil
}

// Revoked returns whether the given certificate has been revoked
func (rc *RevokedCertificates) Revoked(cert *x509.Certificate) bool {
	// From rfc5280# 5.2.1:
	// The identification can be based on either the key
	//   identifier (the subject key identifier in the CRL signer's
	//   certificate) or the issuer name and serial number.

	byAKI := revoked{
		sn:  cert.SerialNumber.String(),
		aki: string(cert.AuthorityKeyId),
	}

	byIssuer := revoked{
		sn:        cert.SerialNumber.String(),
		rawIssuer: string(cert.RawIssuer),
	}

	_, byAKIMatch := rc.m[byAKI]
	_, byIssuerMatch := rc.m[byIssuer]

	return byAKIMatch || byIssuerMatch
}

func hasAuthorityKeyIdExtension(extensions []pkix.Extension) bool {
	for _, e := range extensions {
		if e.Id.Equal([]int{2, 5, 29, 35}) {
			return true
		}
	}
	return false
}
