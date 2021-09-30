/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package keystore

import (
	"errors"

	bccsp "github.com/IBM/idemix/bccsp/schemes"
)

// Dummy is a read-only KeyStore that neither loads nor stores keys.
type Dummy struct {
}

// ReadOnly returns true if this KeyStore is read only, false otherwise.
// If ReadOnly is true then StoreKey will fail.
func (ks *Dummy) ReadOnly() bool {
	return true
}

// GetKey returns a key object whose SKI is the one passed.
func (ks *Dummy) GetKey(ski []byte) (bccsp.Key, error) {
	return nil, errors.New("key not found. This is a dummy KeyStore")
}

// StoreKey stores the key k in this KeyStore.
// If this KeyStore is read only then the method will fail.
func (ks *Dummy) StoreKey(k bccsp.Key) error {
	return errors.New("cannot store key. This is a dummy read-only KeyStore")
}
