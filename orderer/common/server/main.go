/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	_ "net/http/pprof" // This is essentially the main package for the orderer
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-lib-go/healthz"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/flogging"
	floggingmetrics "github.com/hyperledger/fabric/common/flogging/metrics"
	"github.com/hyperledger/fabric/common/grpclogging"
	"github.com/hyperledger/fabric/common/grpcmetrics"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/common/tools/protolator"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/operations"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/orderer/common/bootstrap/file"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/metadata"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/kafka"
	"github.com/hyperledger/fabric/orderer/consensus/solo"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/hyperledger/fabric/protos/utils"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"gopkg.in/alecthomas/kingpin.v2"
)

var logger = flogging.MustGetLogger("orderer.common.server")

// command line flags
var (
	app = kingpin.New("orderer", "Hyperledger Fabric orderer node")

	start     = app.Command("start", "Start the orderer node").Default()
	version   = app.Command("version", "Show version information")
	benchmark = app.Command("benchmark", "Run orderer in benchmark mode")

	clusterTypes = map[string]struct{}{"etcdraft": {}}
)

// Main is the entry point of orderer process
func Main() {
	fullCmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	// "version" command
	if fullCmd == version.FullCommand() {
		fmt.Println(metadata.GetVersionInfo())
		return
	}

	conf, err := localconfig.Load()
	if err != nil {
		logger.Error("failed to parse config: ", err)
		os.Exit(1)
	}
	initializeLogging()
	initializeLocalMsp(conf)

	prettyPrintStruct(conf)
	Start(fullCmd, conf)
}

// Start provides a layer of abstraction for benchmark test
func Start(cmd string, conf *localconfig.TopLevel) {
	bootstrapBlock := extractBootstrapBlock(conf)
	if err := ValidateBootstrapBlock(bootstrapBlock); err != nil {
		logger.Panicf("Failed validating bootstrap block: %v", err)
	}

	clusterType := isClusterType(bootstrapBlock)
	signer := localmsp.NewSigner()

	lf, _ := createLedgerFactory(conf)

	clusterDialer := &cluster.PredicateDialer{}
	clusterClientConfig := initializeClusterClientConfig(conf, clusterType, bootstrapBlock)
	clusterDialer.SetConfig(clusterClientConfig)

	r := createReplicator(lf, bootstrapBlock, conf, clusterClientConfig.SecOpts, signer)
	// Only clusters that are equipped with a recent config block can replicate.
	if clusterType && conf.General.GenesisMethod == "file" {
		r.replicateIfNeeded(bootstrapBlock)
	}

	opsSystem := newOperationsSystem(conf.Operations, conf.Metrics)
	err := opsSystem.Start()
	if err != nil {
		logger.Panicf("failed to initialize operations subsystem: %s", err)
	}
	defer opsSystem.Stop()
	metricsProvider := opsSystem.Provider
	logObserver := floggingmetrics.NewObserver(metricsProvider)
	flogging.Global.SetObserver(logObserver)

	serverConfig := initializeServerConfig(conf, metricsProvider)
	grpcServer := initializeGrpcServer(conf, serverConfig)
	caSupport := &comm.CASupport{
		AppRootCAsByChain:     make(map[string][][]byte),
		OrdererRootCAsByChain: make(map[string][][]byte),
		ClientRootCAs:         serverConfig.SecOpts.ClientRootCAs,
	}

	clusterServerConfig := serverConfig
	clusterGRPCServer := grpcServer
	if clusterType {
		clusterServerConfig, clusterGRPCServer = configureClusterListener(conf, serverConfig, grpcServer, ioutil.ReadFile)
	}

	var servers = []*comm.GRPCServer{grpcServer}
	// If we have a separate gRPC server for the cluster, we need to update its TLS
	// CA certificate pool too.
	if clusterGRPCServer != grpcServer {
		servers = append(servers, clusterGRPCServer)
	}

	tlsCallback := func(bundle *channelconfig.Bundle) {
		// only need to do this if mutual TLS is required or if the orderer node is part of a cluster
		if grpcServer.MutualTLSRequired() || clusterType {
			logger.Debug("Executing callback to update root CAs")
			updateTrustedRoots(caSupport, bundle, servers...)
			if clusterType {
				updateClusterDialer(caSupport, clusterDialer, clusterClientConfig.SecOpts.ServerRootCAs)
			}
		}
	}

	manager := initializeMultichannelRegistrar(bootstrapBlock, r, clusterDialer, clusterServerConfig, clusterGRPCServer, conf, signer, metricsProvider, opsSystem, lf, tlsCallback)
	mutualTLS := serverConfig.SecOpts.UseTLS && serverConfig.SecOpts.RequireClientCert
	server := NewServer(manager, metricsProvider, &conf.Debug, conf.General.Authentication.TimeWindow, mutualTLS)

	logger.Infof("Starting %s", metadata.GetVersionInfo())
	go handleSignals(addPlatformSignals(map[os.Signal]func(){
		syscall.SIGTERM: func() {
			grpcServer.Stop()
			if clusterGRPCServer != grpcServer {
				clusterGRPCServer.Stop()
			}
		},
	}))

	if clusterGRPCServer != grpcServer {
		logger.Info("Starting cluster listener on", clusterGRPCServer.Address())
		go clusterGRPCServer.Start()
	}

	initializeProfilingService(conf)
	ab.RegisterAtomicBroadcastServer(grpcServer.Server(), server)
	logger.Info("Beginning to serve requests")
	grpcServer.Start()
}

