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

// This program is an erroneous chaincode program that attempts to put state in query context - query should return error
package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

// SimpleChaincode example simple Chaincode implementation
type SimpleChaincode struct {
}

// Init takes a string and int. These are stored as a key/value pair in the state
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	var A string // Entity
	var Aval int // Asset holding
	var err error

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	// Initialize the chaincode
	A = args[0]
	Aval, err = strconv.Atoi(args[1])
	if err != nil {
		return nil, errors.New("Expecting integer value for asset holding")
	}
	fmt.Printf("Aval = %d\n", Aval)

	// Write the state to the ledger - this put is legal within Run
	err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Invoke is a no-op
func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return nil, nil
}

// Query callback representing the query of a chaincode
func (t *SimpleChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	if function != "query" {
		return nil, errors.New("Invalid query function name. Expecting \"query\"")
	}
	var A string // Entity
	var Aval int // Asset holding
	var err error

	if len(args) != 2 {
		return nil, errors.New("Incorrect number of arguments. Expecting 2")
	}

	A = args[0]
	Aval, err = strconv.Atoi(args[1])
	if err != nil {
		return nil, errors.New("Expecting integer value for asset holding")
	}
	fmt.Printf("Aval = %d\n", Aval)

	// Write the state to the ledger - this put is illegal within Run
	err = stub.PutState(A, []byte(strconv.Itoa(Aval)))
	if err != nil {
		jsonResp := "{\"Error\":\"Cannot put state within chaincode query\"}"
		return nil, errors.New(jsonResp)
	}

	fmt.Printf("Something is wrong. This query should not have succeeded")
	return nil, nil
}

func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting chaincode: %s", err)
	}
}
