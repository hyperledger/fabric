/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"fmt"

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

var (
	logger = flogging.MustGetLogger("inproccontroller")
)

// SysCCRegisteredErr registered error
type SysCCRegisteredErr string

func (s SysCCRegisteredErr) Error() string {
	return fmt.Sprintf("%s already registered", string(s))
}

// Registry stores registered system chaincodes.
// It implements container.VMProvider and scc.Registrar
type Registry struct {
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
	logger.Debugf("Registering chaincode instance: %s", ccid)
	if _, exists := r.chaincodes[ccid]; exists {
		return SysCCRegisteredErr(ccid.String())
	}

	r.chaincodes[ccid] = cc
	return nil
}

func (r *Registry) LaunchAll(chaincodeSupport ChaincodeStreamHandler) {
	for ccid, chaincode := range r.chaincodes {

		done := chaincodeSupport.LaunchInProc(ccid)

		peerRcvCCSend := make(chan *pb.ChaincodeMessage)
		ccRcvPeerSend := make(chan *pb.ChaincodeMessage)

		go func() {
			logger.Debugf("starting chaincode-support stream for  %s", ccid)
			err := chaincodeSupport.HandleChaincodeStream(newInProcStream(peerRcvCCSend, ccRcvPeerSend))
			logger.Criticalf("shim stream ended with err: %s", err)
		}()

		go func() {
			logger.Debugf("chaincode started for %s", ccid)
			err := shim.StartInProc(ccid.String(), newInProcStream(ccRcvPeerSend, peerRcvCCSend), chaincode)
			logger.Criticalf("system chaincode ended with err: %s", err)
		}()
		<-done
	}
}
