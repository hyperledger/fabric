/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/container/ccintf"
)

// SysCCVersion is a constant used for the version field of system chaincodes.
// Historically, the version of a system chaincode was implicitly tied to the exact
// build version of a peer, which does not work for collecting endorsements across
// sccs across peers.  Until there is a need for separate SCC versions, we use
// this constant here.
const SysCCVersion = "syscc"

// ChaincodeID returns the chaincode ID of a system chaincode of a given name.
func ChaincodeID(ccName string) string {
	return ccName + "." + SysCCVersion
}

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
	// Unique name of the system chaincode
	Name() string

	// Chaincode returns the underlying chaincode
	Chaincode() shim.Chaincode
}

// DeploySysCC is the hook for system chaincodes where system chaincodes are registered with the fabric.
// This call directly registers the chaincode with the chaincode handler and bypasses the other usercc constructs.
func DeploySysCC(sysCC SelfDescribingSysCC, chaincodeStreamHandler ChaincodeStreamHandler) {
	sysccLogger.Infof("deploying system chaincode '%s'", sysCC.Name())

	ccid := ChaincodeID(sysCC.Name())
	done := chaincodeStreamHandler.LaunchInProc(ccid)

	peerRcvCCSend := make(chan *pb.ChaincodeMessage)
	ccRcvPeerSend := make(chan *pb.ChaincodeMessage)

	go func() {
		stream := newInProcStream(peerRcvCCSend, ccRcvPeerSend)
		defer stream.CloseSend()

		sysccLogger.Debugf("starting chaincode-support stream for  %s", ccid)
		err := chaincodeStreamHandler.HandleChaincodeStream(stream)
		sysccLogger.Criticalf("shim stream ended with err: %v", err)
	}()

	go func(sysCC SelfDescribingSysCC) {
		stream := newInProcStream(ccRcvPeerSend, peerRcvCCSend)
		defer stream.CloseSend()

		sysccLogger.Debugf("chaincode started for %s", ccid)
		err := shim.StartInProc(ccid, stream, sysCC.Chaincode())
		sysccLogger.Criticalf("system chaincode ended with err: %v", err)
	}(sysCC)
	<-done
}
