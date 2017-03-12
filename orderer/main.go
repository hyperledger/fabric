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

	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/file"
	"github.com/hyperledger/fabric/orderer/kafka"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/sbft"
	"github.com/hyperledger/fabric/orderer/solo"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/common/localmsp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	logging "github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/main")

func main() {
	conf := config.Load()

	// Set the logging level
	flogging.InitFromSpec(conf.General.LogLevel)
	if conf.Kafka.Verbose {
		sarama.Logger = log.New(os.Stdout, "[sarama] ", log.Lshortfile)
	}

	// Start the profiling service if enabled.
	// The ListenAndServe() call does not return unless an error occurs.
	if conf.General.Profile.Enabled {
		go func() {
			logger.Info("Starting Go pprof profiling service on:", conf.General.Profile.Address)
			logger.Panic("Go pprof service failed:", http.ListenAndServe(conf.General.Profile.Address, nil))
		}()
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort))
	if err != nil {
		logger.Error("Failed to listen:", err)
		return
	}

	// secure server config
	secureConfig := comm.SecureServerConfig{
		UseTLS:            conf.General.TLS.Enabled,
		RequireClientCert: conf.General.TLS.ClientAuthEnabled,
	}
	// check to see if TLS is enabled
	if secureConfig.UseTLS {
		logger.Info("Starting orderer with TLS enabled")
		// load crypto material from files
		serverCertificate, err := ioutil.ReadFile(conf.General.TLS.Certificate)
		if err != nil {
			logger.Fatalf("Failed to load ServerCertificate file '%s' (%s)",
				conf.General.TLS.Certificate, err)
		}
		serverKey, err := ioutil.ReadFile(conf.General.TLS.PrivateKey)
		if err != nil {
			logger.Fatalf("Failed to load PrivateKey file '%s' (%s)",
				conf.General.TLS.PrivateKey, err)
		}
		var serverRootCAs, clientRootCAs [][]byte
		for _, serverRoot := range conf.General.TLS.RootCAs {
			root, err := ioutil.ReadFile(serverRoot)
			if err != nil {
				logger.Fatalf("Failed to load ServerRootCAs file '%s' (%s)",
					err, serverRoot)
			}
			serverRootCAs = append(serverRootCAs, root)
		}
		if secureConfig.RequireClientCert {
			for _, clientRoot := range conf.General.TLS.ClientRootCAs {
				root, err := ioutil.ReadFile(clientRoot)
				if err != nil {
					logger.Fatalf("Failed to load ClientRootCAs file '%s' (%s)",
						err, clientRoot)
				}
				clientRootCAs = append(clientRootCAs, root)
			}
		}
		secureConfig.ServerKey = serverKey
		secureConfig.ServerCertificate = serverCertificate
		secureConfig.ServerRootCAs = serverRootCAs
		secureConfig.ClientRootCAs = clientRootCAs
	}

	// Create GRPC server - return if an error occurs
	grpcServer, err := comm.NewGRPCServerFromListener(lis, secureConfig)
	if err != nil {
		logger.Error("Failed to return new GRPC server:", err)
		return
	}

	// Load local MSP
	err = mspmgmt.LoadLocalMsp(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil { // Handle errors reading the config file
		logger.Panic("Failed to initialize local MSP:", err)
	}

	lf, _ := createLedgerFactory(conf)

	// Are we bootstrapping?
	if len(lf.ChainIDs()) == 0 {
		var genesisBlock *cb.Block

		// Select the bootstrapping mechanism
		switch conf.General.GenesisMethod {
		case "provisional":
			genesisBlock = provisional.New(genesisconfig.Load(conf.General.GenesisProfile)).GenesisBlock()
		case "file":
			genesisBlock = file.New(conf.General.GenesisFile).GenesisBlock()
		default:
			logger.Panic("Unknown genesis method:", conf.General.GenesisMethod)
		}

		chainID, err := utils.GetChainIDFromBlock(genesisBlock)
		if err != nil {
			logger.Error("Failed to parse chain ID from genesis block:", err)
			return
		}
		gl, err := lf.GetOrCreate(chainID)
		if err != nil {
			logger.Error("Failed to create the system chain:", err)
			return
		}

		err = gl.Append(genesisBlock)
		if err != nil {
			logger.Error("Could not write genesis block to ledger:", err)
			return
		}
	} else {
		logger.Info("Not bootstrapping because of existing chains")
	}

	consenters := make(map[string]multichain.Consenter)
	consenters["solo"] = solo.New()
	consenters["kafka"] = kafka.New(conf.Kafka.Version, conf.Kafka.Retry, conf.Kafka.TLS)
	consenters["sbft"] = sbft.New(makeSbftConsensusConfig(conf), makeSbftStackConfig(conf))

	signer := localmsp.NewSigner()

	manager := multichain.NewManagerImpl(lf, consenters, signer)

	server := NewServer(
		manager,
		signer,
	)

	ab.RegisterAtomicBroadcastServer(grpcServer.Server(), server)
	logger.Info("Beginning to serve requests")
	grpcServer.Start()
}
