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
	"sync"
	"syscall"
	"time"

	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/common/fabhttp"
	"github.com/hyperledger/fabric/common/flogging"
	floggingmetrics "github.com/hyperledger/fabric/common/flogging/metrics"
	"github.com/hyperledger/fabric/common/grpclogging"
	"github.com/hyperledger/fabric/common/grpcmetrics"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/operations"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/msp"
	"github.com/hyperledger/fabric/orderer/common/channelparticipation"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/metadata"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"gopkg.in/alecthomas/kingpin.v2"
)

var logger = flogging.MustGetLogger("orderer.common.server")

// command line flags
var (
	app = kingpin.New("orderer", "Hyperledger Fabric orderer node")

	_       = app.Command("start", "Start the orderer node").Default() // preserved for cli compatibility
	version = app.Command("version", "Show version information")

	clusterTypes = map[string]struct{}{
		"etcdraft": {},
		"BFT":      {},
	}
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

	prettyPrintStruct(conf)

	cryptoProvider := factory.GetDefault()

	signer, signErr := loadLocalMSP(conf).GetDefaultSigningIdentity()
	if signErr != nil {
		logger.Panicf("Failed to get local MSP identity: %s", signErr)
	}

	opsSystem := newOperationsSystem(conf.Operations, conf.Metrics)
	if err = opsSystem.Start(); err != nil {
		logger.Panicf("failed to start operations subsystem: %s", err)
	}
	defer opsSystem.Stop()
	metricsProvider := opsSystem.Provider
	logObserver := floggingmetrics.NewObserver(metricsProvider)
	flogging.SetObserver(logObserver)

	serverConfig := initializeServerConfig(conf, metricsProvider)
	grpcServer := initializeGrpcServer(conf, serverConfig)
	caMgr := &caManager{
		appRootCAsByChain:     make(map[string][][]byte),
		ordererRootCAsByChain: make(map[string][][]byte),
		clientRootCAs:         serverConfig.SecOpts.ClientRootCAs,
	}

	lf, err := createLedgerFactory(conf, metricsProvider)
	if err != nil {
		logger.Panicf("Failed to create ledger factory: %v", err)
	}

	switch conf.General.BootstrapMethod {
	case "file":
		logger.Panic("Bootstrap method: 'file' is forbidden, since system channel is no longer supported")
	case "none":
		verifyNoSystemChannel(lf, cryptoProvider)
		verifyNoSystemChannelJoinBlock(conf, cryptoProvider)
	default:
		logger.Panicf("Unknown bootstrap method: %s", conf.General.BootstrapMethod)
	}

	logger.Infof("Starting without a system channel")

	// configure following artifacts properly, always a cluster (e.g. Raft or BFT)
	clusterServerConfig := serverConfig
	clusterGRPCServer := grpcServer // by default, cluster shares the same grpc server
	var clusterClientConfig comm.ClientConfig
	var clusterDialer *cluster.PredicateDialer

	var reuseGrpcListener bool
	var serversToUpdate []*comm.GRPCServer

	logger.Infof("Setting up cluster")
	clusterClientConfig, reuseGrpcListener = initializeClusterClientConfig(conf)
	clusterDialer = &cluster.PredicateDialer{
		Config: clusterClientConfig,
	}

	if !reuseGrpcListener {
		clusterServerConfig, clusterGRPCServer = configureClusterListener(conf, serverConfig, ioutil.ReadFile)
	}

	// If we have a separate gRPC server for the cluster,
	// we need to update its TLS CA certificate pool.
	serversToUpdate = append(serversToUpdate, clusterGRPCServer)

	identityBytes, err := signer.Serialize()
	if err != nil {
		logger.Panicf("Failed serializing signing identity: %v", err)
	}

	expirationLogger := flogging.MustGetLogger("certmonitor")
	crypto.TrackExpiration(
		serverConfig.SecOpts.UseTLS,
		serverConfig.SecOpts.Certificate,
		[][]byte{clusterClientConfig.SecOpts.Certificate},
		identityBytes,
		expirationLogger.Infof,
		expirationLogger.Warnf, // This can be used to piggyback a metric event in the future
		time.Now(),
		time.AfterFunc)

	// if cluster is reusing client-facing server, then it is already
	// appended to serversToUpdate at this point.
	if grpcServer.MutualTLSRequired() && !reuseGrpcListener {
		serversToUpdate = append(serversToUpdate, grpcServer)
	}

	tlsCallback := func(bundle *channelconfig.Bundle) {
		logger.Debug("Executing callback to update root CAs")
		caMgr.updateTrustedRoots(bundle, serversToUpdate...)
		caMgr.updateClusterDialer(
			clusterDialer,
			clusterClientConfig.SecOpts.ServerRootCAs,
		)
	}

	manager := initializeMultichannelRegistrar(clusterDialer, clusterServerConfig, clusterGRPCServer, conf, signer, metricsProvider, lf, cryptoProvider, tlsCallback)

	adminServer := newAdminServer(conf.Admin)
	adminServer.RegisterHandler(
		channelparticipation.URLBaseV1,
		channelparticipation.NewHTTPHandler(conf.ChannelParticipation, manager),
		conf.Admin.TLS.Enabled,
	)
	if err = adminServer.Start(); err != nil {
		logger.Panicf("failed to start admin server: %s", err)
	}
	defer adminServer.Stop()

	mutualTLS := serverConfig.SecOpts.UseTLS && serverConfig.SecOpts.RequireClientCert
	server := NewServer(
		manager,
		metricsProvider,
		&conf.Debug,
		conf.General.Authentication.TimeWindow,
		mutualTLS,
		conf.General.Authentication.NoExpirationChecks,
	)

	logger.Infof("Starting %s", metadata.GetVersionInfo())
	handleSignals(addPlatformSignals(map[os.Signal]func(){
		syscall.SIGTERM: func() {
			grpcServer.Stop()
			if clusterGRPCServer != grpcServer {
				clusterGRPCServer.Stop()
			}
		},
	}))

	if !reuseGrpcListener {
		logger.Info("Starting cluster listener on", clusterGRPCServer.Address())
		go clusterGRPCServer.Start()
	}

	if conf.General.Profile.Enabled {
		go initializeProfilingService(conf)
	}
	ab.RegisterAtomicBroadcastServer(grpcServer.Server(), server)
	logger.Info("Beginning to serve requests")
	if err := grpcServer.Start(); err != nil {
		logger.Fatalf("Atomic Broadcast gRPC server has terminated while serving requests due to: %v", err)
	}
}

