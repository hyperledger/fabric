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
	"path/filepath"
	"syscall"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/cauthdsl"
	ccdef "github.com/hyperledger/fabric/common/chaincode"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/deliver"
	"github.com/hyperledger/fabric/common/flogging"
	floggingmetrics "github.com/hyperledger/fabric/common/flogging/metrics"
	"github.com/hyperledger/fabric/common/grpclogging"
	"github.com/hyperledger/fabric/common/grpcmetrics"
	"github.com/hyperledger/fabric/common/metadata"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/aclmgmt"
	"github.com/hyperledger/fabric/core/aclmgmt/resources"
	"github.com/hyperledger/fabric/core/cclifecycle"
	"github.com/hyperledger/fabric/core/chaincode"
	"github.com/hyperledger/fabric/core/chaincode/accesscontrol"
	"github.com/hyperledger/fabric/core/chaincode/lifecycle"
	"github.com/hyperledger/fabric/core/chaincode/persistence"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/ccmetadata"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/committer/txvalidator/plugin"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/privdata"
	coreconfig "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/container"
	"github.com/hyperledger/fabric/core/container/ccintf"
	"github.com/hyperledger/fabric/core/container/dockercontroller"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/dispatcher"
	"github.com/hyperledger/fabric/core/endorser"
	authHandler "github.com/hyperledger/fabric/core/handlers/auth"
	endorsement2 "github.com/hyperledger/fabric/core/handlers/endorsement/api"
	endorsement3 "github.com/hyperledger/fabric/core/handlers/endorsement/api/identities"
	"github.com/hyperledger/fabric/core/handlers/library"
	validation "github.com/hyperledger/fabric/core/handlers/validation/api"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/operations"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/policyprovider"
	"github.com/hyperledger/fabric/core/scc"
	"github.com/hyperledger/fabric/core/scc/cscc"
	"github.com/hyperledger/fabric/core/scc/lscc"
	"github.com/hyperledger/fabric/core/scc/qscc"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/discovery"
	"github.com/hyperledger/fabric/discovery/endorsement"
	discsupport "github.com/hyperledger/fabric/discovery/support"
	discacl "github.com/hyperledger/fabric/discovery/support/acl"
	ccsupport "github.com/hyperledger/fabric/discovery/support/chaincode"
	"github.com/hyperledger/fabric/discovery/support/config"
	"github.com/hyperledger/fabric/discovery/support/gossip"
	gossipcommon "github.com/hyperledger/fabric/gossip/common"
	gossipmetrics "github.com/hyperledger/fabric/gossip/metrics"
	gossipservice "github.com/hyperledger/fabric/gossip/service"
	peergossip "github.com/hyperledger/fabric/internal/peer/gossip"
	"github.com/hyperledger/fabric/internal/peer/version"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	cb "github.com/hyperledger/fabric/protos/common"
	discprotos "github.com/hyperledger/fabric/protos/discovery"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/token"
	pt "github.com/hyperledger/fabric/protos/transientstore"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/hyperledger/fabric/token/server"
	"github.com/hyperledger/fabric/token/tms/manager"
	"github.com/hyperledger/fabric/token/transaction"
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

