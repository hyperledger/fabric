//go:build pkcs11
// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package pkcs11

import (
	"bytes"
	"crypto/ecdsa"
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
	"io/ioutil"
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
	"github.com/miekg/pkcs11"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{256, "SHA2", true, false},
		{256, "SHA3", false, false},
		{384, "SHA2", false, false},
		{384, "SHA3", false, false},
		{384, "SHA3", true, false},
	}

	if strings.Contains(lib, "softhsm") {
		tests = append(tests, testConfig{256, "SHA2", true, true})
	}

	opts := PKCS11Opts{
		Library: lib,
		Label:   label,
		Pin:     pin,
	}

	for _, config := range tests {
		currentTestConfig = config

		opts.HashFamily = config.hashFamily
		opts.SecLevel = config.securityLevel
		opts.SoftVerify = config.softVerify
		opts.Immutable = config.immutable
		fmt.Printf("Immutable = [%v]\n", opts.Immutable)
		currentBCCSP, err = New(opts, keyStore)
		if err != nil {
			fmt.Printf("Failed initiliazing BCCSP at [%+v] \n%s\n", opts, err)
			return -1
		}

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

func TestKeyGenAESOpts(t *testing.T) {
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Key. It must not be nil")

	_, err = currentBCCSP.Sign(k, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid digest. Cannot be empty")
}

func defaultOptions() PKCS11Opts {
	lib, pin, label := FindPKCS11Lib()
	return PKCS11Opts{
		Library:    lib,
		Label:      label,
		Pin:        pin,
		HashFamily: "SHA2",
		SecLevel:   256,
		SoftVerify: false,
	}
}

func newKeyStore(t *testing.T) (bccsp.KeyStore, func()) {
	tempDir, err := ioutil.TempDir("", "pkcs11_ks")
	require.NoError(t, err)
	ks, err := sw.NewFileBasedKeyStore(nil, tempDir, false)
	require.NoError(t, err)

	return ks, func() { os.RemoveAll(tempDir) }
}

func newProvider(t *testing.T, opts PKCS11Opts, options ...Option) (*impl, func()) {
	ks, ksCleanup := newKeyStore(t)
	csp, err := New(opts, ks, options...)
	require.NoError(t, err)

	cleanup := func() {
		csp.(*impl).ctx.Destroy()
		ksCleanup()
	}
	return csp.(*impl), cleanup
}

type mapper struct {
	input  []byte
	result []byte
}

func (m *mapper) skiToID(ski []byte) []byte {
	m.input = ski
	return m.result
}

func TestKeyMapper(t *testing.T) {
	mapper := &mapper{}
	csp, cleanup := newProvider(t, defaultOptions(), WithKeyMapper(mapper.skiToID))
	defer cleanup()

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	digest, err := csp.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	sess, err := csp.getSession()
	require.NoError(t, err, "failed to get session")
	defer csp.returnSession(sess)

	newID := []byte("mapped-id")
	updateKeyIdentifier(t, csp.ctx, sess, pkcs11.CKO_PUBLIC_KEY, k.SKI(), newID)
	updateKeyIdentifier(t, csp.ctx, sess, pkcs11.CKO_PRIVATE_KEY, k.SKI(), newID)

	t.Run("ToMissingID", func(t *testing.T) {
		csp.clearCaches()
		mapper.result = k.SKI()
		_, err := csp.Sign(k, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Private key not found")
		require.Equal(t, k.SKI(), mapper.input, "expected mapper to receive ski %x, got %x", k.SKI(), mapper.input)
	})
	t.Run("ToNewID", func(t *testing.T) {
		csp.clearCaches()
		mapper.result = newID
		signature, err := csp.Sign(k, digest, nil)
		require.NoError(t, err)
		require.NotEmpty(t, signature, "signature must not be empty")
		require.Equal(t, k.SKI(), mapper.input, "expected mapper to receive ski %x, got %x", k.SKI(), mapper.input)
	})
}

func updateKeyIdentifier(t *testing.T, pctx *pkcs11.Ctx, sess pkcs11.SessionHandle, class uint, currentID, newID []byte) {
	pkt := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, class),
		pkcs11.NewAttribute(pkcs11.CKA_ID, currentID),
	}
	err := pctx.FindObjectsInit(sess, pkt)
	require.NoError(t, err)

	objs, _, err := pctx.FindObjects(sess, 1)
	require.NoError(t, err)
	require.Len(t, objs, 1)

	err = pctx.FindObjectsFinal(sess)
	require.NoError(t, err)

	err = pctx.SetAttributeValue(sess, objs[0], []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_ID, newID),
	})
	require.NoError(t, err)
}

