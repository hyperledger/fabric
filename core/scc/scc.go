/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// XXX should we change this name to actually match the package name? Leaving now for historical consistency
var sysccLogger = flogging.MustGetLogger("sccapi")

// BuiltinSCCs are special system chaincodes, differentiated from other (plugin)
// system chaincodes.  These chaincodes do not need to be initialized in '_lifecycle'
// and may be invoked without a channel context.  It is expected that '_lifecycle'
// will eventually be the only builtin SCCs.
// Note, this field should only be used on _endorsement_ side, never in validation
// as it might change.
type BuiltinSCCs map[string]struct{}

func (bccs BuiltinSCCs) IsSysCC(name string) bool {
	_, ok := bccs[name]
	return ok
}

// A ChaincodeStreamHandler is responsible for handling the ChaincodeStream
// communication between a per and chaincode.
type ChaincodeStreamHandler interface {
	HandleChaincodeStream(ccintf.ChaincodeStream) error
	LaunchInProc(packageID string) <-chan struct{}
}

type SelfDescribingSysCC interface {
	//Unique name of the system chaincode
	Name() string

	// Chaincode returns the underlying chaincode
	Chaincode() shim.Chaincode
}

// DeploySysCC is the hook for system chaincodes where system chaincodes are registered with the fabric.
// This call directly registers the chaincode with the chaincode handler and bypasses the other usercc constructs.
func DeploySysCC(sysCC SelfDescribingSysCC, sysCCVersion string, chaincodeStreamHandler ChaincodeStreamHandler) {
	sysccLogger.Infof("deploying system chaincode '%s'", sysCC.Name())

	ccid := sysCC.Name() + ":" + sysCCVersion

	done := chaincodeStreamHandler.LaunchInProc(ccid)

	peerRcvCCSend := make(chan *pb.ChaincodeMessage)
	ccRcvPeerSend := make(chan *pb.ChaincodeMessage)

	// TODO, these go routines leak in test.
	go func() {
		sysccLogger.Debugf("starting chaincode-support stream for  %s", ccid)
		err := chaincodeStreamHandler.HandleChaincodeStream(newInProcStream(peerRcvCCSend, ccRcvPeerSend))
		sysccLogger.Criticalf("shim stream ended with err: %v", err)
	}()

	go func(sysCC SelfDescribingSysCC) {
		sysccLogger.Debugf("chaincode started for %s", ccid)
		err := shim.StartInProc(ccid, newInProcStream(ccRcvPeerSend, peerRcvCCSend), sysCC.Chaincode())
		sysccLogger.Criticalf("system chaincode ended with err: %v", err)
	}(sysCC)
	<-done
}
