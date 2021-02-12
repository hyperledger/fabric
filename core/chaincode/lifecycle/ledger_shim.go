/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	validatorstate "github.com/hyperledger/fabric/core/handlers/validation/api/state"
	"github.com/hyperledger/fabric/core/ledger"

	"github.com/pkg/errors"
)

type StateIterator interface {
	Close() error
	Next() (*queryresult.KV, error)
}

// StateIteratorToMap takes an iterator, and iterates over the entire thing, encoding the KVs
// into a map, and then closes it.
func StateIteratorToMap(itr StateIterator) (map[string][]byte, error) {
	defer itr.Close()
	result := map[string][]byte{}
	for {
		entry, err := itr.Next()
		if err != nil {
			return nil, errors.WithMessage(err, "could not iterate over range")
		}
		if entry == nil {
			return result, nil
		}
		result[entry.Key] = entry.Value
	}
}

// ChaincodePublicLedgerShim decorates the chaincode shim to support the state interfaces
// required by the serialization code.
type ChaincodePublicLedgerShim struct {
	shim.ChaincodeStubInterface
}

// GetStateRange performs a range query for keys beginning with a particular prefix, and
// returns it as a map. This function assumes that keys contain only ascii chars from \x00 to \x7e.
func (cls *ChaincodePublicLedgerShim) GetStateRange(prefix string) (map[string][]byte, error) {
	itr, err := cls.GetStateByRange(prefix, prefix+"\x7f")
	if err != nil {
		return nil, errors.WithMessage(err, "could not get state iterator")
	}
	return StateIteratorToMap(&ChaincodeResultIteratorShim{ResultsIterator: itr})
}

type ChaincodeResultIteratorShim struct {
	ResultsIterator shim.StateQueryIteratorInterface
}

func (cris *ChaincodeResultIteratorShim) Next() (*queryresult.KV, error) {
	if !cris.ResultsIterator.HasNext() {
		return nil, nil
	}
	return cris.ResultsIterator.Next()
}

func (cris *ChaincodeResultIteratorShim) Close() error {
	return cris.ResultsIterator.Close()
}

// ChaincodePrivateLedgerShim wraps the chaincode shim to make access to keys in a collection
// have the same semantics as normal public keys.
type ChaincodePrivateLedgerShim struct {
	Stub       shim.ChaincodeStubInterface
	Collection string
}

// GetStateRange performs a range query in the configured collection for all keys beginning
// with a particular prefix.  This function assumes that keys contain only ascii chars from \x00 to \x7e.
func (cls *ChaincodePrivateLedgerShim) GetStateRange(prefix string) (map[string][]byte, error) {
	itr, err := cls.Stub.GetPrivateDataByRange(cls.Collection, prefix, prefix+"\x7f")
	if err != nil {
		return nil, errors.WithMessage(err, "could not get state iterator")
	}
	return StateIteratorToMap(&ChaincodeResultIteratorShim{ResultsIterator: itr})
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

func (cls *ChaincodePrivateLedgerShim) CollectionName() string {
	return cls.Collection
}

// SimpleQueryExecutorShim implements the ReadableState and RangeableState interfaces
// based on an underlying ledger.SimpleQueryExecutor
type SimpleQueryExecutorShim struct {
	Namespace           string
	SimpleQueryExecutor ledger.SimpleQueryExecutor
}

func (sqes *SimpleQueryExecutorShim) GetState(key string) ([]byte, error) {
	return sqes.SimpleQueryExecutor.GetState(sqes.Namespace, key)
}

func (sqes *SimpleQueryExecutorShim) GetStateRange(prefix string) (map[string][]byte, error) {
	itr, err := sqes.SimpleQueryExecutor.GetStateRangeScanIterator(sqes.Namespace, prefix, prefix+"\x7f")
	if err != nil {
		return nil, errors.WithMessage(err, "could not get state iterator")
	}
	return StateIteratorToMap(&ResultsIteratorShim{ResultsIterator: itr})
}

type ResultsIteratorShim struct {
	ResultsIterator commonledger.ResultsIterator
}

func (ris *ResultsIteratorShim) Next() (*queryresult.KV, error) {
	res, err := ris.ResultsIterator.Next()
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return res.(*queryresult.KV), err
}

func (ris *ResultsIteratorShim) Close() error {
	ris.ResultsIterator.Close()
	return nil
}

type ValidatorStateShim struct {
	ValidatorState validatorstate.State
	Namespace      string
}

func (vss *ValidatorStateShim) GetState(key string) ([]byte, error) {
	result, err := vss.ValidatorState.GetStateMultipleKeys(vss.Namespace, []string{key})
	if err != nil {
		return nil, errors.WithMessage(err, "could not get state thought validatorstate shim")
	}
	return result[0], nil
}

type PrivateQueryExecutor interface {
	GetPrivateDataHash(namespace, collection, key string) (value []byte, err error)
}

type PrivateQueryExecutorShim struct {
	Namespace  string
	Collection string
	State      PrivateQueryExecutor
}

func (pqes *PrivateQueryExecutorShim) GetStateHash(key string) ([]byte, error) {
	return pqes.State.GetPrivateDataHash(pqes.Namespace, pqes.Collection, key)
}

func (pqes *PrivateQueryExecutorShim) CollectionName() string {
	return pqes.Collection
}

// DummyQueryExecutorShim implements the ReadableState interface. It is
// used to ensure channel-less system chaincode calls don't panic and return
// and error when an invalid operation is attempted (i.e. an InstallChaincode
// invocation against a chaincode other than _lifecycle)
type DummyQueryExecutorShim struct{}

func (*DummyQueryExecutorShim) GetState(key string) ([]byte, error) {
	return nil, errors.New("invalid channel-less operation")
}
