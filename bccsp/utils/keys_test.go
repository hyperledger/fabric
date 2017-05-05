/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOidFromNamedCurve(t *testing.T) {

	var (
		oidNamedCurveP224 = asn1.ObjectIdentifier{1, 3, 132, 0, 33}
		oidNamedCurveP256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7}
		oidNamedCurveP384 = asn1.ObjectIdentifier{1, 3, 132, 0, 34}
		oidNamedCurveP521 = asn1.ObjectIdentifier{1, 3, 132, 0, 35}
	)

	type result struct {
		oid asn1.ObjectIdentifier
		ok  bool
	}

	var tests = []struct {
		name     string
		curve    elliptic.Curve
		expected result
	}{
		{
			name:  "P224",
			curve: elliptic.P224(),
			expected: result{
				oid: oidNamedCurveP224,
				ok:  true,
			},
		},
		{
			name:  "P256",
			curve: elliptic.P256(),
			expected: result{
				oid: oidNamedCurveP256,
				ok:  true,
			},
		},
		{
			name:  "P384",
			curve: elliptic.P384(),
			expected: result{
				oid: oidNamedCurveP384,
				ok:  true,
			},
		},
		{
			name:  "P521",
			curve: elliptic.P521(),
			expected: result{
				oid: oidNamedCurveP521,
				ok:  true,
			},
		},
		{
			name:  "T-1000",
			curve: &elliptic.CurveParams{Name: "T-1000"},
			expected: result{
				oid: nil,
				ok:  false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oid, ok := oidFromNamedCurve(test.curve)
			assert.Equal(t, oid, test.expected.oid)
			assert.Equal(t, ok, test.expected.ok)
		})
	}

}

