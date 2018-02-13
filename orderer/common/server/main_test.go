/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"io/ioutil"
	"log"
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
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/localmsp"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
	"github.com/hyperledger/fabric/core/comm"
	coreconfig "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func init() {
	flogging.SetModuleLevel(pkgLogID, "DEBUG")
}

func TestInitializeLoggingLevel(t *testing.T) {
	initializeLoggingLevel(
		&config.TopLevel{
			// We specify the package name here, in contrast to what's expected
			// in production usage. We do this so as to prevent the unwanted
			// global log level setting in tests of this package (for example,
			// the benchmark-related ones) that would occur otherwise.
			General: config.General{LogLevel: "foo=debug"},
		},
	)
	assert.Equal(t, flogging.GetModuleLevel("foo"), "DEBUG")
}

func TestInitializeProfilingService(t *testing.T) {
	// get a free random port
	listenAddr := func() string {
		l, _ := net.Listen("tcp", "localhost:0")
		l.Close()
		return l.Addr().String()
	}()
	initializeProfilingService(
		&config.TopLevel{
			General: config.General{
				LogLevel: "debug",
				Profile: config.Profile{
					Enabled: true,
					Address: listenAddr,
				}},
			Kafka: config.Kafka{Verbose: true},
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
	conf := &config.TopLevel{
		General: config.General{
			TLS: config.TLS{
				Enabled:            true,
				ClientAuthRequired: true,
				Certificate:        "main.go",
				PrivateKey:         "main.go",
				RootCAs:            []string{"main.go"},
				ClientRootCAs:      []string{"main.go"},
			},
		},
	}
	sc := initializeServerConfig(conf)
	defaultOpts := comm.DefaultKeepaliveOptions()
	assert.Equal(t, defaultOpts.ServerMinInterval, sc.KaOpts.ServerMinInterval)
	assert.Equal(t, time.Duration(0), sc.KaOpts.ServerInterval)
	assert.Equal(t, time.Duration(0), sc.KaOpts.ServerTimeout)
	testDuration := 10 * time.Second
	conf.General.Keepalive = config.Keepalive{
		ServerMinInterval: testDuration,
		ServerInterval:    testDuration,
		ServerTimeout:     testDuration,
	}
	sc = initializeServerConfig(conf)
	assert.Equal(t, testDuration, sc.KaOpts.ServerMinInterval)
	assert.Equal(t, testDuration, sc.KaOpts.ServerInterval)
	assert.Equal(t, testDuration, sc.KaOpts.ServerTimeout)

	goodFile := "main.go"
	badFile := "does_not_exist"

	logger.SetBackend(logging.AddModuleLevel(newPanicOnCriticalBackend()))
	defer func() {
		logger = logging.MustGetLogger("orderer/main")
	}()

	testCases := []struct {
		name              string
		certificate       string
		privateKey        string
		rootCA            string
		clientCertificate string
	}{
		{"BadCertificate", badFile, goodFile, goodFile, goodFile},
		{"BadPrivateKey", goodFile, badFile, goodFile, goodFile},
		{"BadRootCA", goodFile, goodFile, badFile, goodFile},
		{"BadClientCertificate", goodFile, goodFile, goodFile, badFile},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Panics(t, func() {
				initializeServerConfig(
					&config.TopLevel{
						General: config.General{
							TLS: config.TLS{
								Enabled:            true,
								ClientAuthRequired: true,
								Certificate:        tc.certificate,
								PrivateKey:         tc.privateKey,
								RootCAs:            []string{tc.rootCA},
								ClientRootCAs:      []string{tc.clientCertificate},
							},
						},
					})
			},
			)
		})
	}
}

func TestInitializeBootstrapChannel(t *testing.T) {
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
				&config.TopLevel{
					General: config.General{LedgerType: tc.ledgerType},
					FileLedger: config.FileLedger{
						Location: fileLedgerLocation,
					},
				},
			)

			bootstrapConfig := &config.TopLevel{
				General: config.General{
					GenesisMethod:  tc.genesisMethod,
					GenesisProfile: "SampleSingleMSPSolo",
					GenesisFile:    "genesisblock",
					SystemChannel:  genesisconfig.TestChainID,
				},
			}

			if tc.panics {
				assert.Panics(t, func() {
					initializeBootstrapChannel(bootstrapConfig, ledgerFactory)
				})
			} else {
				assert.NotPanics(t, func() {
					initializeBootstrapChannel(bootstrapConfig, ledgerFactory)
				})
			}
		})
	}
}

