/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package main

// This enables govendor to pull in external dependencies in the Docker build
import (
	_ "github.com/hyperledger/fabric-chaincode-go/pkg/attrmgr"
	_ "github.com/hyperledger/fabric-chaincode-go/pkg/cid"
	_ "github.com/hyperledger/fabric-chaincode-go/pkg/statebased"
	_ "github.com/hyperledger/fabric-chaincode-go/shim"
)

func main() {
	return
}
