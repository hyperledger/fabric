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

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

// NewKeyPerInvoke is allows the following transactions
//    "put", "key", val - returns "OK" on success
//    "get", "key" - returns val stored previously
type NewKeyPerInvoke struct {
}

//Init implements chaincode's Init interface
func (t *NewKeyPerInvoke) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

//Invoke implements chaincode's Invoke interface
func (t *NewKeyPerInvoke) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	args := stub.GetArgs()
	if len(args) < 2 {
		return shim.Error(fmt.Sprintf("invalid number of args %d", len(args)))
	}
	f := string(args[0])
	if f == "put" {
		if len(args) < 3 {
			return shim.Error(fmt.Sprintf("invalid number of args for put %d", len(args)))
		}
		err := stub.PutState(string(args[1]), args[2])
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success([]byte("OK"))
	} else if f == "get" {
		// Get the state from the ledger
		val, err := stub.GetState(string(args[1]))
		if err != nil {
			return shim.Error(err.Error())
		}
		return shim.Success(val)
	}
	return shim.Error(fmt.Sprintf("unknown function %s", f))
}

func main() {
	err := shim.Start(new(NewKeyPerInvoke))
	if err != nil {
		fmt.Printf("Error starting New key per invoke: %s", err)
	}
}
