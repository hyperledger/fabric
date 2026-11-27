/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/stretchr/testify/require"
)

func TestSanitizeX509CertECDSA(t *testing.T) {
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	sanitized, err := SanitizeX509Cert(kp.Cert)
	require.NoError(t, err)
	require.NotEmpty(t, sanitized)

	block, _ := pem.Decode(sanitized)
	require.NotNil(t, block)
	_, err = x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
}

func TestSanitizeX509CertNonPEM(t *testing.T) {
	_, err := SanitizeX509Cert([]byte("not a pem block"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to PEM decode identity bytes")
}

// TestSanitizeX509CertNonECDSAPublicKey ensures that a certificate whose
// subject public key is not ECDSA does not crash the sanitizer. Such a
// certificate can still have an ECDSA signature (for example an RSA subject
// key signed by an ECDSA CA), so the ECDSA signature unmarshals successfully
// and execution reaches the public key handling. Before the fix this panicked
// with "interface conversion: ... not *ecdsa.PublicKey".
func TestSanitizeX509CertNonECDSAPublicKey(t *testing.T) {
	// ECDSA CA.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	// RSA subject key, signed by the ECDSA CA. The resulting certificate has
	// an ECDSA signature but an RSA subject public key.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &rsaKey.PublicKey, caKey)
	require.NoError(t, err)
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})

	require.NotPanics(t, func() {
		_, err := SanitizeX509Cert(leafPEM)
		require.Error(t, err)
		require.Contains(t, err.Error(), "only ECDSA public keys are supported")
	})
}
