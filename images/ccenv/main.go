/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package main

// This enables govendor to pull in external dependencies in the Docker build
import (
	_ "github.com/hyperledger/fabric/core/chaincode/shim"
	_ "github.com/hyperledger/fabric/core/chaincode/shim/ext/attrmgr"
	_ "github.com/hyperledger/fabric/core/chaincode/shim/ext/cid"
	_ "github.com/hyperledger/fabric/core/chaincode/shim/ext/entities"
	_ "github.com/hyperledger/fabric/core/chaincode/shim/ext/statebased"
)

func main() {
	return
}
