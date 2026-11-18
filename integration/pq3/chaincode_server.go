/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"
	"os"

	"github.com/hyperledger/fabric-chaincode-go/v2/shim"
)

func main() {
	chaincodeID := os.Getenv("CHAINCODE_ID")
	if chaincodeID == "" {
		log.Fatal("CHAINCODE_ID is required")
	}
	address := os.Getenv("CHAINCODE_SERVER_ADDRESS")
	if address == "" {
		log.Fatal("CHAINCODE_SERVER_ADDRESS is required")
	}

	server := &shim.ChaincodeServer{
		CCID:    chaincodeID,
		Address: address,
		CC:      assetChaincode{},
		TLSProps: shim.TLSProperties{
			Disabled: true,
		},
	}
	if err := server.Start(); err != nil {
		log.Fatalf("start chaincode server: %v", err)
	}
}