func startCmd() *cobra.Command {
	// Set the flags on the node start command.
	flags := nodeStartCmd.Flags()
	flags.BoolVarP(&chaincodeDevMode, "peer-chaincodedev", "", false, "start peer in chaincode development mode")
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

	// Trace RPCs with the golang.org/x/net/trace package. This was moved out of
	// the deliver service connection factory as it has process wide implications
	// and was racy with respect to initialization of gRPC clients and servers.
	grpc.EnableTracing = true

	logger.Infof("Starting %s", version.GetInfo())

	//obtain coreConfiguration
	coreConfig, err := peer.GlobalConfig()
	if err != nil {
		return err
	}

	platformRegistry := platforms.NewRegistry(platforms.SupportedPlatforms...)

	identityDeserializerFactory := func(chainID string) msp.IdentityDeserializer {
		return mgmt.GetManagerForChain(chainID)
	}

	opsSystem := newOperationsSystem(coreConfig)
	err = opsSystem.Start()
	if err != nil {
		return errors.WithMessage(err, "failed to initialize operations subsystems")
	}
	defer opsSystem.Stop()

	metricsProvider := opsSystem.Provider
	logObserver := floggingmetrics.NewObserver(metricsProvider)
	flogging.SetObserver(logObserver)

	membershipInfoProvider := privdata.NewMembershipInfoProvider(createSelfSignedData(), identityDeserializerFactory)

	mspID := coreConfig.LocalMSPID

	chaincodeInstallPath := filepath.Join(coreconfig.GetPath("peer.fileSystemPath"), "lifecycle", "chaincodes")
	ccStore := persistence.NewStore(chaincodeInstallPath)
	ccPackageParser := &persistence.ChaincodePackageParser{
		MetadataProvider: &ccmetadata.PersistenceMetadataProvider{},
	}

	peerHost, _, err := net.SplitHostPort(coreConfig.PeerAddress)
	if err != nil {
		return fmt.Errorf("peer address is not in the format of host:port: %v", err)
	}

	listenAddr := coreConfig.ListenAddress
	serverConfig, err := peer.GetServerConfig()
	if err != nil {
		logger.Fatalf("Error loading secure config for peer (%s)", err)
	}

	serverConfig.Logger = flogging.MustGetLogger("core.comm").With("server", "PeerServer")
	serverConfig.MetricsProvider = metricsProvider
	serverConfig.UnaryInterceptors = append(
		serverConfig.UnaryInterceptors,
		grpcmetrics.UnaryServerInterceptor(grpcmetrics.NewUnaryMetrics(metricsProvider)),
		grpclogging.UnaryServerInterceptor(flogging.MustGetLogger("comm.grpc.server").Zap()),
	)
	serverConfig.StreamInterceptors = append(
		serverConfig.StreamInterceptors,
		grpcmetrics.StreamServerInterceptor(grpcmetrics.NewStreamMetrics(metricsProvider)),
		grpclogging.StreamServerInterceptor(flogging.MustGetLogger("comm.grpc.server").Zap()),
	)

	cs := comm.NewCredentialSupport()
	if serverConfig.SecOpts.UseTLS {
		logger.Info("Starting peer with TLS enabled")
		cs = comm.NewCredentialSupport(serverConfig.SecOpts.ServerRootCAs...)

		// set the cert to use if client auth is requested by remote endpoints
		clientCert, err := peer.GetClientCertificate()
		if err != nil {
			logger.Fatalf("Failed to set TLS client certificate (%s)", err)
		}
		cs.SetClientCertificate(clientCert)
	}

	peerServer, err := comm.NewGRPCServer(listenAddr, serverConfig)
	if err != nil {
		logger.Fatalf("Failed to create peer server (%s)", err)
	}

	peerInstance := &peer.Peer{
		Server:            peerServer,
		ServerConfig:      serverConfig,
		CredentialSupport: cs,
		StoreProvider: transientstore.NewStoreProvider(
			filepath.Join(coreconfig.GetPath("peer.fileSystemPath"), "transientstore"),
		),
	}

	signingIdentity := mgmt.GetLocalSigningIdentityOrPanic()
	policyMgr := policies.PolicyManagerGetterFunc(peerInstance.GetPolicyManager)

	// FIXME: Creating the gossip service has the side effect of starting a bunch
	// of go routines and registration with the grpc server.
	gossipService, err := initGossipService(
		policyMgr,
		metricsProvider,
		peerServer,
		signingIdentity,
		cs,
		coreConfig.PeerAddress,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to initialize gossip service")
	}
	defer gossipService.Stop()

	peerInstance.GossipService = gossipService
	peer.Default = peerInstance

	//startup aclmgmt with default ACL providers (resource based and default 1.0 policies based).
	//Users can pass in their own ACLProvider to RegisterACLProvider (currently unit tests do this)
	aclProvider := aclmgmt.NewACLProvider(
		aclmgmt.ResourceGetter(peerInstance.GetStableChannelConfig),
	)

	// TODO, unfortunately, the lifecycle initialization is very unclean at the
	// moment. This is because ccprovider.SetChaincodePath only works after
	// ledgermgmt.Initialize, but ledgermgmt.Initialize requires a reference to
	// lifecycle.  Finally, lscc requires a reference to the system chaincode
	// provider in order to be created, which requires chaincode support to be
	// up, which also requires, you guessed it, lifecycle. Once we remove the
	// v1.0 lifecycle, we should be good to collapse all of the init of lifecycle
	// to this point.
	lifecycleResources := &lifecycle.Resources{
		Serializer:          &lifecycle.Serializer{},
		ChannelConfigSource: peerInstance,
		ChaincodeStore:      ccStore,
		PackageParser:       ccPackageParser,
	}

	lifecycleValidatorCommitter := &lifecycle.ValidatorCommitter{
		Resources:                    lifecycleResources,
		LegacyDeployedCCInfoProvider: &lscc.DeployedCCInfoProvider{},
	}

	packageProvider := &persistence.PackageProvider{
		LegacyPP: &ccprovider.CCInfoFSImpl{},
		Store:    ccStore,
		Parser:   ccPackageParser,
	}

	// legacyMetadataManager collects metadata information from the legacy
	// lifecycle (lscc). This is expected to disappear with FAB-15061.
	legacyMetadataManager, err := cclifecycle.NewMetadataManager(
		cclifecycle.EnumerateFunc(
			func() ([]ccdef.InstalledChaincode, error) {
				return packageProvider.ListInstalledChaincodesLegacy()
			},
		),
	)
	if err != nil {
		logger.Panicf("Failed creating LegacyMetadataManager: +%v", err)
	}

	// metadataManager aggregates metadata information from _lifecycle and
	// the legacy lifecycle (lscc).
	metadataManager := lifecycle.NewMetadataManager()

	// the purpose of these two managers is to feed per-channel chaincode data
	// into gossip owing to the fact that we are transitioning from lscc to
	// _lifecycle, we still have two providers of such information until v2.1,
	// in which we will remove the legacy.
	//
	// the flow of information is the following
	//
	// gossip <-- metadataManager <-- lifecycleCache  (for _lifecycle)
	//                             \
	//                              - legacyMetadataManager (for lscc)
	//
	// FAB-15061 tracks the work necessary to remove LSCC, at which point we
	// will be able to simplify the flow to simply be
	//
	// gossip <-- lifecycleCache

	lifecycleCache := lifecycle.NewCache(lifecycleResources, mspID, metadataManager)

	txProcessors := customtx.Processors{
		common.HeaderType_CONFIG: &peer.ConfigTxProcessor{},
		common.HeaderType_TOKEN_TRANSACTION: &transaction.Processor{
			TMSManager: &manager.Manager{
				IdentityDeserializerManager: &manager.FabricIdentityDeserializerManager{},
			},
		},
	}

	// initialize resource management exit
	ledgermgmt.Initialize(
		&ledgermgmt.Initializer{
			CustomTxProcessors:              txProcessors,
			PlatformRegistry:                platformRegistry,
			DeployedChaincodeInfoProvider:   lifecycleValidatorCommitter,
			MembershipInfoProvider:          membershipInfoProvider,
			ChaincodeLifecycleEventProvider: lifecycleCache,
			MetricsProvider:                 metricsProvider,
			HealthCheckRegistry:             opsSystem,
			StateListeners:                  []ledger.StateListener{lifecycleCache},
			Config:                          ledgerConfig(),
		},
	)

	// Configure CC package storage
	lsccInstallPath := filepath.Join(coreconfig.GetPath("peer.fileSystemPath"), "chaincodes")
	ccprovider.SetChaincodesPath(lsccInstallPath)

	if err := lifecycleCache.InitializeLocalChaincodes(); err != nil {
		return errors.WithMessage(err, "could not initialize local chaincodes")
	}

	// Parameter overrides must be processed before any parameters are
	// cached. Failures to cache cause the server to terminate immediately.
	if chaincodeDevMode {
		logger.Info("Running in chaincode development mode")
		logger.Info("Disable loading validity system chaincode")

		viper.Set("chaincode.mode", chaincode.DevModeUserRunsChaincode)
	}

	mutualTLS := serverConfig.SecOpts.UseTLS && serverConfig.SecOpts.RequireClientCert
	policyCheckerProvider := func(resourceName string) deliver.PolicyCheckerFunc {
		return func(env *cb.Envelope, channelID string) error {
			return aclProvider.CheckACL(resourceName, channelID, env)
		}
	}

	metrics := deliver.NewMetrics(metricsProvider)
	abServer := &peer.DeliverServer{
		DeliverHandler:        deliver.NewHandler(&peer.DeliverChainManager{}, coreConfig.AuthenticationTimeWindow, mutualTLS, metrics),
		PolicyCheckerProvider: policyCheckerProvider,
	}
	pb.RegisterDeliverServer(peerServer.Server(), abServer)

	// Create a self-signed CA for chaincode service
	ca, err := tlsgen.NewCA()
	if err != nil {
		logger.Panic("Failed creating authentication layer:", err)
	}
	ccSrv, ccEndpoint, err := createChaincodeServer(coreConfig, ca, peerHost)
	if err != nil {
		logger.Panicf("Failed to create chaincode server: %s", err)
	}

	//get user mode
	userRunsCC := chaincode.IsDevMode()
	tlsEnabled := coreConfig.PeerTLSEnabled

	// create chaincode specific tls CA
	authenticator := accesscontrol.NewAuthenticator(ca)
	ipRegistry := inproccontroller.NewRegistry()

	sccp := &scc.Provider{
		Peer:      peerInstance,
		Registrar: ipRegistry,
		Whitelist: scc.GlobalWhitelist(),
	}
	lsccInst := lscc.New(sccp, aclProvider, platformRegistry, peerInstance.GetMSPIDs)

	chaincodeHandlerRegistry := chaincode.NewHandlerRegistry(userRunsCC)
	lifecycleTxQueryExecutorGetter := &chaincode.TxQueryExecutorGetter{
		PackageID:       ccintf.CCID(lifecycle.LifecycleNamespace + ":" + util.GetSysCCVersion()),
		HandlerRegistry: chaincodeHandlerRegistry,
	}
	chaincodeEndorsementInfo := &lifecycle.ChaincodeEndorsementInfo{
		LegacyImpl: lsccInst,
		Resources:  lifecycleResources,
		Cache:      lifecycleCache,
	}

	lifecycleFunctions := &lifecycle.ExternalFunctions{
		Resources:       lifecycleResources,
		InstallListener: lifecycleCache,
	}

	lifecycleSCC := &lifecycle.SCC{
		Dispatcher: &dispatcher.Dispatcher{
			Protobuf: &dispatcher.ProtobufImpl{},
		},
		DeployedCCInfoProvider: lifecycleValidatorCommitter,
		QueryExecutorProvider:  lifecycleTxQueryExecutorGetter,
		Functions:              lifecycleFunctions,
		OrgMSPID:               mspID,
		ChannelConfigSource:    peerInstance,
		ACLProvider:            aclProvider,
	}

	var client *docker.Client
	if coreConfig.VMDockerTLSEnabled {
		client, err = docker.NewTLSClient(coreConfig.VMEndpoint, coreConfig.DockerCert, coreConfig.DockerKey, coreConfig.DockerCA)
	} else {
		client, err = docker.NewClient(coreConfig.VMEndpoint)
	}
	if err != nil {
		logger.Panicf("cannot create docker client: %s", err)
	}

	dockerProvider := &dockercontroller.Provider{
		PeerID:        coreConfig.PeerID,
		NetworkID:     coreConfig.NetworkID,
		BuildMetrics:  dockercontroller.NewBuildMetrics(opsSystem.Provider),
		Client:        client,
		AttachStdOut:  coreConfig.VMDockerAttachStdout,
		HostConfig:    getDockerHostConfig(),
		ChaincodePull: coreConfig.ChaincodePull,
		NetworkMode:   coreConfig.VMNetworkMode,
	}
	dockerVM := dockerProvider.NewVM()

	err = opsSystem.RegisterChecker("docker", dockerVM)
	if err != nil {
		logger.Panicf("failed to register docker health check: %s", err)
	}

	chaincodeConfig := chaincode.GlobalConfig()

	chaincodeVMController := container.NewVMController(
		map[string]container.VMProvider{
			dockercontroller.ContainerType: dockerProvider,
			inproccontroller.ContainerType: ipRegistry,
		},
	)

	containerRuntime := &chaincode.ContainerRuntime{
		CACert:        ca.CertBytes(),
		CertGenerator: authenticator,
		CommonEnv: []string{
			"CORE_CHAINCODE_LOGGING_LEVEL=" + chaincodeConfig.LogLevel,
			"CORE_CHAINCODE_LOGGING_SHIM=" + chaincodeConfig.ShimLogLevel,
			"CORE_CHAINCODE_LOGGING_FORMAT=" + chaincodeConfig.LogFormat,
		},
		DockerClient:     client,
		PeerAddress:      ccEndpoint,
		Processor:        chaincodeVMController,
		PlatformRegistry: platformRegistry,
	}

	// Keep TestQueries working
	if !chaincodeConfig.TLSEnabled {
		containerRuntime.CertGenerator = nil
	}

	chaincodeLauncher := &chaincode.RuntimeLauncher{
		Metrics:         chaincode.NewLaunchMetrics(opsSystem.Provider),
		PackageProvider: packageProvider,
		Registry:        chaincodeHandlerRegistry,
		Runtime:         containerRuntime,
		StartupTimeout:  chaincodeConfig.StartupTimeout,
	}

	chaincodeSupport := &chaincode.ChaincodeSupport{
		ACLProvider:            aclProvider,
		AppConfig:              peerInstance,
		DeployedCCInfoProvider: lifecycleValidatorCommitter,
		ExecuteTimeout:         chaincodeConfig.ExecuteTimeout,
		HandlerRegistry:        chaincodeHandlerRegistry,
		HandlerMetrics:         chaincode.NewHandlerMetrics(opsSystem.Provider),
		Keepalive:              chaincodeConfig.Keepalive,
		Launcher:               chaincodeLauncher,
		Lifecycle:              chaincodeEndorsementInfo,
		Runtime:                containerRuntime,
		SystemCCProvider:       sccp,
		TotalQueryLimit:        chaincodeConfig.TotalQueryLimit,
		UserRunsCC:             userRunsCC,
	}

	ipRegistry.ChaincodeSupport = chaincodeSupport

	ccSupSrv := pb.ChaincodeSupportServer(chaincodeSupport)
	if tlsEnabled {
		ccSupSrv = authenticator.Wrap(ccSupSrv)
	}

	csccInst := cscc.New(
		sccp, aclProvider,
		lifecycleValidatorCommitter,
		lsccInst,
		lifecycleValidatorCommitter,
		policyprovider.GetPolicyChecker(),
	)
	qsccInst := scc.SelfDescribingSysCC(qscc.New(aclProvider, peerInstance))
	if maxConcurrency := coreConfig.LimitsConcurrencyQSCC; maxConcurrency != 0 {
		qsccInst = scc.Throttle(maxConcurrency, qsccInst)
	}

	//Now that chaincode is initialized, register all system chaincodes.
	sccs := scc.CreatePluginSysCCs(sccp)
	for _, cc := range append([]scc.SelfDescribingSysCC{lsccInst, csccInst, qsccInst, lifecycleSCC}, sccs...) {
		sccp.RegisterSysCC(cc)
	}
	pb.RegisterChaincodeSupportServer(ccSrv.Server(), ccSupSrv)

	// start the chaincode specific gRPC listening service
	go ccSrv.Start()

	logger.Debugf("Running peer")

	privDataDist := func(channel string, txID string, privateData *pt.TxPvtReadWriteSetWithConfigInfo, blkHt uint64) error {
		return gossipService.DistributePrivateData(channel, txID, privateData, blkHt)
	}

	libConf := library.Config{}
	if err = viperutil.EnhancedExactUnmarshalKey("peer.handlers", &libConf); err != nil {
		return errors.WithMessage(err, "could not load YAML config")
	}
	reg := library.InitRegistry(libConf)

	authFilters := reg.Lookup(library.Auth).([]authHandler.Filter)
	endorserSupport := &endorser.SupportImpl{
		SignerSerializer: signingIdentity,
		Peer:             peerInstance,
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
		TransientStoreRetriever: peerInstance,
		PluginMapper:            pluginMapper,
		SigningIdentityFetcher:  signingIdentityFetcher,
	})
	endorserSupport.PluginEndorser = pluginEndorser
	serverEndorser := endorser.NewEndorserServer(privDataDist, endorserSupport, platformRegistry, metricsProvider)
	auth := authHandler.ChainFilters(serverEndorser, authFilters...)
	// Register the Endorser server
	pb.RegisterEndorserServer(peerServer.Server(), auth)

	// register prover grpc service
	err = registerProverService(peerInstance, peerServer, aclProvider, signingIdentity)
	if err != nil {
		return err
	}

	// deploy system chaincodes
	ccp := chaincode.NewProvider(chaincodeSupport)
	sccp.DeploySysCCs("", ccp)
	logger.Infof("Deployed system chaincodes")

	// register the lifecycleMetadataManager to get updates from the legacy
	// chaincode; lifecycleMetadataManager will aggregate these updates with
	// the ones from the new lifecycle and deliver both
	// this is expected to disappear with FAB-15061
	legacyMetadataManager.AddListener(metadataManager)

	// register gossip as a listener for updates from lifecycleMetadataManager
	metadataManager.AddListener(lifecycle.HandleMetadataUpdateFunc(func(channel string, chaincodes ccdef.MetadataSet) {
		gossipService.UpdateChaincodes(chaincodes.AsChaincodes(), gossipcommon.ChannelID(channel))
	}))

	ledgermgmt.Initialize(&ledgermgmt.Initializer{
		CustomTxProcessors:            txProcessors,
		PlatformRegistry:              platformRegistry,
		DeployedChaincodeInfoProvider: lifecycleValidatorCommitter,
		MembershipInfoProvider:        membershipInfoProvider,
		MetricsProvider:               metricsProvider,
		Config:                        ledgerConfig(),
	})

	// this brings up all the channels
	peerInstance.Initialize(
		func(cid string) {
			logger.Debugf("Deploying system CC, for channel <%s>", cid)
			sccp.DeploySysCCs(cid, ccp)

			// initialize the metadata for this channel.
			// This call will pre-populate chaincode information for this
			// channel but it won't fire any updates to its listeners
			lifecycleCache.InitializeMetadata(cid)

			// initialize the legacyMetadataManager for this channel.
			// This call will pre-populate chaincode information from
			// the legacy lifecycle for this channel; it will also fire
			// the listener, which will cascade to metadataManager
			// and eventually to gossip to pre-populate data structures.
			// this is expected to disappear with FAB-15061
			sub, err := legacyMetadataManager.NewChannelSubscription(cid, cclifecycle.QueryCreatorFunc(func() (cclifecycle.Query, error) {
				return peerInstance.GetLedger(cid).NewQueryExecutor()
			}))
			if err != nil {
				logger.Panicf("Failed subscribing to chaincode lifecycle updates")
			}

			// register this channel's legacyMetadataManager (sub) to get ledger updates
			// this is expected to disappear with FAB-15061
			cceventmgmt.GetMgr().Register(cid, sub)
		},
		sccp,
		plugin.MapBasedMapper(validationPluginsByName),
		lifecycleValidatorCommitter,
		membershipInfoProvider,
		lsccInst,
		lifecycleValidatorCommitter,
		coreConfig.ValidatorPoolSize,
	)

	if coreConfig.DiscoveryEnabled {
		registerDiscoveryService(
			coreConfig,
			peerInstance,
			peerServer,
			policyMgr,
			lifecycle.NewMetadataProvider(
				lifecycleCache,
				legacyMetadataManager,
				peerInstance,
			),
			gossipService,
		)
	}

	logger.Infof("Starting peer with ID=[%s], network ID=[%s], address=[%s]", coreConfig.PeerID, coreConfig.NetworkID, coreConfig.PeerAddress)

	// Get configuration before starting go routines to avoid
	// racing in tests
	profileEnabled := coreConfig.ProfileEnabled
	profileListenAddress := coreConfig.ProfileListenAddress

	// Start the grpc server. Done in a goroutine so we can deploy the
	// genesis block if needed.
	serve := make(chan error)

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
	if profileEnabled {
		go func() {
			logger.Infof("Starting profiling server with listenAddress = %s", profileListenAddress)
			if profileErr := http.ListenAndServe(profileListenAddress, nil); profileErr != nil {
				logger.Errorf("Error starting profiler: %s", profileErr)
			}
		}()
	}

	go handleSignals(addPlatformSignals(map[os.Signal]func(){
		syscall.SIGINT:  func() { serve <- nil },
		syscall.SIGTERM: func() { serve <- nil },
	}))

	logger.Infof("Started peer with ID=[%s], network ID=[%s], address=[%s]", coreConfig.PeerID, coreConfig.NetworkID, coreConfig.PeerAddress)

	// Block until grpc server exits
	return <-serve
}

