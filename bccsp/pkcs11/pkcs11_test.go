// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"hash"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/signer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/miekg/pkcs11"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

var (
	currentKS         bccsp.KeyStore
	currentBCCSP      *impl
	currentTestConfig testConfig
)

type testConfig struct {
	securityLevel int
	hashFamily    string
	softVerify    bool
	immutable     bool
}

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	tmpDir, err := ioutil.TempDir("", "pkcs11_ks")
	if err != nil {
		fmt.Printf("Failed to create keystore directory [%s]\n", err)
		return -1
	}
	defer os.RemoveAll(tmpDir)

	keyStore, err := sw.NewFileBasedKeyStore(nil, tmpDir, false)
	if err != nil {
		fmt.Printf("Failed initiliazing KeyStore [%s]\n", err)
		return -1
	}
	currentKS = keyStore

	lib, pin, label := FindPKCS11Lib()
	tests := []testConfig{
		{securityLevel: 256, hashFamily: "SHA2", softVerify: true, immutable: false},
		{securityLevel: 256, hashFamily: "SHA3", softVerify: false, immutable: false},
		{securityLevel: 384, hashFamily: "SHA2", softVerify: false, immutable: false},
		{securityLevel: 384, hashFamily: "SHA3", softVerify: false, immutable: false},
		{securityLevel: 384, hashFamily: "SHA3", softVerify: true, immutable: false},
	}

	if strings.Contains(lib, "softhsm") {
		tests = append(tests, testConfig{securityLevel: 256, hashFamily: "SHA2", softVerify: true, immutable: true})
	}

	opts := PKCS11Opts{
		Library: lib,
		Label:   label,
		Pin:     pin,
	}

	for _, config := range tests {
		currentTestConfig = config

		opts.Hash = config.hashFamily
		opts.Security = config.securityLevel
		opts.SoftwareVerify = config.softVerify
		opts.Immutable = config.immutable

		provider, err := New(opts, keyStore)
		if err != nil {
			fmt.Printf("Failed initiliazing BCCSP at [%+v] \n%s\n", opts, err)
			return -1
		}
		currentBCCSP = provider.(*impl)

		ret := m.Run()
		if ret != 0 {
			fmt.Printf("Failed testing at [%+v]\n", opts)
			return -1
		}
	}
	return 0
}

