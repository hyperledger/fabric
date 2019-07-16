/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inproccontroller

import (
	"errors"
	"fmt"
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

func TestLaunchInprocCCSupportChan(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	defer func() {
		_shimStartInProc = oldShimStartInProc
	}()

	_shimStartInProc = func(env []string, args []string, cc shim.Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
		send <- &pb.ChaincodeMessage{}
		return nil
	}

	mockInprocContainer := &inprocContainer{
		chaincode:        MockShim{},
		chaincodeSupport: MockCCSupport{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	_ = &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	err := mockInprocContainer.launchInProc("ID", args, env)
	assert.Nil(t, err, "err should be nil")
}

func TestLaunchprocNoArgs(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	defer func() {
		_shimStartInProc = oldShimStartInProc
	}()

	var args []string
	ipcArgs := []string{"c", "d"}
	env := []string{"a", "b"}

	_shimStartInProc = func(env []string, args []string, cc shim.Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
		send <- &pb.ChaincodeMessage{}

		assert.Equal(t, args, ipcArgs, "args should be changed")

		return nil
	}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincode:        MockShim{},
		args:             ipcArgs,
		chaincodeSupport: MockCCSupport{},
	}

	r.chaincodes["path"] = MockShim{}

	err := mockInprocContainer.launchInProc("ID", args, env)
	assert.Nil(t, err, "err should be nil")
}

func TestLaunchprocNoEnv(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	defer func() {
		_shimStartInProc = oldShimStartInProc
	}()

	var env []string
	ipcEnv := []string{"c", "d"}
	args := []string{"a", "b"}

	_shimStartInProc = func(env []string, args []string, cc shim.Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
		send <- &pb.ChaincodeMessage{}

		assert.Equal(t, env, ipcEnv, "args should be changed")

		return nil
	}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincodeSupport: MockCCSupport{},
		chaincode:        MockShim{},
		env:              ipcEnv,
	}

	r.chaincodes["path"] = MockShim{}

	err := mockInprocContainer.launchInProc("ID", args, env)
	assert.Nil(t, err, "err should be nil")
}

func TestLaunchprocShimStartInProcErr(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	oldInprocLoggerErrorf := _inprocLoggerErrorf
	defer func() {
		_shimStartInProc = oldShimStartInProc
		_inprocLoggerErrorf = oldInprocLoggerErrorf
	}()

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	_shimStartInProc = func(env []string, args []string, cc shim.Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
		return errors.New("error")
	}

	done := make(chan struct{})
	_inprocLoggerErrorfCounter := 0
	_inprocLoggerErrorf = func(format string, args ...interface{}) {
		_inprocLoggerErrorfCounter++
		if _inprocLoggerErrorfCounter == 1 {
			assert.Equal(t, format, "%s", "Format is correct")
			assert.Equal(t, fmt.Sprintf("%s", args[0]), "chaincode-support ended with err: error", "content is correct")
			close(done)
		}
	}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincodeSupport: MockCCSupport{},
		chaincode:        MockShim{},
	}

	r.chaincodes["path"] = MockShim{}

	err := mockInprocContainer.launchInProc("ID", args, env)
	assert.Nil(t, err, "err should be nil")
	<-done
}

type MockCCSupportErr struct{}

func (ccs MockCCSupportErr) HandleChaincodeStream(stream ccintf.ChaincodeStream) error {
	return errors.New("errors")
}

func (ccs MockCCSupportErr) LaunchInProc(ccid ccintf.CCID) <-chan struct{} {
	result := make(chan struct{})
	close(result)
	return result
}

func TestLaunchprocCCSupportHandleChaincodeStreamError(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	oldInprocLoggerErrorf := _inprocLoggerErrorf
	defer func() {
		_shimStartInProc = oldShimStartInProc
		_inprocLoggerErrorf = oldInprocLoggerErrorf
	}()

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	_shimStartInProc = func(env []string, args []string, cc shim.Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
		return nil
	}

	_inprocLoggerErrorf = func(format string, args ...interface{}) {
	}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincodeSupport: MockCCSupportErr{},
		chaincode:        MockShim{},
	}

	r.chaincodes["path"] = MockShim{}

	err := mockInprocContainer.launchInProc("ID", args, env)
	assert.Nil(t, err, "err should be nil")
}
