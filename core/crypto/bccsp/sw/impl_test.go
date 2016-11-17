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
package sw

import (
	"bytes"
	"os"
	"testing"

	"crypto"
	"crypto/rsa"

	"crypto/ecdsa"

	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"time"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
	"github.com/hyperledger/fabric/core/crypto/bccsp/signer"
	"github.com/hyperledger/fabric/core/crypto/primitives"
	"github.com/spf13/viper"
)

var (
	swBCCSPInstance bccsp.BCCSP
)

func getBCCSP(t *testing.T) bccsp.BCCSP {
	if swBCCSPInstance == nil {
		primitives.InitSecurityLevel("SHA2", 256)
		viper.Set("security.bccsp.default.keyStorePath", os.TempDir())

		var err error
		swBCCSPInstance, err = New()
		if err != nil {
			t.Fatalf("Failed initializing key store [%s]", err)
		}
	}

	return swBCCSPInstance
}

func TestECDSAKeyGenEphemeral(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
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

func TestECDSAPrivateKeySKI(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	ski := k.SKI()
	if len(ski) == 0 {
		t.Fatal("SKI not valid. Zero length.")
	}
}

func TestECDSAKeyGenNonEphemeral(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	k2, err := csp.GetKey(k.SKI())
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
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

func TestECDSAKeyReRand(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	reRandomizedKey, err := csp.KeyDeriv(k, &bccsp.ECDSAReRandKeyOpts{Temporary: false, Expansion: []byte{1}})
	if err != nil {
		t.Fatalf("Failed re-randomizing ECDSA key [%s]", err)
	}
	if k == nil {
		t.Fatal("Failed re-randomizing ECDSA key. Re-randomized Key must be different from nil")
	}
	if !reRandomizedKey.Private() {
		t.Fatal("Failed re-randomizing ECDSA key. Re-randomized Key should be private")
	}
	if reRandomizedKey.Symmetric() {
		t.Fatal("Failed re-randomizing ECDSA key. Re-randomized Key should be asymmetric")
	}
}

func TestECDSASign(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}
	if len(signature) == 0 {
		t.Fatal("Failed generating ECDSA key. Signature must be different from nil")
	}
}

func TestECDSAVerify(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := csp.Verify(k, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestECDSAKeyDeriv(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	reRandomizedKey, err := csp.KeyDeriv(k, &bccsp.ECDSAReRandKeyOpts{Temporary: false, Expansion: []byte{1}})
	if err != nil {
		t.Fatalf("Failed re-randomizing ECDSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(reRandomizedKey, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := csp.Verify(reRandomizedKey, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestECDSAKeyImportFromExportedKey(t *testing.T) {
	csp := getBCCSP(t)

	// Generate an ECDSA key
	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
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
	pk2, err := csp.KeyImport(pkRaw, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed importing ECDSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing ECDSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := csp.Verify(pk2, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestECDSAKeyImportFromECDSAPublicKey(t *testing.T) {
	csp := getBCCSP(t)

	// Generate an ECDSA key
	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
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

	pub, err := primitives.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to ecdsa.PublicKey [%s]", err)
	}

	// Import the ecdsa.PublicKey
	pk2, err := csp.KeyImport(pkRaw, &bccsp.ECDSAGoPublicKeyImportOpts{Temporary: true, PK: pub.(*ecdsa.PublicKey)})
	if err != nil {
		t.Fatalf("Failed importing ECDSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing ECDSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := csp.Verify(pk2, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestECDSAKeyImportFromECDSAPrivateKey(t *testing.T) {
	csp := getBCCSP(t)

	// Generate an ECDSA key
	key, err := primitives.NewECDSAKey()
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	// Import the ecdsa.PrivateKey
	priv, err := primitives.PrivateKeyToDER(key)
	if err != nil {
		t.Fatalf("Failed converting raw to ecdsa.PrivateKey [%s]", err)
	}

	sk, err := csp.KeyImport(priv, &bccsp.ECDSAPrivateKeyImportOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed importing ECDSA private key [%s]", err)
	}
	if sk == nil {
		t.Fatal("Failed importing ECDSA private key. Return BCCSP key cannot be nil.")
	}

	// Import the ecdsa.PublicKey
	pub, err := primitives.PublicKeyToDER(&key.PublicKey)
	if err != nil {
		t.Fatalf("Failed converting raw to ecdsa.PublicKey [%s]", err)
	}

	pk, err := csp.KeyImport(pub, &bccsp.ECDSAPKIXPublicKeyImportOpts{Temporary: true})

	if err != nil {
		t.Fatalf("Failed importing ECDSA public key [%s]", err)
	}
	if pk == nil {
		t.Fatal("Failed importing ECDSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(sk, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := csp.Verify(pk, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestKeyImportFromX509ECDSAPublicKey(t *testing.T) {
	csp := getBCCSP(t)

	// Generate an ECDSA key
	k, err := csp.KeyGen(&bccsp.ECDSAKeyGenOpts{Temporary: false})
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

		OCSPServer:            []string{"http://ocsp.example.com"},
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

	signer := &signer.CryptoSigner{}
	err = signer.Init(csp, k)
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

	pub, err := primitives.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to ECDSA.PublicKey [%s]", err)
	}

	certRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, signer)
	if err != nil {
		t.Fatalf("Failed generating self-signed certificate [%s]", err)
	}

	cert, err := primitives.DERToX509Certificate(certRaw)
	if err != nil {
		t.Fatalf("Failed generating X509 certificate object from raw [%s]", err)
	}

	// Import the certificate's public key
	pk2, err := csp.KeyImport(nil, &bccsp.X509PublicKeyImportOpts{Temporary: true, Cert: cert})

	if err != nil {
		t.Fatalf("Failed importing ECDSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing ECDSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, nil)
	if err != nil {
		t.Fatalf("Failed generating ECDSA signature [%s]", err)
	}

	valid, err := csp.Verify(pk2, signature, digest, nil)
	if err != nil {
		t.Fatalf("Failed verifying ECDSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying ECDSA signature. Signature not valid.")
	}
}

func TestAESKeyGen(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
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
}

func TestAESEncrypt(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.AESKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	ct, err := csp.Encrypt(k, []byte("Hello World"), &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}
	if len(ct) == 0 {
		t.Fatal("Failed encrypting. Nil ciphertext")
	}
}

func TestAESDecrypt(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.AESKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	msg := []byte("Hello World")

	ct, err := csp.Encrypt(k, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	pt, err := csp.Decrypt(k, ct, bccsp.AESCBCPKCS7ModeOpts{})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.AESKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	hmcaedKey, err := csp.KeyDeriv(k, &bccsp.HMACTruncated256AESDeriveKeyOpts{Temporary: false, Arg: []byte{1}})
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

	ct, err := csp.Encrypt(hmcaedKey, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	pt, err := csp.Decrypt(hmcaedKey, ct, bccsp.AESCBCPKCS7ModeOpts{})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.AESKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	hmcaedKey, err := csp.KeyDeriv(k, &bccsp.HMACDeriveKeyOpts{Temporary: false, Arg: []byte{1}})

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
	csp := getBCCSP(t)

	raw, err := primitives.GenAESKey()
	if err != nil {
		t.Fatalf("Failed generating AES key [%s]", err)
	}

	k, err := csp.KeyImport(raw, &bccsp.AES256ImportKeyOpts{Temporary: true})
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

	ct, err := csp.Encrypt(k, msg, &bccsp.AESCBCPKCS7ModeOpts{})
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	pt, err := csp.Decrypt(k, ct, bccsp.AESCBCPKCS7ModeOpts{})
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
	csp := getBCCSP(t)

	_, err := csp.KeyImport(nil, &bccsp.AES256ImportKeyOpts{Temporary: true})
	if err == nil {
		t.Fatal("Failed importing key. Must fail on importing nil key")
	}

	_, err = csp.KeyImport([]byte{1}, &bccsp.AES256ImportKeyOpts{Temporary: true})
	if err == nil {
		t.Fatal("Failed importing key. Must fail on importing a key with an invalid length")
	}
}

func TestAES256KeyGenSKI(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.AESKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating AES_256 key [%s]", err)
	}

	k2, err := csp.GetKey(k.SKI())
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
	csp := getBCCSP(t)

	for i := 0; i < 100; i++ {
		b, err := primitives.GetRandomBytes(i)
		if err != nil {
			t.Fatalf("Failed getting random bytes [%s]", err)
		}

		h1, err := csp.Hash(b, &bccsp.SHAOpts{})
		if err != nil {
			t.Fatalf("Failed computing SHA [%s]", err)
		}

		h2 := primitives.Hash(b[:])

		if !bytes.Equal(h1, h2) {
			t.Fatalf("Discrempancy found in HASH result [%x], [%x]!=[%x]", b, h1, h2)
		}
	}
}

func TestRSAKeyGenEphemeral(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
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

func TestRSAPrivateKeySKI(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	ski := k.SKI()
	if len(ski) == 0 {
		t.Fatal("SKI not valid. Zero length.")
	}
}

func TestRSAKeyGenNonEphemeral(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	k2, err := csp.GetKey(k.SKI())
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
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
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}
	if len(signature) == 0 {
		t.Fatal("Failed generating RSA key. Signature must be different from nil")
	}
}

func TestRSAVerify(t *testing.T) {
	csp := getBCCSP(t)

	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: true})
	if err != nil {
		t.Fatalf("Failed generating RSA key [%s]", err)
	}

	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}

	valid, err := csp.Verify(k, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}
}

func TestRSAKeyImportFromRSAPublicKey(t *testing.T) {
	csp := getBCCSP(t)

	// Generate an RSA key
	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
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

	pub, err := primitives.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to RSA.PublicKey [%s]", err)
	}

	// Import the RSA.PublicKey
	pk2, err := csp.KeyImport(pkRaw, &bccsp.RSAGoPublicKeyImportOpts{Temporary: true, PK: pub.(*rsa.PublicKey)})
	if err != nil {
		t.Fatalf("Failed importing RSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing RSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}

	valid, err := csp.Verify(pk2, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}
}

func TestKeyImportFromX509RSAPublicKey(t *testing.T) {
	csp := getBCCSP(t)

	// Generate an RSA key
	k, err := csp.KeyGen(&bccsp.RSAKeyGenOpts{Temporary: false})
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

		OCSPServer:            []string{"http://ocsp.example.com"},
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

	signer := &signer.CryptoSigner{}
	err = signer.Init(csp, k)
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

	pub, err := primitives.DERToPublicKey(pkRaw)
	if err != nil {
		t.Fatalf("Failed converting raw to RSA.PublicKey [%s]", err)
	}

	certRaw, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, signer)
	if err != nil {
		t.Fatalf("Failed generating self-signed certificate [%s]", err)
	}

	cert, err := primitives.DERToX509Certificate(certRaw)
	if err != nil {
		t.Fatalf("Failed generating X509 certificate object from raw [%s]", err)
	}

	// Import the certificate's public key
	pk2, err := csp.KeyImport(nil, &bccsp.X509PublicKeyImportOpts{Temporary: true, Cert: cert})

	if err != nil {
		t.Fatalf("Failed importing RSA public key [%s]", err)
	}
	if pk2 == nil {
		t.Fatal("Failed importing RSA public key. Return BCCSP key cannot be nil.")
	}

	// Sign and verify with the imported public key
	msg := []byte("Hello World")

	digest, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	signature, err := csp.Sign(k, digest, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		t.Fatalf("Failed generating RSA signature [%s]", err)
	}

	valid, err := csp.Verify(pk2, signature, digest, &rsa.PSSOptions{SaltLength: 32, Hash: crypto.SHA256})
	if err != nil {
		t.Fatalf("Failed verifying RSA signature [%s]", err)
	}
	if !valid {
		t.Fatal("Failed verifying RSA signature. Signature not valid.")
	}
}

func TestGetHashAndHashCompatibility(t *testing.T) {
	csp := getBCCSP(t)

	msg1 := []byte("abcd")
	msg2 := []byte("efgh")
	msg := []byte("abcdefgh")

	digest1, err := csp.Hash(msg, &bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	digest2, err := csp.Hash(msg, nil)
	if err != nil {
		t.Fatalf("Failed computing HASH [%s]", err)
	}

	if !bytes.Equal(digest1, digest2) {
		t.Fatalf("Different hash computed. [%x][%x]", digest1, digest2)
	}

	h, err := csp.GetHash(nil)
	if err != nil {
		t.Fatalf("Failed getting hash.Hash instance [%s]", err)
	}
	h.Write(msg1)
	h.Write(msg2)
	digest3 := h.Sum(nil)

	h2, err := csp.GetHash(&bccsp.SHAOpts{})
	if err != nil {
		t.Fatalf("Failed getting SHA hash.Hash instance [%s]", err)
	}
	h2.Write(msg1)
	h2.Write(msg2)
	digest4 := h2.Sum(nil)

	if !bytes.Equal(digest3, digest4) {
		t.Fatalf("Different hash computed. [%x][%x]", digest3, digest4)
	}

	if !bytes.Equal(digest1, digest3) {
		t.Fatalf("Different hash computed. [%x][%x]", digest1, digest3)
	}
}