func TestInitializeLocalMsp(t *testing.T) {
	t.Run("Happy", func(t *testing.T) {
		assert.NotPanics(t, func() {
			localMSPDir, _ := coreconfig.GetDevMspDir()
			initializeLocalMsp(
				&config.TopLevel{
					General: config.General{
						LocalMSPDir: localMSPDir,
						LocalMSPID:  "DEFAULT",
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
		logger.SetBackend(logging.AddModuleLevel(newPanicOnCriticalBackend()))
		defer func() {
			logger = logging.MustGetLogger("orderer/main")
		}()
		assert.Panics(t, func() {
			initializeLocalMsp(
				&config.TopLevel{
					General: config.General{
						LocalMSPDir: "",
						LocalMSPID:  "",
					},
				})
		})
	})
}

func TestInitializeMultiChainManager(t *testing.T) {
	conf := genesisConfig(t)
	assert.NotPanics(t, func() {
		initializeLocalMsp(conf)
		initializeMultichannelRegistrar(conf, localmsp.NewSigner())
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
	conf := &config.TopLevel{
		General: config.General{
			ListenAddress: host,
			ListenPort:    uint16(port),
			TLS: config.TLS{
				Enabled:            false,
				ClientAuthRequired: false,
			},
		},
	}
	assert.NotPanics(t, func() {
		grpcServer := initializeGrpcServer(conf, initializeServerConfig(conf))
		grpcServer.Listener().Close()
	})
}

func TestUpdateTrustedRoots(t *testing.T) {
	initializeLocalMsp(genesisConfig(t))
	// get a free random port
	listenAddr := func() string {
		l, _ := net.Listen("tcp", "localhost:0")
		l.Close()
		return l.Addr().String()
	}()
	port, _ := strconv.ParseUint(strings.Split(listenAddr, ":")[1], 10, 16)
	conf := &config.TopLevel{
		General: config.General{
			ListenAddress: "localhost",
			ListenPort:    uint16(port),
			TLS: config.TLS{
				Enabled:            false,
				ClientAuthRequired: false,
			},
		},
	}
	grpcServer := initializeGrpcServer(conf, initializeServerConfig(conf))
	caSupport := &comm.CASupport{
		AppRootCAsByChain:     make(map[string][][]byte),
		OrdererRootCAsByChain: make(map[string][][]byte),
	}
	callback := func(bundle *channelconfig.Bundle) {
		if grpcServer.MutualTLSRequired() {
			t.Log("callback called")
			updateTrustedRoots(grpcServer, caSupport, bundle)
		}
	}
	initializeMultichannelRegistrar(genesisConfig(t), localmsp.NewSigner(), callback)
	t.Logf("# app CAs: %d", len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	t.Logf("# orderer CAs: %d", len(caSupport.OrdererRootCAsByChain[genesisconfig.TestChainID]))
	// mutual TLS not required so no updates should have occurred
	assert.Equal(t, 0, len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	assert.Equal(t, 0, len(caSupport.OrdererRootCAsByChain[genesisconfig.TestChainID]))
	grpcServer.Listener().Close()

	conf = &config.TopLevel{
		General: config.General{
			ListenAddress: "localhost",
			ListenPort:    uint16(port),
			TLS: config.TLS{
				Enabled:            true,
				ClientAuthRequired: true,
				PrivateKey:         filepath.Join(".", "testdata", "tls", "server.key"),
				Certificate:        filepath.Join(".", "testdata", "tls", "server.crt"),
			},
		},
	}
	grpcServer = initializeGrpcServer(conf, initializeServerConfig(conf))
	caSupport = &comm.CASupport{
		AppRootCAsByChain:     make(map[string][][]byte),
		OrdererRootCAsByChain: make(map[string][][]byte),
	}
	callback = func(bundle *channelconfig.Bundle) {
		if grpcServer.MutualTLSRequired() {
			t.Log("callback called")
			updateTrustedRoots(grpcServer, caSupport, bundle)
		}
	}
	initializeMultichannelRegistrar(genesisConfig(t), localmsp.NewSigner(), callback)
	t.Logf("# app CAs: %d", len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	t.Logf("# orderer CAs: %d", len(caSupport.OrdererRootCAsByChain[genesisconfig.TestChainID]))
	// mutual TLS is required so updates should have occurred
	// we expect an intermediate and root CA for apps and orderers
	assert.Equal(t, 2, len(caSupport.AppRootCAsByChain[genesisconfig.TestChainID]))
	assert.Equal(t, 2, len(caSupport.OrdererRootCAsByChain[genesisconfig.TestChainID]))
	grpcServer.Listener().Close()
}

func genesisConfig(t *testing.T) *config.TopLevel {
	t.Helper()
	localMSPDir, _ := coreconfig.GetDevMspDir()
	return &config.TopLevel{
		General: config.General{
			LedgerType:     "ram",
			GenesisMethod:  "provisional",
			GenesisProfile: "SampleDevModeSolo",
			SystemChannel:  genesisconfig.TestChainID,
			LocalMSPDir:    localMSPDir,
			LocalMSPID:     "DEFAULT",
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

func newPanicOnCriticalBackend() *panicOnCriticalBackend {
	return &panicOnCriticalBackend{
		backend: logging.AddModuleLevel(logging.NewLogBackend(os.Stderr, "", log.LstdFlags)),
	}
}

type panicOnCriticalBackend struct {
	backend logging.Backend
}

func (b *panicOnCriticalBackend) Log(level logging.Level, calldepth int, record *logging.Record) error {
	err := b.backend.Log(level, calldepth, record)
	if level == logging.CRITICAL {
		panic(record.Formatted(calldepth))
	}
	return err
}
