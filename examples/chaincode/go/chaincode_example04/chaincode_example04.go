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
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/util"
)

// This chaincode is a test for chaincode invoking another chaincode - invokes chaincode_example02

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

// Init takes two arguements, a string and int. These are stored in the key/value pair in the state
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	var event string // Indicates whether event has happened. Initially 0
	var eventVal int // State of event
	var err error
	_, args := stub.GetFunctionAndParameters()
	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	// Initialize the chaincode
	event = args[0]
	eventVal, err = strconv.Atoi(args[1])
	if err != nil {
		return nil, errors.New("Expecting integer value for event status")
	}
	fmt.Printf("eventVal = %d\n", eventVal)

	err = stub.PutState(event, []byte(strconv.Itoa(eventVal)))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Invoke invokes another chaincode - chaincode_example02, upon receipt of an event and changes event state
func (t *SimpleChaincode) invoke(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	var event string // Event entity
	var eventVal int // State of event
	var err error

	if len(args) != 3 {
		return nil, errors.New("Incorrect number of arguments. Expecting 3")
	}

	chainCodeToCall := args[0]
	event = args[1]
	eventVal, err = strconv.Atoi(args[2])
	if err != nil {
		return nil, errors.New("Expected integer value for event state change")
	}

	if eventVal != 1 {
		fmt.Printf("Unexpected event. Doing nothing\n")
		return nil, nil
	}

	f := "invoke"
	invokeArgs := util.ToChaincodeArgs(f, "a", "b", "10")
	response, err := stub.InvokeChaincode(chainCodeToCall, invokeArgs)
	if err != nil {
		errStr := fmt.Sprintf("Failed to invoke chaincode. Got error: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}

	fmt.Printf("Invoke chaincode successful. Got response %s", string(response))

	// Write the event state back to the ledger
	err = stub.PutState(event, []byte(strconv.Itoa(eventVal)))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (t *SimpleChaincode) query(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	var event string // Event entity
	var err error

	if len(args) != 1 {
		return nil, errors.New("Incorrect number of arguments. Expecting entity to query")
	}

	event = args[0]

	// Get the state from the ledger
	eventValbytes, err := stub.GetState(event)
	if err != nil {
		jsonResp := "{\"Error\":\"Failed to get state for " + event + "\"}"
		return nil, errors.New(jsonResp)
	}

	if eventValbytes == nil {
		jsonResp := "{\"Error\":\"Nil value for " + event + "\"}"
		return nil, errors.New(jsonResp)
	}

	jsonResp := "{\"Name\":\"" + event + "\",\"Amount\":\"" + string(eventValbytes) + "\"}"
	fmt.Printf("Query Response:%s\n", jsonResp)
	return []byte(jsonResp), nil
}

func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	function, args := stub.GetFunctionAndParameters()
	if function == "invoke" {
		return t.invoke(stub, args)
	} else if function == "query" {
		return t.query(stub, args)
	}

	return nil, errors.New("Invalid invoke function name. Expecting \"invoke\" \"query\"")
}

func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}
