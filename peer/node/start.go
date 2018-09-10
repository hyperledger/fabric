/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	ccdef "github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/admin"
	"github.com/hyperledger/fabric/core/cclifecycle"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/car"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/chaincode/platforms/java"
	"github.com/hyperledger/fabric/core/chaincode/platforms/node"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/dockercontroller"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/endorser"
	authHandler "github.com/hyperledger/fabric/core/handlers/auth"
	endorsement2 "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	endorsement3 "github.com/hyperledger/fabric/core/handlers/endorsement/api/identities"
	"github.com/hyperledger/fabric/core/handlers/library"
	"github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/core/scc/qscc"
	"github.com/hyperledger/fabric/discovery"
	"github.com/hyperledger/fabric/discovery/endorsement"
	discsupport "github.com/hyperledger/fabric/discovery/support"
	discacl "github.com/hyperledger/fabric/discovery/support/acl"
	ccsupport "github.com/hyperledger/fabric/discovery/support/chaincode"
	"github.com/hyperledger/fabric/discovery/support/config"
	"github.com/hyperledger/fabric/discovery/support/gossip"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/service"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/peer/common"
	peergossip "github.com/hyperledger/fabric/peer/gossip"
	"github.com/hyperledger/fabric/peer/version"
	cb "github.com/hyperledger/fabric/protos/common"
	discprotos "github.com/hyperledger/fabric/protos/discovery"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/transientstore"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

const (
	chaincodeAddrKey       = "peer.chaincodeAddress"
	chaincodeListenAddrKey = "peer.chaincodeListenAddress"
	defaultChaincodePort   = 7052
)

var chaincodeDevMode bool
var orderingEndpoint string

func startCmd() *cobra.Command {
	// Set the flags on the node start command.
	flags := nodeStartCmd.Flags()
	flags.BoolVarP(&chaincodeDevMode, "peer-chaincodedev", "", false,
		"Whether peer in chaincode development mode")
	flags.StringVarP(&orderingEndpoint, "orderer", "o", "orderer:7050", "Ordering service endpoint")

	return nodeStartCmd
}

var nodeStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the node.",
	Long:  `Starts a node that interacts with the network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected")
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		return serve(args)
	},
}

func serve(args []string) error {
	// currently the peer only works with the standard MSP
	// because in certain scenarios the MSP has to make sure
	// that from a single credential you only have a single 'identity'.
	// Idemix does not support this *YET* but it can be easily
	// fixed to support it. For now, we just make sure that
	// the peer only comes up with the standard MSP
	mspType := mgmt.GetLocalMSP().GetType()
	if mspType != msp.FABRIC {
		panic("Unsupported msp type " + msp.ProviderTypeToString(mspType))
	}

	// set the logging level for specific modules defined via environment
	// variables or core.yaml
	overrideLogModules := []string{"msp", "gossip", "ledger", "cauthdsl", "policies", "grpc", "peer.gossip"}
	for _, module := range overrideLogModules {
		err := common.SetLogLevelFromViper(module)
		if err != nil {
			logger.Warningf("Error setting log level for module '%s': %s", module, err.Error())
		}
	}

	// Trace RPCs with the golang.org/x/net/trace package. This was moved out of
	// the deliver service connection factory as it has process wide implications
	// and was racy with respect to initialization of gRPC clients and servers.
	grpc.EnableTracing = true

	logger.Infof("Starting %s", version.GetInfo())

	//startup aclmgmt with default ACL providers (resource based and default 1.0 policies based).
	//Users can pass in their own ACLProvider to RegisterACLProvider (currently unit tests do this)
	aclProvider := aclmgmt.NewACLProvider(
		aclmgmt.ResourceGetter(peer.GetStableChannelConfig),
	)

	pr := platforms.NewRegistry(
		&golang.Platform{},
		&node.Platform{},
		&java.Platform{},
		&car.Platform{},
	)

	deployedCCInfoProvider := &lscc.DeployedCCInfoProvider{}

	//initialize resource management exit
	ledgermgmt.Initialize(
		&ledgermgmt.Initializer{
			CustomTxProcessors:            peer.ConfigTxProcessors,
			PlatformRegistry:              pr,
			DeployedChaincodeInfoProvider: deployedCCInfoProvider,
		})

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
	var peerHost string
	peerHost, _, err = net.SplitHostPort(peerEndpoint.Address)
	if err != nil {
		return fmt.Errorf("peer address is not in the format of host:port: %v", err)
	}

	listenAddr := viper.GetString("peer.listenAddress")

	serverConfig, err := peer.GetServerConfig()
	if err != nil {
		logger.Fatalf("Error loading secure config for peer (%s)", err)
	}
	serverConfig.Logger = flogging.MustGetLogger("core/comm").With("server", "PeerServer")
	peerServer, err := peer.NewPeerServer(listenAddr, serverConfig)
	if err != nil {
		logger.Fatalf("Failed to create peer server (%s)", err)
	}

	if serverConfig.SecOpts.UseTLS {
		logger.Info("Starting peer with TLS enabled")
		// set up credential support
		cs := comm.GetCredentialSupport()
		cs.ServerRootCAs = serverConfig.SecOpts.ServerRootCAs

		// set the cert to use if client auth is requested by remote endpoints
		clientCert, err := peer.GetClientCertificate()
		if err != nil {
			logger.Fatalf("Failed to set TLS client certificate (%s)", err)
		}
		comm.GetCredentialSupport().SetClientCertificate(clientCert)
	}

	mutualTLS := serverConfig.SecOpts.UseTLS && serverConfig.SecOpts.RequireClientCert
	policyCheckerProvider := func(resourceName string) deliver.PolicyCheckerFunc {
		return func(env *cb.Envelope, channelID string) error {
			return aclProvider.CheckACL(resourceName, channelID, env)
		}
	}

	abServer := peer.NewDeliverEventsServer(mutualTLS, policyCheckerProvider, &peer.DeliverChainManager{})
	pb.RegisterDeliverServer(peerServer.Server(), abServer)

	// Initialize chaincode service
	chaincodeSupport, ccp, sccp, packageProvider := startChaincodeServer(peerHost, aclProvider, pr)

	logger.Debugf("Running peer")

	// Start the Admin server
	startAdminServer(listenAddr, peerServer.Server())

	privDataDist := func(channel string, txID string, privateData *transientstore.TxPvtReadWriteSetWithConfigInfo, blkHt uint64) error {
		return service.GetGossipService().DistributePrivateData(channel, txID, privateData, blkHt)
	}

	signingIdentity := mgmt.GetLocalSigningIdentityOrPanic()
	serializedIdentity, err := signingIdentity.Serialize()
	if err != nil {
		logger.Panicf("Failed serializing self identity: %v", err)
	}

	libConf := library.Config{}
	if err = viperutil.EnhancedExactUnmarshalKey("peer.handlers", &libConf); err != nil {
		return errors.WithMessage(err, "could not load YAML config")
	}
	reg := library.InitRegistry(libConf)

	authFilters := reg.Lookup(library.Auth).([]authHandler.Filter)
	endorserSupport := &endorser.SupportImpl{
		SignerSupport:    signingIdentity,
		Peer:             peer.Default,
		PeerSupport:      peer.DefaultSupport,
		ChaincodeSupport: chaincodeSupport,
		SysCCProvider:    sccp,
		ACLProvider:      aclProvider,
	}
	endorsementPluginsByName := reg.Lookup(library.Endorsement).(map[string]endorsement2.PluginFactory)
	validationPluginsByName := reg.Lookup(library.Validation).(map[string]validation.PluginFactory)
	signingIdentityFetcher := (endorsement3.SigningIdentityFetcher)(endorserSupport)
	channelStateRetriever := endorser.ChannelStateRetriever(endorserSupport)
	pluginMapper := endorser.MapBasedPluginMapper(endorsementPluginsByName)
	pluginEndorser := endorser.NewPluginEndorser(&endorser.PluginSupport{
		ChannelStateRetriever:   channelStateRetriever,
		TransientStoreRetriever: peer.TransientStoreFactory,
		PluginMapper:            pluginMapper,
		SigningIdentityFetcher:  signingIdentityFetcher,
	})
	endorserSupport.PluginEndorser = pluginEndorser
	serverEndorser := endorser.NewEndorserServer(privDataDist, endorserSupport, pr)
	auth := authHandler.ChainFilters(serverEndorser, authFilters...)
	// Register the Endorser server
	pb.RegisterEndorserServer(peerServer.Server(), auth)

	policyMgr := peer.NewChannelPolicyManagerGetter()

	// Initialize gossip component
	err = initGossipService(policyMgr, peerServer, serializedIdentity, peerEndpoint.Address)
	if err != nil {
		return err
	}
	defer service.GetGossipService().Stop()

	// initialize system chaincodes

	// deploy system chaincodes
	sccp.DeploySysCCs("", ccp)
	logger.Infof("Deployed system chaincodes")

	installedCCs := func() ([]ccdef.InstalledChaincode, error) {
		return packageProvider.ListInstalledChaincodes()
	}
	lifecycle, err := cc.NewLifeCycle(cc.Enumerate(installedCCs))
	if err != nil {
		logger.Panicf("Failed creating lifecycle: +%v", err)
	}
	onUpdate := cc.HandleMetadataUpdate(func(channel string, chaincodes ccdef.MetadataSet) {
		service.GetGossipService().UpdateChaincodes(chaincodes.AsChaincodes(), gossipcommon.ChainID(channel))
	})
	lifecycle.AddListener(onUpdate)

	// this brings up all the channels
	peer.Initialize(func(cid string) {
		logger.Debugf("Deploying system CC, for channel <%s>", cid)
		sccp.DeploySysCCs(cid, ccp)
		sub, err := lifecycle.NewChannelSubscription(cid, cc.QueryCreatorFunc(func() (cc.Query, error) {
			return peer.GetLedger(cid).NewQueryExecutor()
		}))
		if err != nil {
			logger.Panicf("Failed subscribing to chaincode lifecycle updates")
		}
		cceventmgmt.GetMgr().Register(cid, sub)
	}, ccp, sccp, txvalidator.MapBasedPluginMapper(validationPluginsByName), pr, deployedCCInfoProvider)

	if viper.GetBool("peer.discovery.enabled") {
		registerDiscoveryService(peerServer, policyMgr, lifecycle)
	}

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

	// Block until grpc server exits
	return <-serve
}

func localPolicy(policyObject proto.Message) policies.Policy {
	localMSP := mgmt.GetLocalMSP()
	pp := cauthdsl.NewPolicyProvider(localMSP)
	policy, _, err := pp.NewPolicy(utils.MarshalOrPanic(policyObject))
	if err != nil {
		logger.Panicf("Failed creating local policy: +%v", err)
	}
	return policy
}

func registerDiscoveryService(peerServer *comm.GRPCServer, polMgr policies.ChannelPolicyManagerGetter, lc *cc.Lifecycle) {
	mspID := viper.GetString("peer.localMspId")
	localAccessPolicy := localPolicy(cauthdsl.SignedByAnyAdmin([]string{mspID}))
	if viper.GetBool("peer.discovery.orgMembersAllowedAccess") {
		localAccessPolicy = localPolicy(cauthdsl.SignedByAnyMember([]string{mspID}))
	}
	channelVerifier := discacl.NewChannelVerifier(policies.ChannelApplicationWriters, polMgr)
	acl := discacl.NewDiscoverySupport(channelVerifier, localAccessPolicy, discacl.ChannelConfigGetterFunc(peer.GetStableChannelConfig))
	gSup := gossip.NewDiscoverySupport(service.GetGossipService())
	ccSup := ccsupport.NewDiscoverySupport(lc)
	ea := endorsement.NewEndorsementAnalyzer(gSup, ccSup, acl, lc)
	confSup := config.NewDiscoverySupport(config.CurrentConfigBlockGetterFunc(peer.GetCurrConfigBlock))
	support := discsupport.NewDiscoverySupport(acl, gSup, ea, confSup, acl)
	svc := discovery.NewService(discovery.Config{
		TLS:                          peerServer.TLSEnabled(),
		AuthCacheEnabled:             viper.GetBool("peer.discovery.authCacheEnabled"),
		AuthCacheMaxSize:             viper.GetInt("peer.discovery.authCacheMaxSize"),
		AuthCachePurgeRetentionRatio: viper.GetFloat64("peer.discovery.authCachePurgeRetentionRatio"),
	}, support)
	logger.Info("Discovery service activated")
	discprotos.RegisterDiscoveryServer(peerServer.Server(), svc)
}

//create a CC listener using peer.chaincodeListenAddress (and if that's not set use peer.peerAddress)
func createChaincodeServer(ca tlsgen.CA, peerHostname string) (srv *comm.GRPCServer, ccEndpoint string, err error) {
	// before potentially setting chaincodeListenAddress, compute chaincode endpoint at first
	ccEndpoint, err = computeChaincodeEndpoint(peerHostname)
	if err != nil {
		if chaincode.IsDevMode() {
			// if any error for dev mode, we use 0.0.0.0:7052
			ccEndpoint = fmt.Sprintf("%s:%d", "0.0.0.0", defaultChaincodePort)
			logger.Warningf("use %s as chaincode endpoint because of error in computeChaincodeEndpoint: %s", ccEndpoint, err)
		} else {
			// for non-dev mode, we have to return error
			logger.Errorf("Error computing chaincode endpoint: %s", err)
			return nil, "", err
		}
	}

	host, _, err := net.SplitHostPort(ccEndpoint)
	if err != nil {
		logger.Panic("Chaincode service host", ccEndpoint, "isn't a valid hostname:", err)
	}

	cclistenAddress := viper.GetString(chaincodeListenAddrKey)
	if cclistenAddress == "" {
		cclistenAddress = fmt.Sprintf("%s:%d", peerHostname, defaultChaincodePort)
		logger.Warningf("%s is not set, using %s", chaincodeListenAddrKey, cclistenAddress)
		viper.Set(chaincodeListenAddrKey, cclistenAddress)
	}

	config, err := peer.GetServerConfig()
	if err != nil {
		logger.Errorf("Error getting server config: %s", err)
		return nil, "", err
	}

	// set the logger for the server
	config.Logger = flogging.MustGetLogger("core/comm").With("server", "ChaincodeServer")

	// Override TLS configuration if TLS is applicable
	if config.SecOpts.UseTLS {
		// Create a self-signed TLS certificate with a SAN that matches the computed chaincode endpoint
		certKeyPair, err := ca.NewServerCertKeyPair(host)
		if err != nil {
			logger.Panicf("Failed generating TLS certificate for chaincode service: +%v", err)
		}
		config.SecOpts = &comm.SecureOptions{
			UseTLS: true,
			// Require chaincode shim to authenticate itself
			RequireClientCert: true,
			// Trust only client certificates signed by ourselves
			ClientRootCAs: [][]byte{ca.CertBytes()},
			// Use our own self-signed TLS certificate and key
			Certificate: certKeyPair.Cert,
			Key:         certKeyPair.Key,
			// No point in specifying server root CAs since this TLS config is only used for
			// a gRPC server and not a client
			ServerRootCAs: nil,
		}
	}

	// Chaincode keepalive options - static for now
	chaincodeKeepaliveOptions := &comm.KeepaliveOptions{
		ServerInterval:    time.Duration(2) * time.Hour,    // 2 hours - gRPC default
		ServerTimeout:     time.Duration(20) * time.Second, // 20 sec - gRPC default
		ServerMinInterval: time.Duration(1) * time.Minute,  // match ClientInterval
	}
	config.KaOpts = chaincodeKeepaliveOptions

	srv, err = comm.NewGRPCServer(cclistenAddress, config)
	if err != nil {
		logger.Errorf("Error creating GRPC server: %s", err)
		return nil, "", err
	}

	return srv, ccEndpoint, nil
}

// computeChaincodeEndpoint will utilize chaincode address, chaincode listen
// address (these two are from viper) and peer address to compute chaincode endpoint.
// There could be following cases of computing chaincode endpoint:
// Case A: if chaincodeAddrKey is set, use it if not "0.0.0.0" (or "::")
// Case B: else if chaincodeListenAddrKey is set and not "0.0.0.0" or ("::"), use it
// Case C: else use peer address if not "0.0.0.0" (or "::")
// Case D: else return error
func computeChaincodeEndpoint(peerHostname string) (ccEndpoint string, err error) {
	logger.Infof("Entering computeChaincodeEndpoint with peerHostname: %s", peerHostname)
	// set this to the host/ip the chaincode will resolve to. It could be
	// the same address as the peer (such as in the sample docker env using
	// the container name as the host name across the board)
	ccEndpoint = viper.GetString(chaincodeAddrKey)
	if ccEndpoint == "" {
		// the chaincodeAddrKey is not set, try to get the address from listener
		// (may finally use the peer address)
		ccEndpoint = viper.GetString(chaincodeListenAddrKey)
		if ccEndpoint == "" {
			// Case C: chaincodeListenAddrKey is not set, use peer address
			peerIp := net.ParseIP(peerHostname)
			if peerIp != nil && peerIp.IsUnspecified() {
				// Case D: all we have is "0.0.0.0" or "::" which chaincode cannot connect to
				logger.Errorf("ChaincodeAddress and chaincodeListenAddress are nil and peerIP is %s", peerIp)
				return "", errors.New("invalid endpoint for chaincode to connect")
			}

			// use peerAddress:defaultChaincodePort
			ccEndpoint = fmt.Sprintf("%s:%d", peerHostname, defaultChaincodePort)

		} else {
			// Case B: chaincodeListenAddrKey is set
			host, port, err := net.SplitHostPort(ccEndpoint)
			if err != nil {
				logger.Errorf("ChaincodeAddress is nil and fail to split chaincodeListenAddress: %s", err)
				return "", err
			}

			ccListenerIp := net.ParseIP(host)
			// ignoring other values such as Multicast address etc ...as the server
			// wouldn't start up with this address anyway
			if ccListenerIp != nil && ccListenerIp.IsUnspecified() {
				// Case C: if "0.0.0.0" or "::", we have to use peer address with the listen port
				peerIp := net.ParseIP(peerHostname)
				if peerIp != nil && peerIp.IsUnspecified() {
					// Case D: all we have is "0.0.0.0" or "::" which chaincode cannot connect to
					logger.Error("ChaincodeAddress is nil while both chaincodeListenAddressIP and peerIP are 0.0.0.0")
					return "", errors.New("invalid endpoint for chaincode to connect")
				}
				ccEndpoint = fmt.Sprintf("%s:%s", peerHostname, port)
			}

		}

	} else {
		// Case A: the chaincodeAddrKey is set
		if host, _, err := net.SplitHostPort(ccEndpoint); err != nil {
			logger.Errorf("Fail to split chaincodeAddress: %s", err)
			return "", err
		} else {
			ccIP := net.ParseIP(host)
			if ccIP != nil && ccIP.IsUnspecified() {
				logger.Errorf("ChaincodeAddress' IP cannot be %s in non-dev mode", ccIP)
				return "", errors.New("invalid endpoint for chaincode to connect")
			}
		}
	}

	logger.Infof("Exit with ccEndpoint: %s", ccEndpoint)
	return ccEndpoint, nil
}

//NOTE - when we implement JOIN we will no longer pass the chainID as param
//The chaincode support will come up without registering system chaincodes
//which will be registered only during join phase.
func registerChaincodeSupport(grpcServer *comm.GRPCServer, ccEndpoint string, ca tlsgen.CA, packageProvider *persistence.PackageProvider, aclProvider aclmgmt.ACLProvider, pr *platforms.Registry) (*chaincode.ChaincodeSupport, ccprovider.ChaincodeProvider, *scc.Provider) {
	//get user mode
	userRunsCC := chaincode.IsDevMode()
	tlsEnabled := viper.GetBool("peer.tls.enabled")

	authenticator := accesscontrol.NewAuthenticator(ca)
	ipRegistry := inproccontroller.NewRegistry()

	sccp := scc.NewProvider(peer.Default, peer.DefaultSupport, ipRegistry)
	lsccInst := lscc.New(sccp, aclProvider, pr)
	lifecycleSCC := &lifecycle.SCC{}

	chaincodeSupport := chaincode.NewChaincodeSupport(
		chaincode.GlobalConfig(),
		ccEndpoint,
		userRunsCC,
		ca.CertBytes(),
		authenticator,
		packageProvider,
		lsccInst,
		aclProvider,
		container.NewVMController(map[string]container.VMProvider{
			dockercontroller.ContainerType: dockercontroller.NewProvider(
				viper.GetString("peer.id"),
				viper.GetString("peer.networkId"),
			),
			inproccontroller.ContainerType: ipRegistry,
		}),
		sccp,
		pr,
		peer.DefaultSupport,
	)
	ipRegistry.ChaincodeSupport = chaincodeSupport
	ccp := chaincode.NewProvider(chaincodeSupport)

	ccSrv := pb.ChaincodeSupportServer(chaincodeSupport)
	if tlsEnabled {
		ccSrv = authenticator.Wrap(ccSrv)
	}

	csccInst := cscc.New(ccp, sccp, aclProvider)
	qsccInst := qscc.New(aclProvider)

	//Now that chaincode is initialized, register all system chaincodes.
	sccs := scc.CreatePluginSysCCs(sccp)
	for _, cc := range append([]scc.SelfDescribingSysCC{lsccInst, csccInst, qsccInst, lifecycleSCC}, sccs...) {
		sccp.RegisterSysCC(cc)
	}
	pb.RegisterChaincodeSupportServer(grpcServer.Server(), ccSrv)

	return chaincodeSupport, ccp, sccp
}

// startChaincodeServer will finish chaincode related initialization, including:
// 1) setup local chaincode install path
// 2) create chaincode specific tls CA
// 3) start the chaincode specific gRPC listening service
func startChaincodeServer(peerHost string, aclProvider aclmgmt.ACLProvider, pr *platforms.Registry) (*chaincode.ChaincodeSupport, ccprovider.ChaincodeProvider, *scc.Provider, *persistence.PackageProvider) {
	// Setup chaincode path
	chaincodeInstallPath := ccprovider.GetChaincodeInstallPathFromViper()
	ccprovider.SetChaincodesPath(chaincodeInstallPath)

	packageProvider := &persistence.PackageProvider{
		LegacyPP: &ccprovider.CCInfoFSImpl{},
		Store: &persistence.Store{
			Path:       chaincodeInstallPath,
			ReadWriter: &persistence.FilesystemIO{},
		},
	}

	// Create a self-signed CA for chaincode service
	ca, err := tlsgen.NewCA()
	if err != nil {
		logger.Panic("Failed creating authentication layer:", err)
	}
	ccSrv, ccEndpoint, err := createChaincodeServer(ca, peerHost)
	if err != nil {
		logger.Panicf("Failed to create chaincode server: %s", err)
	}
	chaincodeSupport, ccp, sccp := registerChaincodeSupport(
		ccSrv,
		ccEndpoint,
		ca,
		packageProvider,
		aclProvider,
		pr,
	)
	go ccSrv.Start()
	return chaincodeSupport, ccp, sccp, packageProvider
}

func adminHasSeparateListener(peerListenAddr string, adminListenAddress string) bool {
	// By default, admin listens on the same port as the peer data service
	if adminListenAddress == "" {
		return false
	}
	_, peerPort, err := net.SplitHostPort(peerListenAddr)
	if err != nil {
		logger.Panicf("Failed parsing peer listen address")
	}

	_, adminPort, err := net.SplitHostPort(adminListenAddress)
	if err != nil {
		logger.Panicf("Failed parsing admin listen address")
	}
	// Admin service has a separate listener in case it doesn't match the peer's
	// configured service
	return adminPort != peerPort
}

func startAdminServer(peerListenAddr string, peerServer *grpc.Server) {
	adminListenAddress := viper.GetString("peer.adminService.listenAddress")
	separateLsnrForAdmin := adminHasSeparateListener(peerListenAddr, adminListenAddress)
	mspID := viper.GetString("peer.localMspId")
	adminPolicy := localPolicy(cauthdsl.SignedByAnyAdmin([]string{mspID}))
	gRPCService := peerServer
	if separateLsnrForAdmin {
		logger.Info("Creating gRPC server for admin service on", adminListenAddress)
		serverConfig, err := peer.GetServerConfig()
		serverConfig.Logger = flogging.MustGetLogger("core/comm").With("server", "AdminServer")
		if err != nil {
			logger.Fatalf("Error loading secure config for admin service (%s)", err)
		}
		adminServer, err := peer.NewPeerServer(adminListenAddress, serverConfig)
		if err != nil {
			logger.Fatalf("Failed to create admin server (%s)", err)
		}
		gRPCService = adminServer.Server()
		defer func() {
			go adminServer.Start()
		}()
	}

	pb.RegisterAdminServer(gRPCService, admin.NewAdminServer(adminPolicy))
}

// secureDialOpts is the callback function for secure dial options for gossip service
func secureDialOpts() []grpc.DialOption {
	var dialOpts []grpc.DialOption
	// set max send/recv msg sizes
	dialOpts = append(
		dialOpts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(comm.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(comm.MaxSendMsgSize)))
	// set the keepalive options
	kaOpts := comm.DefaultKeepaliveOptions
	if viper.IsSet("peer.keepalive.client.interval") {
		kaOpts.ClientInterval = viper.GetDuration("peer.keepalive.client.interval")
	}
	if viper.IsSet("peer.keepalive.client.timeout") {
		kaOpts.ClientTimeout = viper.GetDuration("peer.keepalive.client.timeout")
	}
	dialOpts = append(dialOpts, comm.ClientKeepaliveOptions(kaOpts)...)

	if viper.GetBool("peer.tls.enabled") {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(comm.GetCredentialSupport().GetPeerCredentials()))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	return dialOpts
}

// initGossipService will initialize the gossip service by:
// 1. Enable TLS if configured;
// 2. Init the message crypto service;
// 3. Init the security advisor;
// 4. Init gossip related struct.
func initGossipService(policyMgr policies.ChannelPolicyManagerGetter, peerServer *comm.GRPCServer, serializedIdentity []byte, peerAddr string) error {
	var certs *gossipcommon.TLSCertificates
	if peerServer.TLSEnabled() {
		serverCert := peerServer.ServerCertificate()
		clientCert, err := peer.GetClientCertificate()
		if err != nil {
			return errors.Wrap(err, "failed obtaining client certificates")
		}
		certs = &gossipcommon.TLSCertificates{}
		certs.TLSServerCert.Store(&serverCert)
		certs.TLSClientCert.Store(&clientCert)
	}

	messageCryptoService := peergossip.NewMCS(
		policyMgr,
		localmsp.NewSigner(),
		mgmt.NewDeserializersManager())
	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	bootstrap := viper.GetStringSlice("peer.gossip.bootstrap")

	return service.InitGossipService(serializedIdentity, peerAddr, peerServer.Server(), certs,
		messageCryptoService, secAdv, secureDialOpts, bootstrap...)
}
