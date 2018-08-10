// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package pkcs11

import (
	"bytes"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"hash"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/signer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/sha3"
)

var (
	currentKS         bccsp.KeyStore
	currentBCCSP      bccsp.BCCSP
	currentTestConfig testConfig
)

type testConfig struct {
	securityLevel int
	hashFamily    string
	softVerify    bool
	immutable     bool
}

func TestMain(m *testing.M) {
	ks, err := sw.NewFileBasedKeyStore(nil, os.TempDir(), false)
	if err != nil {
		fmt.Printf("Failed initiliazing KeyStore [%s]", err)
		os.Exit(-1)
	}
	currentKS = ks

	lib, pin, label := FindPKCS11Lib()
	tests := []testConfig{
		{256, "SHA2", true, false},
		{256, "SHA3", false, false},
		{384, "SHA2", false, false},
		{384, "SHA3", false, false},
		{384, "SHA3", true, false},
	}

	if strings.Contains(lib, "softhsm") {
		tests = append(tests, []testConfig{
			{256, "SHA2", true, false},
			{256, "SHA2", true, true},
		}...)
	}

	opts := PKCS11Opts{
		Library: lib,
		Label:   label,
		Pin:     pin,
	}
	for _, config := range tests {
		var err error
		currentTestConfig = config

		opts.HashFamily = config.hashFamily
		opts.SecLevel = config.securityLevel
		opts.SoftVerify = config.softVerify
		opts.Immutable = config.immutable
		fmt.Printf("Immutable = [%v]", opts.Immutable)
		currentBCCSP, err = New(opts, currentKS)
		if err != nil {
			fmt.Printf("Failed initiliazing BCCSP at [%+v]: [%s]", opts, err)
			os.Exit(-1)
		}

		ret := m.Run()
		if ret != 0 {
			fmt.Printf("Failed testing at [%+v]", opts)
			os.Exit(-1)
		}
	}
	os.Exit(0)
}

func TestNew(t *testing.T) {
	opts := PKCS11Opts{
		HashFamily: "SHA2",
		SecLevel:   256,
		SoftVerify: false,
		Library:    "lib",
		Label:      "ForFabric",
		Pin:        "98765432",
	}

	// Setup PKCS11 library and provide initial set of values
	lib, _, _ := FindPKCS11Lib()
	opts.Library = lib

	// Test for nil keystore
	_, err := New(opts, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid bccsp.KeyStore instance. It must be different from nil.")

	// Test for invalid PKCS11 loadLib
	opts.Library = ""
	_, err = New(opts, currentKS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed initializing PKCS11 library")
}

func TestFindPKCS11LibEnvVars(t *testing.T) {
	const (
		dummy_PKCS11_LIB   = "/usr/lib/pkcs11"
		dummy_PKCS11_PIN   = "98765432"
		dummy_PKCS11_LABEL = "testing"
	)

	// Set environment variables used for test and preserve
	// original values for restoration after test completion
	orig_PKCS11_LIB := os.Getenv("PKCS11_LIB")
	os.Setenv("PKCS11_LIB", dummy_PKCS11_LIB)

	orig_PKCS11_PIN := os.Getenv("PKCS11_PIN")
	os.Setenv("PKCS11_PIN", dummy_PKCS11_PIN)

	orig_PKCS11_LABEL := os.Getenv("PKCS11_LABEL")
	os.Setenv("PKCS11_LABEL", dummy_PKCS11_LABEL)

	lib, pin, label := FindPKCS11Lib()
	assert.EqualValues(t, dummy_PKCS11_LIB, lib, "FindPKCS11Lib did not return expected library")
	assert.EqualValues(t, dummy_PKCS11_PIN, pin, "FindPKCS11Lib did not return expected pin")
	assert.EqualValues(t, dummy_PKCS11_LABEL, label, "FindPKCS11Lib did not return expected label")

	os.Setenv("PKCS11_LIB", orig_PKCS11_LIB)
	os.Setenv("PKCS11_PIN", orig_PKCS11_PIN)
	os.Setenv("PKCS11_LABEL", orig_PKCS11_LABEL)
}

func TestInvalidNewParameter(t *testing.T) {
	lib, pin, label := FindPKCS11Lib()
	opts := PKCS11Opts{
		Library:    lib,
		Label:      label,
		Pin:        pin,
		SoftVerify: true,
	}

	opts.HashFamily = "SHA2"
	opts.SecLevel = 0
	r, err := New(opts, currentKS)
	if err == nil {
		t.Fatal("Error should be different from nil in this case")
	}
	if r != nil {
		t.Fatal("Return value should be equal to nil in this case")
	}

	opts.HashFamily = "SHA8"
	opts.SecLevel = 256
	r, err = New(opts, currentKS)
	if err == nil {
		t.Fatal("Error should be different from nil in this case")
	}
	if r != nil {
		t.Fatal("Return value should be equal to nil in this case")
	}

	opts.HashFamily = "SHA2"
	opts.SecLevel = 256
	r, err = New(opts, nil)
	if err == nil {
		t.Fatal("Error should be different from nil in this case")
	}
	if r != nil {
		t.Fatal("Return value should be equal to nil in this case")
	}

	opts.HashFamily = "SHA3"
	opts.SecLevel = 0
	r, err = New(opts, nil)
	if err == nil {
		t.Fatal("Error should be different from nil in this case")
	}
	if r != nil {
		t.Fatal("Return value should be equal to nil in this case")
	}
}

func TestInvalidSKI(t *testing.T) {
	k, err := currentBCCSP.GetKey(nil)
	if err == nil {
		t.Fatal("Error should be different from nil in this case")
	}
	if k != nil {
		t.Fatal("Return value should be equal to nil in this case")
	}

	k, err = currentBCCSP.GetKey([]byte{0, 1, 2, 3, 4, 5, 6})
	if err == nil {
		t.Fatal("Error should be different from nil in this case")
	}
	if k != nil {
		t.Fatal("Return value should be equal to nil in this case")
	}
}

func TestKeyGenECDSAOpts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestKeyGenECDSAOpts")
	}
	// Curve P256
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA P256 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating ECDSA P256 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating ECDSA P256 key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating ECDSA P256 key. Key should be asymmetric")
	}

	ecdsaKey := k.(*ecdsaPrivateKey).pub
	if elliptic.P256() != ecdsaKey.pub.Curve {
		t.Fatal("P256 generated key in invalid. The curve must be P256.")
	}

	// Curve P384
	k, err = currentBCCSP.KeyGen(&bccsp.ECDSAP384KeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA P384 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating ECDSA P384 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating ECDSA P384 key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating ECDSA P384 key. Key should be asymmetric")
	}

	ecdsaKey = k.(*ecdsaPrivateKey).pub
	if elliptic.P384() != ecdsaKey.pub.Curve {
		t.Fatal("P256 generated key in invalid. The curve must be P384.")
	}
}

