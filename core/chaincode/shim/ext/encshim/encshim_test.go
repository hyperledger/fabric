/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package encshim

import (
	"bytes"
	"testing"

	"github.com/hyperledger/fabric/bccsp/mocks"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

type mockCC struct{}

func (*mockCC) Init(stub shim.ChaincodeStubInterface) peer.Response {
	return shim.Success(nil)
}

func (*mockCC) Invoke(stub shim.ChaincodeStubInterface) peer.Response {
	return shim.Success(nil)
}

func TestEncShimBad(t *testing.T) {
	// no shim
	_, err := NewEncShim(nil)
	assert.Error(t, err)
}

func TestEncShimMockBCCSP(t *testing.T) {
	// get a mock shim
	cc := new(mockCC)
	stub := shim.NewMockStub("cc", cc)

	// get a mock bccsp
	bccsp := &mocks.MockBCCSP{KeyImportValue: &mocks.MockKey{Symm: false, Pvt: true}}

	// generate a mock key
	encKey, err := bccsp.KeyImport(nil, nil)
	assert.NoError(t, err)

	// generate an entity
	e, err := entities.NewEncrypterEntity("ORG", bccsp, encKey, &mocks.EncrypterOpts{}, &mocks.DecrypterOpts{})

	// create the encrypted shim
	eshim, err := NewEncShim(stub)
	assert.NoError(t, err)

	key := "key"
	val := []byte("value")

	stub.MockTransactionStart("1")
	defer stub.MockTransactionEnd("1")
	err = eshim.With(e).PutState(key, val)
	assert.NoError(t, err)

	vb, err := eshim.With(e).GetState(key)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(vb, val))

	// Now we test that calling encshim methods with nil entities won't crash and burn
	_, err = eshim.With(nil).GetState(key)
	assert.Error(t, err)
	err = eshim.With(nil).PutState(key, val)
	assert.Error(t, err)
}

func TestEncShimRealBCCSP(t *testing.T) {
	// get a mock shim
	cc := new(mockCC)
	stub := shim.NewMockStub("cc", cc)

	decEnt, err := entities.GetEncrypterEntityForTest("ALICE")
	assert.NoError(t, err)
	assert.NotNil(t, decEnt)

	encEnt1, err := decEnt.Public()
	assert.NoError(t, err)
	assert.NotNil(t, encEnt1)
	encEnt := encEnt1.(entities.EncrypterEntity)

	// create the encrypted shim
	eshim, err := NewEncShim(stub)
	assert.NoError(t, err)

	key := "key"
	val := []byte("value")

	// test encryption
	stub.MockTransactionStart("1")
	defer stub.MockTransactionEnd("1")
	err = eshim.With(encEnt).PutState(key, val)
	assert.NoError(t, err)

	// test successful decryption
	vb, err := eshim.With(decEnt).GetState(key)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(vb, val))

	// **********************************************
	// NOTE: these tests have been commented out on
	// purpose: BCCSP currently does not support
	// authenticated encryption, and so decrypting
	// with a "wrong" key is not always guaranteed
	// to fail. The only reason for failure is a
	// padding error on decryption, which isn't
	// guaranteed to happen. As soon as we support
	// authenticated encryption these tests can be
	// re-enabled because then we'll have guaranteed
	// decryption failure when decryption is attempted
	// with the wrong key
	// **********************************************
	//// test failed decryption - using the wrong key
	//_, err = eshim.With(decEntBad).GetState(key)
	//assert.Error(t, err)
	//
	//// test failed decryption - using the wrong entity
	//_, err = eshim.With(encEnt).GetState(key)
	//assert.Error(t, err)

	// test failed decryption - non-existing key
	_, err = eshim.With(encEnt).GetState("barf")
	assert.Error(t, err)
	_ = err.Error()
}
