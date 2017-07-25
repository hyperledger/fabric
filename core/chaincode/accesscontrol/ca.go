/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package accesscontrol

// CA defines a certificate authority that can generate
// certificates signed by it
type CA interface {
	// CertBytes returns the certificate of the CA in PEM encoding
	CertBytes() []byte

	// newCertKeyPair returns a certificate and private key pair and nil,
	// or nil, error in case of failure
	// The certificate is signed by the CA
	newCertKeyPair() (*certKeyPair, error)
}

type ca struct {
	caCert *certKeyPair
}

func NewCA() (CA, error) {
	c := &ca{}
	var err error
	c.caCert, err = newCertKeyPair(true, nil, nil)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CertBytes returns the certificate of the CA in PEM encoding
func (c *ca) CertBytes() []byte {
	return c.caCert.certBytes
}

// newCertKeyPair returns a certificate and private key pair and nil,
// or nil, error in case of failure
// The certificate is signed by the CA
func (c *ca) newCertKeyPair() (*certKeyPair, error) {
	return newCertKeyPair(false, c.caCert.Signer, c.caCert.cert)
}