func TestKeyGenRSAOpts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestKeyGenRSAOpts")
	}
	// 1024
	k, err := currentBCCSP.KeyGen(&bccsp.RSA1024KeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA 1024 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating RSA 1024 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating RSA 1024 key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating RSA 1024 key. Key should be asymmetric")
	}

	// 2048
	k, err = currentBCCSP.KeyGen(&bccsp.RSA2048KeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA 2048 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating RSA 2048 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating RSA 2048 key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating RSA 2048 key. Key should be asymmetric")
	}
}

func TestKeyGenAESOpts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestKeyGenAESOpts")
	}
	// AES 128
	k, err := currentBCCSP.KeyGen(&bccsp.AES128KeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES 128 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating AES 128 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating AES 128 key. Key should be private")
	}
	if !k.Symmetric() {
		t.Fatal("Failed generating AES 128 key. Key should be symmetric")
	}

	// AES 192
	k, err = currentBCCSP.KeyGen(&bccsp.AES192KeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES 192 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating AES 192 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating AES 192 key. Key should be private")
	}
	if !k.Symmetric() {
		t.Fatal("Failed generating AES 192 key. Key should be symmetric")
	}

	// AES 256
	k, err = currentBCCSP.KeyGen(&bccsp.AES256KeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES 256 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating AES 256 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating AES 256 key. Key should be private")
	}
	if !k.Symmetric() {
		t.Fatal("Failed generating AES 256 key. Key should be symmetric")
	}
}

