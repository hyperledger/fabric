// Package ecdh encrypts and decrypts data using elliptic curve keys. Data
// is encrypted with AES-128-CBC with HMAC-SHA1 message tags using
// ECDHE to generate a shared key. The P256 curve is chosen in
// keeping with the use of AES-128 for encryption.
package ecdh

import (
	"crypto/aes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"errors"

	"github.com/cloudflare/redoctober/padding"
	"github.com/cloudflare/redoctober/symcrypt"
)

var Curve = elliptic.P256

// Encrypt secures and authenticates its input using the public key
// using ECDHE with AES-128-CBC-HMAC-SHA1.
func Encrypt(pub *ecdsa.PublicKey, in []byte) (out []byte, err error) {
	ephemeral, err := ecdsa.GenerateKey(Curve(), rand.Reader)
	if err != nil {
		return
	}
	x, _ := pub.Curve.ScalarMult(pub.X, pub.Y, ephemeral.D.Bytes())
	if x == nil {
		return nil, errors.New("Failed to generate encryption key")
	}
	shared := sha256.Sum256(x.Bytes())
	iv, err := symcrypt.MakeRandom(16)
	if err != nil {
		return
	}

	paddedIn := padding.AddPadding(in)
	ct, err := symcrypt.EncryptCBC(paddedIn, iv, shared[:16])
	if err != nil {
		return
	}

	ephPub := elliptic.Marshal(pub.Curve, ephemeral.PublicKey.X, ephemeral.PublicKey.Y)
	out = make([]byte, 1+len(ephPub)+16)
	out[0] = byte(len(ephPub))
	copy(out[1:], ephPub)
	copy(out[1+len(ephPub):], iv)
	out = append(out, ct...)

	h := hmac.New(sha1.New, shared[16:])
	h.Write(iv)
	h.Write(ct)
	out = h.Sum(out)
	return
}

// Decrypt authenticates and recovers the original message from
// its input using the private key and the ephemeral key included in
// the message.
func Decrypt(priv *ecdsa.PrivateKey, in []byte) (out []byte, err error) {
	ephLen := int(in[0])
	ephPub := in[1 : 1+ephLen]
	ct := in[1+ephLen:]
	if len(ct) < (sha1.Size + aes.BlockSize) {
		return nil, errors.New("Invalid ciphertext")
	}

	x, y := elliptic.Unmarshal(Curve(), ephPub)
	ok := Curve().IsOnCurve(x, y) // Rejects the identity point too.
	if x == nil || !ok {
		return nil, errors.New("Invalid public key")
	}

	x, _ = priv.Curve.ScalarMult(x, y, priv.D.Bytes())
	if x == nil {
		return nil, errors.New("Failed to generate encryption key")
	}
	shared := sha256.Sum256(x.Bytes())

	tagStart := len(ct) - sha1.Size
	h := hmac.New(sha1.New, shared[16:])
	h.Write(ct[:tagStart])
	mac := h.Sum(nil)
	if !hmac.Equal(mac, ct[tagStart:]) {
		return nil, errors.New("Invalid MAC")
	}

	paddedOut, err := symcrypt.DecryptCBC(ct[aes.BlockSize:tagStart], ct[:aes.BlockSize], shared[:16])
	if err != nil {
		return
	}
	out, err = padding.RemovePadding(paddedOut)
	return
}
