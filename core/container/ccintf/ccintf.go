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

package ccintf

//This package defines the interfaces that support runtime and
//communication between chaincode and peer (chaincode support).
//Currently inproccontroller uses it. dockercontroller does not.

import (
	"encoding/hex"

	"github.com/hyperledger/fabric/common/util"
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
	ChainID       string
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

	//this better be chainless system chaincode!
	if ccid.ChainID != "" {
		hash := util.ComputeSHA256([]byte(ccid.ChainID))
		hexstr := hex.EncodeToString(hash[:])
		name = name + "-" + hexstr
	}

	return name
}