func TestHashOpts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestHashOpts")
	}
	msg := []byte("abcd")

	// SHA256
	digest1, err := currentBCCSP.Hash(msg, &bccsp.SHA256Opts{})
	if err != nil {
		t.Fatalf("Failed computing SHA256 [%s]", err)
	}

	h := sha256.New()
	h.Write(msg)
	digest2 := h.Sum(nil)

	if !bytes.Equal(digest1, digest2) {
		t.Fatalf("Different SHA256 computed. [%x][%x]", digest1, digest2)
	}

	// SHA384
	digest1, err = currentBCCSP.Hash(msg, &bccsp.SHA384Opts{})
	if err != nil {
		t.Fatalf("Failed computing SHA384 [%s]", err)
	}

	h = sha512.New384()
	h.Write(msg)
	digest2 = h.Sum(nil)

	if !bytes.Equal(digest1, digest2) {
		t.Fatalf("Different SHA384 computed. [%x][%x]", digest1, digest2)
	}

	// SHA3_256O
	digest1, err = currentBCCSP.Hash(msg, &bccsp.SHA3_256Opts{})
	if err != nil {
		t.Fatalf("Failed computing SHA3_256 [%s]", err)
	}

	h = sha3.New256()
	h.Write(msg)
	digest2 = h.Sum(nil)

	if !bytes.Equal(digest1, digest2) {
		t.Fatalf("Different SHA3_256 computed. [%x][%x]", digest1, digest2)
	}

	// SHA3_384
	digest1, err = currentBCCSP.Hash(msg, &bccsp.SHA3_384Opts{})
	if err != nil {
		t.Fatalf("Failed computing SHA3_384 [%s]", err)
	}

	h = sha3.New384()
	h.Write(msg)
	digest2 = h.Sum(nil)

	if !bytes.Equal(digest1, digest2) {
		t.Fatalf("Different SHA3_384 computed. [%x][%x]", digest1, digest2)
	}
}

func TestECDSAKeyGenEphemeral(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAKeyGenEphemeral")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating ECDSA key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating ECDSA key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating ECDSA key. Key should be asymmetric")
	}
	raw, err := k.Bytes()
	if err == nil {
		t.Fatal("Failed marshalling to bytes. Marshalling must fail.")
	}
	if len(raw) != 0 {
		t.Fatal("Failed marshalling to bytes. Output should be 0 bytes")
	}
	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting corresponding public key [%s]", err)
	}
	if pk == nil {
		t.Fatal("Public key must be different from nil.")
	}
}

func TestECDSAPrivateKeySKI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAPrivateKeySKI")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	ski := k.SKI()
	if len(ski) == 0 {
		t.Fatal("SKI not valid. Zero length.")
	}
}

func TestECDSAKeyGenNonEphemeral(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAKeyGenNonEphemeral")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating ECDSA key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating ECDSA key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating ECDSA key. Key should be asymmetric")
	}
}

func TestECDSAGetKeyBySKI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAGetKeyBySKI")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	k2, err := currentBCCSP.GetKey(k.SKI())
	if err != nil {
		t.Fatalf("Failed getting ECDSA key [%s]", err)
	}
	if k2 == nil {
		t.Fatal("Failed getting ECDSA key. Key must be different from nil")
	}
	if !k2.Private() {
		t.Fatal("Failed getting ECDSA key. Key should be private")
	}
	if k2.Symmetric() {
		t.Fatal("Failed getting ECDSA key. Key should be asymmetric")
	}

	// Check that the SKIs are the same
	if !bytes.Equal(k.SKI(), k2.SKI()) {
		t.Fatalf("SKIs are different [%x]!=[%x]", k.SKI(), k2.SKI())
	}
}

func TestECDSAPublicKeyFromPrivateKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAPublicKeyFromPrivateKey")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting public key from private ECDSA key [%s]", err)
	}
	if pk == nil {
		t.Fatal("Failed getting public key from private ECDSA key. Key must be different from nil")
	}
	if pk.Private() {
		t.Fatal("Failed generating ECDSA key. Key should be public")
	}
	if pk.Symmetric() {
		t.Fatal("Failed generating ECDSA key. Key should be asymmetric")
	}
}

func TestECDSAPublicKeyBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAPublicKeyBytes")
	}

	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting public key from private ECDSA key [%s]", err)
	}

	raw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed marshalling ECDSA public key [%s]", err)
	}
	if len(raw) == 0 {
		t.Fatal("Failed marshalling ECDSA public key. Zero length")
	}
}

func TestECDSAPublicKeySKI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAPublicKeySKI")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting public key from private ECDSA key [%s]", err)
	}

	ski := pk.SKI()
	if len(ski) == 0 {
		t.Fatal("SKI not valid. Zero length.")
	}
}