func TestECDSAVerify(t *testing.T) {
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
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Key. It must not be nil")

	_, err = currentBCCSP.Verify(pk, nil, digest, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid signature. Cannot be empty")

	_, err = currentBCCSP.Verify(pk, signature, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid digest. Cannot be empty")

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

	pub, err := x509.ParsePKIXPublicKey(pkRaw)
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

func TestX509RSAKeyImport(t *testing.T) {
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err, "key generation failed")
	cert := &x509.Certificate{PublicKey: pk.Public()}
	key, err := currentBCCSP.KeyImport(cert, &bccsp.X509PublicKeyImportOpts{Temporary: false})
	assert.NoError(t, err, "key import failed")
	assert.NotNil(t, key, "key must not be nil")
	assert.Equal(t, &rsaPublicKey{pubKey: &pk.PublicKey}, key)
}

func TestKeyImportFromX509ECDSAPublicKey(t *testing.T) {
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
		IsCA:                  true,

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

	pub, err := x509.ParsePKIXPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to ECDSA.PublicKey [%s]", err)
	}

	certRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, cryptoSigner)
	if err != nil {
		t.Fatalf("Failed generating self-signed certificate [%s]", err)
	}

	cert, err := x509.ParseCertificate(certRaw)
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

func TestKeyGenFailures(t *testing.T) {
	var testOpts bccsp.KeyGenOpts
	ki := currentBCCSP
	_, err := ki.KeyGen(testOpts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid Opts parameter. It must not be nil")
}

func TestInitialize(t *testing.T) {
	// Setup PKCS11 library and provide initial set of values
	lib, pin, label := FindPKCS11Lib()

	// Test for no specified PKCS11 library
	_, err := (&impl{}).initialize(PKCS11Opts{Library: "", Pin: pin, Label: label})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pkcs11: library path not provided")

	// Test for invalid PKCS11 library
	_, err = (&impl{}).initialize(PKCS11Opts{Library: "badLib", Pin: pin, Label: label})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pkcs11: instantiation failed for badLib")

	// Test for invalid label
	_, err = (&impl{}).initialize(PKCS11Opts{Library: lib, Pin: pin, Label: "badLabel"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not find token with label")

	// Test for no pin
	_, err = (&impl{}).initialize(PKCS11Opts{Library: lib, Pin: "", Label: label})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Login failed: pkcs11")
}

func TestNamedCurveFromOID(t *testing.T) {
	// Test for valid P224 elliptic curve
	namedCurve := namedCurveFromOID(oidNamedCurveP224)
	assert.Equal(t, elliptic.P224(), namedCurve, "Did not receive expected named curve for oidNamedCurveP224")

	// Test for valid P256 elliptic curve
	namedCurve = namedCurveFromOID(oidNamedCurveP256)
	assert.Equal(t, elliptic.P256(), namedCurve, "Did not receive expected named curve for oidNamedCurveP256")

	// Test for valid P256 elliptic curve
	namedCurve = namedCurveFromOID(oidNamedCurveP384)
	assert.Equal(t, elliptic.P384(), namedCurve, "Did not receive expected named curve for oidNamedCurveP384")

	// Test for valid P521 elliptic curve
	namedCurve = namedCurveFromOID(oidNamedCurveP521)
	assert.Equal(t, elliptic.P521(), namedCurve, "Did not receive expected named curved for oidNamedCurveP521")

	testAsn1Value := asn1.ObjectIdentifier{4, 9, 15, 1}
	namedCurve = namedCurveFromOID(testAsn1Value)
	if namedCurve != nil {
		t.Fatal("Expected nil to be returned.")
	}
}

func TestPKCS11GetSession(t *testing.T) {
	var sessions []pkcs11.SessionHandle
	for i := 0; i < 3*sessionCacheSize; i++ {
		session, err := currentBCCSP.(*impl).getSession()
		assert.NoError(t, err)
		sessions = append(sessions, session)
	}

	// Return all sessions, should leave sessionCacheSize cached
	for _, session := range sessions {
		currentBCCSP.(*impl).returnSession(session)
	}

	// Should be able to get sessionCacheSize cached sessions
	sessions = nil
	for i := 0; i < sessionCacheSize; i++ {
		session, err := currentBCCSP.(*impl).getSession()
		assert.NoError(t, err)
		sessions = append(sessions, session)
	}

	// Cleanup
	for _, session := range sessions {
		currentBCCSP.(*impl).returnSession(session)
	}
}

func TestSessionHandleCaching(t *testing.T) {
	defer func(s int) { sessionCacheSize = s }(sessionCacheSize)
	opts := PKCS11Opts{
		HashFamily: "SHA2",
		SecLevel:   256,
		SoftVerify: false,
	}
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
	})
}

func TestKeyCache(t *testing.T) {
	defer func(s int) { sessionCacheSize = s }(sessionCacheSize)
	sessionCacheSize = 1

	opts := PKCS11Opts{
		HashFamily: "SHA2",
		SecLevel:   256,
		SoftVerify: false,
	}
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
	msg1 := []byte("This is my very authentic message")
	msg2 := []byte("This is my very unauthentic message")
	hash1, _ := currentBCCSP.Hash(msg1, &bccsp.SHAOpts{})
	hash2, _ := currentBCCSP.Hash(msg2, &bccsp.SHAOpts{})

	var oid asn1.ObjectIdentifier
	if currentTestConfig.securityLevel == 256 {
		oid = oidNamedCurveP256
	} else if currentTestConfig.securityLevel == 384 {
		oid = oidNamedCurveP384
	}

	key, pubKey, err := currentBCCSP.(*impl).generateECKey(oid, true)
	if err != nil {
		t.Fatalf("Failed generating Key [%s]", err)
	}

	R, S, err := currentBCCSP.(*impl).signP11ECDSA(key, hash1)
	if err != nil {
		t.Fatalf("Failed signing message [%s]", err)
	}

	_, _, err = currentBCCSP.(*impl).signP11ECDSA(nil, hash1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Private key not found")

	pass, err := currentBCCSP.(*impl).verifyP11ECDSA(key, hash1, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 1 [%s]", err)
	}
	if pass == false {
		t.Fatal("Signature should match!")
	}

	pass = ecdsa.Verify(pubKey, hash1, R, S)
	if pass == false {
		t.Fatal("Signature should match with software verification!")
	}

	pass, err = currentBCCSP.(*impl).verifyP11ECDSA(key, hash2, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 2 [%s]", err)
	}

	if pass != false {
		t.Fatal("Signature should not match!")
	}

	pass = ecdsa.Verify(pubKey, hash2, R, S)
	if pass != false {
		t.Fatal("Signature should not match with software verification!")
	}
}

func TestHandleSessionReturn(t *testing.T) {
	opts := PKCS11Opts{
		HashFamily: "SHA2",
		SecLevel:   256,
		SoftVerify: false,
	}
	opts.Library, opts.Pin, opts.Label = FindPKCS11Lib()

	provider, err := New(opts, currentKS)
	require.NoError(t, err)
	pi := provider.(*impl)
	defer pi.ctx.Destroy()

	// Retrieve and destroy default session created during initialization
	session, err := pi.getSession()
	require.NoError(t, err)
	pi.closeSession(session)

	// Verify session pool is empty and place invalid session in pool
	require.Empty(t, pi.sessPool, "sessionPool should be empty")
	pi.returnSession(pkcs11.SessionHandle(^uint(0)))

	// Attempt to generate key with invalid session
	_, err = pi.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	require.EqualError(t, err, "Failed generating ECDSA P256 key: P11: keypair generate failed [pkcs11: 0xB3: CKR_SESSION_HANDLE_INVALID]")
	require.Empty(t, pi.sessPool, "sessionPool should be empty")
}
