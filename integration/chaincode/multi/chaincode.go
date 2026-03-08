/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multi

import (
	"fmt"
	"strconv"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

type Operations struct{}

// Init initializes chaincode
// ===========================
func (t *Operations) Init(_ shim.ChaincodeStubInterface) *pb.Response {
	return shim.Success(nil)
}

// Invoke - Our entry point for Invocations
// ========================================
func (t *Operations) Invoke(stub shim.ChaincodeStubInterface) *pb.Response {
	function, args := stub.GetFunctionAndParameters()
	fmt.Println("invoke is running " + function)

	switch function {
	case "invoke":
		startWB, _ := strconv.ParseBool(args[0])
		if startWB {
			stub.StartWriteBatch()
		}
		if len(args) != 2 {
			return shim.Error("Incorrect number of arguments. Expecting 1")
		}
		return t.put(stub, args[1])
	case "get-key":
		if len(args) != 1 {
			return shim.Error("Incorrect number of arguments. Expecting 1")
		}
		return t.getKey(stub, args[0])
	case "put-private-key":
		startWB, _ := strconv.ParseBool(args[0])
		if startWB {
			stub.StartWriteBatch()
		}
		if len(args) != 2 {
			return shim.Error("Incorrect number of arguments. Expecting 1")
		}
		return t.putPrivateKey(stub, args[1])
	case "get-multiple-keys":
		if len(args) != 1 {
			return shim.Error("Incorrect number of arguments. Expecting 1")
		}
		return t.getMultiple(stub, args[0])
	default:
		// error
		fmt.Println("invoke did not find func: " + function)
		return shim.Error("Received unknown function invocation")
	}
}

// put to state keys from "key0" to "key(cntCall-1)"
func (t *Operations) put(stub shim.ChaincodeStubInterface, numberCallsPut string) *pb.Response {
	cntCall, _ := strconv.Atoi(numberCallsPut)

	for i := range cntCall {
		key := "key" + strconv.Itoa(i)
		err := stub.PutState(key, []byte(key))
		if err != nil {
			return shim.Error(err.Error())
		}
	}
	return shim.Success(nil)
}

func (t *Operations) getKey(stub shim.ChaincodeStubInterface, keyUniq string) *pb.Response {
	key := "key" + keyUniq
	resp, err := stub.GetState(key)
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(resp)
}

// put to private key with a collection is not exists
func (t *Operations) putPrivateKey(stub shim.ChaincodeStubInterface, numberCallsPut string) *pb.Response {
	cntCall, _ := strconv.Atoi(numberCallsPut)

	for i := range cntCall {
		key := "key" + strconv.Itoa(i)
		col := "col" + strconv.Itoa(i)
		err := stub.PutPrivateData(col, key, []byte(key))
		if err != nil {
			return shim.Error(err.Error())
		}
	}
	return shim.Success(nil)
}

// getMultiple - get multiple states
func (t *Operations) getMultiple(stub shim.ChaincodeStubInterface, countKeys string) *pb.Response {
	num, _ := strconv.Atoi(countKeys)

	keys := make([]string, 0, num)

	keys = append(keys, "non-exist-key")
	for i := range num {
		key := "key" + strconv.Itoa(i)
		keys = append(keys, key)
	}

	resps, err := stub.GetMultipleStates(keys...)
	if err != nil {
		return shim.Error(err.Error())
	}

	if len(resps) != num+1 {
		return shim.Error("number of results is not correct")
	}

	// non exist key return nil
	if resps[0] != nil {
		errStr := fmt.Sprintf("incorrect result %d elem, got %v", 0, string(resps[0]))
		return shim.Error(errStr)
	}

	if string(resps[1]) != "key"+strconv.Itoa(0) {
		errStr := fmt.Sprintf("incorrect result %d elem, got %v", 0, string(resps[1]))
		return shim.Error(errStr)
	}

	if string(resps[num]) != "key"+strconv.Itoa(num-1) {
		errStr := fmt.Sprintf("incorrect result %d elem, got %v", 0, string(resps[num]))
		return shim.Error(errStr)
	}

	return shim.Success(nil)
}