func TestECDSAKeys(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	// Private Key DER format
	der, err := PrivateKeyToDER(key)
	if err != nil {
		t.Fatalf("Failed converting private key to DER [%s]", err)
	}
	keyFromDER, err := DERToPrivateKey(der)
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaKeyFromDer := keyFromDER.(*ecdsa.PrivateKey)
	// TODO: check the curve
	if key.D.Cmp(ecdsaKeyFromDer.D) != 0 {
		t.Fatal("Failed converting DER to private key. Invalid D.")
	}
	if key.X.Cmp(ecdsaKeyFromDer.X) != 0 {
		t.Fatal("Failed converting DER to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaKeyFromDer.Y) != 0 {
		t.Fatal("Failed converting DER to private key. Invalid Y coordinate.")
	}

	// Private Key PEM format
	rawPEM, err := PrivateKeyToPEM(key, nil)
	if err != nil {
		t.Fatalf("Failed converting private key to PEM [%s]", err)
	}
	pemBlock, _ := pem.Decode(rawPEM)
	if pemBlock.Type != "PRIVATE KEY" {
		t.Fatalf("Expected type 'PRIVATE KEY' but found '%s'", pemBlock.Type)
	}
	_, err = x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse PKCS#8 private key [%s]", err)
	}
	keyFromPEM, err := PEMtoPrivateKey(rawPEM, nil)
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaKeyFromPEM := keyFromPEM.(*ecdsa.PrivateKey)
	// TODO: check the curve
	if key.D.Cmp(ecdsaKeyFromPEM.D) != 0 {
		t.Fatal("Failed converting PEM to private key. Invalid D.")
	}
	if key.X.Cmp(ecdsaKeyFromPEM.X) != 0 {
		t.Fatal("Failed converting PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaKeyFromPEM.Y) != 0 {
		t.Fatal("Failed converting PEM to private key. Invalid Y coordinate.")
	}

	// Nil Private Key <-> PEM
	_, err = PrivateKeyToPEM(nil, nil)
	if err == nil {
		t.Fatal("PublicKeyToPEM should fail on nil")
	}

	_, err = PrivateKeyToPEM((*ecdsa.PrivateKey)(nil), nil)
	if err == nil {
		t.Fatal("PrivateKeyToPEM should fail on nil")
	}

	_, err = PrivateKeyToPEM((*rsa.PrivateKey)(nil), nil)
	if err == nil {
		t.Fatal("PrivateKeyToPEM should fail on nil")
	}

	_, err = PEMtoPrivateKey(nil, nil)
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on nil")
	}

	_, err = PEMtoPrivateKey([]byte{0, 1, 3, 4}, nil)
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail invalid PEM")
	}

	_, err = DERToPrivateKey(nil)
	if err == nil {
		t.Fatal("DERToPrivateKey should fail on nil")
	}

	_, err = DERToPrivateKey([]byte{0, 1, 3, 4})
	if err == nil {
		t.Fatal("DERToPrivateKey should fail on invalid DER")
	}

	_, err = PrivateKeyToDER(nil)
	if err == nil {
		t.Fatal("DERToPrivateKey should fail on nil")
	}

	// Private Key Encrypted PEM format
	encPEM, err := PrivateKeyToPEM(key, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting private key to encrypted PEM [%s]", err)
	}
	_, err = PEMtoPrivateKey(encPEM, nil)
	assert.Error(t, err)
	encKeyFromPEM, err := PEMtoPrivateKey(encPEM, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaKeyFromEncPEM := encKeyFromPEM.(*ecdsa.PrivateKey)
	// TODO: check the curve
	if key.D.Cmp(ecdsaKeyFromEncPEM.D) != 0 {
		t.Fatal("Failed converting encrypted PEM to private key. Invalid D.")
	}
	if key.X.Cmp(ecdsaKeyFromEncPEM.X) != 0 {
		t.Fatal("Failed converting encrypted PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaKeyFromEncPEM.Y) != 0 {
		t.Fatal("Failed converting encrypted PEM to private key. Invalid Y coordinate.")
	}

	// Public Key PEM format
	rawPEM, err = PublicKeyToPEM(&key.PublicKey, nil)
	if err != nil {
		t.Fatalf("Failed converting public key to PEM [%s]", err)
	}
	pemBlock, _ = pem.Decode(rawPEM)
	if pemBlock.Type != "PUBLIC KEY" {
		t.Fatalf("Expected type 'PUBLIC KEY' but found '%s'", pemBlock.Type)
	}
	keyFromPEM, err = PEMtoPublicKey(rawPEM, nil)
	if err != nil {
		t.Fatalf("Failed converting DER to public key [%s]", err)
	}
	ecdsaPkFromPEM := keyFromPEM.(*ecdsa.PublicKey)
	// TODO: check the curve
	if key.X.Cmp(ecdsaPkFromPEM.X) != 0 {
		t.Fatal("Failed converting PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaPkFromPEM.Y) != 0 {
		t.Fatal("Failed converting PEM to private key. Invalid Y coordinate.")
	}

	// Nil Public Key <-> PEM
	_, err = PublicKeyToPEM(nil, nil)
	if err == nil {
		t.Fatal("PublicKeyToPEM should fail on nil")
	}

	_, err = PEMtoPublicKey(nil, nil)
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on nil")
	}

	_, err = PEMtoPublicKey([]byte{0, 1, 3, 4}, nil)
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on invalid PEM")
	}

	// Public Key Encrypted PEM format
	encPEM, err = PublicKeyToPEM(&key.PublicKey, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting private key to encrypted PEM [%s]", err)
	}
	_, err = PEMtoPublicKey(encPEM, nil)
	assert.Error(t, err)
	pkFromEncPEM, err := PEMtoPublicKey(encPEM, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting DER to private key [%s]", err)
	}
	ecdsaPkFromEncPEM := pkFromEncPEM.(*ecdsa.PublicKey)
	// TODO: check the curve
	if key.X.Cmp(ecdsaPkFromEncPEM.X) != 0 {
		t.Fatal("Failed converting encrypted PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaPkFromEncPEM.Y) != 0 {
		t.Fatal("Failed converting encrypted PEM to private key. Invalid Y coordinate.")
	}

	_, err = PEMtoPublicKey(encPEM, []byte("passw"))
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on wrong password")
	}

	_, err = PEMtoPublicKey(encPEM, []byte("passw"))
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on nil password")
	}

	_, err = PEMtoPublicKey(nil, []byte("passwd"))
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on nil PEM")
	}

	_, err = PEMtoPublicKey([]byte{0, 1, 3, 4}, []byte("passwd"))
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on invalid PEM")
	}

	_, err = PEMtoPublicKey(nil, []byte("passw"))
	if err == nil {
		t.Fatal("PEMtoPublicKey should fail on nil PEM and wrong password")
	}

	// Public Key DER format
	der, err = PublicKeyToDER(&key.PublicKey)
	assert.NoError(t, err)
	keyFromDER, err = DERToPublicKey(der)
	assert.NoError(t, err)
	ecdsaPkFromPEM = keyFromDER.(*ecdsa.PublicKey)
	// TODO: check the curve
	if key.X.Cmp(ecdsaPkFromPEM.X) != 0 {
		t.Fatal("Failed converting PEM to private key. Invalid X coordinate.")
	}
	if key.Y.Cmp(ecdsaPkFromPEM.Y) != 0 {
		t.Fatal("Failed converting PEM to private key. Invalid Y coordinate.")
	}
}

