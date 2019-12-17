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
	"hash"

	"github.com/hyperledger/fabric/bccsp"
)

// KeyGenerator is a BCCSP-like interface that provides key generation algorithms
type KeyGenerator interface {

	// KeyGen generates a key using opts.
	KeyGen(opts bccsp.KeyGenOpts) (k bccsp.Key, err error)
}

// KeyDeriver is a BCCSP-like interface that provides key derivation algorithms
type KeyDeriver interface {

	// KeyDeriv derives a key from k using opts.
	// The opts argument should be appropriate for the primitive used.
	KeyDeriv(k bccsp.Key, opts bccsp.KeyDerivOpts) (dk bccsp.Key, err error)
}

// KeyImporter is a BCCSP-like interface that provides key import algorithms
type KeyImporter interface {

	// KeyImport imports a key from its raw representation using opts.
	// The opts argument should be appropriate for the primitive used.
	KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error)
}

// Encryptor is a BCCSP-like interface that provides encryption algorithms
type Encryptor interface {

	// Encrypt encrypts plaintext using key k.
	// The opts argument should be appropriate for the algorithm used.
	Encrypt(k bccsp.Key, plaintext []byte, opts bccsp.EncrypterOpts) (ciphertext []byte, err error)
}

// Decryptor is a BCCSP-like interface that provides decryption algorithms
type Decryptor interface {

	// Decrypt decrypts ciphertext using key k.
	// The opts argument should be appropriate for the algorithm used.
	Decrypt(k bccsp.Key, ciphertext []byte, opts bccsp.DecrypterOpts) (plaintext []byte, err error)
}

// Signer is a BCCSP-like interface that provides signing algorithms
type Signer interface {

	// Sign signs digest using key k.
	// The opts argument should be appropriate for the algorithm used.
	//
	// Note that when a signature of a hash of a larger message is needed,
	// the caller is responsible for hashing the larger message and passing
	// the hash (as digest).
	Sign(k bccsp.Key, digest []byte, opts bccsp.SignerOpts) (signature []byte, err error)
}

// Verifier is a BCCSP-like interface that provides verifying algorithms
type Verifier interface {

	// Verify verifies signature against key k and digest
	// The opts argument should be appropriate for the algorithm used.
	Verify(k bccsp.Key, signature, digest []byte, opts bccsp.SignerOpts) (valid bool, err error)
}

// Hasher is a BCCSP-like interface that provides hash algorithms
type Hasher interface {

	// Hash hashes messages msg using options opts.
	// If opts is nil, the default hash function will be used.
	Hash(msg []byte, opts bccsp.HashOpts) (hash []byte, err error)

	// GetHash returns and instance of hash.Hash using options opts.
	// If opts is nil, the default hash function will be returned.
	GetHash(opts bccsp.HashOpts) (h hash.Hash, err error)
}