func handleSignals(handlers map[os.Signal]func()) {
	var signals []os.Signal
	for sig := range handlers {
		signals = append(signals, sig)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)

	for sig := range signalChan {
		logger.Infof("Received signal: %d (%s)", sig, sig)
		handlers[sig]()
	}
}

func localPolicy(policyObject proto.Message) policies.Policy {
	localMSP := mgmt.GetLocalMSP()
	pp := cauthdsl.NewPolicyProvider(localMSP)
	policy, _, err := pp.NewPolicy(protoutil.MarshalOrPanic(policyObject))
	if err != nil {
		logger.Panicf("Failed creating local policy: +%v", err)
	}
	return policy
}

func createSelfSignedData() protoutil.SignedData {
	sID := mgmt.GetLocalSigningIdentityOrPanic()
	msg := make([]byte, 32)
	sig, err := sID.Sign(msg)
	if err != nil {
		logger.Panicf("Failed creating self signed data because message signing failed: %v", err)
	}
	peerIdentity, err := sID.Serialize()
	if err != nil {
		logger.Panicf("Failed creating self signed data because peer identity couldn't be serialized: %v", err)
	}
	return protoutil.SignedData{
		Data:      msg,
		Signature: sig,
		Identity:  peerIdentity,
	}
}

func registerDiscoveryService(
	coreConfig *peer.Config,
	peerInstance *peer.Peer,
	peerServer *comm.GRPCServer,
	polMgr policies.ChannelPolicyManagerGetter,
	metadataProvider *lifecycle.MetadataProvider,
	gossipService *gossipservice.GossipService,
) {
	mspID := coreConfig.LocalMSPID
	localAccessPolicy := localPolicy(cauthdsl.SignedByAnyAdmin([]string{mspID}))
	if coreConfig.DiscoveryOrgMembersAllowed {
		localAccessPolicy = localPolicy(cauthdsl.SignedByAnyMember([]string{mspID}))
	}
	channelVerifier := discacl.NewChannelVerifier(policies.ChannelApplicationWriters, polMgr)
	acl := discacl.NewDiscoverySupport(channelVerifier, localAccessPolicy, discacl.ChannelConfigGetterFunc(peerInstance.GetStableChannelConfig))
	gSup := gossip.NewDiscoverySupport(gossipService)
	ccSup := ccsupport.NewDiscoverySupport(metadataProvider)
	ea := endorsement.NewEndorsementAnalyzer(gSup, ccSup, acl, metadataProvider)
	confSup := config.NewDiscoverySupport(config.CurrentConfigBlockGetterFunc(peerInstance.GetCurrConfigBlock))
	support := discsupport.NewDiscoverySupport(acl, gSup, ea, confSup, acl)
	svc := discovery.NewService(discovery.Config{
		TLS:                          peerServer.TLSEnabled(),
		AuthCacheEnabled:             coreConfig.DiscoveryAuthCacheEnabled,
		AuthCacheMaxSize:             coreConfig.DiscoveryAuthCacheMaxSize,
		AuthCachePurgeRetentionRatio: coreConfig.DiscoveryAuthCachePurgeRetentionRatio,
	}, support)
	logger.Info("Discovery service activated")
	discprotos.RegisterDiscoveryServer(peerServer.Server(), svc)
}

