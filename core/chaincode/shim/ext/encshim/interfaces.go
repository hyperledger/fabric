/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encshim

import "github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"

// EncShim is an interface that supports and facilitates
// chaincode-level encryption. It's expected to be used as follows
//
// shimInstance.With(entity).GetState(key)
//
// or
//
// shimInstance.With(entity).PutState(key, value)
//
// where GetState and PutState retrieve data from a backing key-value
// store (e.g. the ledger through the chaincode shim) and perform
// on-the-fly encryption and decryption appropriately according to
// the entity that is specified through the With function
type EncShim interface {
	// GetState returns the value associated to the supplied key
	// after decryption using the entity specified through a
	// previous call to With
	GetState(key string) ([]byte, error)

	// PutState encrypts the specified value using the entity
	// specified through a previous call to With and associates
	// it to the supplied key string
	PutState(key string, value []byte) error

	// With supplies the Encrypter Entity that shall later perform
	// encryption/decryption call required by calls to GetState or PutState
	With(e entities.Encrypter) EncShim
}
