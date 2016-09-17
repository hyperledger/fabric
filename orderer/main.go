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

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/fileledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	"github.com/hyperledger/fabric/orderer/solo"

	"google.golang.org/grpc"
)

func main() {

	address := os.Getenv("ORDERER_LISTEN_ADDRESS")
	if address == "" {
		address = "127.0.0.1"
	}

	port := os.Getenv("ORDERER_LISTEN_PORT")
	if port == "" {
		port = "5005"
	}

	lis, err := net.Listen("tcp", address+":"+port)
	if err != nil {
		fmt.Println("Failed to listen:", err)
		return
	}

	grpcServer := grpc.NewServer()

	// Stand in until real config
	ledgerType := os.Getenv("ORDERER_LEDGER_TYPE")
	var rawledger rawledger.ReadWriter
	switch ledgerType {
	case "file":
		name, err := ioutil.TempDir("", "hyperledger") // TODO, config
		if err != nil {
			panic(fmt.Errorf("Error creating temp dir: %s", err))
		}

		rawledger = fileledger.New(name)
	case "ram":
		fallthrough
	default:
		historySize := 10 // TODO, config
		rawledger = ramledger.New(historySize)
	}

	queueSize := 100 // TODO configure
	batchSize := 10
	batchTimeout := 10 * time.Second
	solo.New(queueSize, batchSize, batchTimeout, rawledger, grpcServer)
	grpcServer.Serve(lis)
}
