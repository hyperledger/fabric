// +build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/asn1"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/miekg/pkcs11"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestOIDFromNamedCurve(t *testing.T) {
	// Test for valid OID for P224
	testOID, boolValue := oidFromNamedCurve(elliptic.P224())
	assert.Equal(t, oidNamedCurveP224, testOID, "Did not receive expected OID for elliptic.P224")
	assert.Equal(t, true, boolValue, "Did not receive a true value when acquiring OID for elliptic.P224")

	// Test for valid OID for P256
	testOID, boolValue = oidFromNamedCurve(elliptic.P256())
	assert.Equal(t, oidNamedCurveP256, testOID, "Did not receive expected OID for elliptic.P256")
	assert.Equal(t, true, boolValue, "Did not receive a true value when acquiring OID for elliptic.P256")

	// Test for valid OID for P384
	testOID, boolValue = oidFromNamedCurve(elliptic.P384())
	assert.Equal(t, oidNamedCurveP384, testOID, "Did not receive expected OID for elliptic.P384")
	assert.Equal(t, true, boolValue, "Did not receive a true value when acquiring OID for elliptic.P384")

	// Test for valid OID for P521
	testOID, boolValue = oidFromNamedCurve(elliptic.P521())
	assert.Equal(t, oidNamedCurveP521, testOID, "Did not receive expected OID for elliptic.P521")
	assert.Equal(t, true, boolValue, "Did not receive a true value when acquiring OID for elliptic.P521")

	var testCurve elliptic.Curve
	testOID, _ = oidFromNamedCurve(testCurve)
	if testOID != nil {
		t.Fatal("Expected nil to be returned.")
	}
}

func TestAlternateLabelGeneration(t *testing.T) {
	// We generate a unique Alt ID for the test
	uniqueAltId := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	fakeSKI := []byte("FakeSKI")

	opts := PKCS11Opts{
		HashFamily: currentTestConfig.hashFamily,
		SecLevel:   currentTestConfig.securityLevel,
		SoftVerify: currentTestConfig.softVerify,
		Immutable:  currentTestConfig.immutable,
		AltId:      uniqueAltId,
		Library:    "lib",
		Label:      "ForFabric",
		Pin:        "98765432",
	}

	// Setup PKCS11 library and provide initial set of values
	lib, _, _ := FindPKCS11Lib()
	opts.Library = lib

	// Create temporary BCCSP set with the  label
	testBCCSP, err := New(opts, currentKS)
	require.NoError(t, err)

	testCSP := testBCCSP.(*impl)
	session, err := testCSP.getSession()
	require.NoError(t, err)
	defer testCSP.returnSession(session)

	// Passing fake SKI to ensure that the look up fails if the uniqueAltId is not used
	k, err := testCSP.findKeyPairFromSKI(session, fakeSKI, privateKeyType)
	if err == nil {
		t.Fatalf("Found a key when expected to find none.")
	}

	// Now generate a new EC Key, which should be using the uniqueAltId
	var oid asn1.ObjectIdentifier
	if currentTestConfig.securityLevel == 256 {
		oid = oidNamedCurveP256
	} else if currentTestConfig.securityLevel == 384 {
		oid = oidNamedCurveP384
	}

	_, _, err = testCSP.generateECKey(oid, true)
	if err != nil {
		t.Fatalf("Failed generating Key [%s]", err)
	}

	k, err = testCSP.findKeyPairFromSKI(session, fakeSKI, privateKeyType)
	if err != nil {
		t.Fatalf("Found no key after generating an EC Key based on the AltId.")
	}
	if k == 0 {
		t.Fatalf("No Key returned from the findKeyPairFromSKI call.")
	}
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
	currentBCCSP := currentBCCSP.(*impl)

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

	// Should be able to get sessionCacheSize cached sessions
	sessions = nil
	for i := 0; i < sessionCacheSize; i++ {
		session, err := currentBCCSP.getSession()
		require.NoError(t, err)
		sessions = append(sessions, session)
	}

	// Cleanup
	for _, session := range sessions {
		currentBCCSP.returnSession(session)
	}
}

func TestSessionHandleCaching(t *testing.T) {
	defer func(s int) { sessionCacheSize = s }(sessionCacheSize)

	opts := PKCS11Opts{HashFamily: "SHA2", SecLevel: 256, SoftVerify: false}
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
	opts := PKCS11Opts{HashFamily: "SHA2", SecLevel: 256, SoftVerify: false}
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
	require.Equal(t, key, cached, "key from cache should be what was found")

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
	hash1, _ := currentBCCSP.Hash(msg1, &bccsp.SHAOpts{})
	msg2 := []byte("This is my very unauthentic message")
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
