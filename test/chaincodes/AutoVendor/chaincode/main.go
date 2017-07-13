/*
 * Copyright Greg Haskins All Rights Reserved
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 * The purpose of this test code is to prove that the system properly packages
 * up dependencies.  We therefore synthesize the scenario where a chaincode
 * imports non-standard dependencies both directly and indirectly and then
 * expect a unit-test to verify that the package includes everything needed
 * and ultimately builds properly.
 *
 */

package main

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/test/chaincodes/AutoVendor/directdep"
)

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Error("NOT IMPL")
}

func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Error("NOT IMPL")
}

func main() {
	directdep.PointlessFunction()

	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}
