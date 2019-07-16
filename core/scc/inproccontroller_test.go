/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

type MockShim struct{}

func (MockShim) Init(stub shim.ChaincodeStubInterface) pb.Response   { return pb.Response{} }
func (MockShim) Invoke(stub shim.ChaincodeStubInterface) pb.Response { return pb.Response{} }

func TestError(t *testing.T) {
	err := SysCCRegisteredErr("error")

	assert.Regexp(t, "already registered", err.Error(), "message should be correct")
}

func TestRegisterSuccess(t *testing.T) {
	r := NewRegistry()
	shim := MockShim{}
	err := r.Register(ccintf.CCID("name"), shim)

	assert.Nil(t, err, "err should be nil")
	assert.Equal(t, r.chaincodes["name"], shim, "shim should be correct")
}

func TestRegisterError(t *testing.T) {
	r := NewRegistry()
	r.chaincodes["name"] = MockShim{}
	shim := MockShim{}
	err := r.Register(ccintf.CCID("name"), shim)

	assert.NotNil(t, err, "err should not be nil")
}

type MockCCSupport struct{}

func (ccs MockCCSupport) HandleChaincodeStream(stream ccintf.ChaincodeStream) error {
	return nil
}

func (ccs MockCCSupport) LaunchInProc(ccid ccintf.CCID) <-chan struct{} {
	result := make(chan struct{})
	close(result)
	return result
}
