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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"

	"github.com/hyperledger/fabric/orderer/common/bootstrap"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/static"
	"github.com/hyperledger/fabric/orderer/kafka"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/fileledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	"github.com/hyperledger/fabric/orderer/solo"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/Shopify/sarama"
	"github.com/op/go-logging"
	"google.golang.org/grpc"
)

var logger = logging.MustGetLogger("orderer/main")

func main() {
	conf := config.Load()

	// Start the profiling service if enabled. The ListenAndServe()
	// call does not return unless an error occurs.
	if conf.General.Profile.Enabled {
		go func() {
			logger.Infof("Starting Go pprof profiling service on %s", conf.General.Profile.Address)
			panic(fmt.Errorf("Go pprof service failed: %s", http.ListenAndServe(conf.General.Profile.Address, nil)))
		}()
	}

	switch conf.General.OrdererType {
	case "solo":
		launchGeneric(conf)
	case "kafka":
		launchKafka(conf)
	default:
		panic("Invalid orderer type specified in config")
	}
}

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func launchGeneric(conf *config.TopLevel) {
	grpcServer := grpc.NewServer()

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort))
	if err != nil {
		fmt.Println("Failed to listen:", err)
		return
	}

	var bootstrapper bootstrap.Helper

	// Select the bootstrapping mechanism
	switch conf.General.GenesisMethod {
	case "static":
		bootstrapper = static.New()
	default:
		panic(fmt.Errorf("Unknown genesis method %s", conf.General.GenesisMethod))
	}

	genesisBlock, err := bootstrapper.GenesisBlock()

	if err != nil {
		panic(fmt.Errorf("Error retrieving the genesis block %s", err))
	}

	// Stand in until real config
	ledgerType := os.Getenv("ORDERER_LEDGER_TYPE")
	var lf rawledger.Factory
	switch ledgerType {
	case "file":
		location := conf.FileLedger.Location
		if location == "" {
			var err error
			location, err = ioutil.TempDir("", conf.FileLedger.Prefix)
			if err != nil {
				panic(fmt.Errorf("Error creating temp dir: %s", err))
			}
		}

		lf, _ = fileledger.New(location, genesisBlock)
	case "ram":
		fallthrough
	default:
		lf, _ = ramledger.New(int(conf.RAMLedger.HistorySize), genesisBlock)
	}

	consenters := make(map[string]multichain.Consenter)
	consenters["solo"] = solo.New(conf.General.BatchTimeout)

	manager := multichain.NewManagerImpl(lf, consenters)

	server := NewServer(
		manager,
		int(conf.General.QueueSize),
		int(conf.General.MaxWindowSize),
	)

	ab.RegisterAtomicBroadcastServer(grpcServer, server)
	grpcServer.Serve(lis)
}

func launchKafka(conf *config.TopLevel) {
	var kafkaVersion = sarama.V0_9_0_1 // TODO Ideally we'd set this in the YAML file but its type makes this impossible
	conf.Kafka.Version = kafkaVersion

	var loglevel string
	var verbose bool

	flag.StringVar(&loglevel, "loglevel", "info",
		"Set the logging level for the orderer. (Suggested values: info, debug)")
	flag.BoolVar(&verbose, "verbose", false,
		"Turn on logging for the Kafka library. (Default: \"false\")")
	flag.Parse()

	kafka.SetLogLevel(loglevel)
	if verbose {
		sarama.Logger = log.New(os.Stdout, "[sarama] ", log.Lshortfile)
	}

	ordererSrv := kafka.New(conf)
	defer ordererSrv.Teardown()

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort))
	if err != nil {
		panic(err)
	}
	rpcSrv := grpc.NewServer() // TODO Add TLS support
	ab.RegisterAtomicBroadcastServer(rpcSrv, ordererSrv)
	go rpcSrv.Serve(lis)

	// Trap SIGINT to trigger a shutdown
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	for range signalChan {
		logger.Info("Server shutting down")
		return
	}
}
