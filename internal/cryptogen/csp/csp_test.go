/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package csp_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/internal/cryptogen/csp"
	"github.com/stretchr/testify/require"
)

func TestLoadPrivateKey(t *testing.T) {
	testDir := t.TempDir()
	priv, err := csp.GeneratePrivateKey(testDir)
	if err != nil {
		t.Fatalf("Failed to generate private key: %s", err)
	}
	pkFile := filepath.Join(testDir, "priv_sk")
	require.Equal(t, true, checkForFile(pkFile),
		"Expected to find private key file")
	loadedPriv, err := csp.LoadPrivateKey(testDir)
	require.NoError(t, err, "Failed to load private key")
	require.NotNil(t, loadedPriv, "Should have returned an *ecdsa.PrivateKey")
	require.Equal(t, priv, loadedPriv, "Expected private keys to match")
}

func TestLoadPrivateKey_BadPEM(t *testing.T) {
	testDir := t.TempDir()

	badPEMFile := filepath.Join(testDir, "badpem_sk")

	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %s", err)
	}

	pkcs8Encoded, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("Failed to PKCS8 encode RSA private key: %s", err)
	}
	pkcs8RSAPem := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Encoded})

	pkcs1Encoded := x509.MarshalPKCS1PrivateKey(rsaKey)
	if pkcs1Encoded == nil {
		t.Fatalf("Failed to PKCS1 encode RSA private key: %s", err)
	}
	pkcs1RSAPem := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs1Encoded})

	for _, test := range []struct {
		name   string
		data   []byte
		errMsg string
	}{
		{
			name:   "not pem encoded",
			data:   []byte("wrong_encoding"),
			errMsg: fmt.Sprintf("%s: bytes are not PEM encoded", badPEMFile),
		},
		{
			name:   "not EC key",
			data:   pkcs8RSAPem,
			errMsg: fmt.Sprintf("%s: pem bytes do not contain an EC private key", badPEMFile),
		},
		{
			name:   "not PKCS8 encoded",
			data:   pkcs1RSAPem,
			errMsg: fmt.Sprintf("%s: pem bytes are not PKCS8 encoded", badPEMFile),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ioutil.WriteFile(
				badPEMFile,
				test.data,
				0o755,
			)
			if err != nil {
				t.Fatalf("failed to write to wrong encoding file: %s", err)
			}
			_, err = csp.LoadPrivateKey(badPEMFile)
			require.Contains(t, err.Error(), test.errMsg)
			os.Remove(badPEMFile)
		})
	}
}

func TestGeneratePrivateKey(t *testing.T) {
	testDir := t.TempDir()

	expectedFile := filepath.Join(testDir, "priv_sk")
	priv, err := csp.GeneratePrivateKey(testDir)
	require.NoError(t, err, "Failed to generate private key")
	require.NotNil(t, priv, "Should have returned an *ecdsa.Key")
	require.Equal(t, true, checkForFile(expectedFile),
		"Expected to find private key file")

	_, err = csp.GeneratePrivateKey("notExist")
	require.Contains(t, err.Error(), "no such file or directory")
}

func TestECDSASigner(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate private key: %s", err)
	}

	signer := csp.ECDSASigner{
		PrivateKey: priv,
	}
	require.Equal(t, priv.Public(), signer.Public().(*ecdsa.PublicKey))
	digest := []byte{1}
	sig, err := signer.Sign(rand.Reader, digest, nil)
	if err != nil {
		t.Fatalf("Failed to create signature: %s", err)
	}

	// unmarshal signature
	ecdsaSig := &csp.ECDSASignature{}
	_, err = asn1.Unmarshal(sig, ecdsaSig)
	if err != nil {
		t.Fatalf("Failed to unmarshal signature: %s", err)
	}
	// s should not be greater than half order of curve
	halfOrder := new(big.Int).Div(priv.PublicKey.Curve.Params().N, big.NewInt(2))

	if ecdsaSig.S.Cmp(halfOrder) == 1 {
		t.Error("Expected signature with Low S")
	}

	// ensure signature is valid by using standard verify function
	ok := ecdsa.Verify(&priv.PublicKey, digest, ecdsaSig.R, ecdsaSig.S)
	require.True(t, ok, "Expected valid signature")
}

func checkForFile(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
