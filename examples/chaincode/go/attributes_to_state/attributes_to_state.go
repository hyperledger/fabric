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

	"github.com/hyperledger/fabric/accesscontrol/crypto/attr"
	"github.com/hyperledger/fabric/accesscontrol/impl"
	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// Attributes2State demonstrates how to read attributes from TCerts.
type Attributes2State struct {
}

func (t *Attributes2State) setStateToAttributes(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	attrHandler, err := attr.NewAttributesHandlerImpl(stub)
	if err != nil {
		return shim.Error(err.Error())
	}
	for _, att := range args {
		fmt.Println("Writing attribute " + att)
		attVal, err := attrHandler.GetValue(att)
		if err != nil {
			return shim.Error(err.Error())
		}
		err = stub.PutState(att, attVal)
		if err != nil {
			return shim.Error(err.Error())
		}
	}
	return shim.Success(nil)
}

// Init intializes the chaincode by reading the transaction attributes and storing
// the attrbute values in the state
func (t *Attributes2State) Init(stub shim.ChaincodeStubInterface) pb.Response {
	_, args := stub.GetFunctionAndParameters()
	return t.setStateToAttributes(stub, args)
}

func (t *Attributes2State) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	if function == "delete" {
		return t.delete(stub, args)
	} else if function == "submit" {
		return t.setStateToAttributes(stub, args)
	} else if function == "read" {
		return t.query(stub, args)
	}

	return shim.Error("Invalid invoke function name. Expecting either \"delete\" or \"submit\" or \"read\"")
}

// delete Deletes an entity from the state, returning error if the entity was not found in the state.
func (t *Attributes2State) delete(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting only 1 (attributeName)")
	}

	attributeName := args[0]
	fmt.Printf("Deleting attribute %v", attributeName)
	valBytes, err := stub.GetState(attributeName)
	if err != nil {
		return shim.Error(err.Error())
	}

	if valBytes == nil {
		return shim.Error("Attribute '" + attributeName + "' not found.")
	}

	isOk, err := impl.NewAccessControlShim(stub).VerifyAttribute(attributeName, valBytes)
	if err != nil {
		return shim.Error(err.Error())
	}
	if isOk {
		// Delete the key from the state in ledger
		err = stub.DelState(attributeName)
		if err != nil {
			return shim.Error("Failed to delete state")
		}
	}
	return shim.Success(nil)
}

func (t *Attributes2State) query(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var attributeName string // Name of the attributeName to query.
	var err error

	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting only 1 (attributeName)")
	}

	attributeName = args[0]
	fmt.Printf("Reading attribute %v", attributeName)

	// Get the state from the ledger
	Avalbytes, err := stub.GetState(attributeName)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed to get state for " + attributeName + "\"}"
		fmt.Printf("Query Response:%s\n", jsonResp)
		return shim.Error(jsonResp)
	}

	if Avalbytes == nil {
		jsonResp := "{\"Error\":\"Nil amount for " + attributeName + "\"}"
		fmt.Printf("Query Response:%s\n", jsonResp)
		return shim.Error(jsonResp)
	}

	jsonResp := "{\"Name\":\"" + attributeName + "\",\"Amount\":\"" + string(Avalbytes) + "\"}"
	fmt.Printf("Query Response:%s\n", jsonResp)
	return shim.Success([]byte(jsonResp))
}

func main() {
	err := shim.Start(new(Attributes2State))
	if err != nil {
		fmt.Printf("Error running Attributes2State chaincode: %s", err)
	}
}
