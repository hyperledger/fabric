/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccintf

//This package defines the interfaces that support runtime and
//communication between chaincode and peer (chaincode support).
//Currently inproccontroller uses it. dockercontroller does not.

import (
	"fmt"

	pb "github.com/hyperledger/fabric/protos/peer"
)

// ChaincodeStream interface for stream between Peer and chaincode instance.
type ChaincodeStream interface {
	Send(*pb.ChaincodeMessage) error
	Recv() (*pb.ChaincodeMessage, error)
}

// CCSupport must be implemented by the chaincode support side in peer
// (such as chaincode_support)
type CCSupport interface {
	HandleChaincodeStream(ChaincodeStream) error
}

// GetCCHandlerKey is used to pass CCSupport via context
func GetCCHandlerKey() string {
	return "CCHANDLER"
}

//CCID encapsulates chaincode ID
type CCID struct {
	Name    string
	Version string
}

//GetName returns canonical chaincode name based on the fields of CCID
func (ccid *CCID) GetName() string {
	if ccid.Version != "" {
		return fmt.Sprintf("%s-%s", ccid.Name, ccid.Version)
	}
	return ccid.Name
}