func TestECDSASign(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSASign")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}
	if len(signature) == 0 {
		t.Fatal("Failed generating ECDSA key. Signature must be different from nil")
	}

	_, err = currentBCCSP.Sign(nil, digest, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid Key. It must not be nil")

	_, err = currentBCCSP.Sign(k, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid digest. Cannot be empty")
}

func TestECDSAVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAVerify")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature  [%s]", err)
	}

	valid, err := currentBCCSP.Verify(k, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting corresponding public key [%s]", err)
	}

	valid, err = currentBCCSP.Verify(pk, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}

	_, err = currentBCCSP.Verify(nil, signature, digest, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid Key. It must not be nil")

	_, err = currentBCCSP.Verify(pk, nil, digest, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid signature. Cannot be empty")

	_, err = currentBCCSP.Verify(pk, signature, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid digest. Cannot be empty")

	// Import the exported public key
	pkRaw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed getting ECDSA raw public key [%s]", err)
	}

	// Store public key
	_, err = currentBCCSP.KeyImport(pkRaw, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed storing corresponding public key [%s]", err)
	}

	pk2, err := currentBCCSP.GetKey(pk.SKI())
	if err != nil {
		t.Fatalf("Failed retrieving corresponding public key [%s]", err)
	}

	valid, err = currentBCCSP.Verify(pk2, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestECDSAKeyImportFromExportedKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAKeyImportFromExportedKey")
	}
	// Generate an ECDSA key
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	// Export the public key
	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting ECDSA public key [%s]", err)
	}

	pkRaw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed getting ECDSA raw public key [%s]", err)
	}

	// Import the exported public key
	pk2, err := currentBCCSP.KeyImport(pkRaw, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed importing ECDSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing ECDSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := currentBCCSP.Verify(pk2, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestECDSAKeyImportFromECDSAPublicKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSAKeyImportFromECDSAPublicKey")
	}
	// Generate an ECDSA key
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	// Export the public key
	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting ECDSA public key [%s]", err)
	}

	pkRaw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed getting ECDSA raw public key [%s]", err)
	}

	pub, err := utils.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to ecdsa.PublicKey [%s]", err)
	}

	// Import the ecdsa.PublicKey
	pk2, err := currentBCCSP.KeyImport(pub, &bccsp.ECDSAGoPublicKeyImportOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed importing ECDSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing ECDSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := currentBCCSP.Verify(pk2, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestKeyImportFromX509ECDSAPublicKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestKeyImportFromX509ECDSAPublicKey")
	}
	// Generate an ECDSA key
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	// Generate a self-signed certificate
	testExtKeyUsage := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	testUnknownExtKeyUsage := []asn1.ObjectIdentifier{[]int{1, 2, 3}, []int{2, 59, 1}}
	extraExtensionData := []byte("extra extension")
	commonName := "test.example.com"
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Σ Acme Co"},
			Country:      []string{"US"},
			ExtraNames: []pkix.AttributeTypeAndValue{
				{
					Type:  []int{2, 5, 4, 42},
					Value: "Gopher",
				},
				// This should override the Country, above.
				{
					Type:  []int{2, 5, 4, 6},
					Value: "NL",
				},
			},
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(1 * time.Hour),

		SignatureAlgorithm: x509.ECDSAWithSHA256,

		SubjectKeyId: []byte{1, 2, 3, 4},
		KeyUsage:     x509.KeyUsageCertSign,

		ExtKeyUsage:        testExtKeyUsage,
		UnknownExtKeyUsage: testUnknownExtKeyUsage,

		BasicConstraintsValid: true,
		IsCA: true,

		OCSPServer:            []string{"http://ocurrentBCCSP.example.com"},
		IssuingCertificateURL: []string{"http://crt.example.com/ca1.crt"},

		DNSNames:       []string{"test.example.com"},
		EmailAddresses: []string{"gopher@golang.org"},
		IPAddresses:    []net.IP{net.IPv4(127, 0, 0, 1).To4(), net.ParseIP("2001:4860:0:2001::68")},

		PolicyIdentifiers:   []asn1.ObjectIdentifier{[]int{1, 2, 3}},
		PermittedDNSDomains: []string{".example.com", "example.com"},

		CRLDistributionPoints: []string{"http://crl1.example.com/ca1.crl", "http://crl2.example.com/ca1.crl"},

		ExtraExtensions: []pkix.Extension{
			{
				Id:    []int{1, 2, 3, 4},
				Value: extraExtensionData,
			},
		},
	}

	cryptoSigner, err := signer.New(currentBCCSP, k)
	if err != nil {
		t.Fatalf("Failed initializing CyrptoSigner [%s]", err)
	}

	// Export the public key
	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting ECDSA public key [%s]", err)
	}

	pkRaw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed getting ECDSA raw public key [%s]", err)
	}

	pub, err := utils.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to ECDSA.PublicKey [%s]", err)
	}

	certRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, cryptoSigner)
	if err != nil {
		t.Fatalf("Failed generating self-signed certificate [%s]", err)
	}

	cert, err := utils.DERToX509Certificate(certRaw)
	if err != nil {
		t.Fatalf("Failed generating X509 certificate object from raw [%s]", err)
	}

	// Import the certificate's public key
	pk2, err := currentBCCSP.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: false})

	if err != nil {
		t.Fatalf("Failed importing ECDSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing ECDSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := currentBCCSP.Verify(pk2, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestECDSASignatureEncoding(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSASignatureEncoding")
	}
	v := []byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x02, 0xff, 0xf1}
	_, err := asn1.Unmarshal(v, &utils.ECDSASignature{})
	if err == nil {
		t.Fatalf("Unmarshalling should fail for [% x]", v)
	}
	t.Logf("Unmarshalling correctly failed for [% x] [%s]", v, err)

	v = []byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x02, 0x00, 0x01}
	_, err = asn1.Unmarshal(v, &utils.ECDSASignature{})
	if err == nil {
		t.Fatalf("Unmarshalling should fail for [% x]", v)
	}
	t.Logf("Unmarshalling correctly failed for [% x] [%s]", v, err)

	v = []byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x81, 0x01, 0x01}
	_, err = asn1.Unmarshal(v, &utils.ECDSASignature{})
	if err == nil {
		t.Fatalf("Unmarshalling should fail for [% x]", v)
	}
	t.Logf("Unmarshalling correctly failed for [% x] [%s]", v, err)

	v = []byte{0x30, 0x07, 0x02, 0x01, 0x8F, 0x02, 0x81, 0x01, 0x8F}
	_, err = asn1.Unmarshal(v, &utils.ECDSASignature{})
	if err == nil {
		t.Fatalf("Unmarshalling should fail for [% x]", v)
	}
	t.Logf("Unmarshalling correctly failed for [% x] [%s]", v, err)

	v = []byte{0x30, 0x0A, 0x02, 0x01, 0x8F, 0x02, 0x05, 0x00, 0x00, 0x00, 0x00, 0x8F}
	_, err = asn1.Unmarshal(v, &utils.ECDSASignature{})
	if err == nil {
		t.Fatalf("Unmarshalling should fail for [% x]", v)
	}
	t.Logf("Unmarshalling correctly failed for [% x] [%s]", v, err)

}

