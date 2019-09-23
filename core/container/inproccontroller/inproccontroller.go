/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inproccontroller

import (
	"context"
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// ContainerType is the string which the inproc container type
// is registered with the container.VMController
const ContainerType = "SYSTEM"

type inprocContainer struct {
	ChaincodeSupport ccintf.CCSupport
	chaincode        shim.Chaincode
	running          bool
	args             []string
	env              []string
	stopChan         chan struct{}
}

var (
	inprocLogger = flogging.MustGetLogger("inproccontroller")

	// TODO this is a very hacky way to do testing, we should find other ways
	// to test, or not statically inject these dependencies.
	_shimStartInProc    = shim.StartInProc
	_inprocLoggerErrorf = inprocLogger.Errorf
)

// errors

//SysCCRegisteredErr registered error
type SysCCRegisteredErr string

func (s SysCCRegisteredErr) Error() string {
	return fmt.Sprintf("%s already registered", string(s))
}

// Registry stores registered system chaincodes.
// It implements container.VMProvider and scc.Registrar
type Registry struct {
	mutex        sync.Mutex
	typeRegistry map[string]*inprocContainer
	instRegistry map[string]*inprocContainer

	ChaincodeSupport ccintf.CCSupport
}

// NewRegistry creates an initialized registry, ready to register system chaincodes.
// The returned *Registry is _not_ ready to use as is.  You must set the ChaincodeSupport
// as soon as one is available, before any chaincode invocations occur.  This is because
// the chaincode support used to be a latent dependency, snuck in on the context, but now
// it is being made an explicit part of the startup.
func NewRegistry() *Registry {
	return &Registry{
		typeRegistry: make(map[string]*inprocContainer),
		instRegistry: make(map[string]*inprocContainer),
	}
}

// NewVM creates an inproc VM instance
func (r *Registry) NewVM() container.VM {
	return NewInprocVM(r)
}

//Register registers system chaincode with given path. The deploy should be called to initialize
func (r *Registry) Register(ccid *ccintf.CCID, cc shim.Chaincode) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	name := ccid.GetName()
	inprocLogger.Debugf("Registering chaincode instance: %s", name)
	tmp := r.typeRegistry[name]
	if tmp != nil {
		return SysCCRegisteredErr(name)
	}

	r.typeRegistry[name] = &inprocContainer{chaincode: cc}
	return nil
}

func (r *Registry) getType(name string) *inprocContainer {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.typeRegistry[name]
}

func (r *Registry) getInstance(name string) *inprocContainer {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.instRegistry[name]
}

func (r *Registry) setInstance(name string, inst *inprocContainer) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.instRegistry[name] = inst
}

func (r *Registry) removeInstance(name string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.instRegistry, name)
}

// InprocVM is a vm. It is identified by a executable name
type InprocVM struct {
	id       string
	registry *Registry
}

// NewInprocVM creates a new InprocVM
func NewInprocVM(r *Registry) *InprocVM {
	return &InprocVM{
		registry: r,
	}
}

func (vm *InprocVM) getInstance(ipctemplate *inprocContainer, instName string, args []string, env []string) (*inprocContainer, error) {
	ipc := vm.registry.getInstance(instName)
	if ipc != nil {
		inprocLogger.Warningf("chaincode instance exists for %s", instName)
		return ipc, nil
	}
	ipc = &inprocContainer{
		ChaincodeSupport: vm.registry.ChaincodeSupport,
		args:             args,
		env:              env,
		chaincode:        ipctemplate.chaincode,
		stopChan:         make(chan struct{}),
	}
	vm.registry.setInstance(instName, ipc)
	inprocLogger.Debugf("chaincode instance created for %s", instName)
	return ipc, nil
}

