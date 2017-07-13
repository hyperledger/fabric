/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/core"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/endorser"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/events/producer"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/peer/common"
	peergossip "github.com/hyperledger/fabric/peer/gossip"
	"github.com/hyperledger/fabric/peer/version"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

//function used by chaincode support
type ccEndpointFunc func() (*pb.PeerEndpoint, error)

var chaincodeDevMode bool
var peerDefaultChain bool
var orderingEndpoint string

// XXXDefaultChannelMSPID should not be defined in production code
// It should only be referenced in tests.  However, it is necessary
// to support the 'default chain' setup so temporarily adding until
// this concept can be removed to testing scenarios only
const XXXDefaultChannelMSPID = "DEFAULT"

func startCmd() *cobra.Command {
	// Set the flags on the node start command.
	flags := nodeStartCmd.Flags()
	flags.BoolVarP(&chaincodeDevMode, "peer-chaincodedev", "", false,
		"Whether peer in chaincode development mode")
	flags.BoolVarP(&peerDefaultChain, "peer-defaultchain", "", false,
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
	logger.Infof("Starting %s", version.GetInfo())
	ledgermgmt.Initialize()
	// Parameter overrides must be processed before any parameters are
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

	// enable the cache of chaincode info
	ccprovider.EnableCCInfoCache()

	ccSrv, ccEpFunc := createChaincodeServer(peerServer, listenAddr)
	registerChaincodeSupport(ccSrv.Server(), ccEpFunc)
	go ccSrv.Start()

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

	messageCryptoService := peergossip.NewMCS(
		peer.NewChannelPolicyManagerGetter(),
		localmsp.NewSigner(),
		mgmt.NewDeserializersManager())
	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())

	// callback function for secure dial options for gossip service
	secureDialOpts := func() []grpc.DialOption {
		var dialOpts []grpc.DialOption
		// set max send/recv msg sizes
		dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(comm.MaxRecvMsgSize()),
			grpc.MaxCallSendMsgSize(comm.MaxSendMsgSize())))
		// set the keepalive options
		dialOpts = append(dialOpts, comm.ClientKeepaliveOptions()...)

		if comm.TLSEnabled() {
			tlsCert := peerServer.ServerCertificate()
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(comm.GetCASupport().GetPeerCredentials(tlsCert)))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}
		return dialOpts
	}
	err = service.InitGossipService(serializedIdentity, peerEndpoint.Address, peerServer.Server(),
		messageCryptoService, secAdv, secureDialOpts, bootstrap...)
	if err != nil {
		return err
	}
	defer service.GetGossipService().Stop()

	//initialize system chaincodes
	initSysCCs()

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
		logger.Debugf("sig: %s", sig)
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

	if err := writePid(config.GetPath("peer.fileSystemPath")+"/peer.pid", os.Getpid()); err != nil {
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

	// set the logging level for specific modules defined via environment
	// variables or core.yaml
	overrideLogModules := []string{"msp", "gossip", "ledger", "cauthdsl", "policies", "grpc"}
	for _, module := range overrideLogModules {
		err = common.SetLogLevelFromViper(module)
		if err != nil {
			logger.Warningf("Error setting log level for module '%s': %s", module, err.Error())
		}
	}

	flogging.SetPeerStartupModulesMap()

	// Block until grpc server exits
	return <-serve
}

//create a CC listener using peer.chaincodeListenAddress (and if that's not set use peer.peerAddress)
func createChaincodeServer(peerServer comm.GRPCServer, peerListenAddress string) (comm.GRPCServer, ccEndpointFunc) {
	cclistenAddress := viper.GetString("peer.chaincodeListenAddress")

	var srv comm.GRPCServer
	var ccEpFunc ccEndpointFunc

	//use the chaincode address endpoint function..
	//three cases
	// -  peer.chaincodeListenAddress not specied (use peer's server)
	// -  peer.chaincodeListenAddress identical to peer.listenAddress (use peer's server)
	// -  peer.chaincodeListenAddress different and specified (create chaincode server)
	if cclistenAddress == "" {
		//...but log a warning
		logger.Warningf("peer.chaincodeListenAddress is not set, use peer.listenAddress %s", peerListenAddress)

		//we are using peer address, use peer endpoint
		ccEpFunc = peer.GetPeerEndpoint
		srv = peerServer
	} else if cclistenAddress == peerListenAddress {
		//using peer's endpoint...log a  warning
		logger.Warningf("peer.chaincodeListenAddress is identical to peer.listenAddress %s", cclistenAddress)

		//we are using peer address, use peer endpoint
		ccEpFunc = peer.GetPeerEndpoint
		srv = peerServer
	} else {
		config, err := peer.GetSecureConfig()
		if err != nil {
			panic(err)
		}

		srv, err = comm.NewGRPCServer(cclistenAddress, config)
		if err != nil {
			panic(err)
		}
		ccEpFunc = getChaincodeAddressEndpoint
	}

	return srv, ccEpFunc
}

//NOTE - when we implment JOIN we will no longer pass the chainID as param
//The chaincode support will come up without registering system chaincodes
//which will be registered only during join phase.
func registerChaincodeSupport(grpcServer *grpc.Server, ccEpFunc ccEndpointFunc) {
	//get user mode
	userRunsCC := chaincode.IsDevMode()

	//get chaincode startup timeout
	ccStartupTimeout := viper.GetDuration("chaincode.startuptimeout")
	if ccStartupTimeout < time.Duration(5)*time.Second {
		logger.Warningf("Invalid chaincode startup timeout value %s (should be at least 5s); defaulting to 5s", ccStartupTimeout)
		ccStartupTimeout = time.Duration(5) * time.Second
	} else {
		logger.Debugf("Chaincode startup timeout value set to %s", ccStartupTimeout)
	}

	ccSrv := chaincode.NewChaincodeSupport(ccEpFunc, userRunsCC, ccStartupTimeout)

	//Now that chaincode is initialized, register all system chaincodes.
	scc.RegisterSysCCs()

	pb.RegisterChaincodeSupportServer(grpcServer, ccSrv)
}

func getChaincodeAddressEndpoint() (*pb.PeerEndpoint, error) {
	//need this for the ID to create chaincode endpoint
	peerEndpoint, err := peer.GetPeerEndpoint()
	if err != nil {
		return nil, err
	}

	ccendpoint := viper.GetString("peer.chaincodeListenAddress")
	if ccendpoint == "" {
		return nil, fmt.Errorf("peer.chaincodeListenAddress not specified")
	}

	if _, _, err = net.SplitHostPort(ccendpoint); err != nil {
		return nil, err
	}

	return &pb.PeerEndpoint{
		Id:      peerEndpoint.Id,
		Address: ccendpoint,
	}, nil
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
		logger.Errorf("Failed to return new GRPC server: %s", err)
		return nil, err
	}
	ehServer := producer.NewEventsServer(
		uint(viper.GetInt("peer.events.buffersize")),
		viper.GetDuration("peer.events.timeout"))

	pb.RegisterEventsServer(grpcServer.Server(), ehServer)
	return grpcServer, nil
}

func writePid(fileName string, pid int) error {
	err := os.MkdirAll(filepath.Dir(fileName), 0755)
	if err != nil {
		return err
	}

	buf := strconv.Itoa(pid)
	if err = ioutil.WriteFile(fileName, []byte(buf), 0644); err != nil {
		logger.Errorf("Cannot write pid to %s (err:%s)", fileName, err)
		return err
	}

	return nil
}
