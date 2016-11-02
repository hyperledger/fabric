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
	"github.com/hyperledger/fabric/orderer/common/broadcastfilter"
	// "github.com/hyperledger/fabric/orderer/common/broadcastfilter/configfilter"
	"github.com/hyperledger/fabric/orderer/common/configtx"
	"github.com/hyperledger/fabric/orderer/common/policies"
	"github.com/hyperledger/fabric/orderer/config"
	"github.com/hyperledger/fabric/orderer/kafka"
	"github.com/hyperledger/fabric/orderer/rawledger"
	"github.com/hyperledger/fabric/orderer/rawledger/fileledger"
	"github.com/hyperledger/fabric/orderer/rawledger/ramledger"
	"github.com/hyperledger/fabric/orderer/solo"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
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
		launchSolo(conf)
	case "kafka":
		launchKafka(conf)
	default:
		panic("Invalid orderer type specified in config")
	}
}

// XXX This crypto helper is a stand in until we have a real crypto handler
// it considers all signatures to be valid
type xxxCryptoHelper struct{}

func (xxx xxxCryptoHelper) VerifySignature(msg []byte, ids []byte, sigs []byte) bool {
	return true
}

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func retrieveConfiguration(rl rawledger.Reader) *cb.ConfigurationEnvelope {
	var lastConfigTx *cb.ConfigurationEnvelope

	envelope := new(cb.Envelope)
	payload := new(cb.Payload)
	configurationEnvelope := new(cb.ConfigurationEnvelope)

	it, _ := rl.Iterator(ab.SeekInfo_OLDEST, 0)
	// Iterate over the blockchain, looking for config transactions, track the most recent one encountered
	// This will be the transaction which is returned
	for {
		select {
		case <-it.ReadyChan():
			block, status := it.Next()
			if status != cb.Status_SUCCESS {
				panic(fmt.Errorf("Error parsing blockchain at startup: %v", status))
			}
			if len(block.Data.Data) != 1 {
				continue
			}
			if err := proto.Unmarshal(block.Data.Data[0], envelope); err != nil {
				panic(fmt.Errorf("Block doesn't carry a message envelope: %s", err))
			}
			if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
				panic(fmt.Errorf("Message envelope doesn't carry a payload: %s", err))
			}
			if payload.Header.ChainHeader.Type != int32(cb.HeaderType_CONFIGURATION_TRANSACTION) {
				continue
			}
			if err := proto.Unmarshal(payload.Data, configurationEnvelope); err == nil {
				lastConfigTx = configurationEnvelope
			}
		default:
			return lastConfigTx
		}
	}
}

func bootstrapConfigManager(lastConfigTx *cb.ConfigurationEnvelope) configtx.Manager {
	policyManager := policies.NewManagerImpl(xxxCryptoHelper{})
	configHandlerMap := make(map[cb.ConfigurationItem_ConfigurationType]configtx.Handler)
	for ctype := range cb.ConfigurationItem_ConfigurationType_name {
		rtype := cb.ConfigurationItem_ConfigurationType(ctype)
		switch rtype {
		case cb.ConfigurationItem_Policy:
			configHandlerMap[rtype] = policyManager
		default:
			configHandlerMap[rtype] = configtx.NewBytesHandler()
		}
	}

	configManager, err := configtx.NewConfigurationManager(lastConfigTx, policyManager, configHandlerMap)
	if err != nil {
		panic(err)
	}
	return configManager
}

func createBroadcastRuleset(configManager configtx.Manager) *broadcastfilter.RuleSet {
	return broadcastfilter.NewRuleSet([]broadcastfilter.Rule{
		broadcastfilter.EmptyRejectRule,
		// configfilter.New(configManager),
		broadcastfilter.AcceptRule,
	})
}

func launchSolo(conf *config.TopLevel) {
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
	var rawledger rawledger.ReadWriter
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

		rawledger = fileledger.New(location, genesisBlock)
	case "ram":
		fallthrough
	default:
		rawledger = ramledger.New(int(conf.RAMLedger.HistorySize), genesisBlock)
	}

	lastConfigTx := retrieveConfiguration(rawledger)
	if lastConfigTx == nil {
		panic("No chain configuration found")
	}

	configManager := bootstrapConfigManager(lastConfigTx)

	// XXX actually use the config manager in the future
	_ = configManager

	solo.New(int(conf.General.QueueSize),
		int(conf.General.BatchSize),
		int(conf.General.MaxWindowSize),
		conf.General.BatchTimeout,
		rawledger,
		grpcServer,
		createBroadcastRuleset(configManager),
		configManager,
	)
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
		fmt.Println("Server shutting down")
		return
	}
}
