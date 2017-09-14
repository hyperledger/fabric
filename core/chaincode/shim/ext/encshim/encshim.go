/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encshim

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"
	"github.com/pkg/errors"
)

type encShimImpl struct {
	stub shim.ChaincodeStubInterface
	ent  entities.Encrypter
}

func NewEncShim(stub shim.ChaincodeStubInterface) (EncShim, error) {
	if stub == nil {
		return nil, errors.New("NewEncShim error, nil stub")
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
		return nil, errors.New("nil entity, With should have been called")
	}

	ciphertext, err := s.stub.GetState(key)
	if err != nil {
		return nil, errors.WithMessage(err, "GetState error, stub.GetState returned")
	} else if len(ciphertext) == 0 {
		return nil, &NilKeyError{key: key}
	}

	return s.ent.Decrypt(ciphertext)
}

func (s *encShimImpl) PutState(key string, value []byte) error {
	// stub is guaranteed to be valid by the constructor

	if s.ent == nil {
		return errors.New("nil entity, With should have been called")
	}

	ciphertext, err := s.ent.Encrypt(value)
	if err != nil {
		return errors.WithMessage(err, "PutState error, enc.Encrypt returned")
	}

	err = s.stub.PutState(key, ciphertext)
	if err != nil {
		return errors.WithMessage(err, "PutState error, stub.PutState returned")
	}

	return nil
}

func (s *encShimImpl) With(e entities.Encrypter) EncShim {
	s.ent = e

	return s
}
