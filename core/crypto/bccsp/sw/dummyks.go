package sw

import (
	"errors"

	"github.com/hyperledger/fabric/core/crypto/bccsp"
)

// DummyKeyStore is a read-only KeyStore that neither loads nor stores keys.
type DummyKeyStore struct {
}

// ReadOnly returns true if this KeyStore is read only, false otherwise.
// If ReadOnly is true then StoreKey will fail.
func (ks *DummyKeyStore) ReadOnly() bool {
	return true
}

// GetKey returns a key object whose SKI is the one passed.
func (ks *DummyKeyStore) GetKey(ski []byte) (k bccsp.Key, err error) {
	return nil, errors.New("Key not found. This is a dummy KeyStore")
}

// StoreKey stores the key k in this KeyStore.
// If this KeyStore is read only then the method will fail.
func (ks *DummyKeyStore) StoreKey(k bccsp.Key) (err error) {
	return errors.New("Cannot store key. This is a dummy read-only KeyStore")
}
