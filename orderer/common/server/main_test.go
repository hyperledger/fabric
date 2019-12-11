// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	deliver_mocks "github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	ledger_mocks "github.com/hyperledger/fabric/common/ledger/blockledger/mocks"
	ramledger "github.com/hyperledger/fabric/common/ledger/blockledger/ram"
	"github.com/hyperledger/fabric/common/localmsp"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	"github.com/hyperledger/fabric/common/mocks/crypto"
	"github.com/hyperledger/fabric/common/tools/configtxgen/configtxgentest"
	"github.com/hyperledger/fabric/common/tools/configtxgen/encoder"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	"github.com/hyperledger/fabric/orderer/common/server/mocks"
	server_mocks "github.com/hyperledger/fabric/orderer/common/server/mocks"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestInitializeLogging(t *testing.T) {
	origEnvValue := os.Getenv("FABRIC_LOGGING_SPEC")
	os.Setenv("FABRIC_LOGGING_SPEC", "foo=debug")
	initializeLogging()
	assert.Equal(t, "debug", flogging.Global.Level("foo").String())
	os.Setenv("FABRIC_LOGGING_SPEC", origEnvValue)
}

func TestInitializeProfilingService(t *testing.T) {
	origEnvValue := os.Getenv("FABRIC_LOGGING_SPEC")
	defer os.Setenv("FABRIC_LOGGING_SPEC", origEnvValue)
	os.Setenv("FABRIC_LOGGING_SPEC", "debug")
	// get a free random port
	listenAddr := func() string {
		l, _ := net.Listen("tcp", "localhost:0")
		l.Close()
		return l.Addr().String()
	}()
	initializeProfilingService(
		&localconfig.TopLevel{
			General: localconfig.General{
				Profile: localconfig.Profile{
					Enabled: true,
					Address: listenAddr,
				}},
			Kafka: localconfig.Kafka{Verbose: true},
		},
	)
	time.Sleep(500 * time.Millisecond)
	if _, err := http.Get("http://" + listenAddr + "/" + "/debug/"); err != nil {
		t.Logf("Expected pprof to be up (will retry again in 3 seconds): %s", err)
		time.Sleep(3 * time.Second)
		if _, err := http.Get("http://" + listenAddr + "/" + "/debug/"); err != nil {
			t.Fatalf("Expected pprof to be up: %s", err)
		}
	}
}

