// Package symcrypt contains common symmetric encryption functions.
package symcrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
)

// DecryptCBC decrypt bytes using a key and IV with AES in CBC mode.
func DecryptCBC(data, iv, key []byte) (decryptedData []byte, err error) {
	aesCrypt, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	ivBytes := append([]byte{}, iv...)

	decryptedData = make([]byte, len(data))
	aesCBC := cipher.NewCBCDecrypter(aesCrypt, ivBytes)
	aesCBC.CryptBlocks(decryptedData, data)

	return
}

// EncryptCBC encrypt data using a key and IV with AES in CBC mode.
func EncryptCBC(data, iv, key []byte) (encryptedData []byte, err error) {
	aesCrypt, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	ivBytes := append([]byte{}, iv...)

	encryptedData = make([]byte, len(data))
	aesCBC := cipher.NewCBCEncrypter(aesCrypt, ivBytes)
	aesCBC.CryptBlocks(encryptedData, data)

	return
}

// MakeRandom is a helper that makes a new buffer full of random data.
func MakeRandom(length int) ([]byte, error) {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	return bytes, err
}
