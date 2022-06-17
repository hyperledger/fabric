//go:build pkcs11
// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/bccsp/utils"
	"github.com/miekg/pkcs11"
	"github.com/stretchr/testify/require"
)

func defaultOptions() PKCS11Opts {
	lib, pin, label := FindPKCS11Lib()
	return PKCS11Opts{
		Library:                 lib,
		Label:                   label,
		Pin:                     pin,
		Hash:                    "SHA2",
		Security:                256,
		SoftwareVerify:          false,
		createSessionRetryDelay: time.Millisecond,
	}
}

func newKeyStore(t *testing.T) bccsp.KeyStore {
	tempDir := t.TempDir()
	ks, err := sw.NewFileBasedKeyStore(nil, tempDir, false)
	require.NoError(t, err)

	return ks
}

func newSWProvider(t *testing.T) bccsp.BCCSP {
	ks := newKeyStore(t)
	swCsp, err := sw.NewDefaultSecurityLevelWithKeystore(ks)
	require.NoError(t, err)

	return swCsp
}

func newProvider(t *testing.T, opts PKCS11Opts, options ...Option) (*Provider, func()) {
	ks := newKeyStore(t)
	csp, err := New(opts, ks, options...)
	require.NoError(t, err)

	cleanup := func() {
		csp.ctx.Destroy()
	}
	return csp, cleanup
}

func TestNew(t *testing.T) {
	ks := newKeyStore(t)

	t.Run("DefaultConfig", func(t *testing.T) {
		opts := defaultOptions()
		opts.createSessionRetryDelay = 0

		csp, err := New(opts, ks)
		require.NoError(t, err)
		defer func() { csp.ctx.Destroy() }()

		curve, err := curveForSecurityLevel(opts.Security)
		require.NoError(t, err)

		require.NotNil(t, csp.BCCSP)
		require.Equal(t, opts.Pin, csp.pin)
		require.NotNil(t, csp.ctx)
		require.True(t, curve.Equal(csp.curve))
		require.Equal(t, opts.SoftwareVerify, csp.softVerify)
		require.Equal(t, opts.Immutable, csp.immutable)
		require.Equal(t, defaultCreateSessionRetries, csp.createSessionRetries)
		require.Equal(t, defaultCreateSessionRetryDelay, csp.createSessionRetryDelay)
		require.Equal(t, defaultSessionCacheSize, cap(csp.sessPool))
	})

	t.Run("ConditionalOverride", func(t *testing.T) {
		opts := defaultOptions()
		opts.createSessionRetries = 3
		opts.createSessionRetryDelay = time.Second
		opts.sessionCacheSize = -1

		csp, err := New(opts, ks)
		require.NoError(t, err)
		defer func() { csp.ctx.Destroy() }()

		require.Equal(t, 3, csp.createSessionRetries)
		require.Equal(t, time.Second, csp.createSessionRetryDelay)
		require.Nil(t, csp.sessPool)
	})
}

