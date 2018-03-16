/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package example04

import (
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// This chaincode is a test for chaincode invoking another chaincode - invokes chaincode_example02

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct{}

func toChaincodeArgs(args ...string) [][]byte {
	bargs := make([][]byte, len(args))
	for i, arg := range args {
		bargs[i] = []byte(arg)
	}
	return bargs
}

// Init takes two arguments, a string and int. These are stored in the key/value pair in the state
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	var event string // Indicates whether event has happened. Initially 0
	var eventVal int // State of event
	var err error
	_, args := stub.GetFunctionAndParameters()
	if len(args) != 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2")
	}

	// Initialize the chaincode
	event = args[0]
	eventVal, err = strconv.Atoi(args[1])
	if err != nil {
		return shim.Error("Expecting integer value for event status")
	}
	fmt.Printf("eventVal = %d\n", eventVal)

	err = stub.PutState(event, []byte(strconv.Itoa(eventVal)))
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

// Invoke invokes another chaincode - chaincode_example02, upon receipt of an event and changes event state
func (t *SimpleChaincode) invoke(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var event string // Event entity
	var eventVal int // State of event
	var err error

	if len(args) != 3 && len(args) != 4 {
		return shim.Error("Incorrect number of arguments. Expecting 3 or 4")
	}

	chainCodeToCall := args[0]
	event = args[1]
	eventVal, err = strconv.Atoi(args[2])
	if err != nil {
		return shim.Error("Expected integer value for event state change")
	}
	channelID := ""
	if len(args) == 4 {
		channelID = args[3]
	}

	if eventVal != 1 {
		fmt.Printf("Unexpected event. Doing nothing\n")
		return shim.Success(nil)
	}

	f := "invoke"
	invokeArgs := toChaincodeArgs(f, "a", "b", "10")
	response := stub.InvokeChaincode(chainCodeToCall, invokeArgs, channelID)
	if response.Status != shim.OK {
		errStr := fmt.Sprintf("Failed to invoke chaincode. Got error: %s", string(response.Payload))
		fmt.Printf(errStr)
		return shim.Error(errStr)
	}

	fmt.Printf("Invoke chaincode successful. Got response %s", string(response.Payload))

	// Write the event state back to the ledger
	err = stub.PutState(event, []byte(strconv.Itoa(eventVal)))
	if err != nil {
		return shim.Error(err.Error())
	}

	return response
}

func (t *SimpleChaincode) query(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var event string // Event entity
	var err error

	if len(args) < 1 {
		return shim.Error("Incorrect number of arguments. Expecting entity to query")
	}

	event = args[0]
	var jsonResp string

	// Get the state from the ledger
	eventValbytes, err := stub.GetState(event)
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get state for " + event + "\"}"
		return shim.Error(jsonResp)
	}

	if eventValbytes == nil {
		jsonResp = "{\"Error\":\"Nil value for " + event + "\"}"
		return shim.Error(jsonResp)
	}

	if len(args) > 3 {
		chainCodeToCall := args[1]
		queryKey := args[2]
		channel := args[3]
		f := "query"
		invokeArgs := toChaincodeArgs(f, queryKey)
		response := stub.InvokeChaincode(chainCodeToCall, invokeArgs, channel)
		if response.Status != shim.OK {
			errStr := fmt.Sprintf("Failed to invoke chaincode. Got error: %s", err.Error())
			fmt.Printf(errStr)
			return shim.Error(errStr)
		}
		jsonResp = string(response.Payload)
	} else {
		jsonResp = "{\"Name\":\"" + event + "\",\"Amount\":\"" + string(eventValbytes) + "\"}"
	}
	fmt.Printf("Query Response: %s\n", jsonResp)

	return shim.Success([]byte(jsonResp))
}

// Invoke is called by fabric to execute a transaction
func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	if function == "invoke" {
		return t.invoke(stub, args)
	} else if function == "query" {
		return t.query(stub, args)
	}

	return shim.Error("Invalid invoke function name. Expecting \"invoke\" \"query\"")
}