func TestInitializeServerConfig(t *testing.T) {
	conf := &localconfig.TopLevel{
		General: localconfig.General{
			ConnectionTimeout: 7 * time.Second,
			TLS: localconfig.TLS{
				Enabled:            true,
				ClientAuthRequired: true,
				Certificate:        "main.go",
				PrivateKey:         "main.go",
				RootCAs:            []string{"main.go"},
				ClientRootCAs:      []string{"main.go"},
			},
		},
	}
	sc := initializeServerConfig(conf, nil)
	defaultOpts := comm.DefaultKeepaliveOptions
	assert.Equal(t, defaultOpts.ServerMinInterval, sc.KaOpts.ServerMinInterval)
	assert.Equal(t, time.Duration(0), sc.KaOpts.ServerInterval)
	assert.Equal(t, time.Duration(0), sc.KaOpts.ServerTimeout)
	assert.Equal(t, 7*time.Second, sc.ConnectionTimeout)
	testDuration := 10 * time.Second
	conf.General.Keepalive = localconfig.Keepalive{
		ServerMinInterval: testDuration,
		ServerInterval:    testDuration,
		ServerTimeout:     testDuration,
	}
	sc = initializeServerConfig(conf, nil)
	assert.Equal(t, testDuration, sc.KaOpts.ServerMinInterval)
	assert.Equal(t, testDuration, sc.KaOpts.ServerInterval)
	assert.Equal(t, testDuration, sc.KaOpts.ServerTimeout)

	sc = initializeServerConfig(conf, nil)
	assert.NotNil(t, sc.Logger)
	assert.Equal(t, comm.NewServerStatsHandler(&disabled.Provider{}), sc.ServerStatsHandler)
	assert.Len(t, sc.UnaryInterceptors, 2)
	assert.Len(t, sc.StreamInterceptors, 2)

	sc = initializeServerConfig(conf, &prometheus.Provider{})
	assert.NotNil(t, sc.ServerStatsHandler)

	goodFile := "main.go"
	badFile := "does_not_exist"

	oldLogger := logger
	defer func() { logger = oldLogger }()
	logger, _ = floggingtest.NewTestLogger(t)

	testCases := []struct {
		name           string
		certificate    string
		privateKey     string
		rootCA         string
		clientRootCert string
		clusterCert    string
		clusterKey     string
		clusterCA      string
	}{
		{"BadCertificate", badFile, goodFile, goodFile, goodFile, "", "", ""},
		{"BadPrivateKey", goodFile, badFile, goodFile, goodFile, "", "", ""},
		{"BadRootCA", goodFile, goodFile, badFile, goodFile, "", "", ""},
		{"BadClientRootCertificate", goodFile, goodFile, goodFile, badFile, "", "", ""},
		{"ClusterBadCertificate", goodFile, goodFile, goodFile, goodFile, badFile, goodFile, goodFile},
		{"ClusterBadPrivateKey", goodFile, goodFile, goodFile, goodFile, goodFile, badFile, goodFile},
		{"ClusterBadRootCA", goodFile, goodFile, goodFile, goodFile, goodFile, goodFile, badFile},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conf := &localconfig.TopLevel{
				General: localconfig.General{
					TLS: localconfig.TLS{
						Enabled:            true,
						ClientAuthRequired: true,
						Certificate:        tc.certificate,
						PrivateKey:         tc.privateKey,
						RootCAs:            []string{tc.rootCA},
						ClientRootCAs:      []string{tc.clientRootCert},
					},
					Cluster: localconfig.Cluster{
						ClientCertificate: tc.clusterCert,
						ClientPrivateKey:  tc.clusterKey,
						RootCAs:           []string{tc.clusterCA},
					},
				},
			}
			assert.Panics(t, func() {
				if tc.clusterCert == "" {
					initializeServerConfig(conf, nil)
				} else {
					initializeClusterClientConfig(conf)
				}
			},
			)
		})
	}
}

func TestInitializeBootstrapChannel(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	testCases := []struct {
		genesisMethod string
		ledgerType    string
		panics        bool
	}{
		{"provisional", "ram", false},
		{"provisional", "file", false},
		{"provisional", "json", false},
		{"invalid", "ram", true},
		{"file", "ram", true},
	}

	for _, tc := range testCases {

		t.Run(tc.genesisMethod+"/"+tc.ledgerType, func(t *testing.T) {

			fileLedgerLocation, _ := ioutil.TempDir("", "test-ledger")
			ledgerFactory, _ := createLedgerFactory(
				&localconfig.TopLevel{
					General: localconfig.General{LedgerType: tc.ledgerType},
					FileLedger: localconfig.FileLedger{
						Location: fileLedgerLocation,
					},
				},
				&disabled.Provider{},
			)

			bootstrapConfig := &localconfig.TopLevel{
				General: localconfig.General{
					GenesisMethod:  tc.genesisMethod,
					GenesisProfile: "SampleSingleMSPSolo",
					GenesisFile:    "genesisblock",
					SystemChannel:  genesisconfig.TestChainID,
				},
			}

			if tc.panics {
				assert.Panics(t, func() {
					genesisBlock := extractBootstrapBlock(bootstrapConfig)
					initializeBootstrapChannel(genesisBlock, ledgerFactory)
				})
			} else {
				assert.NotPanics(t, func() {
					genesisBlock := extractBootstrapBlock(bootstrapConfig)
					initializeBootstrapChannel(genesisBlock, ledgerFactory)
				})
			}
		})
	}
}

