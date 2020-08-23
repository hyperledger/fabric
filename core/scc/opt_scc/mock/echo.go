/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"fmt"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/peer"
)

func New(aclProvider aclmgmt.ACLProvider, p *peer.Peer) *EchoSCC {
	return &EchoSCC{}
}

// EchoSCC echos any (string) request back as response
type EchoSCC struct {
}

func (e *EchoSCC) Name() string              { return "mscc" }
func (e *EchoSCC) Chaincode() shim.Chaincode { return e }

var mscclogger = flogging.MustGetLogger("mscc")

func (e *EchoSCC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	mscclogger.Info("Init MSCC")

	return shim.Success(nil)
}

// Invoke is called with args[0] contains the query function name, args[1]
func (e *EchoSCC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetStringArgs()

	mscclogger.Infof("Invoke MSCC(%v)", args)

	bytes := []byte(fmt.Sprintf("%v", args))
	return shim.Success(bytes)
}
