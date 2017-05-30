/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/asn1"
	"encoding/hex"
	"testing"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/miekg/pkcs11"
	"github.com/stretchr/testify/assert"
)

func TestKeyGenFailures(t *testing.T) {
	var testOpts bccsp.KeyGenOpts
	ki := currentBCCSP
	_, err := ki.KeyGen(testOpts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid Opts parameter. It must not be nil.")
}

func TestLoadLib(t *testing.T) {
	// Setup PKCS11 library and provide initial set of values
	lib, pin, label := FindPKCS11Lib()

	// Test for no specified PKCS11 library
	_, _, _, err := loadLib("", pin, label)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "No PKCS11 library default")

	// Test for invalid PKCS11 library
	_, _, _, err = loadLib("badLib", pin, label)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Instantiate failed")

	// Test for invalid label
	_, _, _, err = loadLib(lib, pin, "badLabel")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Could not find token with label")

	// Test for no pin
	_, _, _, err = loadLib(lib, "", label)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "No PIN set")
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
	testOID, boolValue = oidFromNamedCurve(testCurve)
	if testOID != nil {
		t.Fatal("Expected nil to be returned.")
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
	var sessions []pkcs11.SessionHandle
	for i := 0; i < 3*sessionCacheSize; i++ {
		sessions = append(sessions, currentBCCSP.(*impl).getSession())
	}

	// Return all sessions, should leave sessionCacheSize cached
	for _, session := range sessions {
		currentBCCSP.(*impl).returnSession(session)
	}
	sessions = nil

	// Lets break OpenSession, so non-cached session cannot be opened
	oldSlot := currentBCCSP.(*impl).slot
	currentBCCSP.(*impl).slot = ^uint(0)

	// Should be able to get sessionCacheSize cached sessions
	for i := 0; i < sessionCacheSize; i++ {
		sessions = append(sessions, currentBCCSP.(*impl).getSession())
	}

	// This one should fail
	assert.Panics(t, func() {
		currentBCCSP.(*impl).getSession()
	}, "Should not been able to create another session")

	// Cleanup
	for _, session := range sessions {
		currentBCCSP.(*impl).returnSession(session)
	}
	currentBCCSP.(*impl).slot = oldSlot
}

func TestPKCS11ECKeySignVerify(t *testing.T) {
	if currentBCCSP.(*impl).noPrivImport {
		t.Skip("Key import turned off. Skipping Derivation tests as they currently require Key Import.")
	}

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

func TestPKCS11ECKeyImportSignVerify(t *testing.T) {
	if currentBCCSP.(*impl).noPrivImport {
		t.Skip("Key import turned off. Skipping Derivation tests as they currently require Key Import.")
	}

	msg1 := []byte("This is my very authentic message")
	msg2 := []byte("This is my very unauthentic message")
	hash1, _ := currentBCCSP.Hash(msg1, &bccsp.SHAOpts{})
	hash2, err := currentBCCSP.Hash(msg2, &bccsp.SHAOpts{})

	var oid asn1.ObjectIdentifier
	var key *ecdsa.PrivateKey
	if currentTestConfig.securityLevel == 256 {
		oid = oidNamedCurveP256
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("Failed generating ECDSA key [%s]", err)
		}
	} else if currentTestConfig.securityLevel == 384 {
		oid = oidNamedCurveP384
		key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			t.Fatalf("Failed generating ECDSA key [%s]", err)
		}
	}

	ecPt := elliptic.Marshal(key.Curve, key.X, key.Y)
	ski, err := currentBCCSP.(*impl).importECKey(oid, key.D.Bytes(), ecPt, false, privateKeyFlag)
	if err != nil {
		t.Fatalf("Failed getting importing EC Public Key [%s]", err)
	}

	R, S, err := currentBCCSP.(*impl).signP11ECDSA(ski, hash1)

	if err != nil {
		t.Fatalf("Failed signing message [%s]", err)
	}

	pass, err := currentBCCSP.(*impl).verifyP11ECDSA(ski, hash1, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 1 [%s]\n%s\n\n%s", err, hex.Dump(R.Bytes()), hex.Dump(S.Bytes()))
	}
	if pass == false {
		t.Fatalf("Signature should match!\n%s\n\n%s", hex.Dump(R.Bytes()), hex.Dump(S.Bytes()))
	}

	pass = ecdsa.Verify(&key.PublicKey, hash1, R, S)
	if pass == false {
		t.Fatal("Signature should match with software verification!")
	}

	pass, err = currentBCCSP.(*impl).verifyP11ECDSA(ski, hash2, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 2 [%s]", err)
	}

	if pass != false {
		t.Fatal("Signature should not match!")
	}

	pass = ecdsa.Verify(&key.PublicKey, hash2, R, S)
	if pass != false {
		t.Fatal("Signature should not match with software verification!")
	}
}

func TestPKCS11ECKeyExport(t *testing.T) {
	if currentBCCSP.(*impl).noPrivImport {
		t.Skip("Key import turned off. Skipping Derivation tests as they currently require Key Import.")
	}

	msg1 := []byte("This is my very authentic message")
	msg2 := []byte("This is my very unauthentic message")
	hash1, _ := currentBCCSP.Hash(msg1, &bccsp.SHAOpts{})
	hash2, err := currentBCCSP.Hash(msg2, &bccsp.SHAOpts{})

	var oid asn1.ObjectIdentifier
	if currentTestConfig.securityLevel == 256 {
		oid = oidNamedCurveP256
	} else if currentTestConfig.securityLevel == 384 {
		oid = oidNamedCurveP384
	}

	key, pubKey, err := currentBCCSP.(*impl).generateECKey(oid, false)
	if err != nil {
		t.Fatalf("Failed generating Key [%s]", err)
	}

	secret := currentBCCSP.(*impl).getSecretValue(key)
	x, y := pubKey.ScalarBaseMult(secret)

	if 0 != x.Cmp(pubKey.X) {
		t.Fatal("X does not match")
	}

	if 0 != y.Cmp(pubKey.Y) {
		t.Fatal("Y does not match")
	}

	ecPt := elliptic.Marshal(pubKey.Curve, x, y)
	key2, err := currentBCCSP.(*impl).importECKey(oid, secret, ecPt, false, privateKeyFlag)

	R, S, err := currentBCCSP.(*impl).signP11ECDSA(key2, hash1)
	if err != nil {
		t.Fatalf("Failed signing message [%s]", err)
	}

	pass, err := currentBCCSP.(*impl).verifyP11ECDSA(key2, hash1, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 1 [%s]", err)
	}
	if pass == false {
		t.Fatal("Signature should match! [1]")
	}

	pass, err = currentBCCSP.(*impl).verifyP11ECDSA(key, hash1, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 2 [%s]", err)
	}
	if pass == false {
		t.Fatal("Signature should match! [2]")
	}

	pass = ecdsa.Verify(pubKey, hash1, R, S)
	if pass == false {
		t.Fatal("Signature should match with software verification!")
	}

	pass, err = currentBCCSP.(*impl).verifyP11ECDSA(key, hash2, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 3 [%s]", err)
	}

	if pass != false {
		t.Fatal("Signature should not match! [3]")
	}

	pass = ecdsa.Verify(pubKey, hash2, R, S)
	if pass != false {
		t.Fatal("Signature should not match with software verification!")
	}
}
