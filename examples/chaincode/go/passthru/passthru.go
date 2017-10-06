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
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// PassthruChaincode passes thru invoke and query to another chaincode where
//     called ChaincodeID = function
//     called chaincode's function = args[0]
//     called chaincode's args = args[1:]
type PassthruChaincode struct {
}

func toChaincodeArgs(args ...string) [][]byte {
	bargs := make([][]byte, len(args))
	for i, arg := range args {
		bargs[i] = []byte(arg)
	}
	return bargs
}

//Init func will return error if function has string "error" anywhere
func (p *PassthruChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	function, _ := stub.GetFunctionAndParameters()
	if strings.Index(function, "error") >= 0 {
		return shim.Error(function)
	}
	return shim.Success([]byte(function))
}

//helper
func (p *PassthruChaincode) iq(stub shim.ChaincodeStubInterface, function string, args []string) pb.Response {
	if function == "" {
		return shim.Error("Chaincode ID not provided")
	}
	chaincodeID := function

	return stub.InvokeChaincode(chaincodeID, toChaincodeArgs(args...), "")
}

// Invoke passes through the invoke call
func (p *PassthruChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	return p.iq(stub, function, args)
}

func main() {
	err := shim.Start(new(PassthruChaincode))
	if err != nil {
		fmt.Printf("Error starting Passthru chaincode: %s", err)
	}
}