// Searches whether there is a join block for a system channel, and if there is, panic.
func verifyNoSystemChannelJoinBlock(config *localconfig.TopLevel, cryptoProvider bccsp.BCCSP) {
	joinBlockFileRepo, err := multichannel.InitJoinBlockFileRepo(config)
	if err != nil {
		logger.Panicf("Failed initializing join-block file repo: %v", err)
	}

	joinBlockFiles, err := joinBlockFileRepo.List()
	if err != nil {
		logger.Panicf("Failed listing join-block file repo: %v", err)
	}

	for _, fileName := range joinBlockFiles {
		channelName := joinBlockFileRepo.FileToBaseName(fileName)
		blockBytes, err := joinBlockFileRepo.Read(channelName)
		if err != nil {
			logger.Panicf("Failed reading join-block for channel '%s', error: %v", channelName, err)
		}
		block, err := protoutil.UnmarshalBlock(blockBytes)
		if err != nil {
			logger.Panicf("Failed unmarshalling join-block for channel '%s', error: %v", channelName, err)
		}
		if err = validateBootstrapBlock(block, cryptoProvider); err == nil {
			logger.Panicf("Error: found a system channel join-block, channel: %s, block number: %d, file: %s. This version does not support the system channel. Remove it before upgrading.", channelName, block.Header.Number, fileName)
		}
	}
}

func reuseListener(conf *localconfig.TopLevel) bool {
	clusterConf := conf.General.Cluster
	// If listen address is not configured, and the TLS certificate isn't configured,
	// it means we use the general listener of the node.
	if clusterConf.ListenPort == 0 && clusterConf.ServerCertificate == "" && clusterConf.ListenAddress == "" && clusterConf.ServerPrivateKey == "" {
		logger.Info("Cluster listener is not configured, defaulting to use the general listener on port", conf.General.ListenPort)

		if !conf.General.TLS.Enabled {
			logger.Panicf("TLS is required for running ordering nodes of cluster type.")
		}

		return true
	}

	// Else, one of the above is defined, so all 4 properties should be defined.
	if clusterConf.ListenPort == 0 || clusterConf.ServerCertificate == "" || clusterConf.ListenAddress == "" || clusterConf.ServerPrivateKey == "" {
		logger.Panic("Options: General.Cluster.ListenPort, General.Cluster.ListenAddress, General.Cluster.ServerCertificate, General.Cluster.ServerPrivateKey, should be defined altogether.")
	}

	return false
}

