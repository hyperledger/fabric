/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inproccontroller

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/stretchr/testify/assert"
)

type MockShim struct {
}

func (shim MockShim) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return pb.Response{}
}

func (shim MockShim) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	return pb.Response{}
}

func TestError(t *testing.T) {
	err := SysCCRegisteredErr("error")

	assert.Regexp(t, "already registered", err.Error(), "message should be correct")
}

func TestRegisterSuccess(t *testing.T) {
	r := NewRegistry()
	shim := MockShim{}
	err := r.Register(&ccintf.CCID{Name: "name"}, shim)

	assert.Nil(t, err, "err should be nil")
	assert.Equal(t, r.typeRegistry["name"].chaincode, shim, "shim should be correct")
}

func TestRegisterError(t *testing.T) {
	r := NewRegistry()
	r.typeRegistry["name"] = &inprocContainer{}
	shim := MockShim{}
	err := r.Register(&ccintf.CCID{Name: "name"}, shim)

	assert.NotNil(t, err, "err should not be nil")
}

type AStruct struct {
}

type AInterface interface {
	test()
}

func (as AStruct) test() {}

type MockContext struct {
}

func (c MockContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, true
}

func (c MockContext) Done() <-chan struct{} {
	ch := make(<-chan struct{}, 1)
	return ch
}

func (c MockContext) Err() error {
	return nil
}

func (c MockContext) Value(key interface{}) interface{} {
	return MockCCSupport{}
}

func TestGetInstanceChaincodeDoesntExist(t *testing.T) {
	mockContext := MockContext{}
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	r := NewRegistry()
	vm := NewInprocVM(r)
	args := []string{"a", "b"}
	env := []string{"a", "b"}
	container, err := vm.getInstance(mockContext, mockInprocContainer, "instName", args, env)
	assert.NotNil(t, container, "container should not be nil")
	assert.Nil(t, err, "err should be nil")

	if _, ok := r.instRegistry["instName"]; !ok {
		t.Error("correct key hasnt been set on instRegistry")
	}
}

func TestGetInstaceChaincodeExists(t *testing.T) {
	mockContext := MockContext{}
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	r := NewRegistry()
	vm := NewInprocVM(r)
	args := []string{"a", "b"}
	env := []string{"a", "b"}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	r.instRegistry["instName"] = ipc

	container, err := vm.getInstance(mockContext, mockInprocContainer, "instName", args, env)
	assert.NotNil(t, container, "container should not be nil")
	assert.Nil(t, err, "err should be nil")

	assert.Equal(t, r.instRegistry["instName"], ipc, "instRegistry[instName] should contain the correct value")
}

type MockReader struct {
}

func (r MockReader) Read(p []byte) (n int, err error) {
	return 1, nil
}

type MockCCSupport struct {
}

func (ccs MockCCSupport) HandleChaincodeStream(ctx context.Context, stream ccintf.ChaincodeStream) error {
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
	mockContext := MockContext{}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	_ = &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
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
	mockContext := MockContext{}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
		args:      ipcArgs,
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	r.typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
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
	mockContext := MockContext{}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
		env:       ipcEnv,
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	r.typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
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
	mockContext := MockContext{}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	r.typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
	assert.Nil(t, err, "err should be nil")
	<-done
}

type MockCCSupportErr struct {
}

func (ccs MockCCSupportErr) HandleChaincodeStream(ctx context.Context, stream ccintf.ChaincodeStream) error {
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
	mockContext := MockContext{}

	r := NewRegistry()
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	r.typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupportErr{})
	assert.Nil(t, err, "err should be nil")
}

func TestStart(t *testing.T) {
	mockContext := MockContext{}
	r := NewRegistry()
	vm := NewInprocVM(r)

	ccid := ccintf.CCID{Name: "name"}
	mockInprocContainer := &inprocContainer{}

	args := []string{"a", "b"}
	env := []string{"a", "b"}
	files := map[string][]byte{
		"hello": []byte("world"),
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	r.typeRegistry["name"] = ipc

	err := vm.Start(mockContext, ccid, args, env, files, nil)
	assert.Nil(t, err, "err should be nil")
}

func TestStop(t *testing.T) {
	mockContext := MockContext{}
	r := NewRegistry()
	vm := NewInprocVM(r)

	ccid := ccintf.CCID{
		Name:    "name",
		Version: "1",
	}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	stopChan := make(chan struct{})
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: stopChan}
	ipc.running = true

	r.typeRegistry["name-1"] = ipc
	r.instRegistry["name-1"] = ipc

	go func() {
		err := vm.Stop(mockContext, ccid, 1000, true, true)
		assert.Nil(t, err, "err should be nil")
	}()

	msg := <-stopChan
	assert.NotNil(t, msg, "msg should not be nil")
}

func TestStopNoIPCTemplate(t *testing.T) {
	mockContext := MockContext{}
	r := NewRegistry()
	vm := NewInprocVM(r)

	ccid := ccintf.CCID{
		Name:    "name",
		Version: "1",
	}

	err := vm.Stop(mockContext, ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name-1 not registered", "error should be correct")
}

func TestStopNoIPC(t *testing.T) {
	mockContext := MockContext{}
	r := NewRegistry()
	vm := NewInprocVM(r)

	ccid := ccintf.CCID{
		Name:    "name",
		Version: "1",
	}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	stopChan := make(chan struct{})
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: stopChan}

	r.typeRegistry["name-1"] = ipc

	err := vm.Stop(mockContext, ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name-1 not found", "error should be correct")
}

func TestStopIPCNotRunning(t *testing.T) {
	mockContext := MockContext{}
	r := NewRegistry()
	vm := NewInprocVM(r)

	ccid := ccintf.CCID{
		Name:    "name",
		Version: "1",
	}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	stopChan := make(chan struct{})
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: stopChan}

	r.typeRegistry["name-1"] = ipc
	r.instRegistry["name-1"] = ipc

	err := vm.Stop(mockContext, ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name-1 not running", "error should be correct")
}