func TestECDSALowS(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestECDSALowS")
	}
	// Ensure that signature with low-S are generated
	k, err := currentBCCSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	R, S, err := utils.UnmarshalECDSASignature(signature)
	if err != nil {
		t.Fatalf("Failed unmarshalling signature [%s]", err)
	}

	if S.Cmp(utils.GetCurveHalfOrdersAt(k.(*ecdsaPrivateKey).pub.pub.Curve)) >= 0 {
		t.Fatal("Invalid signature. It must have low-S")
	}

	valid, err := currentBCCSP.Verify(k, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}

	// Ensure that signature with high-S are rejected.
	for {
		R, S, err = currentBCCSP.(*impl).signP11ECDSA(k.SKI(), digest)
		if err != nil {
			t.Fatalf("Failed generating signature [%s]", err)
		}

		if S.Cmp(utils.GetCurveHalfOrdersAt(k.(*ecdsaPrivateKey).pub.pub.Curve)) > 0 {
			break
		}
	}

	sig, err := utils.MarshalECDSASignature(R, S)
	if err != nil {
		t.Fatalf("Failing unmarshalling signature [%s]", err)
	}

	valid, err = currentBCCSP.Verify(k, sig, digest, nil)
	if err == nil {
		t.Fatal("Failed verifying ECDSA signature. It must fail for a signature with high-S")
	}
	if valid {
		t.Fatal("Failed verifying ECDSA signature. It must fail for a signature with high-S")
	}
}

func TestAESKeyGen(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestAESKeyGen")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating AES_256 key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating AES_256 key. Key should be private")
	}
	if !k.Symmetric() {
		t.Fatal("Failed generating AES_256 key. Key should be symmetric")
	}

	pk, err := k.PublicKey()
	if err == nil {
		t.Fatal("Error should be different from nil in this case")
	}
	if pk != nil {
		t.Fatal("Return value should be equal to nil in this case")
	}
}

func TestAESEncrypt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestAESEncrypt")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	ct, err := currentBCCSP.Encrypt(k, []byte("Hello World"), &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}
	if len(ct) == 0 {
		t.Fatal("Failed encrypting. Nil ciphertext")
	}
}

func TestAESDecrypt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestAESDecrypt")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	msg := []byte("Hello World")

	ct, err := currentBCCSP.Encrypt(k, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	pt, err := currentBCCSP.Decrypt(k, ct, bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed decrypting [%s]", err)
	}
	if len(ct) == 0 {
		t.Fatal("Failed decrypting. Nil plaintext")
	}

	if !bytes.Equal(msg, pt) {
		t.Fatalf("Failed decrypting. Decrypted plaintext is different from the original. [%x][%x]", msg, pt)
	}
}

