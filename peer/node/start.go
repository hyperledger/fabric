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

package node

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	genesisconfig "github.com/hyperledger/fabric/common/configtx/tool/localconfig"
	"github.com/hyperledger/fabric/common/configtx/tool/provisional"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/events/producer"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/peer/common"
	"github.com/hyperledger/fabric/peer/gossip/mcs"
	cb "github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

var chaincodeDevMode bool
var peerDefaultChain bool
var orderingEndpoint string

// XXXDefaultChannelMSPID should not be defined in production code
// It should only be referenced in tests.  However, it is necessary
// to support the 'default chain' setup so temporarilly adding until
// this concept can be removed to testing scenarios only
const XXXDefaultChannelMSPID = "DEFAULT"

func startCmd() *cobra.Command {
	// Set the flags on the node start command.
	flags := nodeStartCmd.Flags()
	flags.BoolVarP(&chaincodeDevMode, "peer-chaincodedev", "", false,
		"Whether peer in chaincode development mode")
	flags.BoolVarP(&peerDefaultChain, "peer-defaultchain", "", true,
		"Whether to start peer with chain testchainid")
	flags.StringVarP(&orderingEndpoint, "orderer", "o", "orderer:7050", "Ordering service endpoint")

	return nodeStartCmd
}

var nodeStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the node.",
	Long:  `Starts a node that interacts with the network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return serve(args)
	},
}

//start chaincodes
func initSysCCs() {
	//deploy system chaincodes
	scc.DeploySysCCs("")
	logger.Infof("Deployed system chaincodess")
}

func serve(args []string) error {
	ledgermgmt.Initialize()
	// Parameter overrides must be processed before any paramaters are
	// cached. Failures to cache cause the server to terminate immediately.
	if chaincodeDevMode {
		logger.Info("Running in chaincode development mode")
		logger.Info("Disable loading validity system chaincode")

		viper.Set("chaincode.mode", chaincode.DevModeUserRunsChaincode)

	}

	if err := peer.CacheConfiguration(); err != nil {
		return err
	}

	peerEndpoint, err := peer.GetPeerEndpoint()
	if err != nil {
		err = fmt.Errorf("Failed to get Peer Endpoint: %s", err)
		return err
	}

	listenAddr := viper.GetString("peer.listenAddress")

	secureConfig, err := peer.GetSecureConfig()
	if err != nil {
		logger.Fatalf("Error loading secure config for peer (%s)", err)
	}
	peerServer, err := peer.CreatePeerServer(listenAddr, secureConfig)
	if err != nil {
		logger.Fatalf("Failed to create peer server (%s)", err)
	}

	if secureConfig.UseTLS {
		logger.Info("Starting peer with TLS enabled")
		// set up CA support
		caSupport := comm.GetCASupport()
		caSupport.ServerRootCAs = secureConfig.ServerRootCAs
	}

	//TODO - do we need different SSL material for events ?
	ehubGrpcServer, err := createEventHubServer(secureConfig)
	if err != nil {
		grpclog.Fatalf("Failed to create ehub server: %v", err)
	}

	registerChaincodeSupport(peerServer.Server())

	logger.Debugf("Running peer")

	// Register the Admin server
	pb.RegisterAdminServer(peerServer.Server(), core.NewAdminServer())

	// Register the Endorser server
	serverEndorser := endorser.NewEndorserServer()
	pb.RegisterEndorserServer(peerServer.Server(), serverEndorser)

	// Initialize gossip component
	bootstrap := viper.GetStringSlice("peer.gossip.bootstrap")

	serializedIdentity, err := mgmt.GetLocalSigningIdentityOrPanic().Serialize()
	if err != nil {
		logger.Panicf("Failed serializing self identity: %v", err)
	}

	messageCryptoService := mcs.New(
		peer.NewChannelPolicyManagerGetter(),
		localmsp.NewSigner(),
		mgmt.NewDeserializersManager())
	service.InitGossipService(serializedIdentity, peerEndpoint.Address, peerServer.Server(), messageCryptoService, bootstrap...)
	defer service.GetGossipService().Stop()

	//initialize system chaincodes
	initSysCCs()

	// Begin startup of default chain
	if peerDefaultChain {
		if orderingEndpoint == "" {
			logger.Panic("No ordering service endpoint provided, please use -o option.")
		}

		if len(strings.Split(orderingEndpoint, ":")) != 2 {
			logger.Panicf("Invalid format of ordering service endpoint, %s.", orderingEndpoint)
		}

		chainID := util.GetTestChainID()

		var block *cb.Block

		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Fatalf("Peer configured to start with the default test chain, but supporting configuration files did not match.  Please ensure that configtx.yaml contains the unmodified SampleSingleMSPSolo profile and that msp/sampleconfig is present.\n%s", err)
				}
			}()

			genConf := genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile)
			genConf.Orderer.Addresses = []string{orderingEndpoint}
			genConf.Application.Organizations[0].Name = XXXDefaultChannelMSPID
			genConf.Application.Organizations[0].ID = XXXDefaultChannelMSPID
			block = provisional.New(genConf).GenesisBlockForChannel(chainID)
		}()

		//this creates testchainid and sets up gossip
		if err = peer.CreateChainFromBlock(block); err == nil {
			fmt.Printf("create chain [%s]", chainID)
			scc.DeploySysCCs(chainID)
			logger.Infof("Deployed system chaincodes on %s", chainID)
		} else {
			fmt.Printf("create default chain [%s] failed with %s", chainID, err)
		}
	}

	//this brings up all the chains (including testchainid)
	peer.Initialize(func(cid string) {
		logger.Debugf("Deploying system CC, for chain <%s>", cid)
		scc.DeploySysCCs(cid)
	})

	logger.Infof("Starting peer with ID=[%s], network ID=[%s], address=[%s]",
		peerEndpoint.Id, viper.GetString("peer.networkId"), peerEndpoint.Address)

	// Start the grpc server. Done in a goroutine so we can deploy the
	// genesis block if needed.
	serve := make(chan error)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		serve <- nil
	}()

	go func() {
		var grpcErr error
		if grpcErr = peerServer.Start(); grpcErr != nil {
			grpcErr = fmt.Errorf("grpc server exited with error: %s", grpcErr)
		} else {
			logger.Info("peer server exited")
		}
		serve <- grpcErr
	}()

	if err := writePid(viper.GetString("peer.fileSystemPath")+"/peer.pid", os.Getpid()); err != nil {
		return err
	}

	// Start the event hub server
	if ehubGrpcServer != nil {
		go ehubGrpcServer.Start()
	}

	// Start profiling http endpoint if enabled
	if viper.GetBool("peer.profile.enabled") {
		go func() {
			profileListenAddress := viper.GetString("peer.profile.listenAddress")
			logger.Infof("Starting profiling server with listenAddress = %s", profileListenAddress)
			if profileErr := http.ListenAndServe(profileListenAddress, nil); profileErr != nil {
				logger.Errorf("Error starting profiler: %s", profileErr)
			}
		}()
	}

	logger.Infof("Started peer with ID=[%s], network ID=[%s], address=[%s]",
		peerEndpoint.Id, viper.GetString("peer.networkId"), peerEndpoint.Address)

	// sets the logging level for the 'error' and 'msp' modules to the
	// values from core.yaml. they can also be updated dynamically using
	// "peer logging setlevel <module-name> <log-level>"
	common.SetLogLevelFromViper("error")
	common.SetLogLevelFromViper("msp")

	// Block until grpc server exits
	return <-serve
}

//NOTE - when we implment JOIN we will no longer pass the chainID as param
//The chaincode support will come up without registering system chaincodes
//which will be registered only during join phase.
func registerChaincodeSupport(grpcServer *grpc.Server) {
	//get user mode
	userRunsCC := chaincode.IsDevMode()

	//get chaincode startup timeout
	tOut, err := strconv.Atoi(viper.GetString("chaincode.startuptimeout"))
	if err != nil { //what went wrong ?
		fmt.Printf("could not retrive timeout var...setting to 5secs\n")
		tOut = 5000
	}
	ccStartupTimeout := time.Duration(tOut) * time.Millisecond

	ccSrv := chaincode.NewChaincodeSupport(peer.GetPeerEndpoint, userRunsCC, ccStartupTimeout)

	//Now that chaincode is initialized, register all system chaincodes.
	scc.RegisterSysCCs()

	pb.RegisterChaincodeSupportServer(grpcServer, ccSrv)
}

func createEventHubServer(secureConfig comm.SecureServerConfig) (comm.GRPCServer, error) {
	var lis net.Listener
	var err error
	lis, err = net.Listen("tcp", viper.GetString("peer.events.address"))
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer, err := comm.NewGRPCServerFromListener(lis, secureConfig)
	if err != nil {
		fmt.Println("Failed to return new GRPC server: ", err)
		return nil, err
	}
	ehServer := producer.NewEventsServer(
		uint(viper.GetInt("peer.events.buffersize")),
		viper.GetInt("peer.events.timeout"))

	pb.RegisterEventsServer(grpcServer.Server(), ehServer)
	return grpcServer, nil
}

func writePid(fileName string, pid int) error {
	err := os.MkdirAll(filepath.Dir(fileName), 0755)
	if err != nil {
		return err
	}

	fd, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer fd.Close()
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("can't lock '%s', lock is held", fd.Name())
	}

	if _, err := fd.Seek(0, 0); err != nil {
		return err
	}

	if err := fd.Truncate(0); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(fd, "%d", pid); err != nil {
		return err
	}

	if err := fd.Sync(); err != nil {
		return err
	}

	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("can't release lock '%s', lock is held", fd.Name())
	}
	return nil
}
