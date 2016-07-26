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
	"errors"
	"io"
)

var (
	// ErrEncryption Error during encryption
	ErrEncryption = errors.New("Error during encryption.")

	// ErrDecryption Error during decryption
	ErrDecryption = errors.New("Error during decryption.")

	// ErrInvalidSecretKeyType Invalid Secret Key type
	ErrInvalidSecretKeyType = errors.New("Invalid Secret Key type.")

	// ErrInvalidPublicKeyType Invalid Public Key type
	ErrInvalidPublicKeyType = errors.New("Invalid Public Key type.")

	// ErrInvalidKeyParameter Invalid Key Parameter
	ErrInvalidKeyParameter = errors.New("Invalid Key Parameter.")

	// ErrInvalidNilKeyParameter Invalid Nil Key Parameter
	ErrInvalidNilKeyParameter = errors.New("Invalid Nil Key Parameter.")

	// ErrInvalidKeyGeneratorParameter Invalid Key Generator Parameter
	ErrInvalidKeyGeneratorParameter = errors.New("Invalid Key Generator Parameter.")
)

// Parameters is common interface for all the parameters
type Parameters interface {

	// GetRand returns the random generated associated to this parameters
	GetRand() io.Reader
}

// CipherParameters is common interface to represent cipher parameters
type CipherParameters interface {
	Parameters
}

// AsymmetricCipherParameters is common interface to represent asymmetric cipher parameters
type AsymmetricCipherParameters interface {
	CipherParameters

	// IsPublic returns true if the parameters are public, false otherwise.
	IsPublic() bool
}

// PublicKey is common interface to represent public asymmetric cipher parameters
type PublicKey interface {
	AsymmetricCipherParameters
}

// PrivateKey is common interface to represent private asymmetric cipher parameters
type PrivateKey interface {
	AsymmetricCipherParameters

	// GetPublicKey returns the associated public key
	GetPublicKey() PublicKey
}

// KeyGeneratorParameters is common interface to represent key generation parameters
type KeyGeneratorParameters interface {
	Parameters
}

// KeyGenerator defines a key generator
type KeyGenerator interface {
	// Init initializes this generated using the passed parameters
	Init(params KeyGeneratorParameters) error

	// GenerateKey generates a new private key
	GenerateKey() (PrivateKey, error)
}

// AsymmetricCipher defines an asymmetric cipher
type AsymmetricCipher interface {
	// Init initializes this cipher with the passed parameters
	Init(params AsymmetricCipherParameters) error

	// Process processes the byte array given in input
	Process(msg []byte) ([]byte, error)
}

// SecretKey defines a symmetric key
type SecretKey interface {
	CipherParameters
}

// StreamCipher defines a stream cipher
type StreamCipher interface {
	// Init initializes this cipher with the passed parameters
	Init(forEncryption bool, params CipherParameters) error

	// Process processes the byte array given in input
	Process(msg []byte) ([]byte, error)
}

// KeySerializer defines a key serializer/deserializer
type KeySerializer interface {
	// ToBytes converts a key to bytes
	ToBytes(key interface{}) ([]byte, error)

	// ToBytes converts bytes to a key
	FromBytes([]byte) (interface{}, error)
}

// AsymmetricCipherSPI is a Service Provider Interface for AsymmetricCipher
type AsymmetricCipherSPI interface {

	// NewAsymmetricCipherFromPrivateKey creates a new AsymmetricCipher for decryption from a secret key
	NewAsymmetricCipherFromPrivateKey(priv PrivateKey) (AsymmetricCipher, error)

	// NewAsymmetricCipherFromPublicKey creates a new AsymmetricCipher for encryption from a public key
	NewAsymmetricCipherFromPublicKey(pub PublicKey) (AsymmetricCipher, error)

	// NewAsymmetricCipherFromPublicKey creates a new AsymmetricCipher for encryption from a serialized public key
	NewAsymmetricCipherFromSerializedPublicKey(pub []byte) (AsymmetricCipher, error)

	// NewAsymmetricCipherFromPublicKey creates a new AsymmetricCipher for encryption from a serialized public key
	NewAsymmetricCipherFromSerializedPrivateKey(priv []byte) (AsymmetricCipher, error)

	// NewPrivateKey creates a new private key rand and default parameters
	NewDefaultPrivateKey(rand io.Reader) (PrivateKey, error)

	// NewPrivateKey creates a new private key from (rand, params)
	NewPrivateKey(rand io.Reader, params interface{}) (PrivateKey, error)

	// NewPublicKey creates a new public key from (rand, params)
	NewPublicKey(rand io.Reader, params interface{}) (PublicKey, error)

	// SerializePrivateKey serializes a private key
	SerializePrivateKey(priv PrivateKey) ([]byte, error)

	// DeserializePrivateKey deserializes to a private key
	DeserializePrivateKey(bytes []byte) (PrivateKey, error)

	// SerializePrivateKey serializes a private key
	SerializePublicKey(pub PublicKey) ([]byte, error)

	// DeserializePrivateKey deserializes to a private key
	DeserializePublicKey(bytes []byte) (PublicKey, error)
}

// StreamCipherSPI is a Service Provider Interface for StreamCipher
type StreamCipherSPI interface {
	GenerateKey() (SecretKey, error)

	GenerateKeyAndSerialize() (SecretKey, []byte, error)

	NewSecretKey(rand io.Reader, params interface{}) (SecretKey, error)

	// NewStreamCipherForEncryptionFromKey creates a new StreamCipher for encryption from a secret key
	NewStreamCipherForEncryptionFromKey(secret SecretKey) (StreamCipher, error)

	// NewStreamCipherForEncryptionFromSerializedKey creates a new StreamCipher for encryption from a serialized key
	NewStreamCipherForEncryptionFromSerializedKey(secret []byte) (StreamCipher, error)

	// NewStreamCipherForDecryptionFromKey creates a new StreamCipher for decryption from a secret key
	NewStreamCipherForDecryptionFromKey(secret SecretKey) (StreamCipher, error)

	// NewStreamCipherForDecryptionFromKey creates a new StreamCipher for decryption from a serialized key
	NewStreamCipherForDecryptionFromSerializedKey(secret []byte) (StreamCipher, error)

	// SerializePrivateKey serializes a private key
	SerializeSecretKey(secret SecretKey) ([]byte, error)

	// DeserializePrivateKey deserializes to a private key
	DeserializeSecretKey(bytes []byte) (SecretKey, error)
}
