/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package example01

import (
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct{}

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