//create a CC listener using peer.chaincodeListenAddress (and if that's not set use peer.peerAddress)
func createChaincodeServer(coreConfig *peer.Config, ca tlsgen.CA, peerHostname string) (srv *comm.GRPCServer, ccEndpoint string, err error) {
	// before potentially setting chaincodeListenAddress, compute chaincode endpoint at first
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig.ChaincodeAddress, coreConfig.ChaincodeListenAddress, peerHostname)
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

	cclistenAddress := coreConfig.ChaincodeListenAddress
	if cclistenAddress == "" {
		cclistenAddress = fmt.Sprintf("%s:%d", peerHostname, defaultChaincodePort)
		logger.Warningf("%s is not set, using %s", chaincodeListenAddrKey, cclistenAddress)
		coreConfig.ChaincodeListenAddress = cclistenAddress
	}

	config, err := peer.GetServerConfig()
	if err != nil {
		logger.Errorf("Error getting server config: %s", err)
		return nil, "", err
	}

	// set the logger for the server
	config.Logger = flogging.MustGetLogger("core.comm").With("server", "ChaincodeServer")

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
	config.HealthCheckEnabled = true

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
// Case B: else if chaincodeListenAddressKey is set and not "0.0.0.0" or ("::"), use it
// Case C: else use peer address if not "0.0.0.0" (or "::")
// Case D: else return error
func computeChaincodeEndpoint(chaincodeAddress string, chaincodeListenAddress string, peerHostname string) (ccEndpoint string, err error) {
	logger.Infof("Entering computeChaincodeEndpoint with peerHostname: %s", peerHostname)
	// Case A: the chaincodeAddrKey is set
	if chaincodeAddress != "" {
		host, _, err := net.SplitHostPort(chaincodeAddress)
		if err != nil {
			logger.Errorf("Fail to split chaincodeAddress: %s", err)
			return "", err
		}
		ccIP := net.ParseIP(host)
		if ccIP != nil && ccIP.IsUnspecified() {
			logger.Errorf("ChaincodeAddress' IP cannot be %s in non-dev mode", ccIP)
			return "", errors.New("invalid endpoint for chaincode to connect")
		}
		logger.Infof("Exit with ccEndpoint: %s", chaincodeAddress)
		return chaincodeAddress, nil
	}

	// Case B: chaincodeListenAddrKey is set
	if chaincodeListenAddress != "" {
		ccEndpoint = chaincodeListenAddress
		host, port, err := net.SplitHostPort(ccEndpoint)
		if err != nil {
			logger.Errorf("ChaincodeAddress is nil and fail to split chaincodeListenAddress: %s", err)
			return "", err
		}

		ccListenerIP := net.ParseIP(host)
		// ignoring other values such as Multicast address etc ...as the server
		// wouldn't start up with this address anyway
		if ccListenerIP != nil && ccListenerIP.IsUnspecified() {
			// Case C: if "0.0.0.0" or "::", we have to use peer address with the listen port
			peerIP := net.ParseIP(peerHostname)
			if peerIP != nil && peerIP.IsUnspecified() {
				// Case D: all we have is "0.0.0.0" or "::" which chaincode cannot connect to
				logger.Error("ChaincodeAddress is nil while both chaincodeListenAddressIP and peerIP are 0.0.0.0")
				return "", errors.New("invalid endpoint for chaincode to connect")
			}
			ccEndpoint = fmt.Sprintf("%s:%s", peerHostname, port)
		}
		logger.Infof("Exit with ccEndpoint: %s", ccEndpoint)
		return ccEndpoint, nil
	}

	// Case C: chaincodeListenAddrKey is not set, use peer address
	peerIP := net.ParseIP(peerHostname)
	if peerIP != nil && peerIP.IsUnspecified() {
		// Case D: all we have is "0.0.0.0" or "::" which chaincode cannot connect to
		logger.Errorf("ChaincodeAddress and chaincodeListenAddress are nil and peerIP is %s", peerIP)
		return "", errors.New("invalid endpoint for chaincode to connect")
	}

	// use peerAddress:defaultChaincodePort
	ccEndpoint = fmt.Sprintf("%s:%d", peerHostname, defaultChaincodePort)

	logger.Infof("Exit with ccEndpoint: %s", ccEndpoint)
	return ccEndpoint, nil
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

// secureDialOpts is the callback function for secure dial options for gossip service
func secureDialOpts(credSupport *comm.CredentialSupport) func() []grpc.DialOption {
	return func() []grpc.DialOption {
		var dialOpts []grpc.DialOption
		// set max send/recv msg sizes
		dialOpts = append(
			dialOpts,
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(comm.MaxRecvMsgSize), grpc.MaxCallSendMsgSize(comm.MaxSendMsgSize)),
		)
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
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(credSupport.GetPeerCredentials()))
		} else {
			dialOpts = append(dialOpts, grpc.WithInsecure())
		}
		return dialOpts
	}
}

