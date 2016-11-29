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

// This chaincode is a test for chaincode querying another chaincode - invokes chaincode_example02 and computes the sum of a and b and stores it as state

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

// Init takes two arguments, a string and int. The string will be a key with
// the int as a value.
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	var sum string // Sum of asset holdings across accounts. Initially 0
	var sumVal int // Sum of holdings
	var err error
	_, args := stub.GetFunctionAndParameters()
	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	// Initialize the chaincode
	sum = args[0]
	sumVal, err = strconv.Atoi(args[1])
	if err != nil {
		return nil, errors.New("Expecting integer value for sum")
	}
	fmt.Printf("sumVal = %d\n", sumVal)

	// Write the state to the ledger
	err = stub.PutState(sum, []byte(strconv.Itoa(sumVal)))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Invoke queries another chaincode and updates its own state
func (t *SimpleChaincode) invoke(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	var sum string             // Sum entity
	var Aval, Bval, sumVal int // value of sum entity - to be computed
	var err error

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	chaincodeURL := args[0] // Expecting "github.com/hyperledger/fabric/core/example/chaincode/chaincode_example02"
	sum = args[1]

	// Query chaincode_example02
	f := "query"
	queryArgs := util.ToChaincodeArgs(f, "a")
	response, err := stub.InvokeChaincode(chaincodeURL, queryArgs)
	if err != nil {
		errStr := fmt.Sprintf("Failed to query chaincode. Got error: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}
	Aval, err = strconv.Atoi(string(response))
	if err != nil {
		errStr := fmt.Sprintf("Error retrieving state from ledger for queried chaincode: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}

	queryArgs = util.ToChaincodeArgs(f, "b")
	response, err = stub.InvokeChaincode(chaincodeURL, queryArgs)
	if err != nil {
		errStr := fmt.Sprintf("Failed to query chaincode. Got error: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}
	Bval, err = strconv.Atoi(string(response))
	if err != nil {
		errStr := fmt.Sprintf("Error retrieving state from ledger for queried chaincode: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}

	// Compute sum
	sumVal = Aval + Bval

	// Write sumVal back to the ledger
	err = stub.PutState(sum, []byte(strconv.Itoa(sumVal)))
	if err != nil {
		return nil, err
	}

	fmt.Printf("Invoke chaincode successful. Got sum %d\n", sumVal)
	return []byte(strconv.Itoa(sumVal)), nil
}

func (t *SimpleChaincode) query(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	var sum string             // Sum entity
	var Aval, Bval, sumVal int // value of sum entity - to be computed
	var err error

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	chaincodeURL := args[0]
	sum = args[1]

	// Query chaincode_example02
	f := "query"
	queryArgs := util.ToChaincodeArgs(f, "a")
	response, err := stub.InvokeChaincode(chaincodeURL, queryArgs)
	if err != nil {
		errStr := fmt.Sprintf("Failed to query chaincode. Got error: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}
	Aval, err = strconv.Atoi(string(response))
	if err != nil {
		errStr := fmt.Sprintf("Error retrieving state from ledger for queried chaincode: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}

	queryArgs = util.ToChaincodeArgs(f, "b")
	response, err = stub.InvokeChaincode(chaincodeURL, queryArgs)
	if err != nil {
		errStr := fmt.Sprintf("Failed to query chaincode. Got error: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}
	Bval, err = strconv.Atoi(string(response))
	if err != nil {
		errStr := fmt.Sprintf("Error retrieving state from ledger for queried chaincode: %s", err.Error())
		fmt.Printf(errStr)
		return nil, errors.New(errStr)
	}

	// Compute sum
	sumVal = Aval + Bval

	fmt.Printf("Query chaincode successful. Got sum %d\n", sumVal)
	jsonResp := "{\"Name\":\"" + sum + "\",\"Value\":\"" + strconv.Itoa(sumVal) + "\"}"
	fmt.Printf("Query Response:%s\n", jsonResp)
	return []byte(strconv.Itoa(sumVal)), nil
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
