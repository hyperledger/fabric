/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsgen

import (
	"crypto"
	"crypto/x509"
)

// CertKeyPair denotes a TLS certificate and corresponding key,
// both PEM encoded
type CertKeyPair struct {
	// Cert is the certificate, PEM encoded
	Cert []byte
	// Key is the key corresponding to the certificate, PEM encoded
	Key []byte

	crypto.Signer
	TLSCert *x509.Certificate
}

// CA defines a certificate authority that can generate
// certificates signed by it
type CA interface {
	// CertBytes returns the certificate of the CA in PEM encoding
	CertBytes() []byte

	NewIntermediateCA() (CA, error)

	// newCertKeyPair returns a certificate and private key pair and nil,
	// or nil, error in case of failure
	// The certificate is signed by the CA and is used for TLS client authentication
	NewClientCertKeyPair() (*CertKeyPair, error)

	// NewServerCertKeyPair returns a CertKeyPair and nil,
	// with a given custom SAN.
	// The certificate is signed by the CA.
	// Returns nil, error in case of failure
	NewServerCertKeyPair(hosts ...string) (*CertKeyPair, error)

	// Signer returns a crypto.Signer that signs with the CA's private key.
	Signer() crypto.Signer
}

type ca struct {
	caCert *CertKeyPair
}

func NewCA() (CA, error) {
	c := &ca{}
	var err error
	c.caCert, err = newCertKeyPair(true, false, nil, nil)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *ca) NewIntermediateCA() (CA, error) {
	intermediateCA := &ca{}
	var err error
	intermediateCA.caCert, err = newCertKeyPair(true, false, c.caCert.Signer, c.caCert.TLSCert)
	if err != nil {
		return nil, err
	}
	return intermediateCA, nil
}

// CertBytes returns the certificate of the CA in PEM encoding
func (c *ca) CertBytes() []byte {
	return c.caCert.Cert
}

// newClientCertKeyPair returns a certificate and private key pair and nil,
// or nil, error in case of failure
// The certificate is signed by the CA and is used as a client TLS certificate
func (c *ca) NewClientCertKeyPair() (*CertKeyPair, error) {
	return newCertKeyPair(false, false, c.caCert.Signer, c.caCert.TLSCert)
}

// newServerCertKeyPair returns a certificate and private key pair and nil,
// or nil, error in case of failure
// The certificate is signed by the CA and is used as a server TLS certificate
func (c *ca) NewServerCertKeyPair(hosts ...string) (*CertKeyPair, error) {
	keypair, err := newCertKeyPair(false, true, c.caCert.Signer, c.caCert.TLSCert, hosts...)
	if err != nil {
		return nil, err
	}
	return keypair, nil
}

// Signer returns a crypto.Signer that signs with the CA's private key.
func (c *ca) Signer() crypto.Signer {
	return c.caCert.Signer
}
