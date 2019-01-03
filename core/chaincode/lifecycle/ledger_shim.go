/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/hyperledger/fabric/core/chaincode/shim"
)

// ChaincodePrivateLedgerShim wraps the chaincode shim to make access to keys in a collection
// have the same semantics as normal public keys.
type ChaincodePrivateLedgerShim struct {
	Stub       shim.ChaincodeStubInterface
	Collection string
}

// GetState returns the value for the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) GetState(key string) ([]byte, error) {
	return cls.Stub.GetPrivateData(cls.Collection, key)
}

// GetStateHash return the hash of the pre-image for the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) GetStateHash(key string) ([]byte, error) {
	// XXX implement me
	panic("unimplemented")
}

// PutState sets the value for the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) PutState(key string, value []byte) error {
	return cls.Stub.PutPrivateData(cls.Collection, key, value)
}

// DelState deletes the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) DelState(key string) error {
	return cls.Stub.DelPrivateData(cls.Collection, key)
}
