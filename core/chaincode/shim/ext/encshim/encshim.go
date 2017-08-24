/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encshim

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"
)

type encShimImpl struct {
	stub shim.ChaincodeStubInterface
	ent  entities.Encrypter
}

func NewEncShim(stub shim.ChaincodeStubInterface) (EncShim, error) {
	if stub == nil {
		return nil, fmt.Errorf("NewEncShim error, nil stub")
	}

	return &encShimImpl{
		stub: stub,
	}, nil
}

type NilKeyError struct {
	key string
}

func (a NilKeyError) Error() string {
	return fmt.Sprintf("GetState error, stub.GetState returned nil value for key %s", a.key)
}

func (s *encShimImpl) GetState(key string) ([]byte, error) {
	// stub is guaranteed to be valid by the constructor

	if s.ent == nil {
		return nil, fmt.Errorf("nil entity, With should have been called")
	}

	ciphertext, err := s.stub.GetState(key)
	if err != nil {
		return nil, fmt.Errorf("GetState error, stub.GetState returned %s", err)
	} else if len(ciphertext) == 0 {
		return nil, &NilKeyError{key: key}
	}

	return s.ent.Decrypt(ciphertext)
}

func (s *encShimImpl) PutState(key string, value []byte) error {
	// stub is guaranteed to be valid by the constructor

	if s.ent == nil {
		return fmt.Errorf("nil entity, With should have been called")
	}

	ciphertext, err := s.ent.Encrypt(value)
	if err != nil {
		return fmt.Errorf("PutState error, enc.Encrypt returned %s", err)
	}

	err = s.stub.PutState(key, ciphertext)
	if err != nil {
		return fmt.Errorf("PutState error, stub.PutState returned %s", err)
	}

	return nil
}

func (s *encShimImpl) With(e entities.Encrypter) EncShim {
	s.ent = e

	return s
}
