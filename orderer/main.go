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

	"github.com/hyperledger/fabric/orderer/config"
	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/fileledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	"github.com/hyperledger/fabric/orderer/solo"

	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

var logger = logging.MustGetLogger("orderer")

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func main() {

	config := config.Load()
	grpcServer := grpc.NewServer()

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", config.General.ListenAddress, config.General.ListenPort))
	if err != nil {
		fmt.Println("Failed to listen:", err)
		return
	}

	// Stand in until real config
	ledgerType := os.Getenv("ORDERER_LEDGER_TYPE")
	var rawledger rawledger.ReadWriter
	switch ledgerType {
	case "file":
		location := config.FileLedger.Location
		if location == "" {
			var err error
			location, err = ioutil.TempDir("", config.FileLedger.Prefix)
			if err != nil {
				panic(fmt.Errorf("Error creating temp dir: %s", err))
			}
		}

		rawledger = fileledger.New(location)
	case "ram":
		fallthrough
	default:
		rawledger = ramledger.New(int(config.RAMLedger.HistorySize))
	}

	solo.New(int(config.General.QueueSize), int(config.General.BatchSize), int(config.General.MaxWindowSize), config.General.BatchTimeout, rawledger, grpcServer)
	grpcServer.Serve(lis)
}