func TestExtractSysChanLastConfig(t *testing.T) {
	rlf := ramledger.New(10)
	conf := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	genesisBlock := encoder.New(conf).GenesisBlock()

	lastConf := extractSysChanLastConfig(rlf, genesisBlock)
	assert.Nil(t, lastConf)

	rl, err := rlf.GetOrCreate(genesisconfig.TestChainID)
	require.NoError(t, err)

	err = rl.Append(genesisBlock)
	require.NoError(t, err)

	lastConf = extractSysChanLastConfig(rlf, genesisBlock)
	assert.NotNil(t, lastConf)
	assert.Equal(t, uint64(0), lastConf.Header.Number)

	assert.Panics(t, func() {
		_ = extractSysChanLastConfig(rlf, nil)
	})

	configTx, err := utils.CreateSignedEnvelope(common.HeaderType_CONFIG, genesisconfig.TestChainID, nil, &common.ConfigEnvelope{}, 0, 0)
	require.NoError(t, err)

	nextBlock := blockledger.CreateNextBlock(rl, []*common.Envelope{configTx})
	nextBlock.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = utils.MarshalOrPanic(&common.Metadata{
		Value: utils.MarshalOrPanic(&common.LastConfig{Index: rl.Height()}),
	})
	err = rl.Append(nextBlock)
	require.NoError(t, err)

	lastConf = extractSysChanLastConfig(rlf, genesisBlock)
	assert.NotNil(t, lastConf)
	assert.Equal(t, uint64(1), lastConf.Header.Number)
}

func TestSelectClusterBootBlock(t *testing.T) {
	bootstrapBlock := &common.Block{Header: &common.BlockHeader{Number: 100}}
	lastConfBlock := &common.Block{Header: &common.BlockHeader{Number: 100}}

	clusterBoot := selectClusterBootBlock(bootstrapBlock, nil)
	assert.NotNil(t, clusterBoot)
	assert.Equal(t, uint64(100), clusterBoot.Header.Number)
	assert.True(t, bootstrapBlock == clusterBoot)

	clusterBoot = selectClusterBootBlock(bootstrapBlock, lastConfBlock)
	assert.NotNil(t, clusterBoot)
	assert.Equal(t, uint64(100), clusterBoot.Header.Number)
	assert.True(t, bootstrapBlock == clusterBoot)

	lastConfBlock.Header.Number = 200
	clusterBoot = selectClusterBootBlock(bootstrapBlock, lastConfBlock)
	assert.NotNil(t, clusterBoot)
	assert.Equal(t, uint64(200), clusterBoot.Header.Number)
	assert.True(t, lastConfBlock == clusterBoot)

	bootstrapBlock.Header.Number = 300
	clusterBoot = selectClusterBootBlock(bootstrapBlock, lastConfBlock)
	assert.NotNil(t, clusterBoot)
	assert.Equal(t, uint64(300), clusterBoot.Header.Number)
	assert.True(t, bootstrapBlock == clusterBoot)
}

func TestLoadLocalMSP(t *testing.T) {
	t.Run("Happy", func(t *testing.T) {
		assert.NotPanics(t, func() {
			localMSPDir, _ := configtest.GetDevMspDir()
			initializeLocalMsp(
				&localconfig.TopLevel{
					General: localconfig.General{
						LocalMSPDir: localMSPDir,
						LocalMSPID:  "SampleOrg",
						BCCSP: &factory.FactoryOpts{
							ProviderName: "SW",
							SwOpts: &factory.SwOpts{
								HashFamily: "SHA2",
								SecLevel:   256,
								Ephemeral:  true,
							},
						},
					},
				})
		})
	})
	t.Run("Error", func(t *testing.T) {
		oldLogger := logger
		defer func() { logger = oldLogger }()
		logger, _ = floggingtest.NewTestLogger(t)

		assert.Panics(t, func() {
			initializeLocalMsp(
				&localconfig.TopLevel{
					General: localconfig.General{
						LocalMSPDir: "",
						LocalMSPID:  "",
					},
				})
		})
	})
}

