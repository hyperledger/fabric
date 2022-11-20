/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric/integration/chaincode/simple"
)

type config struct {
	ListenAddress string `json:"listen_address,omitempty"`
	Key           string `json:"key,omitempty"`  // PEM encoded key
	Cert          string `json:"cert,omitempty"` // PEM encoded certificate
	CA            string `json:"ca,omitempty"`   // PEM encode CA certificate
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: <package-id>")
		os.Exit(1)
	}

	configData, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read config file: %s", err)
		os.Exit(2)
	}

	var config config
	if err := json.Unmarshal(configData, &config); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read config file: %s", err)
		os.Exit(2)
	}

	server := &shim.ChaincodeServer{
		CCID:    os.Args[1],
		Address: config.ListenAddress,
		CC:      new(simple.SimpleChaincode),
		TLSProps: shim.TLSProperties{
			Key:           []byte(config.Key),
			Cert:          []byte(config.Cert),
			ClientCACerts: []byte(config.CA),
		},
	}

	// do not modify - needed for integration test
	fmt.Printf("Starting chaincode %s at %s\n", server.CCID, server.Address)
	err = server.Start()
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}
