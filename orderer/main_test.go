/*
Copyright IBM Corp. 2017 All Rights Reserved.

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
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/localmsp"
	coreconfig "github.com/hyperledger/fabric/core/config"
	config "github.com/hyperledger/fabric/orderer/localconfig"
	logging "github.com/op/go-logging"
	// logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestInitializeLoggingLevel(t *testing.T) {
	initializeLoggingLevel(
		&config.TopLevel{
			General: config.General{LogLevel: "debug"},
			Kafka:   config.Kafka{Verbose: true},
		},
	)
	assert.Equal(t, flogging.GetModuleLevel("orderer/main"), "DEBUG")
	assert.NotNil(t, sarama.Logger)
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

func TestInitializeSecureServerConfig(t *testing.T) {
	initializeSecureServerConfig(
		&config.TopLevel{
			General: config.General{
				TLS: config.TLS{
					Enabled:           true,
					ClientAuthEnabled: true,
					Certificate:       "main.go",
					PrivateKey:        "main.go",
					RootCAs:           []string{"main.go"},
					ClientRootCAs:     []string{"main.go"},
				},
			},
		})

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
				initializeSecureServerConfig(
					&config.TopLevel{
						General: config.General{
							TLS: config.TLS{
								Enabled:           true,
								ClientAuthEnabled: true,
								Certificate:       tc.certificate,
								PrivateKey:        tc.privateKey,
								RootCAs:           []string{tc.rootCA},
								ClientRootCAs:     []string{tc.clientCertificate},
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
				},
			}

			if tc.panics {
				assert.Panics(t, func() {
					initializeBootstrapChannel(bootstrapConfig, ledgerFactory)
				})
			} else {
				initializeBootstrapChannel(bootstrapConfig, ledgerFactory)
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
	localMSPDir, _ := coreconfig.GetDevMspDir()
	conf := &config.TopLevel{
		General: config.General{
			LedgerType:     "ram",
			GenesisMethod:  "provisional",
			GenesisProfile: "SampleSingleMSPSolo",
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
	assert.NotPanics(t, func() {
		initializeLocalMsp(conf)
		initializeMultiChainManager(conf, localmsp.NewSigner())
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
	assert.NotPanics(t, func() {
		grpcServer := initializeGrpcServer(
			&config.TopLevel{
				General: config.General{
					ListenAddress: host,
					ListenPort:    uint16(port),
					TLS: config.TLS{
						Enabled:           false,
						ClientAuthEnabled: false,
					},
				},
			})
		grpcServer.Listener().Close()
	})
}

// var originalLogger *Logger

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
