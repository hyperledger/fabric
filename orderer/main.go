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
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/file"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/provisional"
	"github.com/hyperledger/fabric/orderer/kafka"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/fileledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	"github.com/hyperledger/fabric/orderer/solo"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/Shopify/sarama"
	logging "github.com/op/go-logging"
	"google.golang.org/grpc"
)

var logger = logging.MustGetLogger("orderer/main")

func main() {
	conf := config.Load()
	flogging.InitFromSpec(conf.General.LogLevel)

	// Start the profiling service if enabled.
	// The ListenAndServe() call does not return unless an error occurs.
	if conf.General.Profile.Enabled {
		go func() {
			logger.Infof("Starting Go pprof profiling service on %s", conf.General.Profile.Address)
			panic(fmt.Errorf("Go pprof service failed: %s", http.ListenAndServe(conf.General.Profile.Address, nil)))
		}()
	}

	grpcServer := grpc.NewServer()

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort))
	if err != nil {
		fmt.Println("Failed to listen:", err)
		return
	}

	var lf rawledger.Factory
	switch conf.General.LedgerType {
	case "file":
		location := conf.FileLedger.Location
		if location == "" {
			var err error
			location, err = ioutil.TempDir("", conf.FileLedger.Prefix)
			if err != nil {
				panic(fmt.Errorf("Error creating temp dir: %s", err))
			}
		}
		lf = fileledger.New(location)
	case "ram":
		fallthrough
	default:
		lf = ramledger.New(int(conf.RAMLedger.HistorySize))
	}

	// Are we bootstrapping
	if len(lf.ChainIDs()) == 0 {
		var genesisBlock *cb.Block

		// Select the bootstrapping mechanism
		switch conf.General.GenesisMethod {
		case "provisional":
			genesisBlock = provisional.New(conf).GenesisBlock()
		case "file":
			genesisBlock = file.New(conf.General.GenesisFile).GenesisBlock()
		default:
			panic(fmt.Errorf("Unknown genesis method %s", conf.General.GenesisMethod))
		}

		chainID, err := utils.GetChainIDFromBlock(genesisBlock)
		if err != nil {
			logger.Errorf("Failed to parse chain ID from genesis block: %s", err)
			return
		}
		gl, err := lf.GetOrCreate(chainID)
		if err != nil {
			logger.Errorf("Failed to create the genesis chain: %s", err)
			return
		}

		err = gl.Append(genesisBlock)
		if err != nil {
			logger.Errorf("Could not write genesis block to ledger: %s", err)
			return
		}
	} else {
		logger.Infof("Not bootstrapping because of existing chains")
	}

	if conf.Kafka.Verbose {
		sarama.Logger = log.New(os.Stdout, "[sarama] ", log.Lshortfile)
	}

	consenters := make(map[string]multichain.Consenter)
	consenters["solo"] = solo.New()
	consenters["kafka"] = kafka.New(conf.Kafka.Version, conf.Kafka.Retry)

	manager := multichain.NewManagerImpl(lf, consenters)

	server := NewServer(
		manager,
		int(conf.General.QueueSize),
		int(conf.General.MaxWindowSize),
	)

	ab.RegisterAtomicBroadcastServer(grpcServer, server)
	grpcServer.Serve(lis)
}