func TestInitializeMultiChainManager(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	conf := genesisConfig(t)
	assert.NotPanics(t, func() {
		initializeLocalMsp(conf)
		lf, _ := createLedgerFactory(conf, &disabled.Provider{})
		bootBlock := encoder.New(genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)).GenesisBlockForChannel("system")
		initializeMultichannelRegistrar(bootBlock, &replicationInitiator{}, &cluster.PredicateDialer{}, comm.ServerConfig{}, nil, conf, localmsp.NewSigner(), &disabled.Provider{}, &mocks.HealthChecker{}, lf)
	})
}

func TestInitializeGrpcServer(t *testing.T) {
	// get a free random port
	listenAddr := func() string {
		l, _ := net.Listen("tcp", "localhost:0")
		l.Close()
		return l.Addr().String()
	}()
	host := strings.Split(listenAddr, ":")[0]
	port, _ := strconv.ParseUint(strings.Split(listenAddr, ":")[1], 10, 16)
	conf := &localconfig.TopLevel{
		General: localconfig.General{
			ListenAddress: host,
			ListenPort:    uint16(port),
			TLS: localconfig.TLS{
				Enabled:            false,
				ClientAuthRequired: false,
			},
		},
	}
	assert.NotPanics(t, func() {
		grpcServer := initializeGrpcServer(conf, initializeServerConfig(conf, nil))
		grpcServer.Listener().Close()
	})
}

