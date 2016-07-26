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

// This package contains unit-tests for the
// github.com/hyperledger/fabric/core/crypto/primitives package
package primitives_test

import (
	"bytes"
	"crypto/aes"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/hyperledger/fabric/core/crypto/primitives"
)

// TestCBCPKCS7EncryptCBCPKCS7Decrypt encrypts using CBCPKCS7Encrypt and decrypts using CBCPKCS7Decrypt.
func TestCBCPKCS7EncryptCBCPKCS7Decrypt(t *testing.T) {

	// Note: The purpose of this test is not to test AES-256 in CBC mode's strength
	// ... but rather to verify the code wrapping/unwrapping the cipher.
	key := make([]byte, primitives.AESKeyLength)
	rand.Reader.Read(key)

	//                  123456789012345678901234567890123456789012
	var ptext = []byte("a message with arbitrary length (42 bytes)")

	encrypted, encErr := primitives.CBCPKCS7Encrypt(key, ptext)
	if encErr != nil {
		t.Fatalf("Error encrypting '%s': %s", ptext, encErr)
	}

	decrypted, dErr := primitives.CBCPKCS7Decrypt(key, encrypted)
	if dErr != nil {
		t.Fatalf("Error decrypting the encrypted '%s': %v", ptext, dErr)
	}

	if string(ptext[:]) != string(decrypted[:]) {
		t.Fatal("Decrypt( Encrypt( ptext ) ) != ptext: Ciphertext decryption with the same key must result in the original plaintext!")
	}

}

// TestPKCS7Padding verifies the PKCS#7 padding, using a human readable plaintext.
func TestPKCS7Padding(t *testing.T) {

	// 0 byte/length ptext
	ptext := []byte("")
	expected := []byte{16, 16, 16, 16,
		16, 16, 16, 16,
		16, 16, 16, 16,
		16, 16, 16, 16}
	result := primitives.PKCS7Padding(ptext)

	if !bytes.Equal(expected, result) {
		t.Fatal("Padding error! Expected: ", expected, "', received: '", result, "'")
	}

	// 1 byte/length ptext
	ptext = []byte("1")
	expected = []byte{'1', 15, 15, 15,
		15, 15, 15, 15,
		15, 15, 15, 15,
		15, 15, 15, 15}
	result = primitives.PKCS7Padding(ptext)

	if !bytes.Equal(expected, result) {
		t.Fatal("Padding error! Expected: '", expected, "', received: '", result, "'")
	}

	// 2 byte/length ptext
	ptext = []byte("12")
	expected = []byte{'1', '2', 14, 14,
		14, 14, 14, 14,
		14, 14, 14, 14,
		14, 14, 14, 14}
	result = primitives.PKCS7Padding(ptext)

	if !bytes.Equal(expected, result) {
		t.Fatal("Padding error! Expected: '", expected, "', received: '", result, "'")
	}

	// 3 to aes.BlockSize-1 byte plaintext
	ptext = []byte("1234567890ABCDEF")
	for i := 3; i < aes.BlockSize; i++ {
		result := primitives.PKCS7Padding(ptext[:i])

		padding := aes.BlockSize - i
		expectedPadding := bytes.Repeat([]byte{byte(padding)}, padding)
		expected = append(ptext[:i], expectedPadding...)

		if !bytes.Equal(result, expected) {
			t.Fatal("Padding error! Expected: '", expected, "', received: '", result, "'")
		}

	}

	// aes.BlockSize length ptext
	ptext = bytes.Repeat([]byte{byte('x')}, aes.BlockSize)
	result = primitives.PKCS7Padding(ptext)

	expectedPadding := bytes.Repeat([]byte{byte(aes.BlockSize)}, aes.BlockSize)
	expected = append(ptext, expectedPadding...)

	if len(result) != 2*aes.BlockSize {
		t.Fatal("Padding error: expected the length of the returned slice to be 2 times aes.BlockSize")
	}

	if !bytes.Equal(expected, result) {
		t.Fatal("Padding error! Expected: '", expected, "', received: '", result, "'")
	}

}