// initGossipService will initialize the gossip service by:
// 1. Enable TLS if configured;
// 2. Init the message crypto service;
// 3. Init the security advisor;
// 4. Init gossip related struct.
func initGossipService(
	policyMgr policies.ChannelPolicyManagerGetter,
	metricsProvider metrics.Provider,
	peerServer *comm.GRPCServer,
	signer msp.SigningIdentity,
	credSupport *comm.CredentialSupport,
	peerAddr string,
) (*gossipservice.GossipService, error) {

	var certs *gossipcommon.TLSCertificates
	if peerServer.TLSEnabled() {
		serverCert := peerServer.ServerCertificate()
		clientCert, err := peer.GetClientCertificate()
		if err != nil {
			return nil, errors.Wrap(err, "failed obtaining client certificates")
		}
		certs = &gossipcommon.TLSCertificates{}
		certs.TLSServerCert.Store(&serverCert)
		certs.TLSClientCert.Store(&clientCert)
	}

	messageCryptoService := peergossip.NewMCS(
		policyMgr,
		signer,
		mgmt.NewDeserializersManager(),
	)
	secAdv := peergossip.NewSecurityAdvisor(mgmt.NewDeserializersManager())
	bootstrap := viper.GetStringSlice("peer.gossip.bootstrap")

	return gossipservice.New(
		signer,
		gossipmetrics.NewGossipMetrics(metricsProvider),
		peerAddr,
		peerServer.Server(),
		certs,
		messageCryptoService,
		secAdv,
		secureDialOpts(credSupport),
		credSupport,
		bootstrap...,
	)
}