func TestUpdateTrustedRoots(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	initializeLocalMsp(genesisConfig(t))
	// get a free random port
	listenAddr := func() string {
		l, _ := net.Listen("tcp", "localhost:0")
		l.Close()
		return l.Addr().String()
	}()
	port, _ := strconv.ParseUint(strings.Split(listenAddr, ":")[1], 10, 16)
	conf := &localconfig.TopLevel{
		General: localconfig.General{
			ListenAddress: "localhost",
			ListenPort:    uint16(port),
			TLS: localconfig.TLS{
				Enabled:            false,
				ClientAuthRequired: false,
			},
		},
	}
	grpcServer := initializeGrpcServer(conf, initializeServerConfig(conf, nil))
	caSupport := &comm.CredentialSupport{
		AppRootCAsByChain:           make(map[string]comm.CertificateBundle),
		OrdererRootCAsByChainAndOrg: make(comm.OrgRootCAs),
	}
	callback := func(bundle *channelconfig.Bundle) {
		if grpcServer.MutualTLSRequired() {
			t.Log("callback called")
			updateTrustedRoots(caSupport, bundle, grpcServer)
		}
	}
	lf, _ := createLedgerFactory(conf, &disabled.Provider{})
	bootBlock := encoder.New(genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)).GenesisBlockForChannel("system")
	initializeMultichannelRegistrar(bootBlock, &replicationInitiator{}, &cluster.PredicateDialer{}, comm.ServerConfig{}, nil, genesisConfig(t), localmsp.NewSigner(), &disabled.Provider{}, &mocks.HealthChecker{}, lf, callback)
	t.Logf("# app CAs: %d", len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	t.Logf("# orderer CAs: %d", len(caSupport.OrdererRootCAsByChainAndOrg[genesisconfig.TestChainID]["SampleOrg"]))
	// mutual TLS not required so no updates should have occurred
	assert.Equal(t, 0, len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	assert.Equal(t, 0, len(caSupport.OrdererRootCAsByChainAndOrg[genesisconfig.TestChainID]["SampleOrg"]))
	grpcServer.Listener().Close()

	conf = &localconfig.TopLevel{
		General: localconfig.General{
			ListenAddress: "localhost",
			ListenPort:    uint16(port),
			TLS: localconfig.TLS{
				Enabled:            true,
				ClientAuthRequired: true,
				PrivateKey:         filepath.Join(".", "testdata", "tls", "server.key"),
				Certificate:        filepath.Join(".", "testdata", "tls", "server.crt"),
			},
		},
	}
	grpcServer = initializeGrpcServer(conf, initializeServerConfig(conf, nil))
	caSupport = &comm.CredentialSupport{
		AppRootCAsByChain:           make(map[string]comm.CertificateBundle),
		OrdererRootCAsByChainAndOrg: make(comm.OrgRootCAs),
	}

	clusterConf := initializeClusterClientConfig(conf)
	predDialer := &cluster.PredicateDialer{
		ClientConfig: clusterConf,
	}

	callback = func(bundle *channelconfig.Bundle) {
		if grpcServer.MutualTLSRequired() {
			t.Log("callback called")
			updateTrustedRoots(caSupport, bundle, grpcServer)
			updateClusterDialer(caSupport, predDialer, clusterConf.SecOpts.ServerRootCAs)
		}
	}
	initializeMultichannelRegistrar(
		bootBlock,
		&replicationInitiator{},
		predDialer,
		comm.ServerConfig{},
		nil,
		genesisConfig(t),
		localmsp.NewSigner(),
		&disabled.Provider{},
		&server_mocks.HealthChecker{},
		lf,
		callback,
	)
	t.Logf("# app CAs: %d", len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	t.Logf("# orderer CAs: %d", len(caSupport.OrdererRootCAsByChainAndOrg[genesisconfig.TestChainID]["SampleOrg"]))
	// mutual TLS is required so updates should have occurred
	// we expect an intermediate and root CA for apps and orderers
	assert.Equal(t, 2, len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	assert.Equal(t, 2, len(caSupport.OrdererRootCAsByChainAndOrg[genesisconfig.TestChainID]["SampleOrg"]))
	assert.Len(t, predDialer.ClientConfig.SecOpts.ServerRootCAs, 2)
	grpcServer.Listener().Close()
}

func TestConfigureClusterListener(t *testing.T) {
	logEntries := make(chan string, 100)

	allocatePort := func() uint16 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		assert.NoError(t, err)
		_, portStr, err := net.SplitHostPort(l.Addr().String())
		assert.NoError(t, err)
		port, err := strconv.ParseInt(portStr, 10, 64)
		assert.NoError(t, err)
		assert.NoError(t, l.Close())
		t.Log("picked unused port", port)
		return uint16(port)
	}

	unUsedPort := allocatePort()

	backupLogger := logger
	logger = logger.With(zap.Hooks(func(entry zapcore.Entry) error {
		logEntries <- entry.Message
		return nil
	}))

	defer func() {
		logger = backupLogger
	}()

	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)
	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	assert.NoError(t, err)

	loadPEM := func(fileName string) ([]byte, error) {
		switch fileName {
		case "cert":
			return serverKeyPair.Cert, nil
		case "key":
			return serverKeyPair.Key, nil
		case "ca":
			return ca.CertBytes(), nil
		default:
			return nil, errors.New("I/O error")
		}
	}

	for _, testCase := range []struct {
		name               string
		conf               *localconfig.TopLevel
		generalConf        comm.ServerConfig
		generalSrv         *comm.GRPCServer
		shouldBeEqual      bool
		expectedPanic      string
		expectedLogEntries []string
	}{
		{
			name:        "invalid certificate",
			generalConf: comm.ServerConfig{},
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: localconfig.Cluster{
						ListenAddress:     "127.0.0.1",
						ListenPort:        5000,
						ServerPrivateKey:  "key",
						ServerCertificate: "bad",
						RootCAs:           []string{"ca"},
					},
				},
			},
			expectedPanic:      "Failed to load cluster server certificate from 'bad' (I/O error)",
			generalSrv:         &comm.GRPCServer{},
			expectedLogEntries: []string{"Failed to load cluster server certificate from 'bad' (I/O error)"},
		},
		{
			name:        "invalid key",
			generalConf: comm.ServerConfig{},
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: localconfig.Cluster{
						ListenAddress:     "127.0.0.1",
						ListenPort:        5000,
						ServerPrivateKey:  "bad",
						ServerCertificate: "cert",
						RootCAs:           []string{"ca"},
					},
				},
			},
			expectedPanic:      "Failed to load cluster server key from 'bad' (I/O error)",
			generalSrv:         &comm.GRPCServer{},
			expectedLogEntries: []string{"Failed to load cluster server certificate from 'bad' (I/O error)"},
		},
		{
			name:        "invalid ca cert",
			generalConf: comm.ServerConfig{},
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: localconfig.Cluster{
						ListenAddress:     "127.0.0.1",
						ListenPort:        5000,
						ServerPrivateKey:  "key",
						ServerCertificate: "cert",
						RootCAs:           []string{"bad"},
					},
				},
			},
			expectedPanic:      "Failed to load CA cert file 'I/O error' (bad)",
			generalSrv:         &comm.GRPCServer{},
			expectedLogEntries: []string{"Failed to load CA cert file 'I/O error' (bad)"},
		},
		{
			name:        "bad listen address",
			generalConf: comm.ServerConfig{},
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: localconfig.Cluster{
						ListenAddress:     "99.99.99.99",
						ListenPort:        unUsedPort,
						ServerPrivateKey:  "key",
						ServerCertificate: "cert",
						RootCAs:           []string{"ca"},
					},
				},
			},
			expectedPanic: fmt.Sprintf("Failed creating gRPC server on 99.99.99.99:%d due "+
				"to listen tcp 99.99.99.99:%d:", unUsedPort, unUsedPort),
			generalSrv: &comm.GRPCServer{},
		},
		{
			name:        "green path",
			generalConf: comm.ServerConfig{},
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: localconfig.Cluster{
						ListenAddress:     "127.0.0.1",
						ListenPort:        5000,
						ServerPrivateKey:  "key",
						ServerCertificate: "cert",
						RootCAs:           []string{"ca"},
					},
				},
			},
			generalSrv: &comm.GRPCServer{},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.shouldBeEqual {
				conf, srv := configureClusterListener(testCase.conf, testCase.generalConf, loadPEM)
				assert.Equal(t, conf, testCase.generalConf)
				assert.Equal(t, srv, testCase.generalSrv)
			}

			if testCase.expectedPanic != "" {
				f := func() {
					configureClusterListener(testCase.conf, testCase.generalConf, loadPEM)
				}
				assert.Contains(t, panicMsg(f), testCase.expectedPanic)
			} else {
				configureClusterListener(testCase.conf, testCase.generalConf, loadPEM)
			}
			// Ensure logged messages that are expected were all logged
			var loggedMessages []string
			for len(logEntries) > 0 {
				logEntry := <-logEntries
				loggedMessages = append(loggedMessages, logEntry)
			}
			assert.Subset(t, testCase.expectedLogEntries, loggedMessages)
		})
	}
}

