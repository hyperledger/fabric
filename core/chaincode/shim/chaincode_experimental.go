// +build experimental

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package shim

import (
	"fmt"
)

// private state functions

// GetPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateData(collection string, key string) ([]byte, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleGetState(collection, key, stub.ChannelId, stub.TxID)
}

// PutPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) PutPrivateData(collection string, key string, value []byte) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	if key == "" {
		return fmt.Errorf("key must not be an empty string")
	}
	return stub.handler.handlePutState(collection, key, value, stub.ChannelId, stub.TxID)
}

// DelPrivateData documentation can be found in interfaces.go
func (stub *ChaincodeStub) DelPrivateData(collection string, key string) error {
	if collection == "" {
		return fmt.Errorf("collection must not be an empty string")
	}
	return stub.handler.handleDelState(collection, key, stub.ChannelId, stub.TxID)
}

// GetPrivateDataByRange documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataByRange(collection, startKey, endKey string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	if startKey == "" {
		startKey = emptyKeySubstitute
	}
	if err := validateSimpleKeys(startKey, endKey); err != nil {
		return nil, err
	}
	return stub.handleGetStateByRange(collection, startKey, endKey)
}

// GetPrivateDataByPartialCompositeKey documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataByPartialCompositeKey(collection, objectType string, attributes []string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	if partialCompositeKey, err := stub.CreateCompositeKey(objectType, attributes); err == nil {
		return stub.handleGetStateByRange(collection, partialCompositeKey, partialCompositeKey+string(maxUnicodeRuneValue))
	} else {
		return nil, err
	}
}

// GetPrivateDataQueryResult documentation can be found in interfaces.go
func (stub *ChaincodeStub) GetPrivateDataQueryResult(collection, query string) (StateQueryIteratorInterface, error) {
	if collection == "" {
		return nil, fmt.Errorf("collection must not be an empty string")
	}
	response, err := stub.handler.handleGetQueryResult(collection, query, stub.ChannelId, stub.TxID)
	if err != nil {
		return nil, err
	}
	return &StateQueryIterator{CommonIterator: &CommonIterator{stub.handler, stub.TxID, stub.ChannelId, response, 0}}, nil
}
