/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsgen

import (
	"crypto"
	"github.com/tjfoc/gmsm/sm2"
)

// CertKeyPair denotes a TLS certificate and corresponding key,
// both PEM encoded
type CertKeyPair struct {
	// Cert is the certificate, PEM encoded
	Cert []byte
	// Key is the key corresponding to the certificate, PEM encoded
	Key []byte

	crypto.Signer
	TLSCert *sm2.Certificate
}

// CA defines a certificate authority that can generate
// certificates signed by it
type CA interface {
	// CertBytes returns the certificate of the CA in PEM encoding
	CertBytes() []byte

	// newCertKeyPair returns a certificate and private key pair and nil,
	// or nil, error in case of failure
	// The certificate is signed by the CA and is used for TLS client authentication
	NewClientCertKeyPair() (*CertKeyPair, error)

	// NewServerCertKeyPair returns a CertKeyPair and nil,
	// with a given custom SAN.
	// The certificate is signed by the CA.
	// Returns nil, error in case of failure
	NewServerCertKeyPair(host string) (*CertKeyPair, error)
}

type ca struct {
	caCert *CertKeyPair
}

func NewCA() (CA, error) {
	c := &ca{}
	var err error
	c.caCert, err = newCertKeyPair(true, false, "", nil, nil)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CertBytes returns the certificate of the CA in PEM encoding
func (c *ca) CertBytes() []byte {
	return c.caCert.Cert
}

// newClientCertKeyPair returns a certificate and private key pair and nil,
// or nil, error in case of failure
// The certificate is signed by the CA and is used as a client TLS certificate
func (c *ca) NewClientCertKeyPair() (*CertKeyPair, error) {
	return newCertKeyPair(false, false, "", c.caCert.Signer, c.caCert.TLSCert)
}

// newServerCertKeyPair returns a certificate and private key pair and nil,
// or nil, error in case of failure
// The certificate is signed by the CA and is used as a server TLS certificate
func (c *ca) NewServerCertKeyPair(host string) (*CertKeyPair, error) {
	keypair, err := newCertKeyPair(false, true, host, c.caCert.Signer, c.caCert.TLSCert)
	if err != nil {
		return nil, err
	}
	return keypair, nil
}