// verifyNoSystemChannel loops through all channels, and panics if the last
// config block for a channel is consistent with a system channel.
func verifyNoSystemChannel(lf blockledger.Factory, bccsp bccsp.BCCSP) {
	for _, cID := range lf.ChannelIDs() {
		channelLedger, err := lf.GetOrCreate(cID)
		if err != nil {
			logger.Panicf("Failed getting channel %v's ledger: %v", cID, err)
		}
		if channelLedger.Height() == 0 {
			continue // Some channels may have an empty ledger and (possibly) a join-block, skip those
		}

		channelConfigBlock := multichannel.ConfigBlockOrPanic(channelLedger)

		err = validateBootstrapBlock(channelConfigBlock, bccsp)
		if err == nil {
			logger.Panicf("Error: found system channel config block in the ledger, channel: %s, block number: %d. This version does not support the system channel. Remove it before upgrading.", cID, channelConfigBlock.Header.Number)
		}
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
	logger.Info("Starting Go pprof profiling service on:", conf.General.Profile.Address)
	// The ListenAndServe() call does not return unless an error occurs.
	logger.Panic("Go pprof service failed:", http.ListenAndServe(conf.General.Profile.Address, nil))
}

func handleSignals(handlers map[os.Signal]func()) {
	var signals []os.Signal
	for sig := range handlers {
		signals = append(signals, sig)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)

	go func() {
		for sig := range signalChan {
			logger.Infof("Received signal: %d (%s)", sig, sig)
			handlers[sig]()
		}
	}()
}

type loadPEMFunc func(string) ([]byte, error)

// configureClusterListener returns a new ServerConfig and a new gRPC server (with its own TLS listener).
func configureClusterListener(conf *localconfig.TopLevel, generalConf comm.ServerConfig, loadPEM loadPEMFunc) (comm.ServerConfig, *comm.GRPCServer) {
	clusterConf := conf.General.Cluster

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
			logger.Panicf("Failed to load CA cert file '%s' (%s)", serverRoot, err)
		}
		clientRootCAs = append(clientRootCAs, rootCACert)
	}

	serverConf := comm.ServerConfig{
		StreamInterceptors: generalConf.StreamInterceptors,
		UnaryInterceptors:  generalConf.UnaryInterceptors,
		ConnectionTimeout:  generalConf.ConnectionTimeout,
		ServerStatsHandler: generalConf.ServerStatsHandler,
		Logger:             generalConf.Logger,
		KaOpts:             generalConf.KaOpts,
		SecOpts: comm.SecureOptions{
			TimeShift:         conf.General.Cluster.TLSHandshakeTimeShift,
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

func initializeClusterClientConfig(conf *localconfig.TopLevel) (comm.ClientConfig, bool) {
	cc := comm.ClientConfig{
		AsyncConnect:   true,
		KaOpts:         comm.DefaultKeepaliveOptions,
		DialTimeout:    conf.General.Cluster.DialTimeout,
		SecOpts:        comm.SecureOptions{},
		MaxRecvMsgSize: int(conf.General.MaxRecvMsgSize),
		MaxSendMsgSize: int(conf.General.MaxSendMsgSize),
	}

	reuseGrpcListener := reuseListener(conf)

	certFile := conf.General.Cluster.ClientCertificate
	keyFile := conf.General.Cluster.ClientPrivateKey
	if certFile == "" && keyFile == "" {
		if !reuseGrpcListener {
			return cc, reuseGrpcListener
		}
		certFile = conf.General.TLS.Certificate
		keyFile = conf.General.TLS.PrivateKey
	}

	certBytes, err := ioutil.ReadFile(certFile)
	if err != nil {
		logger.Fatalf("Failed to load client TLS certificate file '%s' (%s)", certFile, err)
	}

	keyBytes, err := ioutil.ReadFile(keyFile)
	if err != nil {
		logger.Fatalf("Failed to load client TLS key file '%s' (%s)", keyFile, err)
	}

	var serverRootCAs [][]byte
	for _, serverRoot := range conf.General.Cluster.RootCAs {
		rootCACert, err := ioutil.ReadFile(serverRoot)
		if err != nil {
			logger.Fatalf("Failed to load ServerRootCAs file '%s' (%s)", serverRoot, err)
		}
		serverRootCAs = append(serverRootCAs, rootCACert)
	}

	timeShift := conf.General.TLS.TLSHandshakeTimeShift
	if !reuseGrpcListener {
		timeShift = conf.General.Cluster.TLSHandshakeTimeShift
	}

	cc.SecOpts = comm.SecureOptions{
		TimeShift:         timeShift,
		RequireClientCert: true,
		CipherSuites:      comm.DefaultTLSCipherSuites,
		ServerRootCAs:     serverRootCAs,
		Certificate:       certBytes,
		Key:               keyBytes,
		UseTLS:            true,
	}

	return cc, reuseGrpcListener
}

func initializeServerConfig(conf *localconfig.TopLevel, metricsProvider metrics.Provider) comm.ServerConfig {
	// secure server config
	secureOpts := comm.SecureOptions{
		UseTLS:            conf.General.TLS.Enabled,
		RequireClientCert: conf.General.TLS.ClientAuthRequired,
		TimeShift:         conf.General.TLS.TLSHandshakeTimeShift,
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
		secureOpts.ServerRootCAs = serverRootCAs
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
		SecOpts:            secureOpts,
		KaOpts:             kaOpts,
		Logger:             commLogger,
		ServerStatsHandler: comm.NewServerStatsHandler(metricsProvider),
		ConnectionTimeout:  conf.General.ConnectionTimeout,
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
		MaxRecvMsgSize: int(conf.General.MaxRecvMsgSize),
		MaxSendMsgSize: int(conf.General.MaxSendMsgSize),
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

func loadLocalMSP(conf *localconfig.TopLevel) msp.MSP {
	// MUST call GetLocalMspConfig first, so that default BCCSP is properly
	// initialized prior to LoadByType.
	mspConfig, err := msp.GetLocalMspConfig(conf.General.LocalMSPDir, conf.General.BCCSP, conf.General.LocalMSPID)
	if err != nil {
		logger.Panicf("Failed to get local msp config: %v", err)
	}

	typ := msp.ProviderTypeToString(msp.FABRIC)
	opts, found := msp.Options[typ]
	if !found {
		logger.Panicf("MSP option for type %s is not found", typ)
	}

	localmsp, err := msp.New(opts, factory.GetDefault())
	if err != nil {
		logger.Panicf("Failed to load local MSP: %v", err)
	}

	if err = localmsp.Setup(mspConfig); err != nil {
		logger.Panicf("Failed to setup local msp with config: %v", err)
	}

	return localmsp
}

func initializeMultichannelRegistrar(
	clusterDialer *cluster.PredicateDialer,
	srvConf comm.ServerConfig,
	srv *comm.GRPCServer,
	conf *localconfig.TopLevel,
	signer identity.SignerSerializer,
	metricsProvider metrics.Provider,
	lf blockledger.Factory,
	bccsp bccsp.BCCSP,
	callbacks ...channelconfig.BundleActor,
) *multichannel.Registrar {
	dpmr := &DynamicPolicyManagerRegistry{}

	policyManagerCallback := func(bundle *channelconfig.Bundle) {
		dpmr.Update(bundle)
	}
	callbacks = append(callbacks, policyManagerCallback)

	registrar := multichannel.NewRegistrar(*conf, lf, signer, metricsProvider, bccsp, clusterDialer, callbacks...)

	consenters := map[string]consensus.Consenter{}
	consenterType := "etcdraft"

	// TODO this can be removed, both consenter types can be created
	// load consensus type from orderer config
	var consensusConfig localconfig.Consensus
	if err := mapstructure.Decode(conf.Consensus, &consensusConfig); err == nil && consensusConfig.Type != "" {
		consenterType = consensusConfig.Type
	}

	// the orderer can start without channels at all and have an initialized cluster type consenter
	switch consenterType {
	case "etcdraft":
		consenters["etcdraft"] = etcdraft.New(clusterDialer, conf, srvConf, srv, registrar, metricsProvider, bccsp)
	case "BFT":
		consenters["BFT"] = smartbft.New(dpmr.Registry(), signer, clusterDialer, conf, srvConf, srv, registrar, metricsProvider, bccsp)
	default:
		logger.Panicf("Unknown cluster type consenter '%s'", consenterType)
	}

	registrar.Initialize(consenters)
	return registrar
}

func newOperationsSystem(ops localconfig.Operations, metrics localconfig.Metrics) *operations.System {
	return operations.NewSystem(operations.Options{
		Options: fabhttp.Options{
			Logger:        flogging.MustGetLogger("orderer.operations"),
			ListenAddress: ops.ListenAddress,
			TLS: fabhttp.TLS{
				Enabled:            ops.TLS.Enabled,
				CertFile:           ops.TLS.Certificate,
				KeyFile:            ops.TLS.PrivateKey,
				ClientCertRequired: ops.TLS.ClientAuthRequired,
				ClientCACertFiles:  ops.TLS.ClientRootCAs,
			},
		},
		Metrics: operations.MetricsOptions{
			Provider: metrics.Provider,
			Statsd: &operations.Statsd{
				Network:       metrics.Statsd.Network,
				Address:       metrics.Statsd.Address,
				WriteInterval: metrics.Statsd.WriteInterval,
				Prefix:        metrics.Statsd.Prefix,
			},
		},
		Version: metadata.Version,
	})
}

func newAdminServer(admin localconfig.Admin) *fabhttp.Server {
	return fabhttp.NewServer(fabhttp.Options{
		Logger:        flogging.MustGetLogger("orderer.admin"),
		ListenAddress: admin.ListenAddress,
		TLS: fabhttp.TLS{
			Enabled:            admin.TLS.Enabled,
			CertFile:           admin.TLS.Certificate,
			KeyFile:            admin.TLS.PrivateKey,
			ClientCertRequired: admin.TLS.ClientAuthRequired,
			ClientCACertFiles:  admin.TLS.ClientRootCAs,
		},
	})
}

// caMgr manages certificate authorities scoped by channel
type caManager struct {
	sync.Mutex
	appRootCAsByChain     map[string][][]byte
	ordererRootCAsByChain map[string][][]byte
	clientRootCAs         [][]byte
}

func (mgr *caManager) updateTrustedRoots(
	cm channelconfig.Resources,
	servers ...*comm.GRPCServer,
) {
	mgr.Lock()
	defer mgr.Unlock()

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

	cid := cm.ConfigtxValidator().ChannelID()
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
	mgr.appRootCAsByChain[cid] = appRootCAs
	mgr.ordererRootCAsByChain[cid] = ordererRootCAs

	// now iterate over all roots for all app and orderer chains
	trustedRoots := [][]byte{}
	for _, roots := range mgr.appRootCAsByChain {
		trustedRoots = append(trustedRoots, roots...)
	}
	for _, roots := range mgr.ordererRootCAsByChain {
		trustedRoots = append(trustedRoots, roots...)
	}
	// also need to append statically configured root certs
	if len(mgr.clientRootCAs) > 0 {
		trustedRoots = append(trustedRoots, mgr.clientRootCAs...)
	}

	// now update the client roots for the gRPC server
	for _, srv := range servers {
		err = srv.SetClientRootCAs(trustedRoots)
		if err != nil {
			msg := "Failed to update trusted roots for orderer from latest config " +
				"block.  This orderer may not be able to communicate " +
				"with members of channel %s (%s)"
			logger.Warningf(msg, cm.ConfigtxValidator().ChannelID(), err)
		}
	}
}

func (mgr *caManager) updateClusterDialer(
	clusterDialer *cluster.PredicateDialer,
	localClusterRootCAs [][]byte,
) {
	mgr.Lock()
	defer mgr.Unlock()

	// Iterate over all orderer root CAs for all chains and add them
	// to the root CAs
	clusterRootCAs := make(cluster.StringSet)
	for _, orgRootCAs := range mgr.ordererRootCAsByChain {
		for _, rootCA := range orgRootCAs {
			clusterRootCAs[string(rootCA)] = struct{}{}
		}
	}

	// Add the local root CAs too
	for _, localRootCA := range localClusterRootCAs {
		clusterRootCAs[string(localRootCA)] = struct{}{}
	}

	// Convert StringSet to byte slice
	var clusterRootCAsBytes [][]byte
	for root := range clusterRootCAs {
		clusterRootCAsBytes = append(clusterRootCAsBytes, []byte(root))
	}

	// Update the cluster config with the new root CAs
	clusterDialer.UpdateRootCAs(clusterRootCAsBytes)
}

func prettyPrintStruct(i interface{}) {
	params := localconfig.Flatten(i)
	var buffer bytes.Buffer
	for i := range params {
		buffer.WriteString("\n\t")
		buffer.WriteString(params[i])
	}
	logger.Infof("Orderer config values:%s\n", buffer.String())
}