func TestNew(t *testing.T) {
	opts := PKCS11Opts{
		Hash:           "SHA2",
		Security:       256,
		SoftwareVerify: false,
		Library:        "lib",
		Label:          "ForFabric",
		Pin:            "98765432",
	}

	// Setup PKCS11 library and provide initial set of values
	lib, _, _ := FindPKCS11Lib()
	opts.Library = lib

	// Test for nil keystore
	_, err := New(opts, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid bccsp.KeyStore instance. It must be different from nil")

	// Test for invalid PKCS11 loadLib
	opts.Library = ""
	_, err = New(opts, currentKS)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pkcs11: library path not provided")
}

func TestFindPKCS11LibEnvVars(t *testing.T) {
	const (
		dummy_PKCS11_LIB   = "/usr/lib/pkcs11"
		dummy_PKCS11_PIN   = "23456789"
		dummy_PKCS11_LABEL = "testing"
	)

	// Set environment variables used for test and preserve
	// original values for restoration after test completion
	orig_PKCS11_LIB := os.Getenv("PKCS11_LIB")
	orig_PKCS11_PIN := os.Getenv("PKCS11_PIN")
	orig_PKCS11_LABEL := os.Getenv("PKCS11_LABEL")

	t.Run("ExplicitEnvironment", func(t *testing.T) {
		os.Setenv("PKCS11_LIB", dummy_PKCS11_LIB)
		os.Setenv("PKCS11_PIN", dummy_PKCS11_PIN)
		os.Setenv("PKCS11_LABEL", dummy_PKCS11_LABEL)

		lib, pin, label := FindPKCS11Lib()
		require.EqualValues(t, dummy_PKCS11_LIB, lib, "FindPKCS11Lib did not return expected library")
		require.EqualValues(t, dummy_PKCS11_PIN, pin, "FindPKCS11Lib did not return expected pin")
		require.EqualValues(t, dummy_PKCS11_LABEL, label, "FindPKCS11Lib did not return expected label")
	})

	t.Run("MissingEnvironment", func(t *testing.T) {
		os.Unsetenv("PKCS11_LIB")
		os.Unsetenv("PKCS11_PIN")
		os.Unsetenv("PKCS11_LABEL")

		_, pin, label := FindPKCS11Lib()
		require.EqualValues(t, "98765432", pin, "FindPKCS11Lib did not return expected pin")
		require.EqualValues(t, "ForFabric", label, "FindPKCS11Lib did not return expected label")
	})

	os.Setenv("PKCS11_LIB", orig_PKCS11_LIB)
	os.Setenv("PKCS11_PIN", orig_PKCS11_PIN)
	os.Setenv("PKCS11_LABEL", orig_PKCS11_LABEL)
}

func TestInvalidNewParameter(t *testing.T) {
	lib, pin, label := FindPKCS11Lib()
	opts := PKCS11Opts{
		Library:        lib,
		Label:          label,
		Pin:            pin,
		SoftwareVerify: true,
	}

	opts.Hash = "SHA2"
	opts.Security = 0
	_, err := New(opts, currentKS)
	require.EqualError(t, err, "Failed initializing configuration: Security level not supported [0]")

	opts.Hash = "SHA8"
	opts.Security = 256
	_, err = New(opts, currentKS)
	require.EqualError(t, err, "Failed initializing configuration: Hash Family not supported [SHA8]")

	opts.Hash = "SHA2"
	opts.Security = 256
	_, err = New(opts, nil)
	require.EqualError(t, err, "Failed initializing fallback SW BCCSP: Invalid bccsp.KeyStore instance. It must be different from nil.")

	opts.Hash = "SHA3"
	opts.Security = 0
	_, err = New(opts, nil)
	require.EqualError(t, err, "Failed initializing configuration: Security level not supported [0]")
}

func TestInvalidSKI(t *testing.T) {
	_, err := currentBCCSP.GetKey(nil)
	require.EqualError(t, err, "Failed getting key for SKI [[]]: invalid SKI. Cannot be of zero length")

	_, err = currentBCCSP.GetKey([]byte{0, 1, 2, 3, 4, 5, 6})
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Failed getting key for SKI [[0 1 2 3 4 5 6]]: "))
}

func TestKeyGenECDSAOpts(t *testing.T) {
	tests := map[string]struct {
		opts  bccsp.KeyGenOpts
		curve elliptic.Curve
	}{
		"P256": {&bccsp.ECDSAP256KeyGenOpts{Temporary: false}, elliptic.P256()},
		"P384": {&bccsp.ECDSAP384KeyGenOpts{Temporary: false}, elliptic.P384()},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			k, err := currentBCCSP.KeyGen(tt.opts)
			require.NoError(t, err)
			require.True(t, k.Private(), "key should be private")
			require.False(t, k.Symmetric(), "key should be asymmetric")

			ecdsaKey := k.(*ecdsaPrivateKey).pub
			require.Equal(t, tt.curve, ecdsaKey.pub.Curve, "wrong curve")
		})
	}
}

func TestKeyGenAESOpts(t *testing.T) {
	tests := map[string]struct {
		opts bccsp.KeyGenOpts
	}{
		"AES128": {&bccsp.AES128KeyGenOpts{Temporary: false}},
		"AES192": {&bccsp.AES192KeyGenOpts{Temporary: false}},
		"AES256": {&bccsp.AES256KeyGenOpts{Temporary: false}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			k, err := currentBCCSP.KeyGen(tt.opts)
			require.NoError(t, err)
			require.True(t, k.Private(), "key should be private")
			require.True(t, k.Symmetric(), "key should be symmetric")
		})
	}
}

func TestHashOpts(t *testing.T) {
	tests := map[string]struct {
		opts bccsp.HashOpts
		h    hash.Hash
	}{
		"SHA256":   {&bccsp.SHA256Opts{}, sha256.New()},
		"SHA384":   {&bccsp.SHA384Opts{}, sha512.New384()},
		"SHA3_256": {&bccsp.SHA3_256Opts{}, sha3.New256()},
		"SHA3_384": {&bccsp.SHA3_384Opts{}, sha3.New384()},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			msg := []byte("abcd")

			digest, err := currentBCCSP.Hash(msg, tt.opts)
			require.NoError(t, err)

			tt.h.Write(msg)
			require.Equalf(t, tt.h.Sum(nil), digest, "expected %x got %x", tt.h.Sum(nil), digest)
		})
	}
}

func TestECDSAKeyGenEphemeral(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	require.NoError(t, err)
	require.True(t, k.Private(), "key should be private")
	require.False(t, k.Symmetric(), "key should be asymmetric")

	raw, err := k.Bytes()
	require.EqualError(t, err, "Not supported.")
	require.Empty(t, raw, "result should be empty")

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.NotNil(t, pk)
}

func TestECDSAKeyGenNonEphemeral(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)
	require.True(t, k.Private(), "key should be private")
	require.False(t, k.Symmetric(), "key should be asymmetric")
	require.NotEmpty(t, k.SKI(), "SKI must not be empty")
}

