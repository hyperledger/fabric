/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package caller

import (
	"github.com/hyperledger/fabric-chaincode-go/shim"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// CC example simple Chaincode implementation
type CC struct{}

func (t *CC) Init(stub shim.ChaincodeStubInterface) pb.Response {
	err := stub.PutState("foo", []byte("caller:foo"))
	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(nil)
}

func (t *CC) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	fn, args := stub.GetFunctionAndParameters()
	switch fn {
	case "INVOKE":
		err := stub.PutState("foo", []byte("caller:bar"))
		if err != nil {
			return shim.Error(err.Error())
		}

		return stub.InvokeChaincode(string(args[0]), [][]byte{[]byte("INVOKE")}, "")

	case "QUERYCALLEE":
		response := stub.InvokeChaincode(string(args[0]), [][]byte{[]byte("QUERY")}, args[1])
		if response.Status >= 400 {
			return shim.Error(response.Message)
		}

		return shim.Success(response.Payload)

	case "QUERY":
		val, err := stub.GetState("foo")
		if err != nil {
			return shim.Error(err.Error())
		}

		return shim.Success(val)
	default:
		return shim.Error("unknown function")
	}
}
