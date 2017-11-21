/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inproccontroller

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/net/context"

	"io"

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
	oldTypeRegistry := typeRegistry
	defer func() {
		typeRegistry = oldTypeRegistry
	}()

	shim := MockShim{}
	err := Register("path", shim)

	assert.Nil(t, err, "err should be nil")
	assert.Equal(t, typeRegistry["path"].chaincode, shim, "shim should be correct")
}

func TestRegisterError(t *testing.T) {
	oldTypeRegistry := typeRegistry
	defer func() {
		typeRegistry = oldTypeRegistry
	}()
	typeRegistry["path"] = &inprocContainer{}
	shim := MockShim{}
	err := Register("path", shim)

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
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()

	mockContext := MockContext{}
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	vm := InprocVM{}
	args := []string{"a", "b"}
	env := []string{"a", "b"}
	container, err := vm.getInstance(mockContext, mockInprocContainer, "instName", args, env)
	assert.NotNil(t, container, "container should not be nil")
	assert.Nil(t, err, "err should be nil")

	if _, ok := instRegistry["instName"]; !ok {
		t.Error("correct key hasnt been set on instRegistry")
	}
}

func TestGetInstaceChaincodeExists(t *testing.T) {
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()

	mockContext := MockContext{}
	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	vm := InprocVM{}
	args := []string{"a", "b"}
	env := []string{"a", "b"}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	instRegistry["instName"] = ipc

	container, err := vm.getInstance(mockContext, mockInprocContainer, "instName", args, env)
	assert.NotNil(t, container, "container should not be nil")
	assert.Nil(t, err, "err should be nil")

	assert.Equal(t, instRegistry["instName"], ipc, "instRegistry[instName] should contain the correct value")
}

type MockReader struct {
}

func (r MockReader) Read(p []byte) (n int, err error) {
	return 1, nil
}

func TestDeployNotRegistered(t *testing.T) {

	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()

	mockContext := MockContext{}
	vm := InprocVM{
		id: "path",
	}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
			},
		},
	}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}
	mockReader := MockReader{}
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}
	instRegistry["instName"] = ipc

	err := vm.Deploy(mockContext, ccid, args, env, mockReader)

	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "path not registered. Please register the system chaincode in inprocinstances.go", "error message should be correct")
}

func TestDeployNoChaincodeInstance(t *testing.T) {
	mockContext := MockContext{}
	vm := InprocVM{}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
			},
		},
	}

	mockInprocContainer := &inprocContainer{}

	args := []string{"a", "b"}
	env := []string{"a", "b"}
	mockReader := MockReader{}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := vm.Deploy(mockContext, ccid, args, env, mockReader)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "path system chaincode does not contain chaincode instance")
}

func TestDeployChaincode(t *testing.T) {
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()
	mockContext := MockContext{}
	vm := InprocVM{}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
			},
		},
	}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}
	mockReader := MockReader{}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := vm.Deploy(mockContext, ccid, args, env, mockReader)
	assert.Nil(t, err, "err should be nil")
}

type MockCCSupport struct {
}

func (ccs MockCCSupport) HandleChaincodeStream(ctx context.Context, stream ccintf.ChaincodeStream) error {
	return nil
}
func TestLaunchInprocCCSupportChan(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
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

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
	assert.Nil(t, err, "err should be nil")
}

func TestLaunchprocNoArgs(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
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

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
		args:      ipcArgs,
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
	assert.Nil(t, err, "err should be nil")
}

func TestLaunchprocNoEnv(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
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

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
		env:       ipcEnv,
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
	assert.Nil(t, err, "err should be nil")
}