func TestECDSAGetKeyBySKI(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	k2, err := currentBCCSP.GetKey(k.SKI())
	require.NoError(t, err)

	require.True(t, k2.Private(), "key should be private")
	require.False(t, k2.Symmetric(), "key should be asymmetric")
	require.Equalf(t, k.SKI(), k2.SKI(), "expected %x got %x", k.SKI(), k2.SKI())
}

func TestECDSAPublicKeyFromPrivateKey(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.False(t, pk.Private(), "key should be public")
	require.False(t, pk.Symmetric(), "key should be asymmetric")
}

func TestECDSAPublicKeyBytes(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	pk, err := k.PublicKey()
	require.NoError(t, err)

	raw, err := pk.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, raw, "marshaled ECDSA public key must not be empty")
}

func TestECDSAPublicKeySKI(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.NotEmpty(t, pk.SKI(), "SKI must not be empty")
}

func TestECDSASign(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	digest, err := currentBCCSP.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	signature, err := currentBCCSP.Sign(k, digest, nil)
	require.NoError(t, err)
	require.NotEmpty(t, signature, "signature must not be empty")

	t.Run("MissingKey", func(t *testing.T) {
		_, err := currentBCCSP.Sign(nil, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid Key. It must not be nil")
	})

	t.Run("MissingDigest", func(t *testing.T) {
		_, err = currentBCCSP.Sign(k, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid digest. Cannot be empty")
	})
}

func TestECDSAVerify(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	digest, err := currentBCCSP.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	signature, err := currentBCCSP.Sign(k, digest, nil)
	require.NoError(t, err)

	valid, err := currentBCCSP.Verify(k, signature, digest, nil)
	require.NoError(t, err)
	require.True(t, valid, "signature should be valid from private key")

	pk, err := k.PublicKey()
	require.NoError(t, err)

	valid, err = currentBCCSP.Verify(pk, signature, digest, nil)
	require.NoError(t, err)
	require.True(t, valid, "signature should be valid from public key")

	t.Run("MissingKey", func(t *testing.T) {
		_, err := currentBCCSP.Verify(nil, signature, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid Key. It must not be nil")
	})

	t.Run("MissingSignature", func(t *testing.T) {
		_, err := currentBCCSP.Verify(pk, nil, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid signature. Cannot be empty")
	})

	t.Run("MissingDigest", func(t *testing.T) {
		_, err = currentBCCSP.Verify(pk, signature, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid digest. Cannot be empty")
	})

	t.Run("ImportedKey", func(t *testing.T) {
		pkRaw, err := pk.Bytes()
		require.NoError(t, err)

		_, err = currentBCCSP.KeyImport(pkRaw, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: false})
		require.NoError(t, err)

		pk2, err := currentBCCSP.GetKey(pk.SKI())
		require.NoError(t, err)

		valid, err = currentBCCSP.Verify(pk2, signature, digest, nil)
		require.NoError(t, err)
		require.True(t, valid, "signature should be valid from public key")
	})
}

func TestECDSAKeyImportFromExportedKey(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	// Export the public key
	pk, err := k.PublicKey()
	require.NoError(t, err)
	pkRaw, err := pk.Bytes()
	require.NoError(t, err)

	// Import the exported public key
	pk2, err := currentBCCSP.KeyImport(pkRaw, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: false})
	require.NoError(t, err)

	// Sign and verify with the imported public key
	digest, err := currentBCCSP.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	signature, err := currentBCCSP.Sign(k, digest, nil)
	require.NoError(t, err)

	valid, err := currentBCCSP.Verify(pk2, signature, digest, nil)
	require.NoError(t, err)
	require.True(t, valid, "signature should be valid from public key")
}

func TestECDSAKeyImportFromECDSAPublicKey(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	// Export the public key
	pk, err := k.PublicKey()
	require.NoError(t, err)

	pkRaw, err := pk.Bytes()
	require.NoError(t, err)

	pub, err := x509.ParsePKIXPublicKey(pkRaw)
	require.NoError(t, err)

	// Import the ecdsa.PublicKey
	pk2, err := currentBCCSP.KeyImport(pub, &bccsp.ECDSAGoPublicKeyImportOpts{Temporary: false})
	require.NoError(t, err)

	// Sign and verify with the imported public key
	digest, err := currentBCCSP.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	signature, err := currentBCCSP.Sign(k, digest, nil)
	require.NoError(t, err)

	valid, err := currentBCCSP.Verify(pk2, signature, digest, nil)
	require.NoError(t, err)
	require.True(t, valid, "signature should be valid from public key")
}

func TestKeyImportFromX509ECDSAPublicKey(t *testing.T) {
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "test.example.com",
			Organization: []string{"Σ Acme Co"},
			Country:      []string{"US"},
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: []int{2, 5, 4, 42}, Value: "Gopher"},
				{Type: []int{2, 5, 4, 6}, Value: "NL"}, // This should override the Country, above.
			},
		},
		NotBefore:          time.Now().Add(-1 * time.Hour),
		NotAfter:           time.Now().Add(1 * time.Hour),
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		SubjectKeyId:       []byte{1, 2, 3, 4},
		KeyUsage:           x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		UnknownExtKeyUsage: []asn1.ObjectIdentifier{
			[]int{1, 2, 3},
			[]int{2, 59, 1},
		},
		BasicConstraintsValid: true,
		IsCA:                  true,
		OCSPServer:            []string{"http://ocurrentBCCSP.example.com"},
		IssuingCertificateURL: []string{"http://crt.example.com/ca1.crt"},
		DNSNames:              []string{"test.example.com"},
		EmailAddresses:        []string{"gopher@golang.org"},
		IPAddresses: []net.IP{
			net.IPv4(127, 0, 0, 1).To4(),
			net.ParseIP("2001:4860:0:2001::68"),
		},
		PolicyIdentifiers: []asn1.ObjectIdentifier{
			[]int{1, 2, 3},
		},
		PermittedDNSDomains: []string{
			".example.com",
			"example.com",
		},
		CRLDistributionPoints: []string{
			"http://crl1.example.com/ca1.crl",
			"http://crl2.example.com/ca1.crl",
		},
		ExtraExtensions: []pkix.Extension{
			{Id: []int{1, 2, 3, 4}, Value: []byte("extra extension")},
		},
	}

	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	cryptoSigner, err := signer.New(currentBCCSP, k)
	require.NoError(t, err)

	// Export the public key
	pk, err := k.PublicKey()
	require.NoError(t, err)
	pkRaw, err := pk.Bytes()
	require.NoError(t, err)
	pub, err := x509.ParsePKIXPublicKey(pkRaw)
	require.NoError(t, err)

	// Generate a self-signed certificate
	certRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, cryptoSigner)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certRaw)
	require.NoError(t, err)

	// Import the certificate's public key
	pk2, err := currentBCCSP.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: false})
	require.NoError(t, err)

	// Sign and verify with the imported public key
	digest, err := currentBCCSP.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)
	signature, err := currentBCCSP.Sign(k, digest, nil)
	require.NoError(t, err)

	valid, err := currentBCCSP.Verify(pk2, signature, digest, nil)
	require.NoError(t, err)
	require.True(t, valid, "signature should be valid from public key")
}