// TestPKCS7UnPadding verifies the PKCS#7 unpadding, using a human readable plaintext.
func TestPKCS7UnPadding(t *testing.T) {

	// 0 byte/length ptext
	expected := []byte("")
	ptext := []byte{16, 16, 16, 16,
		16, 16, 16, 16,
		16, 16, 16, 16,
		16, 16, 16, 16}

	result, _ := primitives.PKCS7UnPadding(ptext)

	if !bytes.Equal(expected, result) {
		t.Fatal("UnPadding error! Expected: '", expected, "', received: '", result, "'")
	}

	// 1 byte/length ptext
	expected = []byte("1")
	ptext = []byte{'1', 15, 15, 15,
		15, 15, 15, 15,
		15, 15, 15, 15,
		15, 15, 15, 15}

	result, _ = primitives.PKCS7UnPadding(ptext)

	if !bytes.Equal(expected, result) {
		t.Fatal("UnPadding error! Expected: '", expected, "', received: '", result, "'")
	}

	// 2 byte/length ptext
	expected = []byte("12")
	ptext = []byte{'1', '2', 14, 14,
		14, 14, 14, 14,
		14, 14, 14, 14,
		14, 14, 14, 14}

	result, _ = primitives.PKCS7UnPadding(ptext)

	if !bytes.Equal(expected, result) {
		t.Fatal("UnPadding error! Expected: '", expected, "', received: '", result, "'")
	}

	// 3 to aes.BlockSize-1 byte plaintext
	base := []byte("1234567890ABCDEF")
	for i := 3; i < aes.BlockSize; i++ {
		iPad := aes.BlockSize - i
		padding := bytes.Repeat([]byte{byte(iPad)}, iPad)
		ptext = append(base[:i], padding...)

		expected := base[:i]
		result, _ := primitives.PKCS7UnPadding(ptext)

		if !bytes.Equal(result, expected) {
			t.Fatal("UnPadding error! Expected: '", expected, "', received: '", result, "'")
		}

	}

	// aes.BlockSize length ptext
	expected = bytes.Repeat([]byte{byte('x')}, aes.BlockSize)
	padding := bytes.Repeat([]byte{byte(aes.BlockSize)}, aes.BlockSize)
	ptext = append(expected, padding...)

	result, _ = primitives.PKCS7UnPadding(ptext)

	if !bytes.Equal(expected, result) {
		t.Fatal("UnPadding error! Expected: '", expected, "', received: '", result, "'")
	}
}

// TestCBCEncryptCBCPKCS7Decrypt_BlockSizeLengthPlaintext verifies that CBCPKCS7Decrypt returns an error
// when attempting to decrypt ciphertext of an irreproducible length.
func TestCBCEncryptCBCPKCS7Decrypt_BlockSizeLengthPlaintext(t *testing.T) {

	// One of the purposes of this test is to also document and clarify the expected behavior, i.e., that an extra
	// block is appended to the message at the padding stage, as per the spec of PKCS#7 v1.5 [see RFC-2315 p.21]
	key := make([]byte, primitives.AESKeyLength)
	rand.Reader.Read(key)

	//                  1234567890123456
	var ptext = []byte("a 16 byte messag")

	encrypted, encErr := primitives.CBCEncrypt(key, ptext)
	if encErr != nil {
		t.Fatalf("Error encrypting '%s': %v", ptext, encErr)
	}

	decrypted, dErr := primitives.CBCPKCS7Decrypt(key, encrypted)
	if dErr == nil {
		t.Fatalf("Expected an error decrypting ptext '%s'. Decrypted to '%v'", dErr, decrypted)
	}
}

// TestCBCPKCS7EncryptCBCDecrypt_ExpectingCorruptMessage verifies that CBCDecrypt can decrypt the unpadded
// version of the ciphertext, of a message of BlockSize length.
func TestCBCPKCS7EncryptCBCDecrypt_ExpectingCorruptMessage(t *testing.T) {

	// One of the purposes of this test is to also document and clarify the expected behavior, i.e., that an extra
	// block is appended to the message at the padding stage, as per the spec of PKCS#7 v1.5 [see RFC-2315 p.21]
	key := make([]byte, primitives.AESKeyLength)
	rand.Reader.Read(key)

	//                  0123456789ABCDEF
	var ptext = []byte("a 16 byte messag")

	encrypted, encErr := primitives.CBCPKCS7Encrypt(key, ptext)
	if encErr != nil {
		t.Fatalf("Error encrypting ptext %v", encErr)
	}

	decrypted, dErr := primitives.CBCDecrypt(key, encrypted)
	if dErr != nil {
		t.Fatalf("Error encrypting ptext %v, %v", dErr, decrypted)
	}

	if string(ptext[:]) != string(decrypted[:aes.BlockSize]) {
		t.Log("ptext: ", ptext)
		t.Log("decrypted: ", decrypted[:aes.BlockSize])
		t.Fatal("Encryption->Decryption with same key should result in original ptext")
	}

	if !bytes.Equal(decrypted[aes.BlockSize:], bytes.Repeat([]byte{byte(aes.BlockSize)}, aes.BlockSize)) {
		t.Fatal("Expected extra block with padding in encrypted ptext", decrypted)
	}

}

