package sw

import (
	"bytes"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
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

	valid, err := csp.Verify(k, signature, digest)
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

	valid, err := csp.Verify(reRandomizedKey, signature, digest)
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