func createReplicator(
	lf blockledger.Factory,
	bootstrapBlock *cb.Block,
	conf *localconfig.TopLevel,
	secOpts *comm.SecureOptions,
	signer crypto.LocalSigner,
) *replicationInitiator {
	logger := flogging.MustGetLogger("orderer.common.cluster")

	vl := &verifierLoader{
		verifierFactory: &cluster.BlockVerifierAssembler{Logger: logger},
		onFailure: func(block *cb.Block) {
			protolator.DeepMarshalJSON(os.Stdout, block)
		},
		ledgerFactory: lf,
		logger:        logger,
	}

	systemChannelName, err := utils.GetChainIDFromBlock(bootstrapBlock)
	if err != nil {
		logger.Panicf("Failed extracting system channel name from bootstrap block: %v", err)
	}

	// System channel is not verified because we trust the bootstrap block
	// and use backward hash chain verification.
	verifiersByChannel := vl.loadVerifiers()
	verifiersByChannel[systemChannelName] = &cluster.NoopBlockVerifier{}

	vr := &cluster.VerificationRegistry{
		LoadVerifier:       vl.loadVerifier,
		Logger:             logger,
		VerifiersByChannel: verifiersByChannel,
		VerifierFactory:    &cluster.BlockVerifierAssembler{Logger: logger},
	}

	ledgerFactory := &ledgerFactory{
		Factory:       lf,
		onBlockCommit: vr.BlockCommitted,
	}
	return &replicationInitiator{
		registerChain:     vr.RegisterVerifier,
		verifierRetriever: vr,
		logger:            logger,
		secOpts:           secOpts,
		conf:              conf,
		lf:                ledgerFactory,
		signer:            signer,
	}
}

func initializeLogging() {
	loggingSpec := os.Getenv("FABRIC_LOGGING_SPEC")
	loggingFormat := os.Getenv("FABRIC_LOGGING_FORMAT")
	flogging.Init(flogging.Config{
		Format:  loggingFormat,
		Writer:  os.Stderr,
		LogSpec: loggingSpec,
	})
}

