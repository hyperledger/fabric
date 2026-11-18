/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
	peer "github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

type assetChaincode struct{}

func (assetChaincode) Init(shim.ChaincodeStubInterface) *peer.Response {
	return shim.Success(nil)
}

func (assetChaincode) Invoke(stub shim.ChaincodeStubInterface) *peer.Response {
	function, args := stub.GetFunctionAndParameters()
	switch function {
	case "InitAsset":
		if len(args) != 2 || args[0] == "" || args[1] == "" {
			return shim.Error("InitAsset expects a non-empty key and value")
		}
		current, err := stub.GetState(args[0])
		if err != nil {
			return shim.Error(fmt.Sprintf("read asset: %v", err))
		}
		if current != nil {
			return shim.Error("asset already exists")
		}
		if err := stub.PutState(args[0], []byte(args[1])); err != nil {
			return shim.Error(fmt.Sprintf("write asset: %v", err))
		}
		if err := stub.SetEvent("AssetInitialized", []byte(args[0])); err != nil {
			return shim.Error(fmt.Sprintf("emit initialization event: %v", err))
		}
		return shim.Success(nil)
	case "Set":
		if len(args) != 2 || args[0] == "" || args[1] == "" {
			return shim.Error("Set expects a non-empty key and value")
		}
		current, err := stub.GetState(args[0])
		if err != nil {
			return shim.Error(fmt.Sprintf("read asset: %v", err))
		}
		if current == nil {
			return shim.Error("asset does not exist")
		}
		if err := stub.PutState(args[0], []byte(args[1])); err != nil {
			return shim.Error(fmt.Sprintf("write asset: %v", err))
		}
		if err := stub.SetEvent("AssetUpdated", []byte(args[0])); err != nil {
			return shim.Error(fmt.Sprintf("emit update event: %v", err))
		}
		return shim.Success(nil)
	case "Get":
		if len(args) != 1 || args[0] == "" {
			return shim.Error("Get expects a non-empty key")
		}
		value, err := stub.GetState(args[0])
		if err != nil {
			return shim.Error(fmt.Sprintf("read asset: %v", err))
		}
		if value == nil {
			return shim.Error("asset does not exist")
		}
		return shim.Success(value)
	default:
		return shim.Error("expected InitAsset, Set, or Get")
	}
}