func TestReuseListener(t *testing.T) {
	t.Run("good to reuse", func(t *testing.T) {
		top := &localconfig.TopLevel{General: localconfig.General{TLS: localconfig.TLS{Enabled: true}}}
		require.True(t, reuseListener(top, "foo"))
	})

	t.Run("reuse tls disabled", func(t *testing.T) {
		top := &localconfig.TopLevel{}
		require.PanicsWithValue(
			t,
			"TLS is required for running ordering nodes of type foo.",
			func() { reuseListener(top, "foo") },
		)
	})

	t.Run("good not to reuse", func(t *testing.T) {
		top := &localconfig.TopLevel{
			General: localconfig.General{
				Cluster: localconfig.Cluster{
					ListenAddress:     "127.0.0.1",
					ListenPort:        5000,
					ServerPrivateKey:  "key",
					ServerCertificate: "bad",
				},
			},
		}
		require.False(t, reuseListener(top, "foo"))
	})

	t.Run("partial config", func(t *testing.T) {
		top := &localconfig.TopLevel{
			General: localconfig.General{
				Cluster: localconfig.Cluster{
					ListenAddress:     "127.0.0.1",
					ListenPort:        5000,
					ServerCertificate: "bad",
				},
			},
		}
		require.PanicsWithValue(
			t,
			"Options: General.Cluster.ListenPort, General.Cluster.ListenAddress,"+
				" General.Cluster.ServerCertificate, General.Cluster.ServerPrivateKey, should be defined altogether.",
			func() { reuseListener(top, "foo") },
		)
	})
}

