// Package localca implements a localca that is useful for testing the
// transport package. To use the localca, see the New and Load
// functions.
package localca

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

	"github.com/cloudflare/cfssl/config"
	"github.com/cloudflare/cfssl/csr"
	"github.com/cloudflare/cfssl/helpers"
	"github.com/cloudflare/cfssl/initca"
	"github.com/cloudflare/cfssl/signer"
	"github.com/cloudflare/cfssl/signer/local"
	"github.com/kisom/goutils/assert"
)

// CA is a local transport CertificateAuthority that is useful for
// tests.
type CA struct {
	s        *local.Signer
	disabled bool

	// Label and Profile are used to select the CFSSL signer
	// components if they should be anything but the default.
	Label   string `json:"label"`
	Profile string `json:"profile"`

	// The KeyFile and CertFile are required when using Load to
	// construct a CA.
	KeyFile  string `json:"private_key,omitempty"`
	CertFile string `json:"certificate,omitempty"`
}

// Toggle switches the CA between operable mode and inoperable
// mode. This is useful in testing to verify behaviours when a CA is
// unavailable.
func (lca *CA) Toggle() {
	lca.disabled = !lca.disabled
}

var errNotSetup = errors.New("transport: local CA has not been setup")

// CACertificate returns the certificate authority's certificate.
func (lca *CA) CACertificate() ([]byte, error) {
	if lca.s == nil {
		return nil, errNotSetup
	}

	cert, err := lca.s.Certificate(lca.Label, lca.Profile)
	if err != nil {
		return nil, err
	}

	p := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(p), nil
}

var errDisabled = errors.New("transport: local CA is deactivated")

// SignCSR submits a PKCS #10 certificate signing request to a CA for
// signing.
func (lca *CA) SignCSR(csrPEM []byte) ([]byte, error) {
	if lca == nil || lca.s == nil {
		return nil, errNotSetup
	}

	if lca.disabled {
		return nil, errDisabled
	}

	p, _ := pem.Decode(csrPEM)
	if p == nil || p.Type != "CERTIFICATE REQUEST" {
		return nil, errors.New("transport: invalid PEM-encoded certificate signing request")
	}

	csr, err := x509.ParseCertificateRequest(p.Bytes)
	if err != nil {
		return nil, err
	}

	hosts := make([]string, 0, len(csr.DNSNames)+len(csr.IPAddresses))
	copy(hosts, csr.DNSNames)

	for i := range csr.IPAddresses {
		hosts = append(hosts, csr.IPAddresses[i].String())
	}

	sreq := signer.SignRequest{
		Hosts:   hosts,
		Request: string(csrPEM),
		Profile: lca.Profile,
		Label:   lca.Label,
	}

	return lca.s.Sign(sreq)
}

// ExampleRequest can be used as a sample request, or the returned
// request can be modified.
func ExampleRequest() *csr.CertificateRequest {
	return &csr.CertificateRequest{
		Hosts: []string{"localhost"},
		KeyRequest: &csr.BasicKeyRequest{
			A: "ecdsa",
			S: 256,
		},
		CN: "Transport Failover Test Local CA",
		CA: &csr.CAConfig{
			PathLength: 1,
			Expiry:     "30m",
		},
	}
}

// ExampleSigningConfig returns a sample config.Signing with only a
// default profile.
func ExampleSigningConfig() *config.Signing {
	return &config.Signing{
		Default: &config.SigningProfile{
			Expiry: 15 * time.Minute,
			Usage: []string{
				"server auth", "client auth",
				"signing", "key encipherment",
			},
		},
	}
}

// New generates a new CA from a certificate request and signing profile.
func New(req *csr.CertificateRequest, profiles *config.Signing) (*CA, error) {
	certPEM, _, keyPEM, err := initca.New(req)
	if err != nil {
		return nil, err
	}

	// If initca returns successfully, the following (which are
	// all CFSSL internal functions) should not return an
	// error. If they do, we should abort --- something about
	// CFSSL has become inconsistent, and it can't be trusted.

	priv, err := helpers.ParsePrivateKeyPEM(keyPEM)
	assert.NoError(err, "CFSSL-generated private key can't be parsed")

	cert, err := helpers.ParseCertificatePEM(certPEM)
	assert.NoError(err, "CFSSL-generated certificate can't be parsed")

	s, err := local.NewSigner(priv, cert, helpers.SignerAlgo(priv), profiles)
	assert.NoError(err, "a signer could not be constructed")

	return NewFromSigner(s), nil
}

// NewFromSigner constructs a local CA from a CFSSL signer.
func NewFromSigner(s *local.Signer) *CA {
	return &CA{s: s}
}

// Load reads the key and certificate from the files specified in the
// CA.
func Load(lca *CA, profiles *config.Signing) (err error) {
	lca.s, err = local.NewSignerFromFile(lca.CertFile, lca.KeyFile, profiles)
	return err
}