func TestLaunchprocShimStartInProcErr(t *testing.T) {
	oldShimStartInProc := _shimStartInProc
	oldInprocLoggerErrorf := _inprocLoggerErrorf
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
		_shimStartInProc = oldShimStartInProc
		_inprocLoggerErrorf = oldInprocLoggerErrorf
	}()

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	_shimStartInProc = func(env []string, args []string, cc shim.Chaincode, recv <-chan *pb.ChaincodeMessage, send chan<- *pb.ChaincodeMessage) error {
		return errors.New("error")
	}

	_inprocLoggerErrorfCounter := 0
	_inprocLoggerErrorf = func(format string, args ...interface{}) {
		_inprocLoggerErrorfCounter = _inprocLoggerErrorfCounter + 1

		if _inprocLoggerErrorfCounter == 2 {
			assert.Equal(t, format, "%s", "Format is correct")
			assert.Equal(t, args[0], "chaincode-support ended with err: error", "content is correct")
		}
	}
	mockContext := MockContext{}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupport{})
	assert.Nil(t, err, "err should be nil")
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
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
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

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := mockInprocContainer.launchInProc(mockContext, "ID", args, env, MockCCSupportErr{})
	assert.Nil(t, err, "err should be nil")
}

type MockIOReader struct{}

func (io MockIOReader) Read(p []byte) (int, error) {
	return 0, nil
}

func TestStart(t *testing.T) {
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()
	mockContext := MockContext{}
	vm := InprocVM{}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
			},
		},
	}

	mockInprocContainer := &inprocContainer{}

	args := []string{"a", "b"}
	env := []string{"a", "b"}
	files := map[string][]byte{
		"hello": []byte("world"),
	}

	mockBuilder := func() (io.Reader, error) {
		return MockIOReader{}, nil
	}

	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: make(chan struct{})}

	typeRegistry["path"] = ipc

	err := vm.Start(mockContext, ccid, args, env, files, mockBuilder, nil)
	assert.Nil(t, err, "err should be nil")
}

func TestStop(t *testing.T) {
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()
	mockContext := MockContext{}
	vm := InprocVM{}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
				Name: "name",
			},
		},
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

	typeRegistry["path"] = ipc
	instRegistry["name-1"] = ipc

	go func() {
		err := vm.Stop(mockContext, ccid, 1000, true, true)
		assert.Nil(t, err, "err should be nil")
	}()

	msg := <-stopChan
	assert.NotNil(t, msg, "msg should not be nil")
}

func TestStopNoIPCTemplate(t *testing.T) {
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()
	mockContext := MockContext{}
	vm := InprocVM{}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
				Name: "name",
			},
		},
		Version: "1",
	}

	err := vm.Stop(mockContext, ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "path not registered", "error should be correct")
}

func TestStopNoIPC(t *testing.T) {
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()
	mockContext := MockContext{}
	vm := InprocVM{}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
				Name: "name",
			},
		},
		Version: "1",
	}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	stopChan := make(chan struct{})
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: stopChan}

	typeRegistry["path"] = ipc

	err := vm.Stop(mockContext, ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name-1 not found", "error should be correct")
}

func TestStopIPCNotRunning(t *testing.T) {
	defer func() {
		typeRegistry = make(map[string]*inprocContainer)
		instRegistry = make(map[string]*inprocContainer)
	}()
	mockContext := MockContext{}
	vm := InprocVM{}

	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
				Name: "name",
			},
		},
		Version: "1",
	}

	mockInprocContainer := &inprocContainer{
		chaincode: MockShim{},
	}

	args := []string{"a", "b"}
	env := []string{"a", "b"}

	stopChan := make(chan struct{})
	ipc := &inprocContainer{args: args, env: env, chaincode: mockInprocContainer.chaincode, stopChan: stopChan}

	typeRegistry["path"] = ipc
	instRegistry["name-1"] = ipc

	err := vm.Stop(mockContext, ccid, 1000, true, true)
	assert.NotNil(t, err, "err should not be nil")
	assert.Equal(t, err.Error(), "name-1 not running", "error should be correct")
}

func TestDestroy(t *testing.T) {
	vm := InprocVM{}
	mockContext := MockContext{}
	ccid := ccintf.CCID{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Path: "path",
				Name: "name",
			},
		},
		Version: "1",
	}
	err := vm.Destroy(mockContext, ccid, true, true)
	assert.Nil(t, err, "err should be nil")
}