func TestInitializeEtcdraftConsenter(t *testing.T) {
	consenters := make(map[string]consensus.Consenter)
	rlf := ramledger.New(10)

	conf := configtxgentest.Load(genesisconfig.SampleInsecureSoloProfile)
	genesisBlock := encoder.New(conf).GenesisBlock()

	ca, _ := tlsgen.NewCA()
	crt, _ := ca.NewServerCertKeyPair("127.0.0.1")

	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{})
	assert.NoError(t, err)

	initializeEtcdraftConsenter(consenters,
		&localconfig.TopLevel{},
		rlf,
		&cluster.PredicateDialer{},
		genesisBlock, &replicationInitiator{},
		comm.ServerConfig{
			SecOpts: &comm.SecureOptions{
				Certificate: crt.Cert,
				Key:         crt.Key,
				UseTLS:      true,
			},
		}, srv, &multichannel.Registrar{}, &disabled.Provider{})
	assert.NotNil(t, consenters["etcdraft"])
}

func genesisConfig(t *testing.T) *localconfig.TopLevel {
	t.Helper()
	localMSPDir, _ := configtest.GetDevMspDir()
	return &localconfig.TopLevel{
		General: localconfig.General{
			LedgerType:     "ram",
			GenesisMethod:  "provisional",
			GenesisProfile: "SampleDevModeSolo",
			SystemChannel:  genesisconfig.TestChainID,
			LocalMSPDir:    localMSPDir,
			LocalMSPID:     "SampleOrg",
			BCCSP: &factory.FactoryOpts{
				ProviderName: "SW",
				SwOpts: &factory.SwOpts{
					HashFamily: "SHA2",
					SecLevel:   256,
					Ephemeral:  true,
				},
			},
		},
	}
}

func panicMsg(f func()) string {
	var message interface{}
	func() {

		defer func() {
			message = recover()
		}()

		f()

	}()

	return message.(string)

}

func TestCreateReplicator(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	bootBlock := encoder.New(genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)).GenesisBlockForChannel("system")

	iterator := &deliver_mocks.BlockIterator{}
	iterator.NextReturnsOnCall(0, bootBlock, common.Status_SUCCESS)
	iterator.NextReturnsOnCall(1, bootBlock, common.Status_SUCCESS)

	ledger := &ledger_mocks.ReadWriter{}
	ledger.On("Height").Return(uint64(1))
	ledger.On("Iterator", mock.Anything).Return(iterator, uint64(1))

	ledgerFactory := &server_mocks.Factory{}
	ledgerFactory.On("GetOrCreate", "mychannel").Return(ledger, nil)
	ledgerFactory.On("ChainIDs").Return([]string{"mychannel"})

	signer := &crypto.LocalSigner{}
	r := createReplicator(ledgerFactory, bootBlock, &localconfig.TopLevel{}, &comm.SecureOptions{}, signer)

	err := r.verifierRetriever.RetrieveVerifier("mychannel").VerifyBlockSignature(nil, nil)
	assert.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 1 of the 'Writers' sub-policies to be satisfied")

	err = r.verifierRetriever.RetrieveVerifier("system").VerifyBlockSignature(nil, nil)
	assert.NoError(t, err)
}
