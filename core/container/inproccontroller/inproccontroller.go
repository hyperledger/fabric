/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package inproccontroller

import (
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// ContainerType is the string which the inproc container type
// is registered with the container.VMController
const ContainerType = "SYSTEM"

// A ChaincodeStreamHandler is responsible for handling the ChaincodeStream
// communication between a per and chaincode.
type ChaincodeStreamHandler interface {
	HandleChaincodeStream(ccintf.ChaincodeStream) error
	LaunchInProc(packageID ccintf.CCID) <-chan struct{}
}

type inprocContainer struct {
	chaincodeSupport ChaincodeStreamHandler
	chaincode        shim.Chaincode
	running          bool
	args             []string
	env              []string
	stopChan         chan struct{}
}

var (
	inprocLogger = flogging.MustGetLogger("inproccontroller")

	// TODO this is a very hacky way to do testing, we should find other ways
	// to test, or not statically inject these depenencies.
	_shimStartInProc    = shim.StartInProc
	_inprocLoggerErrorf = inprocLogger.Errorf
)

// SysCCRegisteredErr registered error
type SysCCRegisteredErr string

func (s SysCCRegisteredErr) Error() string {
	return fmt.Sprintf("%s already registered", string(s))
}

// Registry stores registered system chaincodes.
// It implements container.VMProvider and scc.Registrar
type Registry struct {
	mutex      sync.Mutex
	chaincodes map[ccintf.CCID]shim.Chaincode
}

// NewRegistry creates an initialized registry, ready to register system chaincodes.
// The returned *Registry is _not_ ready to use as is.  You must set the ChaincodeSupport
// as soon as one is available, before any chaincode invocations occur.  This is because
// the chaincode support used to be a latent dependency, snuck in on the context, but now
// it is being made an explicit part of the startup.
func NewRegistry() *Registry {
	return &Registry{
		chaincodes: make(map[ccintf.CCID]shim.Chaincode),
	}
}

// Register registers system chaincode with given path. The deploy should be called to initialize
func (r *Registry) Register(ccid ccintf.CCID, cc shim.Chaincode) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	inprocLogger.Debugf("Registering chaincode instance: %s", ccid)
	if _, exists := r.chaincodes[ccid]; exists {
		return SysCCRegisteredErr(ccid.String())
	}

	r.chaincodes[ccid] = cc
	return nil
}

func (r *Registry) LaunchAll(chaincodeSupport ChaincodeStreamHandler) {
	for ccid, chaincode := range r.chaincodes {
		ipc := &inprocContainer{
			chaincodeSupport: chaincodeSupport,
			chaincode:        chaincode,
			env:              []string{"CORE_CHAINCODE_ID_NAME=" + ccid.String()},
			stopChan:         make(chan struct{}),
		}

		done := chaincodeSupport.LaunchInProc(ccid)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					inprocLogger.Criticalf("caught panic from chaincode  %s", ccid)
				}
			}()
			ipc.launchInProc(ccid, nil, nil)
		}()
		<-done
	}
}

// InprocVM is a vm. It is identified by a executable name
type InprocVM struct {
	registry *Registry
}

func (ipc *inprocContainer) launchInProc(id ccintf.CCID, args, env []string) error {
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
		err := ipc.chaincodeSupport.HandleChaincodeStream(inprocStream)
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
