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

package ecies

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

type TestParameters struct {
	hashFamily    string
	securityLevel int
}

func (t *TestParameters) String() string {
	return t.hashFamily + "-" + string(t.securityLevel)
}

var testParametersSet = []*TestParameters{
	&TestParameters{"SHA3", 256},
	&TestParameters{"SHA3", 384},
	&TestParameters{"SHA2", 256},
	&TestParameters{"SHA2", 384}}

func TestMain(m *testing.M) {
	for _, params := range testParametersSet {
		err := primitives.SetSecurityLevel(params.hashFamily, params.securityLevel)
		if err == nil {
			m.Run()
		} else {
			panic(fmt.Errorf("Failed initiliazing crypto layer at [%s]", params.String()))
		}
	}
	os.Exit(0)
}

func TestSPINewDefaultPrivateKey(t *testing.T) {
	spi := NewSPI()

	if _, err := spi.NewDefaultPrivateKey(rand.Reader); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	if _, err := spi.NewDefaultPrivateKey(nil); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}
}

func TestSPINewPrivateKeyFromCurve(t *testing.T) {
	spi := NewSPI()

	if _, err := spi.NewPrivateKey(rand.Reader, primitives.GetDefaultCurve()); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	if _, err := spi.NewPrivateKey(nil, primitives.GetDefaultCurve()); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	if _, err := spi.NewPrivateKey(nil, nil); err == nil {
		t.Fatalf("Generating key should file with nil params.")
	}
}

func TestSPINewPrivateKeyFromECDSAKey(t *testing.T) {
	spi := NewSPI()

	ecdsaKey, err := ecdsa.GenerateKey(primitives.GetDefaultCurve(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	if _, err := spi.NewPrivateKey(rand.Reader, ecdsaKey); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	if _, err := spi.NewPrivateKey(nil, ecdsaKey); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}
}

func TestSPINewPublicKeyFromECDSAKey(t *testing.T) {
	spi := NewSPI()

	ecdsaKey, err := ecdsa.GenerateKey(primitives.GetDefaultCurve(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed generating ECDSA key [%s]", err)
	}

	if _, err := spi.NewPublicKey(rand.Reader, &ecdsaKey.PublicKey); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	if _, err := spi.NewPublicKey(nil, &ecdsaKey.PublicKey); err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	if _, err := spi.NewPublicKey(nil, nil); err == nil {
		t.Fatalf("Generating key should file with nil params.")
	}
}

func TestSPINewAsymmetricCipherFrom(t *testing.T) {
	spi := NewSPI()

	key, err := spi.NewDefaultPrivateKey(nil)
	if err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	if _, err := spi.NewAsymmetricCipherFromPrivateKey(key); err != nil {
		t.Fatalf("Failed creating AsymCipher from private key [%s]", err)
	}

	if _, err := spi.NewAsymmetricCipherFromPrivateKey(nil); err == nil {
		t.Fatalf("Creating AsymCipher from private key shoud fail with nil key")
	}

	if _, err := spi.NewAsymmetricCipherFromPublicKey(key.GetPublicKey()); err != nil {
		t.Fatalf("Failed creating AsymCipher from public key [%s]", err)
	}

	if _, err := spi.NewAsymmetricCipherFromPublicKey(nil); err == nil {
		t.Fatalf("Creating AsymCipher from public key shoud fail with nil key")
	}
}

func TestSPIEncryption(t *testing.T) {
	spi := NewSPI()

	key, err := spi.NewDefaultPrivateKey(nil)
	if err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	// Encrypt
	aCipher, err := spi.NewAsymmetricCipherFromPublicKey(key.GetPublicKey())
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from public key [%s]", err)
	}
	msg := []byte("Hello World")
	ct, err := aCipher.Process(msg)
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	// Decrypt
	aCipher, err = spi.NewAsymmetricCipherFromPublicKey(key)
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from private key [%s]", err)
	}
	recoveredMsg, err := aCipher.Process(ct)
	if err != nil {
		t.Fatalf("Failed decrypting [%s]", err)
	}
	if !reflect.DeepEqual(msg, recoveredMsg) {
		t.Fatalf("Failed decrypting. Output is different [%x][%x]", msg, recoveredMsg)
	}
}

