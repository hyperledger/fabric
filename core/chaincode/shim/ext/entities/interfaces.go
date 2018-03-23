/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package entities

// Entity is the basic interface for all crypto entities
// that are used by the library to obtain cc-level encryption
type Entity interface {
	// ID returns an identifier for the entity;
	// the identifier can be set arbitrarily by
	// the entity's constructor in a manner that
	// is relevant for its usage at the cc-level
	ID() string

	// Equals compares this entity with the supplied
	// one and returns a boolean that is true if the
	// two entities are identical. This includes any
	// and all key material that the entity uses
	Equals(Entity) bool

	// Public returns the public version of this entity
	// in case asymmetric cryptography is used. If not,
	// Public returns itself
	Public() (Entity, error)
}

// Signer is an interface that provides basic sign/verify capabilities
type Signer interface {
	// Sign returns a signature of the supplied message (or an error)
	Sign(msg []byte) (signature []byte, err error)

	// Verify checks whether the supplied signature
	// over the supplied message is valid according to this interface
	Verify(signature, msg []byte) (valid bool, err error)
}

// Encrypter is an interface that provides basic encrypt/decrypt capabilities
type Encrypter interface {
	// Encrypt returns the ciphertext for the supplied plaintext message
	Encrypt(plaintext []byte) (ciphertext []byte, err error)

	// Decrypt returns the plaintext for the supplied ciphertext message
	Decrypt(ciphertext []byte) (plaintext []byte, err error)
}

// Encrypter entity is an entity which is capable of performing encryption
type EncrypterEntity interface {
	Entity
	Encrypter
}

// SignerEntity is an entity which is capable of signing
type SignerEntity interface {
	Entity
	Signer
}

// EncrypterSignerEntity is an entity which is capable of performing
// encryption and of generating signatures
type EncrypterSignerEntity interface {
	Entity
	Encrypter
	Signer
}