// TestCBCPKCS7Encrypt_EmptyPlaintext encrypts and pad an empty ptext. Verifying as well that the ciphertext length is as expected.
func TestCBCPKCS7Encrypt_EmptyPlaintext(t *testing.T) {

	key := make([]byte, primitives.AESKeyLength)
	rand.Reader.Read(key)

	t.Log("Generated key: ", key)

	var emptyPlaintext = []byte("")
	t.Log("Plaintext length: ", len(emptyPlaintext))

	ciphertext, encErr := primitives.CBCPKCS7Encrypt(key, emptyPlaintext)
	if encErr != nil {
		t.Fatalf("Error encrypting '%v'", encErr)
	}

	// Expected ciphertext length: primitives.AESKeyLength (=32)
	// As part of the padding, at least one block gets encrypted (while the first block is the IV)
	const expectedLength = aes.BlockSize + aes.BlockSize
	if len(ciphertext) != expectedLength {
		t.Fatalf("Wrong ciphertext length. Expected %d, recieved %d", expectedLength, len(ciphertext))
	}

	t.Log("Ciphertext length: ", len(ciphertext))
	t.Log("Cipher: ", ciphertext)
}

// TestCBCEncrypt_EmptyPlaintext encrypts an empty message. Verifying as well that the ciphertext length is as expected.
func TestCBCEncrypt_EmptyPlaintext(t *testing.T) {

	key := make([]byte, primitives.AESKeyLength)
	rand.Reader.Read(key)
	t.Log("Generated key: ", key)

	var emptyPlaintext = []byte("")
	t.Log("Message length: ", len(emptyPlaintext))

	ciphertext, encErr := primitives.CBCEncrypt(key, emptyPlaintext)
	if encErr != nil {
	}

	t.Log("Ciphertext length: ", len(ciphertext))

	// Expected cipher length: aes.BlockSize, the first and only block is the IV
	var expectedLength = aes.BlockSize

	if len(ciphertext) != expectedLength {
		t.Fatalf("Wrong ciphertext length. Expected: '%d', received: '%d'", expectedLength, len(ciphertext))
	}
	t.Log("Ciphertext: ", ciphertext)
}

// TestCBCPKCS7Encrypt_VerifyRandomIVs encrypts twice with same key. The first 16 bytes should be different if IV is generated randomly.
func TestCBCPKCS7Encrypt_VerifyRandomIVs(t *testing.T) {

	key := make([]byte, aes.BlockSize)
	rand.Reader.Read(key)
	t.Log("Key 1", key)

	var ptext = []byte("a message to encrypt")

	ciphertext1, err := primitives.CBCPKCS7Encrypt(key, ptext)
	if err != nil {
		t.Fatalf("Error encrypting '%s': %s", ptext, err)
	}

	// Expecting a different IV if same message is encrypted with same key
	ciphertext2, err := primitives.CBCPKCS7Encrypt(key, ptext)
	if err != nil {
		t.Fatalf("Error encrypting '%s': %s", ptext, err)
	}

	iv1 := ciphertext1[:aes.BlockSize]
	iv2 := ciphertext2[:aes.BlockSize]

	t.Log("Ciphertext1: ", iv1)
	t.Log("Ciphertext2: ", iv2)
	t.Log("bytes.Equal: ", bytes.Equal(iv1, iv2))

	if bytes.Equal(iv1, iv2) {
		t.Fatal("Error: ciphertexts contain identical initialization vectors (IVs)")
	}
}

// TestCBCPKCS7Encrypt_CorrectCiphertextLengthCheck verifies that the returned ciphertext lengths are as expected.
func TestCBCPKCS7Encrypt_CorrectCiphertextLengthCheck(t *testing.T) {

	key := make([]byte, aes.BlockSize)
	rand.Reader.Read(key)

	// length of message (in bytes) == aes.BlockSize (16 bytes)
	// The expected cipher length = IV length (1 block) + 1 block message

	var ptext = []byte("0123456789ABCDEF")

	for i := 1; i < aes.BlockSize; i++ {
		ciphertext, err := primitives.CBCPKCS7Encrypt(key, ptext[:i])
		if err != nil {
			t.Fatal("Error encrypting '", ptext, "'")
		}

		expectedLength := aes.BlockSize + aes.BlockSize
		if len(ciphertext) != expectedLength {
			t.Fatalf("Incorrect ciphertext incorrect: expected '%d', received '%d'", expectedLength, len(ciphertext))
		}
	}
}