func TestSPIStressEncryption(t *testing.T) {
	spi := NewSPI()

	key, err := spi.NewDefaultPrivateKey(nil)
	if err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	// Encrypt
	aCipher, err := spi.NewAsymmetricCipherFromPublicKey(key.GetPublicKey())
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from public key [%s]", err)
	}
	_, err = aCipher.Process(nil)
	if err == nil {
		t.Fatalf("Encrypting nil should fail")
	}

}

func TestSPIStressDecryption(t *testing.T) {
	spi := NewSPI()

	key, err := spi.NewDefaultPrivateKey(nil)
	if err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	// Decrypt
	aCipher, err := spi.NewAsymmetricCipherFromPublicKey(key)
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from private key [%s]", err)
	}
	_, err = aCipher.Process(nil)
	if err == nil {
		t.Fatalf("Decrypting nil should fail")
	}

	_, err = aCipher.Process([]byte{0, 1, 2, 3})
	if err == nil {
		t.Fatalf("Decrypting invalid ciphertxt should fail")
	}

}

func TestPrivateKeySerialization(t *testing.T) {
	spi := NewSPI()

	aKey, err := spi.NewDefaultPrivateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	bytes, err := spi.SerializePrivateKey(aKey)
	if err != nil {
		t.Fatalf("Failed serializing private key [%s]", err)
	}

	recoveredKey, err := spi.DeserializePrivateKey(bytes)
	if err != nil {
		t.Fatalf("Failed serializing private key [%s]", err)
	}

	// Encrypt
	aCipher, err := spi.NewAsymmetricCipherFromPublicKey(aKey.GetPublicKey())
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from public key [%s]", err)
	}
	msg := []byte("Hello World")
	ct, err := aCipher.Process(msg)
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	// Decrypt
	aCipher, err = spi.NewAsymmetricCipherFromPublicKey(recoveredKey)
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from private key [%s]", err)
	}
	recoveredMsg, err := aCipher.Process(ct)
	if err != nil {
		t.Fatalf("Failed decrypting [%s]", err)
	}
	if !reflect.DeepEqual(msg, recoveredMsg) {
		t.Fatalf("Failed decrypting. Output is different [%x][%x]", msg, recoveredMsg)
	}
}

func TestPublicKeySerialization(t *testing.T) {
	spi := NewSPI()

	aKey, err := spi.NewDefaultPrivateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed generating key [%s]", err)
	}

	bytes, err := spi.SerializePublicKey(aKey.GetPublicKey())
	if err != nil {
		t.Fatalf("Failed serializing private key [%s]", err)
	}

	pk, err := spi.DeserializePublicKey(bytes)
	if err != nil {
		t.Fatalf("Failed serializing private key [%s]", err)
	}

	// Encrypt
	aCipher, err := spi.NewAsymmetricCipherFromPublicKey(pk)
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from public key [%s]", err)
	}
	msg := []byte("Hello World")
	ct, err := aCipher.Process(msg)
	if err != nil {
		t.Fatalf("Failed encrypting [%s]", err)
	}

	// Decrypt
	aCipher, err = spi.NewAsymmetricCipherFromPublicKey(aKey)
	if err != nil {
		t.Fatalf("Failed creating AsymCipher from private key [%s]", err)
	}
	recoveredMsg, err := aCipher.Process(ct)
	if err != nil {
		t.Fatalf("Failed decrypting [%s]", err)
	}
	if !reflect.DeepEqual(msg, recoveredMsg) {
		t.Fatalf("Failed decrypting. Output is different [%x][%x]", msg, recoveredMsg)
	}
}