func TestAESKey(t *testing.T) {
	k := []byte{0, 1, 2, 3, 4, 5}
	pem := AEStoPEM(k)

	k2, err := PEMtoAES(pem, nil)
	assert.NoError(t, err)
	assert.Equal(t, k, k2)

	pem, err = AEStoEncryptedPEM(k, k)
	assert.NoError(t, err)

	k2, err = PEMtoAES(pem, k)
	assert.NoError(t, err)
	assert.Equal(t, k, k2)

	_, err = PEMtoAES(pem, nil)
	assert.Error(t, err)

	_, err = AEStoEncryptedPEM(k, nil)
	assert.NoError(t, err)

	k2, err = PEMtoAES(pem, k)
	assert.NoError(t, err)
	assert.Equal(t, k, k2)
}

func TestDERToPublicKey(t *testing.T) {
	_, err := DERToPublicKey(nil)
	assert.Error(t, err)
}

func TestNil(t *testing.T) {
	_, err := PrivateKeyToEncryptedPEM(nil, nil)
	assert.Error(t, err)

	_, err = PrivateKeyToEncryptedPEM((*ecdsa.PrivateKey)(nil), nil)
	assert.Error(t, err)

	_, err = PrivateKeyToEncryptedPEM("Hello World", nil)
	assert.Error(t, err)

	_, err = PEMtoAES(nil, nil)
	assert.Error(t, err)

	_, err = AEStoEncryptedPEM(nil, nil)
	assert.Error(t, err)

	_, err = PublicKeyToPEM(nil, nil)
	assert.Error(t, err)
	_, err = PublicKeyToPEM((*ecdsa.PublicKey)(nil), nil)
	assert.Error(t, err)
	_, err = PublicKeyToPEM((*rsa.PublicKey)(nil), nil)
	assert.Error(t, err)
	_, err = PublicKeyToPEM(nil, []byte("hello world"))
	assert.Error(t, err)

	_, err = PublicKeyToPEM("hello world", nil)
	assert.Error(t, err)
	_, err = PublicKeyToPEM("hello world", []byte("hello world"))
	assert.Error(t, err)

	_, err = PublicKeyToDER(nil)
	assert.Error(t, err)
	_, err = PublicKeyToDER((*ecdsa.PublicKey)(nil))
	assert.Error(t, err)
	_, err = PublicKeyToDER((*rsa.PublicKey)(nil))
	assert.Error(t, err)
	_, err = PublicKeyToDER("hello world")
	assert.Error(t, err)

	_, err = PublicKeyToEncryptedPEM(nil, nil)
	assert.Error(t, err)
	_, err = PublicKeyToEncryptedPEM((*ecdsa.PublicKey)(nil), nil)
	assert.Error(t, err)
	_, err = PublicKeyToEncryptedPEM("hello world", nil)
	assert.Error(t, err)
	_, err = PublicKeyToEncryptedPEM("hello world", []byte("Hello world"))
	assert.Error(t, err)

}

func TestPrivateKeyToPEM(t *testing.T) {
	_, err := PrivateKeyToPEM(nil, nil)
	assert.Error(t, err)

	_, err = PrivateKeyToPEM("hello world", nil)
	assert.Error(t, err)

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	assert.NoError(t, err)
	pem, err := PrivateKeyToPEM(key, nil)
	assert.NoError(t, err)
	assert.NotNil(t, pem)
	key2, err := PEMtoPrivateKey(pem, nil)
	assert.NoError(t, err)
	assert.NotNil(t, key2)
	assert.Equal(t, key.D, key2.(*rsa.PrivateKey).D)

	pem, err = PublicKeyToPEM(&key.PublicKey, nil)
	assert.NoError(t, err)
	assert.NotNil(t, pem)
	key3, err := PEMtoPublicKey(pem, nil)
	assert.NoError(t, err)
	assert.NotNil(t, key2)
	assert.Equal(t, key.PublicKey.E, key3.(*rsa.PublicKey).E)
	assert.Equal(t, key.PublicKey.N, key3.(*rsa.PublicKey).N)
}