func (ipc *inprocContainer) launchInProc(id string, args []string, env []string) error {
	if ipc.ChaincodeSupport == nil {
		inprocLogger.Panicf("Chaincode support is nil, most likely you forgot to set it immediately after calling inproccontroller.NewRegsitry()")
	}

	peerRcvCCSend := make(chan *pb.ChaincodeMessage)
	ccRcvPeerSend := make(chan *pb.ChaincodeMessage)
	var err error
	ccchan := make(chan struct{}, 1)
	ccsupportchan := make(chan struct{}, 1)
	shimStartInProc := _shimStartInProc // shadow to avoid race in test
	go func() {
		defer close(ccchan)
		inprocLogger.Debugf("chaincode started for %s", id)
		if args == nil {
			args = ipc.args
		}
		if env == nil {
			env = ipc.env
		}
		err := shimStartInProc(env, args, ipc.chaincode, ccRcvPeerSend, peerRcvCCSend)
		if err != nil {
			err = fmt.Errorf("chaincode-support ended with err: %s", err)
			_inprocLoggerErrorf("%s", err)
		}
		inprocLogger.Debugf("chaincode ended for %s with err: %s", id, err)
	}()

	// shadow function to avoid data race
	inprocLoggerErrorf := _inprocLoggerErrorf
	go func() {
		defer close(ccsupportchan)
		inprocStream := newInProcStream(peerRcvCCSend, ccRcvPeerSend)
		inprocLogger.Debugf("chaincode-support started for  %s", id)
		err := ipc.ChaincodeSupport.HandleChaincodeStream(inprocStream)
		if err != nil {
			err = fmt.Errorf("chaincode ended with err: %s", err)
			inprocLoggerErrorf("%s", err)
		}
		inprocLogger.Debugf("chaincode-support ended for %s with err: %s", id, err)
	}()

	select {
	case <-ccchan:
		close(peerRcvCCSend)
		inprocLogger.Debugf("chaincode %s quit", id)
	case <-ccsupportchan:
		close(ccRcvPeerSend)
		inprocLogger.Debugf("chaincode support %s quit", id)
	case <-ipc.stopChan:
		close(ccRcvPeerSend)
		close(peerRcvCCSend)
		inprocLogger.Debugf("chaincode %s stopped", id)
	}
	return err
}

//Start starts a previously registered system codechain
func (vm *InprocVM) Start(ccid ccintf.CCID, args []string, env []string, filesToUpload map[string][]byte, builder container.Builder) error {
	path := ccid.GetName()

	ipctemplate := vm.registry.getType(path)
	if ipctemplate == nil {
		return fmt.Errorf(fmt.Sprintf("%s not registered", path))
	}

	instName := vm.GetVMName(ccid)

	ipc, err := vm.getInstance(ipctemplate, instName, args, env)

	if err != nil {
		return fmt.Errorf(fmt.Sprintf("could not create instance for %s", instName))
	}

	if ipc.running {
		return fmt.Errorf(fmt.Sprintf("chaincode running %s", path))
	}

	ipc.running = true

	go func() {
		defer func() {
			if r := recover(); r != nil {
				inprocLogger.Criticalf("caught panic from chaincode  %s", instName)
			}
		}()
		ipc.launchInProc(instName, args, env)
	}()

	return nil
}

//Stop stops a system codechain
func (vm *InprocVM) Stop(ccid ccintf.CCID, timeout uint, dontkill bool, dontremove bool) error {
	path := ccid.GetName()

	ipctemplate := vm.registry.getType(path)
	if ipctemplate == nil {
		return fmt.Errorf("%s not registered", path)
	}

	instName := vm.GetVMName(ccid)

	ipc := vm.registry.getInstance(instName)
	if ipc == nil {
		return fmt.Errorf("%s not found", instName)
	}

	if !ipc.running {
		return fmt.Errorf("%s not running", instName)
	}

	ipc.running = false
	close(ipc.stopChan)
	vm.registry.removeInstance(instName)

	//TODO stop
	return nil
}

// HealthCheck is provided in order to implement the VMProvider interface.
// It always returns nil..
func (vm *InprocVM) HealthCheck(ctx context.Context) error {
	return nil
}

// Wait will block until the chaincode is stopped.
func (vm *InprocVM) Wait(ccid ccintf.CCID) (int, error) {
	instName := vm.GetVMName(ccid)
	ipc := vm.registry.getInstance(instName)
	if ipc == nil {
		return 0, fmt.Errorf("%s not found", instName)
	}

	<-ipc.stopChan

	return 0, nil
}

// GetVMName ignores the peer and network name as it just needs to be unique in
// process.  It accepts a format function parameter to allow different
// formatting based on the desired use of the name.
func (vm *InprocVM) GetVMName(ccid ccintf.CCID) string {
	return ccid.GetName()
}
