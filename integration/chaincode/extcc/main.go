/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric/integration/chaincode/simple"
)

func main() {
	if len(os.Args) < 6 {
		fmt.Println("usage: <package-id> <listener-address>")
		os.Exit(1)
	}

	key, err := ioutil.ReadFile(os.Args[3])
	if err != nil {
		panic(fmt.Sprintf("Cannot read key file %s, error: %s", os.Args[3], err))
	}
	cert, err := ioutil.ReadFile(os.Args[4])
	if err != nil {
		panic(fmt.Sprintf("Cannot read cert file %s, error: %s", os.Args[3], err))
	}
	clientCACert, err := ioutil.ReadFile(os.Args[5])
	if err != nil {
		panic(fmt.Sprintf("Cannot read key file %s, error: %s", os.Args[3], err))
	}

	server := &shim.ChaincodeServer{
		CCID:    os.Args[1],
		Address: os.Args[2],
		CC:      new(simple.SimpleChaincode),
		TLSProps: shim.TLSProperties{
			Key:           key,
			Cert:          cert,
			ClientCACerts: clientCACert,
		},
	}
	// do not modify - needed for integration test
	fmt.Printf("Starting chaincode %s at %s\n", server.CCID, server.Address)
	err = server.Start()
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}