func TestInvalidNewParameter(t *testing.T) {
	ks := newKeyStore(t)

	t.Run("BadSecurityLevel", func(t *testing.T) {
		opts := defaultOptions()
		opts.Security = 0

		_, err := New(opts, ks)
		require.EqualError(t, err, "Failed initializing configuration: Security level not supported [0]")
	})

	t.Run("BadHashFamily", func(t *testing.T) {
		opts := defaultOptions()
		opts.Hash = "SHA8"

		_, err := New(opts, ks)
		require.EqualError(t, err, "Failed initializing fallback SW BCCSP: Failed initializing configuration at [256,SHA8]: Hash Family not supported [SHA8]")
	})

	t.Run("BadKeyStore", func(t *testing.T) {
		_, err := New(defaultOptions(), nil)
		require.EqualError(t, err, "Failed initializing fallback SW BCCSP: Invalid bccsp.KeyStore instance. It must be different from nil.")
	})

	t.Run("MissingLibrary", func(t *testing.T) {
		opts := defaultOptions()
		opts.Library = ""

		_, err := New(opts, ks)
		require.Error(t, err)
		require.Contains(t, err.Error(), "pkcs11: library path not provided")
	})
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

func TestInvalidSKI(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	_, err := csp.GetKey(nil)
	require.EqualError(t, err, "Failed getting key for SKI [[]]: invalid SKI. Cannot be of zero length")

	_, err = csp.GetKey([]byte{0, 1, 2, 3, 4, 5, 6})
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Failed getting key for SKI [[0 1 2 3 4 5 6]]: "))
}

func TestKeyGenECDSAOpts(t *testing.T) {
	tests := map[string]struct {
		curve     elliptic.Curve
		immutable bool
		opts      bccsp.KeyGenOpts
	}{
		"Default":             {elliptic.P256(), false, &bccsp.ECDSAKeyGenOpts{Temporary: false}},
		"P256":                {elliptic.P256(), false, &bccsp.ECDSAP256KeyGenOpts{Temporary: false}},
		"P384":                {elliptic.P384(), false, &bccsp.ECDSAP384KeyGenOpts{Temporary: false}},
		"Immutable":           {elliptic.P384(), true, &bccsp.ECDSAP384KeyGenOpts{Temporary: false}},
		"Ephemeral/Default":   {elliptic.P256(), false, &bccsp.ECDSAKeyGenOpts{Temporary: true}},
		"Ephemeral/P256":      {elliptic.P256(), false, &bccsp.ECDSAP256KeyGenOpts{Temporary: true}},
		"Ephemeral/P384":      {elliptic.P384(), false, &bccsp.ECDSAP384KeyGenOpts{Temporary: true}},
		"Ephemeral/Immutable": {elliptic.P384(), true, &bccsp.ECDSAP384KeyGenOpts{Temporary: false}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			opts := defaultOptions()
			opts.Immutable = tt.immutable
			csp, cleanup := newProvider(t, opts)
			defer cleanup()

			k, err := csp.KeyGen(tt.opts)
			require.NoError(t, err)
			require.True(t, k.Private(), "key should be private")
			require.False(t, k.Symmetric(), "key should be asymmetric")

			ecdsaKey := k.(*ecdsaPrivateKey).pub
			require.Equal(t, tt.curve, ecdsaKey.pub.Curve, "wrong curve")

			raw, err := k.Bytes()
			require.EqualError(t, err, "Not supported.")
			require.Empty(t, raw, "result should be empty")

			pk, err := k.PublicKey()
			require.NoError(t, err)
			require.NotNil(t, pk)

			sess, err := csp.getSession()
			require.NoError(t, err)
			defer csp.returnSession(sess)

			for _, kt := range []keyType{publicKeyType, privateKeyType} {
				handle, err := csp.findKeyPairFromSKI(sess, k.SKI(), kt)
				require.NoError(t, err)

				attr, err := csp.ctx.GetAttributeValue(sess, handle, []*pkcs11.Attribute{{Type: pkcs11.CKA_TOKEN}})
				require.NoError(t, err)
				require.Len(t, attr, 1)

				if tt.opts.Ephemeral() {
					require.Equal(t, []byte{0}, attr[0].Value)
				} else {
					require.Equal(t, []byte{1}, attr[0].Value)
				}

				attr, err = csp.ctx.GetAttributeValue(sess, handle, []*pkcs11.Attribute{{Type: pkcs11.CKA_MODIFIABLE}})
				require.NoError(t, err)
				require.Len(t, attr, 1)

				if tt.immutable {
					require.Equal(t, []byte{0}, attr[0].Value)
				} else {
					require.Equal(t, []byte{1}, attr[0].Value)
				}
			}
		})
	}
}

func TestKeyGenMissingOpts(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	_, err := csp.KeyGen(bccsp.KeyGenOpts(nil))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid Opts parameter. It must not be nil")
}

func TestECDSAGetKeyBySKI(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	k2, err := csp.GetKey(k.SKI())
	require.NoError(t, err)

	require.True(t, k2.Private(), "key should be private")
	require.False(t, k2.Symmetric(), "key should be asymmetric")
	require.Equalf(t, k.SKI(), k2.SKI(), "expected %x got %x", k.SKI(), k2.SKI())
}

func TestECDSAPublicKeyFromPrivateKey(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	pk, err := k.PublicKey()
	require.NoError(t, err)
	require.False(t, pk.Private(), "key should be public")
	require.False(t, pk.Symmetric(), "key should be asymmetric")
	require.Equal(t, k.SKI(), pk.SKI(), "SKI should be the same")

	raw, err := pk.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, raw, "marshaled ECDSA public key must not be empty")
}

