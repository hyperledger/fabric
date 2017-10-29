/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/json"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"
	"github.com/pkg/errors"
)

// the functions below show some best practices on how
// to use entities to perform cryptographic operations
// over the ledger state

// getStateAndDecrypt retrieves the value associated to key,
// decrypts it with the supplied entity and returns the result
// of the decryption
func getStateAndDecrypt(stub shim.ChaincodeStubInterface, ent entities.Encrypter, key string) ([]byte, error) {
	// at first we retrieve the ciphertext from the ledger
	ciphertext, err := stub.GetState(key)
	if err != nil {
		return nil, err
	}

	// GetState will return a nil slice if the key does not exist.
	// Note that the chaincode logic may want to distinguish between
	// nil slice (key doesn't exist in state db) and empty slice
	// (key found in state db but value is empty). We do not
	// distinguish the case here
	if len(ciphertext) == 0 {
		return nil, errors.New("no ciphertext to decrypt")
	}

	return ent.Decrypt(ciphertext)
}

// encryptAndPutState encrypts the supplied value using the
// supplied entity and puts it to the ledger associated to
// the supplied KVS key
func encryptAndPutState(stub shim.ChaincodeStubInterface, ent entities.Encrypter, key string, value []byte) error {
	// at first we use the supplied entity to encrypt the value
	ciphertext, err := ent.Encrypt(value)
	if err != nil {
		return err
	}

	return stub.PutState(key, ciphertext)
}

// getStateDecryptAndVerify retrieves the value associated to key,
// decrypts it with the supplied entity, verifies the signature
// over it and returns the result of the decryption in case of
// success
func getStateDecryptAndVerify(stub shim.ChaincodeStubInterface, ent entities.EncrypterSignerEntity, key string) ([]byte, error) {
	// here we retrieve and decrypt the state associated to key
	val, err := getStateAndDecrypt(stub, ent, key)
	if err != nil {
		return nil, err
	}

	// we unmarshal a SignedMessage from the decrypted state
	msg := &entities.SignedMessage{}
	err = msg.FromBytes(val)
	if err != nil {
		return nil, err
	}

	// we verify the signature
	ok, err := msg.Verify(ent)
	if err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("invalid signature")
	}

	return msg.Payload, nil
}

// signEncryptAndPutState signs the supplied value, encrypts
// the supplied value together with its signature using the
// supplied entity and puts it to the ledger associated to
// the supplied KVS key
func signEncryptAndPutState(stub shim.ChaincodeStubInterface, ent entities.EncrypterSignerEntity, key string, value []byte) error {
	// here we create a SignedMessage, set its payload
	// to value and the ID of the entity and
	// sign it with the entity
	msg := &entities.SignedMessage{Payload: value, ID: []byte(ent.ID())}
	err := msg.Sign(ent)
	if err != nil {
		return err
	}

	// here we serialize the SignedMessage
	b, err := msg.ToBytes()
	if err != nil {
		return err
	}

	// here we encrypt the serialized version associated to args[0]
	return encryptAndPutState(stub, ent, key, b)
}

type keyValuePair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// getStateByRangeAndDecrypt retrieves a range of KVS pairs from the
// ledger and decrypts each value with the supplied entity; it returns
// a json-marshalled slice of keyValuePair
func getStateByRangeAndDecrypt(stub shim.ChaincodeStubInterface, ent entities.Encrypter, startKey, endKey string) ([]byte, error) {
	// we call get state by range to go through the entire range
	iterator, err := stub.GetStateByRange(startKey, endKey)
	if err != nil {
		return nil, err
	}
	defer iterator.Close()

	// we decrypt each entry - the assumption is that they have all been encrypted with the same key
	keyvalueset := []keyValuePair{}
	for iterator.HasNext() {
		el, err := iterator.Next()
		if err != nil {
			return nil, err
		}

		v, err := ent.Decrypt(el.Value)
		if err != nil {
			return nil, err
		}

		keyvalueset = append(keyvalueset, keyValuePair{el.Key, string(v)})
	}

	bytes, err := json.Marshal(keyvalueset)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}
