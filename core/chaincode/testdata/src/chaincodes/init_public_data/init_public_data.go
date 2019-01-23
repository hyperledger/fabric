/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

// Init initializes a public state
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	if err := stub.PutState("dummyKey", []byte("dummyValue")); err != nil {
		return shim.Error(fmt.Sprintf("put operation failed. Error storing state: %s", err))
	}
	return shim.Success(nil)
}

// Invoke is a no-op
func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting chaincode: %s", err)
	}
}
