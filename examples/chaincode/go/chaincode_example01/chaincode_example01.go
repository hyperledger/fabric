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
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

var A, B string
var Aval, Bval, X int

// Init callback representing the invocation of a chaincode
// This chaincode will manage two accounts A and B and will transfer X units from A to B upon invoke
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	var err error
	_, args := stub.GetFunctionAndParameters()
	if len(args) != 4 {
		return shim.Error("Incorrect number of arguments. Expecting 4")
	}

	// Initialize the chaincode
	A = args[0]
	Aval, err = strconv.Atoi(args[1])
	if err != nil {
		return shim.Error("Expecting integer value for asset holding")
	}
	B = args[2]
	Bval, err = strconv.Atoi(args[3])
	if err != nil {
		return shim.Error("Expecting integer value for asset holding")
	}
	fmt.Printf("Aval = %d, Bval = %d\n", Aval, Bval)

	/************
			// Write the state to the ledger
			err = stub.PutState(A, []byte(strconv.Itoa(Aval))
			if err != nil {
				return nil, err
			}

			stub.PutState(B, []byte(strconv.Itoa(Bval))
			err = stub.PutState(B, []byte(strconv.Itoa(Bval))
			if err != nil {
				return nil, err
			}
	************/
	return shim.Success(nil)
}

func (t *SimpleChaincode) invoke(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	// Transaction makes payment of X units from A to B
	X, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Printf("Error convert %s to integer: %s", args[0], err)
		return shim.Error(fmt.Sprintf("Error convert %s to integer: %s", args[0], err))
	}
	Aval = Aval - X
	Bval = Bval + X
	ts, err2 := stub.GetTxTimestamp()
	if err2 != nil {
		fmt.Printf("Error getting transaction timestamp: %s", err2)
		return shim.Error(fmt.Sprintf("Error getting transaction timestamp: %s", err2))
	}
	fmt.Printf("Transaction Time: %v,Aval = %d, Bval = %d\n", ts, Aval, Bval)
	return shim.Success(nil)
}

func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	if function == "invoke" {
		return t.invoke(stub, args)
	}

	return shim.Error("Invalid invoke function name. Expecting \"invoke\"")
}

func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}
