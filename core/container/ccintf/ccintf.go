/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccintf

//This package defines the interfaces that support runtime and
//communication between chaincode and peer (chaincode support).
//Currently inproccontroller uses it. dockercontroller does not.

import (
	pb "github.com/hyperledger/fabric/protos/peer"
	"golang.org/x/net/context"
)

// ChaincodeStream interface for stream between Peer and chaincode instance.
type ChaincodeStream interface {
	Send(*pb.ChaincodeMessage) error
	Recv() (*pb.ChaincodeMessage, error)
}

// CCSupport must be implemented by the chaincode support side in peer
// (such as chaincode_support)
type CCSupport interface {
	HandleChaincodeStream(context.Context, ChaincodeStream) error
}

// GetCCHandlerKey is used to pass CCSupport via context
func GetCCHandlerKey() string {
	return "CCHANDLER"
}

//CCID encapsulates chaincode ID
type CCID struct {
	ChaincodeSpec *pb.ChaincodeSpec
	NetworkID     string
	PeerID        string
	Version       string
}

//GetName returns canonical chaincode name based on chain name
func (ccid *CCID) GetName() string {
	if ccid.ChaincodeSpec == nil {
		panic("nil chaincode spec")
	}

	name := ccid.ChaincodeSpec.ChaincodeId.Name
	if ccid.Version != "" {
		name = name + "-" + ccid.Version
	}

	return name
}
