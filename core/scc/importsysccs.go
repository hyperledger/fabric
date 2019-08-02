/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/ccintf"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// A ChaincodeStreamHandler is responsible for handling the ChaincodeStream
// communication between a per and chaincode.
type ChaincodeStreamHandler interface {
	HandleChaincodeStream(ccintf.ChaincodeStream) error
	LaunchInProc(packageID ccintf.CCID) <-chan struct{}
}

// DeploySysCCs is the hook for system chaincodes where system chaincodes are registered with the fabric.
// This call directly registers the chaincode with the chaincode handler and bypasses the other usercc constructs.
func (p *Provider) DeploySysCCs(sysCCVersion string, chaincodeStreamHandler ChaincodeStreamHandler) {
	for _, sysCC := range p.SysCCs {
		if !sysCC.Enabled() || !p.isWhitelisted(sysCC) {
			sysccLogger.Infof("System chaincode '%s' is disabled", sysCC.Name())
			continue
		}
		sysccLogger.Infof("deploying system chaincode '%s'", sysCC.Name())

		ccid := ccintf.CCID(sysCC.Name() + ":" + sysCCVersion)

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
			err := shim.StartInProc(ccid.String(), newInProcStream(ccRcvPeerSend, peerRcvCCSend), sysCC.Chaincode())
			sysccLogger.Criticalf("system chaincode ended with err: %v", err)
		}(sysCC)
		<-done
	}
}
