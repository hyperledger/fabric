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
)

func TestPKCS11GetSession(t *testing.T) {
	if !enablePKCS11tests {
		t.SkipNow()
	}

	session := getSession()
	defer returnSession(session)
}

func TestPKCS11ECKeySignVerify(t *testing.T) {
	if !enablePKCS11tests {
		t.SkipNow()
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

	key, pubKey, err := generateECKey(oid, true)
	if err != nil {
		t.Fatal("Failed generating Key [%s]", err)
	}

	R, S, err := signECDSA(key, hash1)

	if err != nil {
		t.Fatal("Failed signing message [%s]", err)
	}

	pass, err := verifyECDSA(key, hash1, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatal("Error verifying message 1 [%s]", err)
	}
	if pass == false {
		t.Fatal("Signature should match!")
	}

	pass = ecdsa.Verify(pubKey, hash1, R, S)
	if pass == false {
		t.Fatal("Signature should match with software verification!")
	}

	pass, err = verifyECDSA(key, hash2, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatal("Error verifying message 2 [%s]", err)
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
	if !enablePKCS11tests {
		t.SkipNow()
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
	ski, err := importECKey(oid, key.D.Bytes(), ecPt, false, isPrivateKey)
	if err != nil {
		t.Fatalf("Failed getting importing EC Public Key [%s]", err)
	}

	R, S, err := signECDSA(ski, hash1)

	if err != nil {
		t.Fatal("Failed signing message [%s]", err)
	}

	pass, err := verifyECDSA(ski, hash1, R, S, currentTestConfig.securityLevel/8)
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

	pass, err = verifyECDSA(ski, hash2, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatal("Error verifying message 2 [%s]", err)
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
	if !enablePKCS11tests {
		t.SkipNow()
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

	key, pubKey, err := generateECKey(oid, false)
	if err != nil {
		t.Fatal("Failed generating Key [%s]", err)
	}

	secret := getSecretValue(key)
	x, y := pubKey.ScalarBaseMult(secret)

	if 0 != x.Cmp(pubKey.X) {
		t.Fatal("X does not match")
	}

	if 0 != y.Cmp(pubKey.Y) {
		t.Fatal("Y does not match")
	}

	ecPt := elliptic.Marshal(pubKey.Curve, x, y)
	key2, err := importECKey(oid, secret, ecPt, false, isPrivateKey)

	R, S, err := signECDSA(key2, hash1)
	if err != nil {
		t.Fatalf("Failed signing message [%s]", err)
	}

	pass, err := verifyECDSA(key2, hash1, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatalf("Error verifying message 1 [%s]", err)
	}
	if pass == false {
		t.Fatal("Signature should match! [1]")
	}

	pass, err = verifyECDSA(key, hash1, R, S, currentTestConfig.securityLevel/8)
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

	pass, err = verifyECDSA(key, hash2, R, S, currentTestConfig.securityLevel/8)
	if err != nil {
		t.Fatal("Error verifying message 3 [%s]", err)
	}

	if pass != false {
		t.Fatal("Signature should not match! [3]")
	}

	pass = ecdsa.Verify(pubKey, hash2, R, S)
	if pass != false {
		t.Fatal("Signature should not match with software verification!")
	}
}
