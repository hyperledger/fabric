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

package primitives

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/hyperledger/fabric/core/crypto/utils"
)

// PrivateKeyToDER marshals a private key to der
func PrivateKeyToDER(privateKey *ecdsa.PrivateKey) ([]byte, error) {
	if privateKey == nil {
		return nil, utils.ErrNilArgument
	}

	return x509.MarshalECPrivateKey(privateKey)
}

// PrivateKeyToPEM converts a private key to PEM
func PrivateKeyToPEM(privateKey interface{}, pwd []byte) ([]byte, error) {
	if len(pwd) != 0 {
		return PrivateKeyToEncryptedPEM(privateKey, pwd)
	}

	switch x := privateKey.(type) {
	case *ecdsa.PrivateKey:
		raw, err := x509.MarshalECPrivateKey(x)

		if err != nil {
			return nil, err
		}

		return pem.EncodeToMemory(
			&pem.Block{
				Type:  "ECDSA PRIVATE KEY",
				Bytes: raw,
			},
		), nil
	default:
		return nil, utils.ErrInvalidKey
	}
}

// PrivateKeyToEncryptedPEM converts a private key to an encrypted PEM
func PrivateKeyToEncryptedPEM(privateKey interface{}, pwd []byte) ([]byte, error) {
	switch x := privateKey.(type) {
	case *ecdsa.PrivateKey:
		raw, err := x509.MarshalECPrivateKey(x)

		if err != nil {
			return nil, err
		}

		block, err := x509.EncryptPEMBlock(
			rand.Reader,
			"ECDSA PRIVATE KEY",
			raw,
			pwd,
			x509.PEMCipherAES256)

		if err != nil {
			return nil, err
		}

		return pem.EncodeToMemory(block), nil

	default:
		return nil, utils.ErrInvalidKey
	}
}

// DERToPrivateKey unmarshals a der to private key
func DERToPrivateKey(der []byte) (key interface{}, err error) {
	//fmt.Printf("DER [%s]\n", EncodeBase64(der))

	if key, err = x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	//fmt.Printf("DERToPrivateKey Err [%s]\n", err)
	if key, err = x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key.(type) {
		case *rsa.PrivateKey, *ecdsa.PrivateKey:
			return
		default:
			return nil, errors.New("Found unknown private key type in PKCS#8 wrapping")
		}
	}
	//fmt.Printf("DERToPrivateKey Err [%s]\n", err)
	if key, err = x509.ParseECPrivateKey(der); err == nil {
		return
	}
	//fmt.Printf("DERToPrivateKey Err [%s]\n", err)

	return nil, errors.New("Failed to parse private key")
}

// PEMtoPrivateKey unmarshals a pem to private key
func PEMtoPrivateKey(raw []byte, pwd []byte) (interface{}, error) {
	if len(raw) == 0 {
		return nil, utils.ErrNilArgument
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("Failed decoding [% x]", raw)
	}

	// TODO: derive from header the type of the key

	if x509.IsEncryptedPEMBlock(block) {
		if len(pwd) == 0 {
			return nil, errors.New("Encrypted Key. Need a password!!!")
		}

		decrypted, err := x509.DecryptPEMBlock(block, pwd)
		if err != nil {
			return nil, errors.New("Failed decryption!!!")
		}

		key, err := DERToPrivateKey(decrypted)
		if err != nil {
			return nil, err
		}
		return key, err
	}

	cert, err := DERToPrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return cert, err
}

// PEMtoAES extracts from the PEM an AES key
func PEMtoAES(raw []byte, pwd []byte) ([]byte, error) {
	if len(raw) == 0 {
		return nil, utils.ErrNilArgument
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("Failed decoding [% x]", raw)
	}

	if x509.IsEncryptedPEMBlock(block) {
		if len(pwd) == 0 {
			return nil, errors.New("Encrypted Key. Need a password!!!")
		}

		decrypted, err := x509.DecryptPEMBlock(block, pwd)
		if err != nil {
			return nil, err
		}
		return decrypted, nil
	}

	return block.Bytes, nil
}

// AEStoPEM encapsulates an AES key in the PEM format
func AEStoPEM(raw []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "AES PRIVATE KEY", Bytes: raw})
}

// AEStoEncryptedPEM encapsulates an AES key in the encrypted PEM format
func AEStoEncryptedPEM(raw []byte, pwd []byte) ([]byte, error) {
	if len(pwd) == 0 {
		return AEStoPEM(raw), nil
	}

	block, err := x509.EncryptPEMBlock(
		rand.Reader,
		"AES PRIVATE KEY",
		raw,
		pwd,
		x509.PEMCipherAES256)

	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(block), nil
}

/*
func PublicKeyToDER(publicKey interface{}) ([]byte, error) {
	return x509.MarshalPKIXPublicKey(publicKey)
}

func DERToPublicKey(derBytes []byte) (pub interface{}, err error) {
	key, err := x509.ParsePKIXPublicKey(derBytes)

	return key, err
}
*/

// PublicKeyToPEM marshals a public key to the pem forma
func PublicKeyToPEM(publicKey interface{}, pwd []byte) ([]byte, error) {
	if len(pwd) != 0 {
		return PublicKeyToEncryptedPEM(publicKey, pwd)
	}

	switch x := publicKey.(type) {
	case *ecdsa.PublicKey:
		PubASN1, err := x509.MarshalPKIXPublicKey(x)
		if err != nil {
			return nil, err
		}

		return pem.EncodeToMemory(
			&pem.Block{
				Type:  "ECDSA PUBLIC KEY",
				Bytes: PubASN1,
			},
		), nil

	default:
		return nil, utils.ErrInvalidKey
	}
}

// PublicKeyToEncryptedPEM converts a public key to encrypted pem
func PublicKeyToEncryptedPEM(publicKey interface{}, pwd []byte) ([]byte, error) {
	switch x := publicKey.(type) {
	case *ecdsa.PublicKey:
		raw, err := x509.MarshalPKIXPublicKey(x)

		if err != nil {
			return nil, err
		}

		block, err := x509.EncryptPEMBlock(
			rand.Reader,
			"ECDSA PUBLIC KEY",
			raw,
			pwd,
			x509.PEMCipherAES256)

		if err != nil {
			return nil, err
		}

		return pem.EncodeToMemory(block), nil

	default:
		return nil, utils.ErrInvalidKey
	}
}

// PEMtoPublicKey unmarshals a pem to public key
func PEMtoPublicKey(raw []byte, pwd []byte) (interface{}, error) {
	if len(raw) == 0 {
		return nil, utils.ErrNilArgument
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("Failed decoding [% x]", raw)
	}

	// TODO: derive from header the type of the key
	if x509.IsEncryptedPEMBlock(block) {
		if len(pwd) == 0 {
			return nil, errors.New("Encrypted Key. Need a password!!!")
		}

		decrypted, err := x509.DecryptPEMBlock(block, pwd)
		if err != nil {
			return nil, errors.New("Failed decryption!!!")
		}

		key, err := DERToPublicKey(decrypted)
		if err != nil {
			return nil, err
		}
		return key, err
	}

	cert, err := DERToPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return cert, err
}

// DERToPublicKey unmarshals a der to public key
func DERToPublicKey(derBytes []byte) (pub interface{}, err error) {
	key, err := x509.ParsePKIXPublicKey(derBytes)

	return key, err
}
