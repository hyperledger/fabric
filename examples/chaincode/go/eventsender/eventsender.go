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

//WARNING - this chaincode's ID is hard-coded in chaincode_example04 to illustrate one way of
//calling chaincode from a chaincode. If this example is modified, chaincode_example04.go has
//to be modified as well with the new ID of chaincode_example02.
//chaincode_example05 show's how chaincode ID can be passed in as a parameter instead of
//hard-coding.

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

// EventSender example simple Chaincode implementation
type EventSender struct {
}

// Init function
func (t *EventSender) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	err := stub.PutState("noevents", []byte("0"))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Invoke function
func (t *EventSender) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	b, err := stub.GetState("noevents")
	if err != nil {
		return nil, errors.New("Failed to get state")
	}
	noevts, _ := strconv.Atoi(string(b))

	tosend := "Event " + string(b)
	for _, s := range args {
		tosend = tosend + "," + s
	}

	err = stub.PutState("noevents", []byte(strconv.Itoa(noevts+1)))
	if err != nil {
		return nil, err
	}

	err = stub.SetEvent("evtsender", []byte(tosend))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// Query function
func (t *EventSender) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	b, err := stub.GetState("noevents")
	if err != nil {
		return nil, errors.New("Failed to get state")
	}
	jsonResp := "{\"NoEvents\":\"" + string(b) + "\"}"
	return []byte(jsonResp), nil
}

func main() {
	err := shim.Start(new(EventSender))
	if err != nil {
		fmt.Printf("Error starting EventSender chaincode: %s", err)
	}
}
