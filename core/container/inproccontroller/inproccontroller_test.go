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
	r.ChaincodeSupport = MockCCSupport{}
	shim := MockShim{}
	err := r.Register(ccintf.CCID("name"), shim)

	assert.Nil(t, err, "err should be nil")
	assert.Equal(t, r.chaincodes["name"], shim, "shim should be correct")
}

func TestRegisterError(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	r.chaincodes["name"] = MockShim{}
	shim := MockShim{}
	err := r.Register(ccintf.CCID("name"), shim)

	assert.NotNil(t, err, "err should not be nil")
}

func TestGetInstanceChaincodeDoesntExist(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}
	args := []string{"a", "b"}
	env := []string{"a", "b"}
	container, err := vm.getInstance(MockShim{}, "instName", args, env)
	assert.NotNil(t, container, "container should not be nil")
	assert.Nil(t, err, "err should be nil")

	if _, ok := r.instRegistry["instName"]; !ok {
		t.Error("correct key hasnt been set on instRegistry")
	}
}

func TestGetInstaceChaincodeExists(t *testing.T) {
	var chaincode MockShim

	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}
	args := []string{"a", "b"}
	env := []string{"a", "b"}

	ipc := &inprocContainer{args: args, env: env, chaincode: chaincode, stopChan: make(chan struct{})}
	r.instRegistry["instName"] = ipc

	container, err := vm.getInstance(chaincode, "instName", args, env)
	assert.NotNil(t, container, "container should not be nil")
	assert.Nil(t, err, "err should be nil")

	assert.Equal(t, r.instRegistry["instName"], ipc, "instRegistry[instName] should contain the correct value")
}

type MockCCSupport struct{}

func (ccs MockCCSupport) HandleChaincodeStream(stream ccintf.ChaincodeStream) error {
	return nil
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
		ChaincodeSupport: MockCCSupport{},
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
	r.ChaincodeSupport = MockCCSupport{}
	mockInprocContainer := &inprocContainer{
		chaincode:        MockShim{},
		args:             ipcArgs,
		ChaincodeSupport: MockCCSupport{},
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
	r.ChaincodeSupport = MockCCSupport{}
	mockInprocContainer := &inprocContainer{
		ChaincodeSupport: MockCCSupport{},
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
	r.ChaincodeSupport = MockCCSupport{}
	mockInprocContainer := &inprocContainer{
		ChaincodeSupport: MockCCSupport{},
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
	r.ChaincodeSupport = MockCCSupport{}
	mockInprocContainer := &inprocContainer{
		ChaincodeSupport: MockCCSupportErr{},
		chaincode:        MockShim{},
	}

	r.chaincodes["path"] = MockShim{}

	err := mockInprocContainer.launchInProc("ID", args, env)
	assert.Nil(t, err, "err should be nil")
}

func TestStart(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}

	ccid := ccintf.CCID("name")

	args := []string{"a", "b"}
	env := []string{"a", "b"}
	files := map[string][]byte{
		"hello": []byte("world"),
	}

	r.chaincodes["name"] = MockShim{}

	err := vm.Start(ccid, args, env, files, nil)
	assert.Nil(t, err, "err should be nil")
}

func TestStop(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}

	ccid := ccintf.CCID("name:1")

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	stopChan := make(chan struct{})
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: stopChan}
	ipc.running = true

	r.chaincodes["name:1"] = MockShim{}
	r.instRegistry["name:1"] = ipc

	go func() {
		err := vm.Stop(ccid, 1000, true, true)
		assert.Nil(t, err, "err should be nil")
	}()

	_, ok := <-stopChan
	assert.False(t, ok, "channel should be closed")
}

func TestStopNoIPCTemplate(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}

	ccid := ccintf.CCID("name:1")

	err := vm.Stop(ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name:1 not registered", "error should be correct")
}

func TestStopNoIPC(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}

	ccid := ccintf.CCID("name:1")

	r.chaincodes["name:1"] = MockShim{}

	err := vm.Stop(ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name:1 not found", "error should be correct")
}

func TestStopIPCNotRunning(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}

	ccid := ccintf.CCID("name:1")

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	stopChan := make(chan struct{})
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: stopChan}

	r.chaincodes["name:1"] = MockShim{}
	r.instRegistry["name:1"] = ipc

	err := vm.Stop(ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name:1 not running", "error should be correct")
}

func TestWait(t *testing.T) {
	r := NewRegistry()
	r.ChaincodeSupport = MockCCSupport{}
	vm := InprocVM{registry: r}

	closed := make(chan struct{})
	close(closed)
	ipc := &inprocContainer{chaincode: MockShim{}, stopChan: closed}
	ipc.running = true

	ccid := ccintf.CCID("name:1")
	r.chaincodes["name:1"] = MockShim{}
	r.instRegistry["name:1"] = ipc

	exitCode, err := vm.Wait(ccid)
	assert.Equal(t, 0, exitCode)
	assert.NoError(t, err)

	_, err = vm.Wait(ccintf.CCID("name:2"))
	assert.EqualError(t, err, "name:2 not found")
}
