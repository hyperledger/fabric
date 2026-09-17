/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvexecutor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
)

// KVExecutor is a chaincode implementation that takes a KVData array as read parameter
// and a KVData array as write parameter, and then calls GetXXX/PutXXX methods to read and write
// state/collection data. Both input params should be marshalled json data and then base64 encoded.
type KVExecutor struct{}

// KVData contains the data to read/write a key.
// Key is required. Value is required for write and ignored for read.
// When Collection is empty string "", it will read/write state data.
// When Collection is not empty string "", it will read/write private data in the collection.
type KVData struct {
	Collection string `json:"collection"` // optional
	Key        string `json:"key"`        // required
	Value      string `json:"value"`      // required for read, ignored for write
}

// Init initializes chaincode
// ===========================
func (t *KVExecutor) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

// Invoke - Our entry point for Invocations
// ========================================
func (t *KVExecutor) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	fmt.Println("invoke is running " + function)

	switch function {
	case "readWriteKVs":
		return t.readWriteKVs(stub, args)
	default:
		// error
		fmt.Println("invoke did not find func: " + function)
		return shim.Error("Received unknown function invocation")
	}
}

// both params should be marshalled json data and base64 encoded
func (t *KVExecutor) readWriteKVs(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	if len(args) != 2 {
		return shim.Error("Incorrect number of arguments. Expecting 2 (readInputs and writeInputs)")
	}
	readInputs, err := parseArg(args[0])
	if err != nil {
		return shim.Error(err.Error())
	}
	writeInputs, err := parseArg(args[1])
	if err != nil {
		return shim.Error(err.Error())
	}

	results := make([]*KVData, 0)
	var val []byte
	for _, input := range readInputs {
		if input.Collection == "" {
			val, err = stub.GetState(input.Key)
		} else {
			val, err = stub.GetPrivateData(input.Collection, input.Key)
		}
		if err != nil {
			return shim.Error(fmt.Sprintf("failed to read data for %+v: %s", input, err))
		}
		result := KVData{Collection: input.Collection, Key: input.Key, Value: string(val)}
		results = append(results, &result)
	}

	for _, input := range writeInputs {
		if input.Collection == "" {
			err = stub.PutState(input.Key, []byte(input.Value))
		} else {
			err = stub.PutPrivateData(input.Collection, input.Key, []byte(input.Value))
		}
		if err != nil {
			return shim.Error(fmt.Sprintf("failed to write data for %+v: %s", input, err))
		}
	}

	resultBytes, err := json.Marshal(results)
	if err != nil {
		return shim.Error(fmt.Sprintf("failed to marshal results %+v: %s", results, err))
	}
	return shim.Success(resultBytes)
}

func parseArg(inputParam string) ([]*KVData, error) {
	kvdata := make([]*KVData, 0)
	inputBytes, err := base64.StdEncoding.DecodeString(inputParam)
	if inputParam == "" {
		return kvdata, nil
	}
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to base64 decode input %s", inputParam)
	}
	if err = json.Unmarshal(inputBytes, &kvdata); err != nil {
		return nil, errors.WithMessagef(err, "failed to unmarshal kvdata %s", string(inputBytes))
	}
	return kvdata, nil
}