func TestECDSASign(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	digest, err := csp.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	signature, err := csp.Sign(k, digest, nil)
	require.NoError(t, err)
	require.NotEmpty(t, signature, "signature must not be empty")

	t.Run("NoKey", func(t *testing.T) {
		_, err := csp.Sign(nil, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid Key. It must not be nil")
	})

	t.Run("BadSKI", func(t *testing.T) {
		_, err := csp.Sign(&ecdsaPrivateKey{ski: []byte("bad-ski")}, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Private key not found")
	})

	t.Run("MissingDigest", func(t *testing.T) {
		_, err = csp.Sign(k, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid digest. Cannot be empty")
	})
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
		require.ErrorContains(t, err, "Private key not found")
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

func TestVerify(t *testing.T) {
	pkcs11CSP, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	digest, err := pkcs11CSP.Hash([]byte("Hello, World."), &bccsp.SHAOpts{})
	require.NoError(t, err)
	otherDigest, err := pkcs11CSP.Hash([]byte("Bye, World."), &bccsp.SHAOpts{})
	require.NoError(t, err)

	pkcs11Key, err := pkcs11CSP.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	require.NoError(t, err)
	pkcs11PublicKey, err := pkcs11Key.PublicKey()
	require.NoError(t, err)
	b, err := pkcs11PublicKey.Bytes()
	require.NoError(t, err)

	swCSP := newSWProvider(t)
	swKey, err := swCSP.KeyImport(b, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: false})
	require.NoError(t, err)

	signature, err := pkcs11CSP.Sign(pkcs11Key, digest, nil)
	require.NoError(t, err)

	swPublicKey, err := swKey.PublicKey()
	require.NoError(t, err)

	valid, err := pkcs11CSP.Verify(swPublicKey, signature, digest, nil)
	require.NoError(t, err)
	require.True(t, valid, "signature should be valid from software public key")

	valid, err = pkcs11CSP.Verify(swPublicKey, signature, otherDigest, nil)
	require.NoError(t, err)
	require.False(t, valid, "signature should not be valid from software public key")

	valid, err = pkcs11CSP.Verify(pkcs11Key, signature, digest, nil)
	require.Error(t, err)
	require.False(t, valid, "Verify should not handle a pkcs11 key")
}

func TestECDSAVerify(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)
	pk, err := k.PublicKey()
	require.NoError(t, err)

	digest, err := csp.Hash([]byte("Hello, World."), &bccsp.SHAOpts{})
	require.NoError(t, err)
	otherDigest, err := csp.Hash([]byte("Bye, World."), &bccsp.SHAOpts{})
	require.NoError(t, err)

	signature, err := csp.Sign(k, digest, nil)
	require.NoError(t, err)

	tests := map[string]bool{
		"WithSoftVerify":    true,
		"WithoutSoftVerify": false,
	}

	pkcs11PublicKey := pk.(*ecdsaPublicKey)

	for name, softVerify := range tests {
		t.Run(name, func(t *testing.T) {
			opts := defaultOptions()
			opts.SoftwareVerify = softVerify
			csp, cleanup := newProvider(t, opts)
			defer cleanup()

			valid, err := csp.verifyECDSA(*pkcs11PublicKey, signature, digest)
			require.NoError(t, err)
			require.True(t, valid, "signature should be valid from public key")

			valid, err = csp.verifyECDSA(*pkcs11PublicKey, signature, otherDigest)
			require.NoError(t, err)
			require.False(t, valid, "signature should not be valid from public key")
		})
	}

	t.Run("MissingKey", func(t *testing.T) {
		_, err := csp.Verify(nil, signature, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid Key. It must not be nil")
	})

	t.Run("MissingSignature", func(t *testing.T) {
		_, err := csp.Verify(pk, nil, digest, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid signature. Cannot be empty")
	})

	t.Run("MissingDigest", func(t *testing.T) {
		_, err = csp.Verify(pk, signature, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid digest. Cannot be empty")
	})
}

func TestECDSALowS(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	require.NoError(t, err)

	digest, err := csp.Hash([]byte("Hello World"), &bccsp.SHAOpts{})
	require.NoError(t, err)

	// Ensure that signature with low-S are generated
	t.Run("GeneratesLowS", func(t *testing.T) {
		signature, err := csp.Sign(k, digest, nil)
		require.NoError(t, err)

		_, S, err := utils.UnmarshalECDSASignature(signature)
		require.NoError(t, err)

		if S.Cmp(utils.GetCurveHalfOrdersAt(k.(*ecdsaPrivateKey).pub.pub.Curve)) >= 0 {
			t.Fatal("Invalid signature. It must have low-S")
		}

		pk, err := k.PublicKey()
		require.NoError(t, err)

		valid, err := csp.verifyECDSA(*(pk.(*ecdsaPublicKey)), signature, digest)
		require.NoError(t, err)
		require.True(t, valid, "signature should be valid")
	})

	// Ensure that signature with high-S are rejected.
	t.Run("RejectsHighS", func(t *testing.T) {
		for {
			R, S, err := csp.signP11ECDSA(k.SKI(), digest)
			require.NoError(t, err)
			if S.Cmp(utils.GetCurveHalfOrdersAt(k.(*ecdsaPrivateKey).pub.pub.Curve)) > 0 {
				sig, err := utils.MarshalECDSASignature(R, S)
				require.NoError(t, err)

				valid, err := csp.Verify(k, sig, digest, nil)
				require.Error(t, err, "verification must fail for a signature with high-S")
				require.False(t, valid, "signature must not be valid with high-S")
				return
			}
		}
	})
}

func TestInitialize(t *testing.T) {
	// Setup PKCS11 library and provide initial set of values
	lib, pin, label := FindPKCS11Lib()

	t.Run("MissingLibrary", func(t *testing.T) {
		_, err := (&Provider{}).initialize(PKCS11Opts{Library: "", Pin: pin, Label: label})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pkcs11: library path not provided")
	})

	t.Run("BadLibraryPath", func(t *testing.T) {
		_, err := (&Provider{}).initialize(PKCS11Opts{Library: "badLib", Pin: pin, Label: label})
		require.Error(t, err)
		require.Contains(t, err.Error(), "pkcs11: instantiation failed for badLib")
	})

	t.Run("BadLabel", func(t *testing.T) {
		_, err := (&Provider{}).initialize(PKCS11Opts{Library: lib, Pin: pin, Label: "badLabel"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "could not find token with label")
	})

	t.Run("MissingPin", func(t *testing.T) {
		_, err := (&Provider{}).initialize(PKCS11Opts{Library: lib, Pin: "", Label: label})
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

func TestCurveForSecurityLevel(t *testing.T) {
	tests := map[int]struct {
		expectedErr string
		curve       asn1.ObjectIdentifier
	}{
		256: {curve: oidNamedCurveP256},
		384: {curve: oidNamedCurveP384},
		512: {expectedErr: "Security level not supported [512]"},
	}

	for level, tt := range tests {
		t.Run(strconv.Itoa(level), func(t *testing.T) {
			curve, err := curveForSecurityLevel(level)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.curve, curve)
		})
	}
}

func TestPKCS11GetSession(t *testing.T) {
	opts := defaultOptions()
	opts.sessionCacheSize = 5
	csp, cleanup := newProvider(t, opts)
	defer cleanup()

	sessionCacheSize := opts.sessionCacheSize
	var sessions []pkcs11.SessionHandle
	for i := 0; i < 3*sessionCacheSize; i++ {
		session, err := csp.getSession()
		require.NoError(t, err)
		sessions = append(sessions, session)
	}

	// Return all sessions, should leave sessionCacheSize cached
	for _, session := range sessions {
		csp.returnSession(session)
	}

	// Should be able to get sessionCacheSize cached sessions
	sessions = nil
	for i := 0; i < sessionCacheSize; i++ {
		session, err := csp.getSession()
		require.NoError(t, err)
		sessions = append(sessions, session)
	}

	// Cleanup
	for _, session := range sessions {
		csp.returnSession(session)
	}
}

func TestSessionHandleCaching(t *testing.T) {
	verifyHandleCache := func(t *testing.T, csp *Provider, sess pkcs11.SessionHandle, k bccsp.Key) {
		pubHandle, err := csp.findKeyPairFromSKI(sess, k.SKI(), publicKeyType)
		require.NoError(t, err)
		h, ok := csp.cachedHandle(publicKeyType, k.SKI())
		require.True(t, ok)
		require.Equal(t, h, pubHandle)

		privHandle, err := csp.findKeyPairFromSKI(sess, k.SKI(), privateKeyType)
		require.NoError(t, err)
		h, ok = csp.cachedHandle(privateKeyType, k.SKI())
		require.True(t, ok)
		require.Equal(t, h, privHandle)
	}

	t.Run("SessionCacheDisabled", func(t *testing.T) {
		opts := defaultOptions()
		opts.sessionCacheSize = -1

		csp, cleanup := newProvider(t, opts)
		defer cleanup()

		require.Nil(t, csp.sessPool, "sessPool channel should be nil")
		require.Empty(t, csp.sessions, "sessions set should be empty")
		require.Empty(t, csp.handleCache, "handleCache should be empty")

		sess1, err := csp.getSession()
		require.NoError(t, err)
		require.Len(t, csp.sessions, 1, "expected one open session")

		sess2, err := csp.getSession()
		require.NoError(t, err)
		require.Len(t, csp.sessions, 2, "expected two open sessions")

		// Generate a key
		k, err := csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
		require.NoError(t, err)
		verifyHandleCache(t, csp, sess1, k)
		require.Len(t, csp.handleCache, 2, "expected two handles in handle cache")

		csp.returnSession(sess1)
		require.Len(t, csp.sessions, 1, "expected one open session")
		verifyHandleCache(t, csp, sess1, k)
		require.Len(t, csp.handleCache, 2, "expected two handles in handle cache")

		csp.returnSession(sess2)
		require.Empty(t, csp.sessions, "expected sessions to be empty")
		require.Empty(t, csp.handleCache, "expected handles to be cleared")
	})

	t.Run("SessionCacheEnabled", func(t *testing.T) {
		opts := defaultOptions()
		opts.sessionCacheSize = 1

		csp, cleanup := newProvider(t, opts)
		defer cleanup()

		require.NotNil(t, csp.sessPool, "sessPool channel should not be nil")
		require.Equal(t, 1, cap(csp.sessPool))
		require.Len(t, csp.sessions, 1, "sessions should contain login session")
		require.Len(t, csp.sessPool, 1, "sessionPool should hold login session")
		require.Empty(t, csp.handleCache, "handleCache should be empty")

		sess1, err := csp.getSession()
		require.NoError(t, err)
		require.Len(t, csp.sessions, 1, "expected one open session (sess1 from login)")
		require.Len(t, csp.sessPool, 0, "sessionPool should be empty")

		sess2, err := csp.getSession()
		require.NoError(t, err)
		require.Len(t, csp.sessions, 2, "expected two open sessions (sess1 and sess2)")
		require.Len(t, csp.sessPool, 0, "sessionPool should be empty")

		// Generate a key
		k, err := csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
		require.NoError(t, err)
		verifyHandleCache(t, csp, sess1, k)
		require.Len(t, csp.handleCache, 2, "expected two handles in handle cache")

		csp.returnSession(sess1)
		require.Len(t, csp.sessions, 2, "expected two open sessions (sess2 in-use, sess1 cached)")
		require.Len(t, csp.sessPool, 1, "sessionPool should have one handle (sess1)")
		verifyHandleCache(t, csp, sess1, k)
		require.Len(t, csp.handleCache, 2, "expected two handles in handle cache")

		csp.returnSession(sess2)
		require.Len(t, csp.sessions, 1, "expected one cached session (sess1)")
		require.Len(t, csp.sessPool, 1, "sessionPool should have one handle (sess1)")
		require.Len(t, csp.handleCache, 2, "expected two handles in handle cache")

		_, err = csp.getSession()
		require.NoError(t, err)
		require.Len(t, csp.sessions, 1, "expected one open session (sess1)")
		require.Len(t, csp.sessPool, 0, "sessionPool should be empty")
		require.Len(t, csp.handleCache, 2, "expected two handles in handle cache")
	})
}

func TestKeyCache(t *testing.T) {
	opts := defaultOptions()
	opts.sessionCacheSize = 1
	csp, cleanup := newProvider(t, opts)
	defer cleanup()

	require.Empty(t, csp.keyCache)

	_, err := csp.GetKey([]byte("nonsense-key"))
	require.Error(t, err) // message comes from software keystore
	require.Empty(t, csp.keyCache)

	k, err := csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	require.NoError(t, err)
	_, ok := csp.cachedKey(k.SKI())
	require.False(t, ok, "created keys are not (currently) cached")

	key, err := csp.GetKey(k.SKI())
	require.NoError(t, err)
	cached, ok := csp.cachedKey(k.SKI())
	require.True(t, ok, "key should be cached")
	require.Same(t, key, cached, "key from cache should be what was found")

	// Retrieve last session
	sess, err := csp.getSession()
	require.NoError(t, err)
	require.Empty(t, csp.sessPool, "sessionPool should be empty")

	// Force caches to be cleared by closing last session
	csp.closeSession(sess)
	require.Empty(t, csp.sessions, "sessions should be empty")
	require.Empty(t, csp.keyCache, "key cache should be empty")

	_, ok = csp.cachedKey(k.SKI())
	require.False(t, ok, "key should not be in cache")
}

// This helps verify that we're delegating to the software provider.
// This is not intended to test the software provider implementation.
func TestDelegation(t *testing.T) {
	csp, cleanup := newProvider(t, defaultOptions())
	defer cleanup()

	k, err := csp.KeyGen(&bccsp.AES256KeyGenOpts{})
	require.NoError(t, err)

	t.Run("KeyGen", func(t *testing.T) {
		k, err := csp.KeyGen(&bccsp.AES256KeyGenOpts{})
		require.NoError(t, err)
		require.True(t, k.Private())
		require.True(t, k.Symmetric())
	})

	t.Run("KeyDeriv", func(t *testing.T) {
		k, err := csp.KeyDeriv(k, &bccsp.HMACDeriveKeyOpts{Arg: []byte{1}})
		require.NoError(t, err)
		require.True(t, k.Private())
	})

	t.Run("KeyImport", func(t *testing.T) {
		raw := make([]byte, 32)
		_, err := rand.Read(raw)
		require.NoError(t, err)

		k, err := csp.KeyImport(raw, &bccsp.AES256ImportKeyOpts{})
		require.NoError(t, err)
		require.True(t, k.Private())
	})

	t.Run("GetKey", func(t *testing.T) {
		k, err := csp.GetKey(k.SKI())
		require.NoError(t, err)
		require.True(t, k.Private())
	})

	t.Run("Hash", func(t *testing.T) {
		digest, err := csp.Hash([]byte("message"), &bccsp.SHA3_384Opts{})
		require.NoError(t, err)
		require.NotEmpty(t, digest)
	})

	t.Run("GetHash", func(t *testing.T) {
		h, err := csp.GetHash(&bccsp.SHA256Opts{})
		require.NoError(t, err)
		require.Equal(t, sha256.New(), h)
	})

	t.Run("Sign", func(t *testing.T) {
		_, err := csp.Sign(k, []byte("message"), nil)
		require.EqualError(t, err, "Unsupported 'SignKey' provided [*sw.aesPrivateKey]")
	})

	t.Run("Verify", func(t *testing.T) {
		_, err := csp.Verify(k, []byte("signature"), []byte("digest"), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Unsupported 'VerifyKey' provided")
	})

	t.Run("EncryptDecrypt", func(t *testing.T) {
		msg := []byte("message")
		ct, err := csp.Encrypt(k, msg, &bccsp.AESCBCPKCS7ModeOpts{})
		require.NoError(t, err)

		pt, err := csp.Decrypt(k, ct, &bccsp.AESCBCPKCS7ModeOpts{})
		require.NoError(t, err)
		require.Equal(t, msg, pt)
	})
}

func TestHandleSessionReturn(t *testing.T) {
	opts := defaultOptions()
	opts.sessionCacheSize = 5
	csp, cleanup := newProvider(t, opts)
	defer cleanup()

	// Retrieve and destroy default session created during initialization
	session, err := csp.getSession()
	require.NoError(t, err)
	csp.closeSession(session)

	// Verify session pool is empty and place invalid session in pool
	require.Empty(t, csp.sessPool, "sessionPool should be empty")
	csp.returnSession(pkcs11.SessionHandle(^uint(0)))

	// Attempt to generate key with invalid session
	_, err = csp.KeyGen(&bccsp.ECDSAP256KeyGenOpts{Temporary: false})
	require.EqualError(t, err, "Failed generating ECDSA P256 key: P11: keypair generate failed [pkcs11: 0xB3: CKR_SESSION_HANDLE_INVALID]")
	require.Empty(t, csp.sessPool, "sessionPool should be empty")
}
