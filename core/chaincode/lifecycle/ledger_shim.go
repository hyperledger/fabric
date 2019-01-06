/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/hyperledger/fabric/core/chaincode/shim"

	"github.com/pkg/errors"
)

// StateIteratorToMap takes an iterator, and iterates over the entire thing, encoding the KVs
// into a map, and then closes it.
func StateIteratorToMap(itr shim.StateQueryIteratorInterface) (map[string][]byte, error) {
	defer itr.Close()
	result := map[string][]byte{}
	for itr.HasNext() {
		entry, err := itr.Next()
		if err != nil {
			return nil, errors.WithMessage(err, "could not iterate over range")
		}
		result[entry.Key] = entry.Value
	}
	return result, nil
}

// ChaincodePublicLedgerShim decorates the chaincode shim to support the state interfaces
// required by the serialization code.
type ChaincodePublicLedgerShim struct {
	shim.ChaincodeStubInterface
}

// GetStateRange performs a range query for keys beginning with a particular prefix, and
// returns it as a map. This function assumes that keys contains only ascii chars from \x00 to \x7e.
func (cls *ChaincodePublicLedgerShim) GetStateRange(prefix string) (map[string][]byte, error) {
	itr, err := cls.GetStateByRange(prefix, prefix+"\x7f")
	if err != nil {
		return nil, errors.WithMessage(err, "could not get state iterator")
	}
	return StateIteratorToMap(itr)
}

// ChaincodePrivateLedgerShim wraps the chaincode shim to make access to keys in a collection
// have the same semantics as normal public keys.
type ChaincodePrivateLedgerShim struct {
	Stub       shim.ChaincodeStubInterface
	Collection string
}

// GetStateRange performs a range query in the configured collection for all keys beginning
// with a particular prefix.  This function assumes that keys contains only ascii chars from \x00 to \x7e.
func (cls *ChaincodePrivateLedgerShim) GetStateRange(prefix string) (map[string][]byte, error) {
	itr, err := cls.Stub.GetPrivateDataByRange(cls.Collection, prefix, prefix+"\x7f")
	if err != nil {
		return nil, errors.WithMessage(err, "could not get state iterator")
	}
	return StateIteratorToMap(itr)
}

// GetState returns the value for the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) GetState(key string) ([]byte, error) {
	return cls.Stub.GetPrivateData(cls.Collection, key)
}

// GetStateHash return the hash of the pre-image for the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) GetStateHash(key string) ([]byte, error) {
	return cls.Stub.GetPrivateDataHash(cls.Collection, key)
}

// PutState sets the value for the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) PutState(key string, value []byte) error {
	return cls.Stub.PutPrivateData(cls.Collection, key, value)
}

// DelState deletes the key in the configured collection.
func (cls *ChaincodePrivateLedgerShim) DelState(key string) error {
	return cls.Stub.DelPrivateData(cls.Collection, key)
}
