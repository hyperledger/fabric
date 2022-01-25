/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signer

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/hyperledger/fabric/common/util"
	"github.com/stretchr/testify/require"
)

func TestSigner(t *testing.T) {
	conf := Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert.pem"),
		KeyPath:      filepath.Join("testdata", "signer", "8150cb2d09628ccc89727611ebb736189f6482747eff9b8aaaa27e9a382d2e93_sk"),
	}

	signer, err := NewSigner(conf)
	require.NoError(t, err)

	msg := []byte("foo")
	sig, err := signer.Sign(msg)
	require.NoError(t, err)

	r, s, err := utils.UnmarshalECDSASignature(sig)
	require.NoError(t, err)
	ecdsa.Verify(&signer.key.PublicKey, util.ComputeSHA256(msg), r, s)
}

func TestSignerDifferentFormats(t *testing.T) {
	key := `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIOwCtOQIkowasuWoDQpXHgC547VHq+aBFaSyPOoV8mnGoAoGCCqGSM49
AwEHoUQDQgAEEsrroAkPez9reWvJukufUqyfouJjakrKuhNBYuclkldqsLZ/TO+w
ZsQXrlIqlmNalfYPX+NDDELqlpXQBeEqnA==
-----END EC PRIVATE KEY-----`

	pemBlock, _ := pem.Decode([]byte(key))
	require.NotNil(t, pemBlock)

	ecPK, err := x509.ParseECPrivateKey(pemBlock.Bytes)
	require.NoError(t, err)

	ec1, err := x509.MarshalECPrivateKey(ecPK)
	require.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(ecPK)
	require.NoError(t, err)

	for _, testCase := range []struct {
		description string
		keyBytes    []byte
	}{
		{
			description: "EC1",
			keyBytes:    pem.EncodeToMemory(&pem.Block{Type: "EC Private Key", Bytes: ec1}),
		},
		{
			description: "PKCS8",
			keyBytes:    pem.EncodeToMemory(&pem.Block{Type: "Private Key", Bytes: pkcs8}),
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			tmpFile, err := ioutil.TempFile("", "key")
			require.NoError(t, err)

			defer os.Remove(tmpFile.Name())

			err = ioutil.WriteFile(tmpFile.Name(), []byte(testCase.keyBytes), 0o600)
			require.NoError(t, err)

			signer, err := NewSigner(Config{
				MSPID:        "MSPID",
				IdentityPath: filepath.Join("testdata", "signer", "cert.pem"),
				KeyPath:      tmpFile.Name(),
			})

			require.NoError(t, err)
			require.NotNil(t, signer)
		})
	}
}

func TestSignerBadConfig(t *testing.T) {
	conf := Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "non_existent_cert"),
	}

	signer, err := NewSigner(conf)
	require.EqualError(t, err, "open testdata/signer/non_existent_cert: no such file or directory")
	require.Nil(t, signer)

	conf = Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert.pem"),
		KeyPath:      filepath.Join("testdata", "signer", "non_existent_cert"),
	}

	signer, err = NewSigner(conf)
	require.EqualError(t, err, "open testdata/signer/non_existent_cert: no such file or directory")
	require.Nil(t, signer)

	conf = Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert.pem"),
		KeyPath:      filepath.Join("testdata", "signer", "broken_private_key"),
	}

	signer, err = NewSigner(conf)
	require.EqualError(t, err, "failed to decode PEM block from testdata/signer/broken_private_key")
	require.Nil(t, signer)

	conf = Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert.pem"),
		KeyPath:      filepath.Join("testdata", "signer", "empty_private_key"),
	}

	signer, err = NewSigner(conf)
	require.EqualError(t, err, "failed to parse private key: x509: failed to parse EC private key: asn1: syntax error: sequence truncated")
	require.Nil(t, signer)

	conf = Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert_invalid_PEM.pem"),
		KeyPath:      filepath.Join("testdata", "signer", ""),
	}

	signer, err = NewSigner(conf)
	require.EqualError(t, err, "enrollment certificate isn't a valid PEM block")
	require.Nil(t, signer)

	conf = Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert_invalid_type.pem"),
		KeyPath:      filepath.Join("testdata", "signer", ""),
	}

	signer, err = NewSigner(conf)
	require.EqualError(t, err, "enrollment certificate should be a certificate, got a public key instead")
	require.Nil(t, signer)

	conf = Config{
		MSPID:        "SampleOrg",
		IdentityPath: filepath.Join("testdata", "signer", "cert_invalid_certificate.pem"),
		KeyPath:      filepath.Join("testdata", "signer", ""),
	}

	signer, err = NewSigner(conf)
	require.EqualError(t, err, "enrollment certificate is not a valid x509 certificate: x509: malformed certificate")
	require.Nil(t, signer)
}