func newOperationsSystem(coreConfig *peer.Config) *operations.System {
	return operations.NewSystem(operations.Options{
		Logger:        flogging.MustGetLogger("peer.operations"),
		ListenAddress: coreConfig.OperationsListenAddress,
		Metrics: operations.MetricsOptions{
			Provider: coreConfig.MetricsProvider,
			Statsd: &operations.Statsd{
				Network:       coreConfig.StatsdNetwork,
				Address:       coreConfig.StatsdAaddress,
				WriteInterval: coreConfig.StatsdWriteInterval,
				Prefix:        coreConfig.StatsdPrefix,
			},
		},
		TLS: operations.TLS{
			Enabled:            coreConfig.OperationsTLSEnabled,
			CertFile:           coreConfig.OperationsTLSCertFile,
			KeyFile:            coreConfig.OperationsTLSKeyFile,
			ClientCertRequired: coreConfig.OperationsTLSClientAuthRequired,
			ClientCACertFiles:  coreConfig.OperationsTLSClientRootCAs,
		},
		Version: metadata.Version,
	})
}

func registerProverService(peerInstance *peer.Peer, peerServer *comm.GRPCServer, aclProvider aclmgmt.ACLProvider, signingIdentity msp.SigningIdentity) error {
	policyChecker := &server.PolicyBasedAccessControl{
		ACLProvider: aclProvider,
		ACLResources: &server.ACLResources{
			IssueTokens:    resources.Token_Issue,
			TransferTokens: resources.Token_Transfer,
			ListTokens:     resources.Token_List,
		},
	}

	responseMarshaler, err := server.NewResponseMarshaler(signingIdentity)
	if err != nil {
		logger.Errorf("Failed to create prover service: %s", err)
		return err
	}

	prover := &server.Prover{
		CapabilityChecker: &server.TokenCapabilityChecker{
			ChannelConfigGetter: peerInstance,
		},
		Marshaler:     responseMarshaler,
		PolicyChecker: policyChecker,
		TMSManager: &server.Manager{
			LedgerManager: &server.PeerLedgerManager{},
			TokenOwnerValidatorManager: &server.PeerTokenOwnerValidatorManager{
				IdentityDeserializerManager: &manager.FabricIdentityDeserializerManager{},
			},
		},
	}
	token.RegisterProverServer(peerServer.Server(), prover)
	return nil
}