// Start the profiling service if enabled.
func initializeProfilingService(conf *localconfig.TopLevel) {
	if conf.General.Profile.Enabled {
		go func() {
			logger.Info("Starting Go pprof profiling service on:", conf.General.Profile.Address)
			// The ListenAndServe() call does not return unless an error occurs.
			logger.Panic("Go pprof service failed:", http.ListenAndServe(conf.General.Profile.Address, nil))
		}()
	}
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

type loadPEMFunc func(string) ([]byte, error)

// configureClusterListener gets a ServerConfig and a GRPCServer, and:
// 1) If the TopLevel configuration states that the cluster configuration for the cluster gRPC service is missing, returns them back.
// 2) Else, returns a new ServerConfig and a new gRPC server (with its own TLS listener on a different port).
func configureClusterListener(conf *localconfig.TopLevel, generalConf comm.ServerConfig, generalSrv *comm.GRPCServer, loadPEM loadPEMFunc) (comm.ServerConfig, *comm.GRPCServer) {
	clusterConf := conf.General.Cluster
	// If listen address is not configured, or the TLS certificate isn't configured,
	// it means we use the general listener of the node.
	if clusterConf.ListenPort == 0 && clusterConf.ServerCertificate == "" && clusterConf.ListenAddress == "" && clusterConf.ServerPrivateKey == "" {
		logger.Info("Cluster listener is not configured, defaulting to use the general listener on port", conf.General.ListenPort)
		return generalConf, generalSrv
	}

	// Else, one of the above is defined, so all 4 properties should be defined.
	if clusterConf.ListenPort == 0 || clusterConf.ServerCertificate == "" || clusterConf.ListenAddress == "" || clusterConf.ServerPrivateKey == "" {
		logger.Panic("Options: General.Cluster.ListenPort, General.Cluster.ListenAddress, General.Cluster.ServerCertificate," +
			" General.Cluster.ServerPrivateKey, should be defined altogether.")
	}

	cert, err := loadPEM(clusterConf.ServerCertificate)
	if err != nil {
		logger.Panicf("Failed to load cluster server certificate from '%s' (%s)", clusterConf.ServerCertificate, err)
	}

	key, err := loadPEM(clusterConf.ServerPrivateKey)
	if err != nil {
		logger.Panicf("Failed to load cluster server key from '%s' (%s)", clusterConf.ServerPrivateKey, err)
	}

	port := fmt.Sprintf("%d", clusterConf.ListenPort)
	bindAddr := net.JoinHostPort(clusterConf.ListenAddress, port)

	var clientRootCAs [][]byte
	for _, serverRoot := range conf.General.Cluster.RootCAs {
		rootCACert, err := loadPEM(serverRoot)
		if err != nil {
			logger.Panicf("Failed to load CA cert file '%s' (%s)",
				err, serverRoot)
		}
		clientRootCAs = append(clientRootCAs, rootCACert)
	}

	serverConf := comm.ServerConfig{
		StreamInterceptors: generalConf.StreamInterceptors,
		UnaryInterceptors:  generalConf.UnaryInterceptors,
		ConnectionTimeout:  generalConf.ConnectionTimeout,
		MetricsProvider:    generalConf.MetricsProvider,
		Logger:             generalConf.Logger,
		KaOpts:             generalConf.KaOpts,
		SecOpts: &comm.SecureOptions{
			CipherSuites:      comm.DefaultTLSCipherSuites,
			ClientRootCAs:     clientRootCAs,
			RequireClientCert: true,
			Certificate:       cert,
			UseTLS:            true,
			Key:               key,
		},
	}

	srv, err := comm.NewGRPCServer(bindAddr, serverConf)
	if err != nil {
		logger.Panicf("Failed creating gRPC server on %s:%d due to %v", clusterConf.ListenAddress, clusterConf.ListenPort, err)
	}

	return serverConf, srv
}

func initializeClusterClientConfig(conf *localconfig.TopLevel, clusterType bool, bootstrapBlock *cb.Block) comm.ClientConfig {
	if clusterType && !conf.General.TLS.Enabled {
		logger.Panicf("TLS is required for running ordering nodes of type %s.", consensusType(bootstrapBlock))
	}
	cc := comm.ClientConfig{
		AsyncConnect: true,
		KaOpts:       comm.DefaultKeepaliveOptions,
		Timeout:      conf.General.Cluster.DialTimeout,
		SecOpts:      &comm.SecureOptions{},
	}

	if (!conf.General.TLS.Enabled) || conf.General.Cluster.ClientCertificate == "" {
		return cc
	}

	certFile := conf.General.Cluster.ClientCertificate
	certBytes, err := ioutil.ReadFile(certFile)
	if err != nil {
		logger.Fatalf("Failed to load client TLS certificate file '%s' (%s)", certFile, err)
	}

	keyFile := conf.General.Cluster.ClientPrivateKey
	keyBytes, err := ioutil.ReadFile(keyFile)
	if err != nil {
		logger.Fatalf("Failed to load client TLS key file '%s' (%s)", keyFile, err)
	}

	var serverRootCAs [][]byte
	for _, serverRoot := range conf.General.Cluster.RootCAs {
		rootCACert, err := ioutil.ReadFile(serverRoot)
		if err != nil {
			logger.Fatalf("Failed to load ServerRootCAs file '%s' (%s)",
				err, serverRoot)
		}
		serverRootCAs = append(serverRootCAs, rootCACert)
	}

	cc.SecOpts = &comm.SecureOptions{
		RequireClientCert: true,
		CipherSuites:      comm.DefaultTLSCipherSuites,
		ServerRootCAs:     serverRootCAs,
		Certificate:       certBytes,
		Key:               keyBytes,
		UseTLS:            true,
	}

	return cc
}

func initializeServerConfig(conf *localconfig.TopLevel, metricsProvider metrics.Provider) comm.ServerConfig {
	// secure server config
	secureOpts := &comm.SecureOptions{
		UseTLS:            conf.General.TLS.Enabled,
		RequireClientCert: conf.General.TLS.ClientAuthRequired,
	}
	// check to see if TLS is enabled
	if secureOpts.UseTLS {
		msg := "TLS"
		// load crypto material from files
		serverCertificate, err := ioutil.ReadFile(conf.General.TLS.Certificate)
		if err != nil {
			logger.Fatalf("Failed to load server Certificate file '%s' (%s)",
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
		if secureOpts.RequireClientCert {
			for _, clientRoot := range conf.General.TLS.ClientRootCAs {
				root, err := ioutil.ReadFile(clientRoot)
				if err != nil {
					logger.Fatalf("Failed to load ClientRootCAs file '%s' (%s)",
						err, clientRoot)
				}
				clientRootCAs = append(clientRootCAs, root)
			}
			msg = "mutual TLS"
		}
		secureOpts.Key = serverKey
		secureOpts.Certificate = serverCertificate
		secureOpts.ClientRootCAs = clientRootCAs
		logger.Infof("Starting orderer with %s enabled", msg)
	}
	kaOpts := comm.DefaultKeepaliveOptions
	// keepalive settings
	// ServerMinInterval must be greater than 0
	if conf.General.Keepalive.ServerMinInterval > time.Duration(0) {
		kaOpts.ServerMinInterval = conf.General.Keepalive.ServerMinInterval
	}
	kaOpts.ServerInterval = conf.General.Keepalive.ServerInterval
	kaOpts.ServerTimeout = conf.General.Keepalive.ServerTimeout

	commLogger := flogging.MustGetLogger("core.comm").With("server", "Orderer")
	if metricsProvider == nil {
		metricsProvider = &disabled.Provider{}
	}

	return comm.ServerConfig{
		SecOpts:         secureOpts,
		KaOpts:          kaOpts,
		Logger:          commLogger,
		MetricsProvider: metricsProvider,
		StreamInterceptors: []grpc.StreamServerInterceptor{
			grpcmetrics.StreamServerInterceptor(grpcmetrics.NewStreamMetrics(metricsProvider)),
			grpclogging.StreamServerInterceptor(flogging.MustGetLogger("comm.grpc.server").Zap()),
		},
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			grpcmetrics.UnaryServerInterceptor(grpcmetrics.NewUnaryMetrics(metricsProvider)),
			grpclogging.UnaryServerInterceptor(
				flogging.MustGetLogger("comm.grpc.server").Zap(),
				grpclogging.WithLeveler(grpclogging.LevelerFunc(grpcLeveler)),
			),
		},
	}
}

func grpcLeveler(ctx context.Context, fullMethod string) zapcore.Level {
	switch fullMethod {
	case "/orderer.Cluster/Step":
		return flogging.DisabledLevel
	default:
		return zapcore.InfoLevel
	}
}

func extractBootstrapBlock(conf *localconfig.TopLevel) *cb.Block {
	var bootstrapBlock *cb.Block

	// Select the bootstrapping mechanism
	switch conf.General.GenesisMethod {
	case "provisional":
		bootstrapBlock = encoder.New(genesisconfig.Load(conf.General.GenesisProfile)).GenesisBlockForChannel(conf.General.SystemChannel)
	case "file":
		bootstrapBlock = file.New(conf.General.GenesisFile).GenesisBlock()
	default:
		logger.Panic("Unknown genesis method:", conf.General.GenesisMethod)
	}

	return bootstrapBlock
}

func initializeBootstrapChannel(genesisBlock *cb.Block, lf blockledger.Factory) {
	chainID, err := utils.GetChainIDFromBlock(genesisBlock)
	if err != nil {
		logger.Fatal("Failed to parse chain ID from genesis block:", err)
	}
	gl, err := lf.GetOrCreate(chainID)
	if err != nil {
		logger.Fatal("Failed to create the system chain:", err)
	}

	if err := gl.Append(genesisBlock); err != nil {
		logger.Fatal("Could not write genesis block to ledger:", err)
	}
}

func isClusterType(genesisBlock *cb.Block) bool {
	_, exists := clusterTypes[consensusType(genesisBlock)]
	return exists
}

func consensusType(genesisBlock *cb.Block) string {
	if genesisBlock.Data == nil || len(genesisBlock.Data.Data) == 0 {
		logger.Fatalf("Empty genesis block")
	}
	env := &cb.Envelope{}
	if err := proto.Unmarshal(genesisBlock.Data.Data[0], env); err != nil {
		logger.Fatalf("Failed to unmarshal the genesis block's envelope: %v", err)
	}
	bundle, err := channelconfig.NewBundleFromEnvelope(env)
	if err != nil {
		logger.Fatalf("Failed creating bundle from the genesis block: %v", err)
	}
	ordConf, exists := bundle.OrdererConfig()
	if !exists {
		logger.Fatalf("Orderer config doesn't exist in bundle derived from genesis block")
	}
	return ordConf.ConsensusType()
}

func initializeGrpcServer(conf *localconfig.TopLevel, serverConfig comm.ServerConfig) *comm.GRPCServer {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", conf.General.ListenAddress, conf.General.ListenPort))
	if err != nil {
		logger.Fatal("Failed to listen:", err)
	}

	// Create GRPC server - return if an error occurs
	grpcServer, err := comm.NewGRPCServerFromListener(lis, serverConfig)
	if err != nil {
		logger.Fatal("Failed to return new GRPC server:", err)
	}

	return grpcServer
}

func initializeLocalMsp(conf *localconfig.TopLevel) {
	// Load local MSP
	err := mspmgmt.LoadLocalMsp(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil { // Handle errors reading the config file
		logger.Fatal("Failed to initialize local MSP:", err)
	}
}

//go:generate counterfeiter -o mocks/health_checker.go -fake-name HealthChecker . healthChecker

// HealthChecker defines the contract for health checker
type healthChecker interface {
	RegisterChecker(component string, checker healthz.HealthChecker) error
}

func initializeMultichannelRegistrar(
	bootstrapBlock *cb.Block,
	ri *replicationInitiator,
	clusterDialer *cluster.PredicateDialer,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	conf *localconfig.TopLevel,
	signer crypto.LocalSigner,
	metricsProvider metrics.Provider,
	healthChecker healthChecker,
	lf blockledger.Factory,
	callbacks ...channelconfig.BundleActor,
) *multichannel.Registrar {
	genesisBlock := extractBootstrapBlock(conf)
	// Are we bootstrapping?
	if len(lf.ChainIDs()) == 0 {
		initializeBootstrapChannel(genesisBlock, lf)
	} else {
		logger.Info("Not bootstrapping because of existing chains")
	}

	consenters := make(map[string]consensus.Consenter)

	registrar := multichannel.NewRegistrar(lf, signer, metricsProvider, callbacks...)

	consenters["solo"] = solo.New()
	var kafkaMetrics *kafka.Metrics
	consenters["kafka"], kafkaMetrics = kafka.New(conf, metricsProvider, healthChecker, registrar)
	// Note, we pass a 'nil' channel here, we could pass a channel that
	// closes if we wished to cleanup this routine on exit.
	go kafkaMetrics.PollGoMetricsUntilStop(time.Minute, nil)
	if isClusterType(bootstrapBlock) {
		initializeEtcdraftConsenter(consenters, conf, lf, clusterDialer, bootstrapBlock, ri, srvConf, srv, registrar, metricsProvider)
	}
	registrar.Initialize(consenters)
	return registrar
}

func initializeEtcdraftConsenter(
	consenters map[string]consensus.Consenter,
	conf *localconfig.TopLevel,
	lf blockledger.Factory,
	clusterDialer *cluster.PredicateDialer,
	bootstrapBlock *cb.Block,
	ri *replicationInitiator,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	registrar *multichannel.Registrar,
	metricsProvider metrics.Provider,
) {
	replicationRefreshInterval := conf.General.Cluster.ReplicationBackgroundRefreshInterval
	if replicationRefreshInterval == 0 {
		replicationRefreshInterval = defaultReplicationBackgroundRefreshInterval
	}

	systemChannelName, err := utils.GetChainIDFromBlock(bootstrapBlock)
	if err != nil {
		ri.logger.Panicf("Failed extracting system channel name from bootstrap block: %v", err)
	}
	systemLedger, err := lf.GetOrCreate(systemChannelName)
	if err != nil {
		ri.logger.Panicf("Failed obtaining system channel (%s) ledger: %v", systemChannelName, err)
	}
	getConfigBlock := func() *cb.Block {
		return multichannel.ConfigBlock(systemLedger)
	}

	exponentialSleep := exponentialDurationSeries(replicationBackgroundInitialRefreshInterval, replicationRefreshInterval)
	ticker := newTicker(exponentialSleep)

	icr := &inactiveChainReplicator{
		logger:                            logger,
		scheduleChan:                      ticker.C,
		quitChan:                          make(chan struct{}),
		replicator:                        ri,
		chains2CreationCallbacks:          make(map[string]chainCreation),
		retrieveLastSysChannelConfigBlock: getConfigBlock,
		registerChain:                     ri.registerChain,
	}

	// Use the inactiveChainReplicator as a channel lister, since it has knowledge
	// of all inactive chains.
	// This is to prevent us pulling the entire system chain when attempting to enumerate
	// the channels in the system.
	ri.channelLister = icr

	go icr.run()
	raftConsenter := etcdraft.New(clusterDialer, conf, srvConf, srv, registrar, icr, metricsProvider)
	consenters["etcdraft"] = raftConsenter
}

func newOperationsSystem(ops localconfig.Operations, metrics localconfig.Metrics) *operations.System {
	return operations.NewSystem(operations.Options{
		Logger:        flogging.MustGetLogger("orderer.operations"),
		ListenAddress: ops.ListenAddress,
		Metrics: operations.MetricsOptions{
			Provider: metrics.Provider,
			Statsd: &operations.Statsd{
				Network:       metrics.Statsd.Network,
				Address:       metrics.Statsd.Address,
				WriteInterval: metrics.Statsd.WriteInterval,
				Prefix:        metrics.Statsd.Prefix,
			},
		},
		TLS: operations.TLS{
			Enabled:            ops.TLS.Enabled,
			CertFile:           ops.TLS.Certificate,
			KeyFile:            ops.TLS.PrivateKey,
			ClientCertRequired: ops.TLS.ClientAuthRequired,
			ClientCACertFiles:  ops.TLS.ClientRootCAs,
		},
		Version: metadata.Version,
	})
}

func updateTrustedRoots(rootCASupport *comm.CASupport, cm channelconfig.Resources, servers ...*comm.GRPCServer) {
	rootCASupport.Lock()
	defer rootCASupport.Unlock()

	appRootCAs := [][]byte{}
	ordererRootCAs := [][]byte{}
	appOrgMSPs := make(map[string]struct{})
	ordOrgMSPs := make(map[string]struct{})

	if ac, ok := cm.ApplicationConfig(); ok {
		// loop through app orgs and build map of MSPIDs
		for _, appOrg := range ac.Organizations() {
			appOrgMSPs[appOrg.MSPID()] = struct{}{}
		}
	}

	if ac, ok := cm.OrdererConfig(); ok {
		// loop through orderer orgs and build map of MSPIDs
		for _, ordOrg := range ac.Organizations() {
			ordOrgMSPs[ordOrg.MSPID()] = struct{}{}
		}
	}

	if cc, ok := cm.ConsortiumsConfig(); ok {
		for _, consortium := range cc.Consortiums() {
			// loop through consortium orgs and build map of MSPIDs
			for _, consortiumOrg := range consortium.Organizations() {
				appOrgMSPs[consortiumOrg.MSPID()] = struct{}{}
			}
		}
	}

	cid := cm.ConfigtxValidator().ChainID()
	logger.Debugf("updating root CAs for channel [%s]", cid)
	msps, err := cm.MSPManager().GetMSPs()
	if err != nil {
		logger.Errorf("Error getting root CAs for channel %s (%s)", cid, err)
		return
	}
	for k, v := range msps {
		// check to see if this is a FABRIC MSP
		if v.GetType() == msp.FABRIC {
			for _, root := range v.GetTLSRootCerts() {
				// check to see of this is an app org MSP
				if _, ok := appOrgMSPs[k]; ok {
					logger.Debugf("adding app root CAs for MSP [%s]", k)
					appRootCAs = append(appRootCAs, root)
				}
				// check to see of this is an orderer org MSP
				if _, ok := ordOrgMSPs[k]; ok {
					logger.Debugf("adding orderer root CAs for MSP [%s]", k)
					ordererRootCAs = append(ordererRootCAs, root)
				}
			}
			for _, intermediate := range v.GetTLSIntermediateCerts() {
				// check to see of this is an app org MSP
				if _, ok := appOrgMSPs[k]; ok {
					logger.Debugf("adding app root CAs for MSP [%s]", k)
					appRootCAs = append(appRootCAs, intermediate)
				}
				// check to see of this is an orderer org MSP
				if _, ok := ordOrgMSPs[k]; ok {
					logger.Debugf("adding orderer root CAs for MSP [%s]", k)
					ordererRootCAs = append(ordererRootCAs, intermediate)
				}
			}
		}
	}
	rootCASupport.AppRootCAsByChain[cid] = appRootCAs
	rootCASupport.OrdererRootCAsByChain[cid] = ordererRootCAs

	// now iterate over all roots for all app and orderer chains
	trustedRoots := [][]byte{}
	for _, roots := range rootCASupport.AppRootCAsByChain {
		trustedRoots = append(trustedRoots, roots...)
	}
	for _, roots := range rootCASupport.OrdererRootCAsByChain {
		trustedRoots = append(trustedRoots, roots...)
	}
	// also need to append statically configured root certs
	if len(rootCASupport.ClientRootCAs) > 0 {
		trustedRoots = append(trustedRoots, rootCASupport.ClientRootCAs...)
	}

	// now update the client roots for the gRPC server
	for _, srv := range servers {
		err = srv.SetClientRootCAs(trustedRoots)
		if err != nil {
			msg := "Failed to update trusted roots for orderer from latest config " +
				"block.  This orderer may not be able to communicate " +
				"with members of channel %s (%s)"
			logger.Warningf(msg, cm.ConfigtxValidator().ChainID(), err)
		}
	}
}

func updateClusterDialer(rootCASupport *comm.CASupport, clusterDialer *cluster.PredicateDialer, localClusterRootCAs [][]byte) {
	rootCASupport.Lock()
	defer rootCASupport.Unlock()

	// Iterate over all orderer root CAs for all chains and add them
	// to the root CAs
	var clusterRootCAs [][]byte
	for _, roots := range rootCASupport.OrdererRootCAsByChain {
		clusterRootCAs = append(clusterRootCAs, roots...)
	}

	// Add the local root CAs too
	clusterRootCAs = append(clusterRootCAs, localClusterRootCAs...)
	// Update the cluster config with the new root CAs
	clusterConfig := clusterDialer.Config.Load().(comm.ClientConfig)
	clusterConfig.SecOpts.ServerRootCAs = clusterRootCAs
	clusterDialer.SetConfig(clusterConfig)
}

func prettyPrintStruct(i interface{}) {
	params := util.Flatten(i)
	var buffer bytes.Buffer
	for i := range params {
		buffer.WriteString("\n\t")
		buffer.WriteString(params[i])
	}
	logger.Infof("Orderer config values:%s\n", buffer.String())
}
