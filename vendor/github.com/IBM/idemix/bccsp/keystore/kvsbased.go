/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package keystore

import (
	"encoding/hex"
	"fmt"

	"github.com/IBM/idemix/bccsp/handlers"
	idemix "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	bccsp "github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
)

type KVS interface {
	Put(id string, state any) error
	Get(id string, state any) error
}

type NymSecretKey struct {
	Ski        []byte
	Sk         []byte
	Pk         *amcl.ECP
	Exportable bool
}

type UserSecretKey struct {
	Sk         []byte
	Exportable bool
}

type entry struct {
	NymSecretKey  *NymSecretKey  `json:",omitempty"`
	UserSecretKey *UserSecretKey `json:",omitempty"`
}

// KVSStore is a read-only KeyStore that neither loads nor stores keys.
type KVSStore struct {
	KVS
	Translator idemix.Translator
	Curve      *math.Curve
}

// ReadOnly returns true if this KeyStore is read only, false otherwise.
// If ReadOnly is true then StoreKey will fail.
func (ks *KVSStore) ReadOnly() bool {
	return false
}

// GetKey returns a key object whose SKI is the one passed.
func (ks *KVSStore) GetKey(ski []byte) (bccsp.Key, error) {
	id := hex.EncodeToString(ski)

	entry := &entry{}
	err := ks.Get(id, entry)
	if err != nil {
		return nil, fmt.Errorf("could not get key [%s] from kvs: %w", id, err)
	}

	switch {
	case entry.NymSecretKey != nil:
		pk, err := ks.Translator.G1FromProto(entry.NymSecretKey.Pk)
		if err != nil {
			return nil, err
		}

		return &handlers.NymSecretKey{
			Exportable: entry.NymSecretKey.Exportable,
			Sk:         ks.Curve.NewZrFromBytes(entry.NymSecretKey.Sk),
			Ski:        entry.NymSecretKey.Ski,
			Pk:         pk,
			Translator: ks.Translator,
		}, nil
	case entry.UserSecretKey != nil:
		return &handlers.UserSecretKey{
			Exportable: entry.UserSecretKey.Exportable,
			Sk:         ks.Curve.NewZrFromBytes(entry.UserSecretKey.Sk),
		}, nil
	default:
		return nil, fmt.Errorf("key not found for [%s]", id)
	}
}

// StoreKey stores the key k in this KeyStore.
// If this KeyStore is read only then the method will fail.
func (ks *KVSStore) StoreKey(k bccsp.Key) error {
	entry := &entry{}
	var id string

	switch key := k.(type) {
	case *handlers.NymSecretKey:
		entry.NymSecretKey = &NymSecretKey{
			Ski:        key.Ski,
			Sk:         key.Sk.Bytes(),
			Pk:         ks.Translator.G1ToProto(key.Pk),
			Exportable: key.Exportable,
		}

		pk, err := k.PublicKey()
		if err != nil {
			return fmt.Errorf("could not get public version for key [%s]", k.SKI())
		}

		id = hex.EncodeToString(pk.SKI())
	case *handlers.UserSecretKey:
		entry.UserSecretKey = &UserSecretKey{
			Sk:         key.Sk.Bytes(),
			Exportable: key.Exportable,
		}
		id = hex.EncodeToString(k.SKI())
	default:
		return fmt.Errorf("unknown type [%T] for the supplied key", key)
	}

	return ks.Put(id, entry)
}
