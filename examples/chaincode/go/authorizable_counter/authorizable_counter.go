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
)

// AuthorizableCounterChaincode is an example that use Attribute Based Access Control to control the access to a counter by users with an specific role.
// In this case only users which TCerts contains the attribute position with the value "Software Engineer" will be able to increment the counter.
type AuthorizableCounterChaincode struct {
}

//Init the chaincode asigned the value "0" to the counter in the state.
func (t *AuthorizableCounterChaincode) Init(stub shim.ChaincodeStubInterface) ([]byte, error) {
	err := stub.PutState("counter", []byte("0"))
	return nil, err
}

//Invoke makes increment counter
func (t *AuthorizableCounterChaincode) increment(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	val, err := stub.ReadCertAttribute("position")
	fmt.Printf("Position => %v error %v \n", string(val), err)
	isOk, _ := stub.VerifyAttribute("position", []byte("Software Engineer")) // Here the ABAC API is called to verify the attribute, just if the value is verified the counter will be incremented.
	if isOk {
		counter, err := stub.GetState("counter")
		if err != nil {
			return nil, err
		}
		var cInt int
		cInt, err = strconv.Atoi(string(counter))
		if err != nil {
			return nil, err
		}
		cInt = cInt + 1
		counter = []byte(strconv.Itoa(cInt))
		stub.PutState("counter", counter)
	}
	return nil, nil

}

func (t *AuthorizableCounterChaincode) read(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	var err error

	// Get the state from the ledger
	Avalbytes, err := stub.GetState("counter")
	if err != nil {
		jsonResp := "{\"Error\":\"Failed to get state for counter\"}"
		return nil, errors.New(jsonResp)
	}

	if Avalbytes == nil {
		jsonResp := "{\"Error\":\"Nil amount for counter\"}"
		return nil, errors.New(jsonResp)
	}

	jsonResp := "{\"Name\":\"counter\",\"Amount\":\"" + string(Avalbytes) + "\"}"
	fmt.Printf("Query Response:%s\n", jsonResp)
	return Avalbytes, nil
}

// Invoke  method is the interceptor of all invocation transactions, its job is to direct
// invocation transactions to intended APIs
func (t *AuthorizableCounterChaincode) Invoke(stub shim.ChaincodeStubInterface) ([]byte, error) {
	function, args := stub.GetFunctionAndParameters()

	//	 Handle different functions
	if function == "increment" {
		return t.increment(stub, args)
	} else if function == "read" {
		return t.read(stub, args)
	}
	return nil, errors.New("Received unknown function invocation, Expecting \"increment\" \"read\"")
}

func main() {
	err := shim.Start(new(AuthorizableCounterChaincode))
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}