func getDockerHostConfig() *docker.HostConfig {
	dockerKey := func(key string) string { return "vm.docker.hostConfig." + key }
	getInt64 := func(key string) int64 { return int64(viper.GetInt(dockerKey(key))) }

	var logConfig docker.LogConfig
	err := viper.UnmarshalKey(dockerKey("LogConfig"), &logConfig)
	if err != nil {
		logger.Panicf("unable to parse Docker LogConfig: %s", err)
	}

	networkMode := viper.GetString(dockerKey("NetworkMode"))
	if networkMode == "" {
		networkMode = "host"
	}

	memorySwappiness := getInt64("MemorySwappiness")
	oomKillDisable := viper.GetBool(dockerKey("OomKillDisable"))

	return &docker.HostConfig{
		CapAdd:  viper.GetStringSlice(dockerKey("CapAdd")),
		CapDrop: viper.GetStringSlice(dockerKey("CapDrop")),

		DNS:         viper.GetStringSlice(dockerKey("Dns")),
		DNSSearch:   viper.GetStringSlice(dockerKey("DnsSearch")),
		ExtraHosts:  viper.GetStringSlice(dockerKey("ExtraHosts")),
		NetworkMode: networkMode,
		IpcMode:     viper.GetString(dockerKey("IpcMode")),
		PidMode:     viper.GetString(dockerKey("PidMode")),
		UTSMode:     viper.GetString(dockerKey("UTSMode")),
		LogConfig:   logConfig,

		ReadonlyRootfs:   viper.GetBool(dockerKey("ReadonlyRootfs")),
		SecurityOpt:      viper.GetStringSlice(dockerKey("SecurityOpt")),
		CgroupParent:     viper.GetString(dockerKey("CgroupParent")),
		Memory:           getInt64("Memory"),
		MemorySwap:       getInt64("MemorySwap"),
		MemorySwappiness: &memorySwappiness,
		OOMKillDisable:   &oomKillDisable,
		CPUShares:        getInt64("CpuShares"),
		CPUSet:           viper.GetString(dockerKey("Cpuset")),
		CPUSetCPUs:       viper.GetString(dockerKey("CpusetCPUs")),
		CPUSetMEMs:       viper.GetString(dockerKey("CpusetMEMs")),
		CPUQuota:         getInt64("CpuQuota"),
		CPUPeriod:        getInt64("CpuPeriod"),
		BlkioWeight:      getInt64("BlkioWeight"),
	}
}
