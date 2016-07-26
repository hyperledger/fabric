/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package inproccontroller

import (
	"fmt"
	"io"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos"
	"github.com/op/go-logging"

	"golang.org/x/net/context"
)

type inprocContainer struct {
	chaincode shim.Chaincode
	running   bool
	args      []string
	env       []string
	stopChan  chan struct{}
}

var (
	inprocLogger = logging.MustGetLogger("inproccontroller")
	typeRegistry = make(map[string]*inprocContainer)
	instRegistry = make(map[string]*inprocContainer)
)

//Register registers system chaincode with given path. The deploy should be called to initialize
func Register(path string, cc shim.Chaincode) error {
	tmp := typeRegistry[path]
	if tmp != nil {
		return fmt.Errorf(fmt.Sprintf("%s is registered", path))
	}

	typeRegistry[path] = &inprocContainer{chaincode: cc}
	return nil
}

//InprocVM is a vm. It is identified by a executable name
type InprocVM struct {
	id string
}

func (vm *InprocVM) getInstance(ctxt context.Context, ipctemplate *inprocContainer, ccid ccintf.CCID, args []string, env []string) (*inprocContainer, error) {
	ipc := instRegistry[ccid.ChaincodeSpec.ChaincodeID.Name]
	if ipc != nil {
		inprocLogger.Warningf("chaincode instance exists for %s", ccid.ChaincodeSpec.ChaincodeID.Name)
		return ipc, nil
	}
	ipc = &inprocContainer{args: args, env: env, chaincode: ipctemplate.chaincode, stopChan: make(chan struct{})}
	instRegistry[ccid.ChaincodeSpec.ChaincodeID.Name] = ipc
	inprocLogger.Debugf("chaincode instance created for %s", ccid.ChaincodeSpec.ChaincodeID.Name)
	return ipc, nil
}

//Deploy verifies chaincode is registered and creates an instance for it. Currently only one instance can be created
func (vm *InprocVM) Deploy(ctxt context.Context, ccid ccintf.CCID, args []string, env []string, attachstdin bool, attachstdout bool, reader io.Reader) error {
	path := ccid.ChaincodeSpec.ChaincodeID.Path

	ipctemplate := typeRegistry[path]
	if ipctemplate == nil {
		return fmt.Errorf(fmt.Sprintf("%s not registered. Please register the system chaincode in inprocinstances.go", path))
	}

	if ipctemplate.chaincode == nil {
		return fmt.Errorf(fmt.Sprintf("%s system chaincode does not contain chaincode instance", path))
	}

	_, err := vm.getInstance(ctxt, ipctemplate, ccid, args, env)

	//FUTURE ... here is where we might check code for safety
	inprocLogger.Debugf("registered : %s", path)

	return err
}

func (ipc *inprocContainer) launchInProc(ctxt context.Context, id string, args []string, env []string, ccSupport ccintf.CCSupport) error {
	peerRcvCCSend := make(chan *pb.ChaincodeMessage)
	ccRcvPeerSend := make(chan *pb.ChaincodeMessage)
	var err error
	ccchan := make(chan struct{}, 1)
	ccsupportchan := make(chan struct{}, 1)
	go func() {
		defer close(ccchan)
		inprocLogger.Debugf("chaincode started for %s", id)
		if args == nil {
			args = ipc.args
		}
		if env == nil {
			env = ipc.env
		}
		err := shim.StartInProc(env, args, ipc.chaincode, ccRcvPeerSend, peerRcvCCSend)
		if err != nil {
			err = fmt.Errorf("chaincode-support ended with err: %s", err)
			inprocLogger.Errorf("%s", err)
		}
		inprocLogger.Debugf("chaincode ended with for  %s with err: %s", id, err)
	}()

	go func() {
		defer close(ccsupportchan)
		inprocStream := newInProcStream(peerRcvCCSend, ccRcvPeerSend)
		inprocLogger.Debugf("chaincode-support started for  %s", id)
		err := ccSupport.HandleChaincodeStream(ctxt, inprocStream)
		if err != nil {
			err = fmt.Errorf("chaincode ended with err: %s", err)
			inprocLogger.Errorf("%s", err)
		}
		inprocLogger.Debugf("chaincode-support ended with for  %s with err: %s", id, err)
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
func (vm *InprocVM) Start(ctxt context.Context, ccid ccintf.CCID, args []string, env []string, attachstdin bool, attachstdout bool, reader io.Reader) error {
	path := ccid.ChaincodeSpec.ChaincodeID.Path

	ipctemplate := typeRegistry[path]

	if ipctemplate == nil {
		return fmt.Errorf(fmt.Sprintf("%s not registered", path))
	}

	ipc, err := vm.getInstance(ctxt, ipctemplate, ccid, args, env)

	if err != nil {
		return fmt.Errorf(fmt.Sprintf("could not create instance for %s", ccid.ChaincodeSpec.ChaincodeID.Name))
	}

	if ipc.running {
		return fmt.Errorf(fmt.Sprintf("chaincode running %s", path))
	}

	//TODO VALIDITY CHECKS ?

	ccSupport, ok := ctxt.Value(ccintf.GetCCHandlerKey()).(ccintf.CCSupport)
	if !ok || ccSupport == nil {
		return fmt.Errorf("in-process communication generator not supplied")
	}

	ipc.running = true

	go func() {
		defer func() {
			if r := recover(); r != nil {
				inprocLogger.Criticalf("caught panic from chaincode  %s", ccid.ChaincodeSpec.ChaincodeID.Name)
			}
		}()
		ipc.launchInProc(ctxt, ccid.ChaincodeSpec.ChaincodeID.Name, args, env, ccSupport)
	}()

	return nil
}

//Stop stops a system codechain
func (vm *InprocVM) Stop(ctxt context.Context, ccid ccintf.CCID, timeout uint, dontkill bool, dontremove bool) error {
	path := ccid.ChaincodeSpec.ChaincodeID.Path

	ipctemplate := typeRegistry[path]
	if ipctemplate == nil {
		return fmt.Errorf("%s not registered", path)
	}

	ipc := instRegistry[ccid.ChaincodeSpec.ChaincodeID.Name]

	if ipc == nil {
		return fmt.Errorf("%s not found", ccid.ChaincodeSpec.ChaincodeID.Name)
	}

	if !ipc.running {
		return fmt.Errorf("%s not running", ccid.ChaincodeSpec.ChaincodeID.Name)
	}

	ipc.stopChan <- struct{}{}

	delete(instRegistry, ccid.ChaincodeSpec.ChaincodeID.Name)
	//TODO stop
	return nil
}

//Destroy destroys an image
func (vm *InprocVM) Destroy(ctxt context.Context, ccid ccintf.CCID, force bool, noprune bool) error {
	//not implemented
	return nil
}

//GetVMName ignores the peer and network name as it just needs to be unique in process
func (vm *InprocVM) GetVMName(ccid ccintf.CCID) (string, error) {
	return ccid.ChaincodeSpec.ChaincodeID.Name, nil
}
