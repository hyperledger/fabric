/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package handlers

import (
	"crypto/sha256"

	bccsp "github.com/IBM/idemix/bccsp/schemes"
	math "github.com/IBM/mathlib"
	"github.com/pkg/errors"
)

// UserSecretKey contains the User secret key
type UserSecretKey struct {
	// Sk is the idemix reference to the User key
	Sk *math.Zr
	// Exportable if true, sk can be exported via the Bytes function
	Exportable bool
}

func NewUserSecretKey(sk *math.Zr, exportable bool) *UserSecretKey {
	return &UserSecretKey{Sk: sk, Exportable: exportable}
}

func (k *UserSecretKey) Bytes() ([]byte, error) {
	if k.Exportable {
		return k.Sk.Bytes(), nil
	}

	return nil, errors.New("not exportable")
}

func (k *UserSecretKey) SKI() []byte {
	raw := k.Sk.Bytes()
	hash := sha256.New()
	hash.Write(raw)
	return hash.Sum(nil)
}

func (*UserSecretKey) Symmetric() bool {
	return true
}

func (*UserSecretKey) Private() bool {
	return true
}

func (k *UserSecretKey) PublicKey() (bccsp.Key, error) {
	return nil, errors.New("cannot call this method on a symmetric key")
}

type UserKeyGen struct {
	// Exportable is a flag to allow an issuer secret key to be marked as Exportable.
	// If a secret key is marked as Exportable, its Bytes method will return the key's byte representation.
	Exportable bool
	// User implements the underlying cryptographic algorithms
	User User
}

func (g *UserKeyGen) KeyGen(opts bccsp.KeyGenOpts) (bccsp.Key, error) {
	sk, err := g.User.NewKey()
	if err != nil {
		return nil, err
	}

	return &UserSecretKey{Exportable: g.Exportable, Sk: sk}, nil
}

// UserKeyImporter import user keys
type UserKeyImporter struct {
	// Exportable is a flag to allow a secret key to be marked as Exportable.
	// If a secret key is marked as Exportable, its Bytes method will return the key's byte representation.
	Exportable bool
	// User implements the underlying cryptographic algorithms
	User User
}

func (i *UserKeyImporter) KeyImport(raw interface{}, opts bccsp.KeyImportOpts) (k bccsp.Key, err error) {
	der, ok := raw.([]byte)
	if !ok {
		return nil, errors.New("invalid raw, expected byte array")
	}

	if len(der) == 0 {
		return nil, errors.New("invalid raw, it must not be nil")
	}

	sk, err := i.User.NewKeyFromBytes(raw.([]byte))
	if err != nil {
		return nil, err
	}

	return &UserSecretKey{Exportable: i.Exportable, Sk: sk}, nil
}
