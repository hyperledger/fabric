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
	"errors"
	"fmt"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/util"
)

// PassthruChaincode passes thru invoke and query to another chaincode where
//     called ChaincodeID = function
//     called chaincode's function = args[0]
//     called chaincode's args = args[1:]
type PassthruChaincode struct {
}

//Init func will return error if function has string "error" anywhere
func (p *PassthruChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {

	if strings.Index(function, "error") >= 0 {
		return nil, errors.New(function)
	}
	return []byte(function), nil
}

//helper
func (p *PassthruChaincode) iq(invoke bool, stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	if function == "" {
		return nil, errors.New("Chaincode ID not provided")
	}
	chaincodeID := function

	if invoke {
		return stub.InvokeChaincode(chaincodeID, util.ToChaincodeArgs(args...))
	}
	return stub.QueryChaincode(chaincodeID, util.ToChaincodeArgs(args...))
}

// Invoke passes through the invoke call
func (p *PassthruChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return p.iq(true, stub, function, args)
}

// Query passes through the query call
func (p *PassthruChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return p.iq(false, stub, function, args)
}

func main() {
	err := shim.Start(new(PassthruChaincode))
	if err != nil {
		fmt.Printf("Error starting Passthru chaincode: %s", err)
	}
}