func TestECDSASignatureEncoding(t *testing.T) {
	tests := []struct {
		v []byte
	}{
		{[]byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x02, 0xff, 0xf1}},
		{[]byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x02, 0x00, 0x01}},
		{[]byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x81, 0x01, 0x01}},
		{[]byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x81, 0x01, 0x8F}},
		{[]byte{0x30, 0x0A, 0x02, 0x01, 0x8F, 0x02, 0x05, 0x00, 0x00, 0x00, 0x00, 0x8F}},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			_, err := asn1.Unmarshal(tt.v, &utils.ECDSASignature{})
			require.Errorf(t, err, "unmarshal for %x should fail", tt.v)
			require.True(t, strings.HasPrefix(err.Error(), "asn1: structure error: "))
		})
	}
}

func TestECDSALowS(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	digest, err := currentBCCSP.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	// Ensure that signature with low-S are generated
	t.Run("GeneratesLowS", func(t *testing.T) {
		signature, err := currentBCCSP.Sign(k, digest, nil)
		require.NoError(t, err)

		_, S, err := utils.UnmarshalECDSASignature(signature)
		require.NoError(t, err)

		if S.Cmp(utils.GetCurveHalfOrdersAt(k.(*ecdsaPrivateKey).pub.pub.Curve)) >= 0 {
			t.Fatal("Invalid signature. It must have low-S")
		}

		valid, err := currentBCCSP.Verify(k, signature, digest, nil)
		require.NoError(t, err)
		require.True(t, valid, "signature should be valid")
	})

	// Ensure that signature with high-S are rejected.
	t.Run("RejectsHighS", func(t *testing.T) {
		for {
			R, S, err := currentBCCSP.signP11ECDSA(k.SKI(), digest)
			require.NoError(t, err)
			if S.Cmp(utils.GetCurveHalfOrdersAt(k.(*ecdsaPrivateKey).pub.pub.Curve)) > 0 {
				sig, err := utils.MarshalECDSASignature(R, S)
				require.NoError(t, err)

				valid, err := currentBCCSP.Verify(k, sig, digest, nil)
				require.Error(t, err, "verification must fail for a signature with high-S")
				require.False(t, valid, "signature must not be valid with high-S")
				return
			}
		}
	})
}

