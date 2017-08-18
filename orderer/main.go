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
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/file"
	"github.com/hyperledger/fabric/orderer/kafka"
	"github.com/hyperledger/fabric/orderer/ledger"
	"github.com/hyperledger/fabric/orderer/localconfig"
	"github.com/hyperledger/fabric/orderer/metadata"
	"github.com/hyperledger/fabric/orderer/multichain"
	"github.com/hyperledger/fabric/orderer/solo"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/common/localmsp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	logging "github.com/op/go-logging"
	"gopkg.in/alecthomas/kingpin.v2"
)

var logger = logging.MustGetLogger("orderer/main")

//command line flags
var (
	app = kingpin.New("orderer", "Hyperledger Fabric orderer node")

	start   = app.Command("start", "Start the orderer node").Default()
	version = app.Command("version", "Show version information")
)

func main() {

	kingpin.Version("0.0.1")
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {

	// "start" command
	case start.FullCommand():
		logger.Infof("Starting %s", metadata.GetVersionInfo())
		conf := config.Load()
		initializeLoggingLevel(conf)
		initializeProfilingService(conf)
		grpcServer := initializeGrpcServer(conf)
		initializeLocalMsp(conf)
		signer := localmsp.NewSigner()
		manager := initializeMultiChainManager(conf, signer)
		server := NewServer(manager, signer)
		ab.RegisterAtomicBroadcastServer(grpcServer.Server(), server)
		logger.Info("Beginning to serve requests")
		grpcServer.Start()
	// "version" command
	case version.FullCommand():
		fmt.Println(metadata.GetVersionInfo())
	}

}

// Set the logging level
func initializeLoggingLevel(conf *config.TopLevel) {
	flogging.InitBackend(flogging.SetFormat(conf.General.LogFormat), os.Stderr)
	flogging.InitFromSpec(conf.General.LogLevel)
	if conf.Kafka.Verbose {
		sarama.Logger = log.New(os.Stdout, "[sarama] ", log.Ldate|log.Lmicroseconds|log.Lshortfile)
	}
}

// Start the profiling service if enabled.
func initializeProfilingService(conf *config.TopLevel) {
	if conf.General.Profile.Enabled {
		go func() {
			logger.Info("Starting Go pprof profiling service on:", conf.General.Profile.Address)
			// The ListenAndServe() call does not return unless an error occurs.
			logger.Panic("Go pprof service failed:", http.ListenAndServe(conf.General.Profile.Address, nil))
		}()
	}
}

func initializeSecureServerConfig(conf *config.TopLevel) comm.SecureServerConfig {
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
	return secureConfig
}

func initializeBootstrapChannel(conf *config.TopLevel, lf ledger.Factory) {
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
		logger.Fatal("Failed to parse chain ID from genesis block:", err)
	}
	gl, err := lf.GetOrCreate(chainID)
	if err != nil {
		logger.Fatal("Failed to create the system chain:", err)
	}

	err = gl.Append(genesisBlock)
	if err != nil {
		logger.Fatal("Could not write genesis block to ledger:", err)
	}
}

func initializeGrpcServer(conf *config.TopLevel) comm.GRPCServer {
	secureConfig := initializeSecureServerConfig(conf)

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort))
	if err != nil {
		logger.Fatal("Failed to listen:", err)
	}

	// Create GRPC server - return if an error occurs
	grpcServer, err := comm.NewGRPCServerFromListener(lis, secureConfig)
	if err != nil {
		logger.Fatal("Failed to return new GRPC server:", err)
	}

	return grpcServer
}

func initializeLocalMsp(conf *config.TopLevel) {
	// Load local MSP
	err := mspmgmt.LoadLocalMsp(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil { // Handle errors reading the config file
		logger.Fatal("Failed to initialize local MSP:", err)
	}
}

func initializeMultiChainManager(conf *config.TopLevel, signer crypto.LocalSigner) multichain.Manager {
	lf, _ := createLedgerFactory(conf)
	// Are we bootstrapping?
	if len(lf.ChainIDs()) == 0 {
		initializeBootstrapChannel(conf, lf)
	} else {
		logger.Info("Not bootstrapping because of existing chains")
	}

	consenters := make(map[string]multichain.Consenter)
	consenters["solo"] = solo.New()
	consenters["kafka"] = kafka.New(conf.Kafka.TLS, conf.Kafka.Retry, conf.Kafka.Version)

	return multichain.NewManagerImpl(lf, consenters, signer)
}
