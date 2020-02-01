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

package main

import (
	"fmt"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// PassthruChaincode passes thru invoke and query to another chaincode where
//     called ChaincodeID = args[0]
//     called chaincode's function = args[1]
//     called chaincode's args = args[2:len-2]
//     called ChannelID = args[len-1]
type PassthruChaincode struct {
}

func toChaincodeArgs(args ...string) [][]byte {
	bargs := make([][]byte, len(args))
	for i, arg := range args {
		bargs[i] = []byte(arg)
	}
	return bargs
}

func (p *PassthruChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	fmt.Println("passthru Init")
	function, _ := stub.GetFunctionAndParameters()
	return shim.Success([]byte(function))
}

// helper
func (p *PassthruChaincode) iq(stub shim.ChaincodeStubInterface, chaincode string, args []string, channel string) pb.Response {
	return stub.InvokeChaincode(chaincode, toChaincodeArgs(args...), channel)
}

// Invoke passes through the invoke call
// The chaincode ID to call MUST be specified as first argument
// The channel ID to call the chaincode on MUST be specified as the last argument
func (p *PassthruChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetStringArgs()
	fmt.Println("passthru Invoke ", args)

	if len(args) < 2 {
		return shim.Error("Channel to call on not provided")
	}
	channelToCallOn := args[len(args)-1]
	if len(args) < 1 {
		return shim.Error("Chaincode to call not provided")
	}
	chaincodeToCall := args[0]

	response := p.iq(stub, chaincodeToCall, args[1:len(args)-1], channelToCallOn)
	if response.Status != shim.OK {
		msg := response.Message
		if response.Payload != nil {
			msg = string(response.Payload)
		}
		errStr := fmt.Sprintf("Failed to invoke chaincode. Got error: %s", msg)
		fmt.Printf(errStr)
		return shim.Error(errStr)
	}
	return response
}

func main() {
	err := shim.Start(new(PassthruChaincode))
	if err != nil {
		fmt.Printf("Error starting Passthru chaincode: %s", err)
	}
}