func TestAESKeyGen(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	require.NoError(t, err)
	require.True(t, k.Private(), "key should be private")
	require.True(t, k.Symmetric(), "key should be symmetric")

	pk, err := k.PublicKey()
	require.EqualError(t, err, "Cannot call this method on a symmetric key.")
	require.Nil(t, pk)
}

func TestAESEncryptDecrypt(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	msg := []byte("Hello World")

	ct, err := currentBCCSP.Encrypt(k, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	require.NoError(t, err)
	require.NotEmpty(t, ct, "ciphertext must not be empty")

	pt, err := currentBCCSP.Decrypt(k, ct, bccsp.AESCBCPKCS7ModeOpts{})
	require.NoError(t, err)
	require.Equalf(t, msg, pt, "expected %x but got %x", msg, pt)
}

func TestHMACTruncated256KeyDerivOverAES256Key(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	hmacedKey, err := currentBCCSP.KeyDeriv(k, &bccsp.HMACTruncated256AESDeriveKeyOpts{Temporary: false, Arg: []byte{1}})
	require.NoError(t, err)
	require.True(t, hmacedKey.Private(), "HMACed key should be private")
	require.True(t, hmacedKey.Symmetric(), "HMACed key should be symmetric")

	raw, err := hmacedKey.Bytes()
	require.EqualError(t, err, "Not supported.")
	require.Empty(t, raw)

	msg := []byte("Hello World")

	ct, err := currentBCCSP.Encrypt(hmacedKey, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	require.NoError(t, err)
	pt, err := currentBCCSP.Decrypt(hmacedKey, ct, bccsp.AESCBCPKCS7ModeOpts{})
	require.NoError(t, err)

	require.Equalf(t, msg, pt, "expected %x but got %x", msg, pt)
}

func TestHMACKeyDerivOverAES256Key(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	hmacedKey, err := currentBCCSP.KeyDeriv(k, &bccsp.HMACDeriveKeyOpts{Temporary: false, Arg: []byte{1}})
	require.NoError(t, err)
	require.True(t, hmacedKey.Private(), "HMACed key should be private")
	require.True(t, hmacedKey.Symmetric(), "HMACed key should be symmetric")

	raw, err := hmacedKey.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestAES256KeyImport(t *testing.T) {
	raw, err := sw.GetRandomBytes(32)
	require.NoError(t, err)

	k, err := currentBCCSP.KeyImport(raw, &bccsp.AES256ImportKeyOpts{Temporary: false})
	require.NoError(t, err)
	require.True(t, k.Private(), "imported key should be private")
	require.True(t, k.Symmetric(), "imported key should be symmetric")

	raw, err = k.Bytes()
	require.EqualError(t, err, "Not supported.")
	require.Empty(t, raw)

	msg := []byte("Hello World")

	ct, err := currentBCCSP.Encrypt(k, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	require.NoError(t, err)
	pt, err := currentBCCSP.Decrypt(k, ct, bccsp.AESCBCPKCS7ModeOpts{})
	require.NoError(t, err)

	require.Equalf(t, msg, pt, "expected %x but got %x", msg, pt)
}

func TestAES256KeyImportBadPaths(t *testing.T) {
	t.Run("MissingKey", func(t *testing.T) {
		_, err := currentBCCSP.KeyImport(nil, &bccsp.AES256ImportKeyOpts{Temporary: false})
		require.EqualError(t, err, "Invalid raw. Cannot be nil")
	})

	t.Run("InvalidKey", func(t *testing.T) {
		_, err := currentBCCSP.KeyImport([]byte{1}, &bccsp.AES256ImportKeyOpts{Temporary: false})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid Key Length")
	})
}

func TestAES256KeyGenSKI(t *testing.T) {
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	k2, err := currentBCCSP.GetKey(k.SKI())
	require.NoError(t, err)
	require.True(t, k2.Private(), "key should be private")
	require.True(t, k2.Symmetric(), "key should be symmetric")

	require.Equalf(t, k.SKI(), k2.SKI(), "expected %x but got %x", k.SKI(), k2.SKI())
}

func TestSHA(t *testing.T) {
	for i := 0; i < 100; i++ {
		b, err := sw.GetRandomBytes(i)
		require.NoError(t, err)

		h1, err := currentBCCSP.Hash(b, &bccsp.SHAOpts{})
		require.NoError(t, err)

		var h hash.Hash
		switch currentTestConfig.hashFamily {
		case "SHA2":
			switch currentTestConfig.securityLevel {
			case 256:
				h = sha256.New()
			case 384:
				h = sha512.New384()
			default:
				t.Fatalf("Invalid security level [%d]", currentTestConfig.securityLevel)
			}
		case "SHA3":
			switch currentTestConfig.securityLevel {
			case 256:
				h = sha3.New256()
			case 384:
				h = sha3.New384()
			default:
				t.Fatalf("Invalid security level [%d]", currentTestConfig.securityLevel)
			}
		default:
			t.Fatalf("Invalid hash family [%s]", currentTestConfig.hashFamily)
		}

		h.Write(b)
		h2 := h.Sum(nil)
		require.Equalf(t, h2, h1, "hashing %x, expected %x but got %x", b, h2, h1)
	}
}

func TestKeyGenFailures(t *testing.T) {
	_, err := currentBCCSP.KeyGen(bccsp.KeyGenOpts(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Opts parameter. It must not be nil")
}

func TestInitialize(t *testing.T) {
	// Setup PKCS11 library and provide initial set of values
	lib, pin, label := FindPKCS11Lib()

	t.Run("MissingLibrary", func(t *testing.T) {
		_, err := (&impl{}).initialize(PKCS11Opts{Library: "", Pin: pin, Label: label})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pkcs11: library path not provided")
	})

	t.Run("BadLibraryPath", func(t *testing.T) {
		_, err := (&impl{}).initialize(PKCS11Opts{Library: "badLib", Pin: pin, Label: label})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pkcs11: instantiation failed for badLib")
	})

	t.Run("BadLabel", func(t *testing.T) {
		_, err := (&impl{}).initialize(PKCS11Opts{Library: lib, Pin: pin, Label: "badLabel"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find token with label")
	})

	t.Run("MissingPin", func(t *testing.T) {
		_, err := (&impl{}).initialize(PKCS11Opts{Library: lib, Pin: "", Label: label})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Login failed: pkcs11")
	})
}

func TestNamedCurveFromOID(t *testing.T) {
	tests := map[string]struct {
		oid   asn1.ObjectIdentifier
		curve elliptic.Curve
	}{
		"P224":    {oidNamedCurveP224, elliptic.P224()},
		"P256":    {oidNamedCurveP256, elliptic.P256()},
		"P384":    {oidNamedCurveP384, elliptic.P384()},
		"P521":    {oidNamedCurveP521, elliptic.P521()},
		"unknown": {asn1.ObjectIdentifier{4, 9, 15, 1}, nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.curve, namedCurveFromOID(tt.oid))
		})
	}
}

func TestPKCS11GetSession(t *testing.T) {
	var sessions []pkcs11.SessionHandle
	for i := 0; i < 3*sessionCacheSize; i++ {
		session, err := currentBCCSP.getSession()
		require.NoError(t, err)
		sessions = append(sessions, session)
	}

	// Return all sessions, should leave sessionCacheSize cached
	for _, session := range sessions {
		currentBCCSP.returnSession(session)
	}

	// Lets break OpenSession, so non-cached session cannot be opened
	oldSlot := currentBCCSP.slot
	currentBCCSP.slot = ^uint(0)

	// Should be able to get sessionCacheSize cached sessions
	sessions = nil
	for i := 0; i < sessionCacheSize; i++ {
		session, err := currentBCCSP.getSession()
		require.NoError(t, err)
		sessions = append(sessions, session)
	}

	_, err := currentBCCSP.getSession()
	require.EqualError(t, err, "OpenSession failed: pkcs11: 0x3: CKR_SLOT_ID_INVALID")

	// Load cache with bad sessions
	for i := 0; i < sessionCacheSize; i++ {
		currentBCCSP.returnSession(pkcs11.SessionHandle(^uint(0)))
	}

	// Fix OpenSession so non-cached sessions can be opened
	currentBCCSP.slot = oldSlot

	// Request a session, return, and re-acquire. The pool should be emptied
	// before creating a new session so when returned, it should be the only
	// session in the cache.
	sess, err := currentBCCSP.getSession()
	require.NoError(t, err)
	currentBCCSP.returnSession(sess)
	sess2, err := currentBCCSP.getSession()
	require.NoError(t, err)
	require.Equal(t, sess, sess2, "expected to get back the same session")

	// Cleanup
	for _, session := range sessions {
		currentBCCSP.returnSession(session)
	}
}

func TestSessionHandleCaching(t *testing.T) {
	defer func(s int) { sessionCacheSize = s }(sessionCacheSize)

	opts := PKCS11Opts{Hash: "SHA2", Security: 256, SoftwareVerify: false}
	opts.Library, opts.Pin, opts.Label = FindPKCS11Lib()

	verifyHandleCache := func(t *testing.T, pi *impl, sess pkcs11.SessionHandle, k bccsp.Key) {
		pubHandle, err := pi.findKeyPairFromSKI(sess, k.SKI(), publicKeyType)
		require.NoError(t, err)
		h, ok := pi.cachedHandle(publicKeyType, k.SKI())
		require.True(t, ok)
		require.Equal(t, h, pubHandle)

		privHandle, err := pi.findKeyPairFromSKI(sess, k.SKI(), privateKeyType)
		require.NoError(t, err)
		h, ok = pi.cachedHandle(privateKeyType, k.SKI())
		require.True(t, ok)
		require.Equal(t, h, privHandle)
	}

	t.Run("SessionCacheDisabled", func(t *testing.T) {
		sessionCacheSize = 0

		provider, err := New(opts, currentKS)
		require.NoError(t, err)
		pi := provider.(*impl)
		defer pi.ctx.Destroy()

		require.Nil(t, pi.sessPool, "sessPool channel should be nil")
		require.Empty(t, pi.sessions, "sessions set should be empty")
		require.Empty(t, pi.handleCache, "handleCache should be empty")

		sess1, err := pi.getSession()
		require.NoError(t, err)
		require.Len(t, pi.sessions, 1, "expected one open session")

		sess2, err := pi.getSession()
		require.NoError(t, err)
		require.Len(t, pi.sessions, 2, "expected two open sessions")

		// Generate a key
		k, err := pi.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
		require.NoError(t, err)
		verifyHandleCache(t, pi, sess1, k)
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		pi.returnSession(sess1)
		require.Len(t, pi.sessions, 1, "expected one open session")
		verifyHandleCache(t, pi, sess1, k)
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		pi.returnSession(sess2)
		require.Empty(t, pi.sessions, "expected sessions to be empty")
		require.Empty(t, pi.handleCache, "expected handles to be cleared")

		pi.slot = ^uint(0) // break OpenSession
		_, err = pi.getSession()
		require.EqualError(t, err, "OpenSession failed: pkcs11: 0x3: CKR_SLOT_ID_INVALID")
		require.Empty(t, pi.sessions, "expected sessions to be empty")
	})

	t.Run("SessionCacheEnabled", func(t *testing.T) {
		sessionCacheSize = 1

		provider, err := New(opts, currentKS)
		require.NoError(t, err)
		pi := provider.(*impl)
		defer pi.ctx.Destroy()

		require.NotNil(t, pi.sessPool, "sessPool channel should not be nil")
		require.Equal(t, 1, cap(pi.sessPool))
		require.Len(t, pi.sessions, 1, "sessions should contain login session")
		require.Len(t, pi.sessPool, 1, "sessionPool should hold login session")
		require.Empty(t, pi.handleCache, "handleCache should be empty")

		sess1, err := pi.getSession()
		require.NoError(t, err)
		require.Len(t, pi.sessions, 1, "expected one open session (sess1 from login)")
		require.Len(t, pi.sessPool, 0, "sessionPool should be empty")

		sess2, err := pi.getSession()
		require.NoError(t, err)
		require.Len(t, pi.sessions, 2, "expected two open sessions (sess1 and sess2)")
		require.Len(t, pi.sessPool, 0, "sessionPool should be empty")

		// Generate a key
		k, err := pi.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
		require.NoError(t, err)
		verifyHandleCache(t, pi, sess1, k)
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		pi.returnSession(sess1)
		require.Len(t, pi.sessions, 2, "expected two open sessions (sess2 in-use, sess1 cached)")
		require.Len(t, pi.sessPool, 1, "sessionPool should have one handle (sess1)")
		verifyHandleCache(t, pi, sess1, k)
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		pi.returnSession(sess2)
		require.Len(t, pi.sessions, 1, "expected one cached session (sess1)")
		require.Len(t, pi.sessPool, 1, "sessionPool should have one handle (sess1)")
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		sess1, err = pi.getSession()
		require.NoError(t, err)
		require.Len(t, pi.sessions, 1, "expected one open session (sess1)")
		require.Len(t, pi.sessPool, 0, "sessionPool should be empty")
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		pi.slot = ^uint(0) // break OpenSession
		_, err = pi.getSession()
		require.EqualError(t, err, "OpenSession failed: pkcs11: 0x3: CKR_SLOT_ID_INVALID")
		require.Len(t, pi.sessions, 1, "expected one active session (sess1)")
		require.Len(t, pi.sessPool, 0, "sessionPool should be empty")
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		// Return a busted session that should be cached
		pi.returnSession(pkcs11.SessionHandle(^uint(0)))
		require.Len(t, pi.sessions, 1, "expected one active session (sess1)")
		require.Len(t, pi.sessPool, 1, "sessionPool should contain busted session")
		require.Len(t, pi.handleCache, 2, "expected two handles in handle cache")

		// Return sess1 that should be discarded
		pi.returnSession(sess1)
		require.Len(t, pi.sessions, 0, "expected sess1 to be removed")
		require.Len(t, pi.sessPool, 1, "sessionPool should contain busted session")
		require.Empty(t, pi.handleCache, "expected handles to be purged on removal of last tracked session")

		// Try to get broken session from cache
		_, err = pi.getSession()
		require.EqualError(t, err, "OpenSession failed: pkcs11: 0x3: CKR_SLOT_ID_INVALID")
		require.Empty(t, pi.sessions, "expected sessions to be empty")
		require.Len(t, pi.sessPool, 0, "sessionPool should be empty")
	})
}

func TestKeyCache(t *testing.T) {
	defer func(s int) { sessionCacheSize = s }(sessionCacheSize)

	sessionCacheSize = 1
	opts := PKCS11Opts{Hash: "SHA2", Security: 256, SoftwareVerify: false}
	opts.Library, opts.Pin, opts.Label = FindPKCS11Lib()

	provider, err := New(opts, currentKS)
	require.NoError(t, err)
	pi := provider.(*impl)
	defer pi.ctx.Destroy()

	require.Empty(t, pi.keyCache)

	_, err = pi.GetKey([]byte("nonsense-key"))
	require.Error(t, err) // message comes from software keystore
	require.Empty(t, pi.keyCache)

	k, err := pi.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	require.NoError(t, err)
	_, ok := pi.cachedKey(k.SKI())
	require.False(t, ok, "created keys are not (currently) cached")

	key, err := pi.GetKey(k.SKI())
	require.NoError(t, err)
	cached, ok := pi.cachedKey(k.SKI())
	require.True(t, ok, "key should be cached")
	require.Same(t, key, cached, "key from cache should be what was found")

	// Kill all valid cached sessions
	pi.slot = ^uint(0)
	sess, err := pi.getSession()
	require.NoError(t, err)
	require.Len(t, pi.sessions, 1, "should have one active session")
	require.Len(t, pi.sessPool, 0, "sessionPool should be empty")

	pi.returnSession(pkcs11.SessionHandle(^uint(0)))
	require.Len(t, pi.sessions, 1, "should have one active session")
	require.Len(t, pi.sessPool, 1, "sessionPool should be empty")

	_, ok = pi.cachedKey(k.SKI())
	require.True(t, ok, "key should remain in cache due to active sessions")

	// Force caches to be cleared
	pi.returnSession(sess)
	require.Empty(t, pi.sessions, "sessions should be empty")
	require.Empty(t, pi.keyCache, "key cache should be empty")

	_, ok = pi.cachedKey(k.SKI())
	require.False(t, ok, "key should not be in cache")
}

func TestPKCS11ECKeySignVerify(t *testing.T) {
	hash1, err := currentBCCSP.Hash([]byte("authentic"), &bccsp.SHAOpts{})
	require.NoError(t, err)
	hash2, err := currentBCCSP.Hash([]byte("unauthentic"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	var oid asn1.ObjectIdentifier
	if currentTestConfig.securityLevel == 256 {
		oid = oidNamedCurveP256
	} else if currentTestConfig.securityLevel == 384 {
		oid = oidNamedCurveP384
	}

	key, pubKey, err := currentBCCSP.generateECKey(oid, true)
	require.NoError(t, err)

	R, S, err := currentBCCSP.signP11ECDSA(key, hash1)
	require.NoError(t, err)

	_, _, err = currentBCCSP.signP11ECDSA(nil, hash1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Private key not found")

	pass, err := currentBCCSP.verifyP11ECDSA(key, hash1, R, S, currentTestConfig.securityLevel/8)
	require.NoError(t, err)
	require.True(t, pass, "signature should match")

	pass = ecdsa.Verify(pubKey, hash1, R, S)
	require.True(t, pass, "signature should match with software verification")

	pass, err = currentBCCSP.verifyP11ECDSA(key, hash2, R, S, currentTestConfig.securityLevel/8)
	require.NoError(t, err)
	require.False(t, pass, "signature should not match")

	pass = ecdsa.Verify(pubKey, hash2, R, S)
	require.False(t, pass, "signature should not match with software verification")
}
