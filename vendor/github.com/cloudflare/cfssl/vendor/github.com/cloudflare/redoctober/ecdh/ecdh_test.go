package ecdh

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

var testKey *ecdsa.PrivateKey

func TestGenerateKey(t *testing.T) {
	var err error
	testKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCrypt(t *testing.T) {
	message := []byte("One ping only, please.")
	out, err := Encrypt(&testKey.PublicKey, message)
	if err != nil {
		t.Fatalf("%v", err)
	}

	out, err = Decrypt(testKey, out)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if !bytes.Equal(out, message) {
		t.Fatal("Decryption return different plaintext than original message.")
	}
}