// TestCBCEncryptCBCDecrypt_KeyMismatch attempts to decrypt with a different key than the one used for encryption.
func TestCBCEncryptCBCDecrypt_KeyMismatch(t *testing.T) {

	// Generate a random key
	key := make([]byte, aes.BlockSize)
	rand.Reader.Read(key)

	// Clone & tamper with the key
	wrongKey := make([]byte, aes.BlockSize)
	copy(wrongKey, key[:])
	wrongKey[0] = key[0] + 1

	var ptext = []byte("1234567890ABCDEF")
	encrypted, encErr := primitives.CBCEncrypt(key, ptext)
	if encErr != nil {
		t.Fatalf("Error encrypting '%s': %v", ptext, encErr)
	}

	decrypted, decErr := primitives.CBCDecrypt(wrongKey, encrypted)
	if decErr != nil {
		t.Fatalf("Error decrypting '%s': %v", ptext, decErr)
	}

	if string(ptext[:]) == string(decrypted[:]) {
		t.Fatal("Decrypting a ciphertext with a different key than the one used for encrypting it - should not result in the original plaintext.")
	}
}

// TestCBCEncryptCBCDecrypt encrypts with CBCEncrypt and decrypt with CBCDecrypt.
func TestCBCEncryptCBCDecrypt(t *testing.T) {

	key := make([]byte, primitives.AESKeyLength)
	rand.Reader.Read(key)

	//                  1234567890123456
	var ptext = []byte("a 16 byte messag")

	encrypted, encErr := primitives.CBCEncrypt(key, ptext)
	if encErr != nil {
		t.Fatalf("Error encrypting '%s': %v", ptext, encErr)
	}

	decrypted, decErr := primitives.CBCDecrypt(key, encrypted)
	if decErr != nil {
		t.Fatalf("Error decrypting '%s': %v", ptext, decErr)
	}

	if string(ptext[:]) != string(decrypted[:]) {
		t.Fatal("Encryption->Decryption with same key should result in the original plaintext.")
	}
}

// TestAESRelatedUtilFunctions tests various functions commonly used in fabric wrt AES
func TestAESRelatedUtilFunctions(t *testing.T) {

	key, err := primitives.GenAESKey()
	if err != nil {
		t.Fatalf("Failed generating AES key [%s]", err)
	}

	for i := 1; i < 100; i++ {
		len, err := rand.Int(rand.Reader, big.NewInt(1024))
		if err != nil {
			t.Fatalf("Failed generating AES key [%s]", err)
		}
		msg, err := primitives.GetRandomBytes(int(len.Int64()) + 1)
		if err != nil {
			t.Fatalf("Failed generating AES key [%s]", err)
		}

		ct, err := primitives.CBCPKCS7Encrypt(key, msg)
		if err != nil {
			t.Fatalf("Failed encrypting [%s]", err)
		}

		msg2, err := primitives.CBCPKCS7Decrypt(key, ct)
		if err != nil {
			t.Fatalf("Failed decrypting [%s]", err)
		}

		if 0 != bytes.Compare(msg, msg2) {
			t.Fatalf("Wrong decryption output [%x][%x]", msg, msg2)
		}

	}

}

// TestVariousAESKeyEncoding tests some AES <-> PEM conversions
func TestVariousAESKeyEncoding(t *testing.T) {
	key, err := primitives.GenAESKey()
	if err != nil {
		t.Fatalf("Failed generating AES key [%s]", err)
	}

	// PEM format
	pem := primitives.AEStoPEM(key)
	keyFromPEM, err := primitives.PEMtoAES(pem, nil)
	if err != nil {
		t.Fatalf("Failed converting PEM to AES key [%s]", err)
	}
	if 0 != bytes.Compare(key, keyFromPEM) {
		t.Fatalf("Failed converting PEM to AES key. Keys are different [%x][%x]", key, keyFromPEM)
	}

	// Encrypted PEM format
	pem, err = primitives.AEStoEncryptedPEM(key, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting AES key to Encrypted PEM [%s]", err)
	}
	keyFromPEM, err = primitives.PEMtoAES(pem, []byte("passwd"))
	if err != nil {
		t.Fatalf("Failed converting encrypted PEM to AES key [%s]", err)
	}
	if 0 != bytes.Compare(key, keyFromPEM) {
		t.Fatalf("Failed converting encrypted PEM to AES key. Keys are different [%x][%x]", key, keyFromPEM)
	}
}