func TestHMACTruncated256KeyDerivOverAES256Key(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestHMACTruncated256KeyDerivOverAES256Key")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	hmcaedKey, err := currentBCCSP.KeyDeriv(k, &bccsp.HMACTruncated256AESDeriveKeyOpts{Temporary: false, Arg: []byte{1}})
	if err != nil {
		t.Fatalf("Failed HMACing AES_256 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed HMACing AES_256 key. HMACed Key must be different from nil")
	}
	if !hmcaedKey.Private() {
		t.Fatal("Failed HMACing AES_256 key. HMACed Key should be private")
	}
	if !hmcaedKey.Symmetric() {
		t.Fatal("Failed HMACing AES_256 key. HMACed Key should be asymmetric")
	}
	raw, err := hmcaedKey.Bytes()
	if err == nil {
		t.Fatal("Failed marshalling to bytes. Operation must be forbidden")
	}
	if len(raw) != 0 {
		t.Fatal("Failed marshalling to bytes. Operation must return 0 bytes")
	}

	msg := []byte("Hello World")

	ct, err := currentBCCSP.Encrypt(hmcaedKey, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	pt, err := currentBCCSP.Decrypt(hmcaedKey, ct, bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed decrypting [%s]", err)
	}
	if len(ct) == 0 {
		t.Fatal("Failed decrypting. Nil plaintext")
	}

	if !bytes.Equal(msg, pt) {
		t.Fatalf("Failed decrypting. Decrypted plaintext is different from the original. [%x][%x]", msg, pt)
	}

}

func TestHMACKeyDerivOverAES256Key(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestHMACKeyDerivOverAES256Key")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	hmcaedKey, err := currentBCCSP.KeyDeriv(k, &bccsp.HMACDeriveKeyOpts{Temporary: false, Arg: []byte{1}})

	if err != nil {
		t.Fatalf("Failed HMACing AES_256 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed HMACing AES_256 key. HMACed Key must be different from nil")
	}
	if !hmcaedKey.Private() {
		t.Fatal("Failed HMACing AES_256 key. HMACed Key should be private")
	}
	if !hmcaedKey.Symmetric() {
		t.Fatal("Failed HMACing AES_256 key. HMACed Key should be asymmetric")
	}
	raw, err := hmcaedKey.Bytes()
	if err != nil {
		t.Fatalf("Failed marshalling to bytes [%s]", err)
	}
	if len(raw) == 0 {
		t.Fatal("Failed marshalling to bytes. 0 bytes")
	}
}

func TestAES256KeyImport(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestAES256KeyImport")
	}
	raw, err := sw.GetRandomBytes(32)
	if err != nil {
		t.Fatalf("Failed generating AES key [%s]", err)
	}

	k, err := currentBCCSP.KeyImport(raw, &bccsp.AES256ImportKeyOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed importing AES_256 key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed importing AES_256 key. Imported Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed HMACing AES_256 key. Imported Key should be private")
	}
	if !k.Symmetric() {
		t.Fatal("Failed HMACing AES_256 key. Imported Key should be asymmetric")
	}
	raw, err = k.Bytes()
	if err == nil {
		t.Fatal("Failed marshalling to bytes. Marshalling must fail.")
	}
	if len(raw) != 0 {
		t.Fatal("Failed marshalling to bytes. Output should be 0 bytes")
	}

	msg := []byte("Hello World")

	ct, err := currentBCCSP.Encrypt(k, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	pt, err := currentBCCSP.Decrypt(k, ct, bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed decrypting [%s]", err)
	}
	if len(ct) == 0 {
		t.Fatal("Failed decrypting. Nil plaintext")
	}

	if !bytes.Equal(msg, pt) {
		t.Fatalf("Failed decrypting. Decrypted plaintext is different from the original. [%x][%x]", msg, pt)
	}
}

func TestAES256KeyImportBadPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestAES256KeyImportBadPaths")
	}
	_, err := currentBCCSP.KeyImport(nil, &bccsp.AES256ImportKeyOpts{Temporary: false})
	if err == nil {
		t.Fatal("Failed importing key. Must fail on importing nil key")
	}

	_, err = currentBCCSP.KeyImport([]byte{1}, &bccsp.AES256ImportKeyOpts{Temporary: false})
	if err == nil {
		t.Fatal("Failed importing key. Must fail on importing a key with an invalid length")
	}
}

