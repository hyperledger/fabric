package symcrypt

import (
	"bytes"
	"testing"

	"github.com/cloudflare/redoctober/padding"
)

func TestCrypt(t *testing.T) {
	msg := []byte("One ping only, please.")
	padMsg := padding.AddPadding(msg)

	key, err := MakeRandom(16)
	if err != nil {
		t.Fatalf("%v", err)
	}

	iv, err := MakeRandom(16)
	if err != nil {
		t.Fatalf("%v", err)
	}

	out, err := EncryptCBC(padMsg, iv, key)
	if err != nil {
		t.Fatalf("%v", err)
	}

	out, err = DecryptCBC(out, iv, key)
	if err != nil {
		t.Fatalf("%v", err)
	}

	unpadOut, err := padding.RemovePadding(out)
	if err != nil {
		t.Fatalf("%v", err)
	} else if !bytes.Equal(unpadOut, msg) {
		t.Fatal("Decrypted message doesn't match original plaintext.")
	}
}