func TestAES256KeyGenSKI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestAES256KeyGenSKI")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	k2, err := currentBCCSP.GetKey(k.SKI())
	if err != nil {
		t.Fatalf("Failed getting AES_256 key [%s]", err)
	}
	if k2 == nil {
		t.Fatal("Failed getting AES_256 key. Key must be different from nil")
	}
	if !k2.Private() {
		t.Fatal("Failed getting AES_256 key. Key should be private")
	}
	if !k2.Symmetric() {
		t.Fatal("Failed getting AES_256 key. Key should be symmetric")
	}

	// Check that the SKIs are the same
	if !bytes.Equal(k.SKI(), k2.SKI()) {
		t.Fatalf("SKIs are different [%x]!=[%x]", k.SKI(), k2.SKI())
	}

}

func TestSHA(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestSHA")
	}
	for i := 0; i < 100; i++ {
		b, err := sw.GetRandomBytes(i)
		if err != nil {
			t.Fatalf("Failed getting random bytes [%s]", err)
		}

		h1, err := currentBCCSP.Hash(b, &bccsp.SHAOpts{})
		if err != nil {
			t.Fatalf("Failed computing SHA [%s]", err)
		}

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
		if !bytes.Equal(h1, h2) {
			t.Fatalf("Discrempancy found in HASH result [%x], [%x]!=[%x]", b, h1, h2)
		}
	}
}

func TestRSAKeyGenEphemeral(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAKeyGenEphemeral")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating RSA key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating RSA key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating RSA key. Key should be asymmetric")
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed generating RSA corresponding public key [%s]", err)
	}
	if pk == nil {
		t.Fatal("PK must be different from nil")
	}

	b, err := k.Bytes()
	if err == nil {
		t.Fatal("Secret keys cannot be exported. It must fail in this case")
	}
	if len(b) != 0 {
		t.Fatal("Secret keys cannot be exported. It must be nil")
	}

}

func TestRSAPrivateKeySKI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAPrivateKeySKI")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	ski := k.SKI()
	if len(ski) == 0 {
		t.Fatal("SKI not valid. Zero length.")
	}
}

func TestRSAKeyGenNonEphemeral(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAKeyGenNonEphemeral")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed generating RSA key. Key must be different from nil")
	}
	if !k.Private() {
		t.Fatal("Failed generating RSA key. Key should be private")
	}
	if k.Symmetric() {
		t.Fatal("Failed generating RSA key. Key should be asymmetric")
	}
}

func TestRSAGetKeyBySKI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAGetKeyBySKI")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	k2, err := currentBCCSP.GetKey(k.SKI())
	if err != nil {
		t.Fatalf("Failed getting RSA key [%s]", err)
	}
	if k2 == nil {
		t.Fatal("Failed getting RSA key. Key must be different from nil")
	}
	if !k2.Private() {
		t.Fatal("Failed getting RSA key. Key should be private")
	}
	if k2.Symmetric() {
		t.Fatal("Failed getting RSA key. Key should be asymmetric")
	}

	// Check that the SKIs are the same
	if !bytes.Equal(k.SKI(), k2.SKI()) {
		t.Fatalf("SKIs are different [%x]!=[%x]", k.SKI(), k2.SKI())
	}
}

func TestRSAPublicKeyFromPrivateKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAPublicKeyFromPrivateKey")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting public key from private RSA key [%s]", err)
	}
	if pk == nil {
		t.Fatal("Failed getting public key from private RSA key. Key must be different from nil")
	}
	if pk.Private() {
		t.Fatal("Failed generating RSA key. Key should be public")
	}
	if pk.Symmetric() {
		t.Fatal("Failed generating RSA key. Key should be asymmetric")
	}
}

func TestRSAPublicKeyBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAPublicKeyBytes")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting public key from private RSA key [%s]", err)
	}

	raw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed marshalling RSA public key [%s]", err)
	}
	if len(raw) == 0 {
		t.Fatal("Failed marshalling RSA public key. Zero length")
	}
}

func TestRSAPublicKeySKI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAPublicKeySKI")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting public key from private RSA key [%s]", err)
	}

	ski := pk.SKI()
	if len(ski) == 0 {
		t.Fatal("SKI not valid. Zero length.")
	}
}

func TestRSASign(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSASign")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}
	if len(signature) == 0 {
		t.Fatal("Failed generating RSA key. Signature must be different from nil")
	}
}

func TestRSAVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAVerify")
	}
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}

	valid, err := currentBCCSP.Verify(k, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}

	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting corresponding public key [%s]", err)
	}

	valid, err = currentBCCSP.Verify(pk, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}

	// Store public key
	err = currentKS.StoreKey(pk)
	if err != nil {
		t.Fatalf("Failed storing corresponding public key [%s]", err)
	}

	pk2, err := currentKS.GetKey(pk.SKI())
	if err != nil {
		t.Fatalf("Failed retrieving corresponding public key [%s]", err)
	}

	valid, err = currentBCCSP.Verify(pk2, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}

}

func TestRSAKeyImportFromRSAPublicKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestRSAKeyImportFromRSAPublicKey")
	}
	// Generate an RSA key
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	// Export the public key
	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting RSA public key [%s]", err)
	}

	pkRaw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed getting RSA raw public key [%s]", err)
	}

	pub, err := utils.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to RSA.PublicKey [%s]", err)
	}

	// Import the RSA.PublicKey
	pk2, err := currentBCCSP.KeyImport(pub, &bccsp.RSAGoPublicKeyImportOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed importing RSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing RSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}

	valid, err := currentBCCSP.Verify(pk2, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}
}

func TestKeyImportFromX509RSAPublicKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestKeyImportFromX509RSAPublicKey")
	}
	// Generate an RSA key
	k, err := currentBCCSP.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	// Generate a self-signed certificate
	testExtKeyUsage := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	testUnknownExtKeyUsage := []asn1.ObjectIdentifier{[]int{1, 2, 3}, []int{2, 59, 1}}
	extraExtensionData := []byte("extra extension")
	commonName := "test.example.com"
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Σ Acme Co"},
			Country:      []string{"US"},
			ExtraNames: []pkix.AttributeTypeAndValue{
				{
					Type:  []int{2, 5, 4, 42},
					Value: "Gopher",
				},
				// This should override the Country, above.
				{
					Type:  []int{2, 5, 4, 6},
					Value: "NL",
				},
			},
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(1 * time.Hour),

		SignatureAlgorithm: x509.SHA256WithRSA,

		SubjectKeyId: []byte{1, 2, 3, 4},
		KeyUsage:     x509.KeyUsageCertSign,

		ExtKeyUsage:        testExtKeyUsage,
		UnknownExtKeyUsage: testUnknownExtKeyUsage,

		BasicConstraintsValid: true,
		IsCA: true,

		OCSPServer:            []string{"http://ocurrentBCCSP.example.com"},
		IssuingCertificateURL: []string{"http://crt.example.com/ca1.crt"},

		DNSNames:       []string{"test.example.com"},
		EmailAddresses: []string{"gopher@golang.org"},
		IPAddresses:    []net.IP{net.IPv4(127, 0, 0, 1).To4(), net.ParseIP("2001:4860:0:2001::68")},

		PolicyIdentifiers:   []asn1.ObjectIdentifier{[]int{1, 2, 3}},
		PermittedDNSDomains: []string{".example.com", "example.com"},

		CRLDistributionPoints: []string{"http://crl1.example.com/ca1.crl", "http://crl2.example.com/ca1.crl"},

		ExtraExtensions: []pkix.Extension{
			{
				Id:    []int{1, 2, 3, 4},
				Value: extraExtensionData,
			},
		},
	}

	cryptoSigner, err := signer.New(currentBCCSP, k)
	if err != nil {
		t.Fatalf("Failed initializing CyrptoSigner [%s]", err)
	}

	// Export the public key
	pk, err := k.PublicKey()
	if err != nil {
		t.Fatalf("Failed getting RSA public key [%s]", err)
	}

	pkRaw, err := pk.Bytes()
	if err != nil {
		t.Fatalf("Failed getting RSA raw public key [%s]", err)
	}

	pub, err := utils.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to RSA.PublicKey [%s]", err)
	}

	certRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, cryptoSigner)
	if err != nil {
		t.Fatalf("Failed generating self-signed certificate [%s]", err)
	}

	cert, err := utils.DERToX509Certificate(certRaw)
	if err != nil {
		t.Fatalf("Failed generating X509 certificate object from raw [%s]", err)
	}

	// Import the certificate's public key
	pk2, err := currentBCCSP.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: false})

	if err != nil {
		t.Fatalf("Failed importing RSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing RSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := currentBCCSP.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := currentBCCSP.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}

	valid, err := currentBCCSP.Verify(pk2, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: getCryptoHashIndex(t)})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}
}

func getCryptoHashIndex(t *testing.T) crypto.Hash {
	switch currentTestConfig.hashFamily {
	case "SHA2":
		switch currentTestConfig.securityLevel {
		case 256:
			return crypto.SHA256
		case 384:
			return crypto.SHA384
		default:
			t.Fatalf("Invalid security level [%d]", currentTestConfig.securityLevel)
		}
	case "SHA3":
		switch currentTestConfig.securityLevel {
		case 256:
			return crypto.SHA3_256
		case 384:
			return crypto.SHA3_384
		default:
			t.Fatalf("Invalid security level [%d]", currentTestConfig.securityLevel)
		}
	default:
		t.Fatalf("Invalid hash family [%s]", currentTestConfig.hashFamily)
	}

	return crypto.SHA3_256
}
