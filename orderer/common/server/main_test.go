// Copyright IBM Corp. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/flogging/floggingtest"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger/fileledger"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/metrics/prometheus"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/filerepo"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/common/multichannel"
	server_mocks "github.com/hyperledger/fabric/orderer/common/server/mocks"
	"github.com/hyperledger/fabric/orderer/consensus/etcdraft"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer

type signerSerializer interface {
	identity.SignerSerializer
}

// The path to cryptogen which can be used by tests to create certificates
var cryptogen string

func TestMain(m *testing.M) {
	var err error

	cryptogen, err = gexec.Build("github.com/hyperledger/fabric/cmd/cryptogen")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryptogen build failed: %v", err)
		os.Exit(-1)
	}
	defer gexec.CleanupBuildArtifacts()

	fmt.Println("TestMain built cryptogen")

	os.Exit(m.Run())
}

func copyYamlFiles(src, dst string) {
	for _, file := range []string{"configtx.yaml", "examplecom-config.yaml", "orderer.yaml"} {
		fileBytes, err := ioutil.ReadFile(filepath.Join(src, file))
		if err != nil {
			os.Exit(-1)
		}
		err = ioutil.WriteFile(filepath.Join(dst, file), fileBytes, 0o644)
		if err != nil {
			os.Exit(-1)
		}
	}
}

func TestInitializeLogging(t *testing.T) {
	origEnvValue := os.Getenv("FABRIC_LOGGING_SPEC")
	os.Setenv("FABRIC_LOGGING_SPEC", "foo=debug")
	initializeLogging()
	require.Equal(t, "debug", flogging.LoggerLevel("foo"))
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
	go initializeProfilingService(
		&localconfig.TopLevel{
			General: localconfig.General{
				Profile: localconfig.Profile{
					Enabled: true,
					Address: listenAddr,
				},
			},
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
	expectedContent, _ := ioutil.ReadFile("main.go")
	require.Equal(t, expectedContent, sc.SecOpts.Certificate)
	require.Equal(t, expectedContent, sc.SecOpts.Key)
	require.Equal(t, [][]byte{expectedContent}, sc.SecOpts.ServerRootCAs)
	require.Equal(t, [][]byte{expectedContent}, sc.SecOpts.ClientRootCAs)

	sc = initializeServerConfig(conf, nil)
	defaultOpts := comm.DefaultKeepaliveOptions
	require.Equal(t, defaultOpts.ServerMinInterval, sc.KaOpts.ServerMinInterval)
	require.Equal(t, time.Duration(0), sc.KaOpts.ServerInterval)
	require.Equal(t, time.Duration(0), sc.KaOpts.ServerTimeout)
	require.Equal(t, 7*time.Second, sc.ConnectionTimeout)
	testDuration := 10 * time.Second
	conf.General.Keepalive = localconfig.Keepalive{
		ServerMinInterval: testDuration,
		ServerInterval:    testDuration,
		ServerTimeout:     testDuration,
	}
	sc = initializeServerConfig(conf, nil)
	require.Equal(t, testDuration, sc.KaOpts.ServerMinInterval)
	require.Equal(t, testDuration, sc.KaOpts.ServerInterval)
	require.Equal(t, testDuration, sc.KaOpts.ServerTimeout)

	sc = initializeServerConfig(conf, nil)
	require.NotNil(t, sc.Logger)
	require.Equal(t, comm.NewServerStatsHandler(&disabled.Provider{}), sc.ServerStatsHandler)
	require.Len(t, sc.UnaryInterceptors, 2)
	require.Len(t, sc.StreamInterceptors, 2)

	sc = initializeServerConfig(conf, &prometheus.Provider{})
	require.NotNil(t, sc.ServerStatsHandler)

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
		isCluster      bool
		expectedPanic  string
	}{
		{
			name:           "BadCertificate",
			certificate:    badFile,
			privateKey:     goodFile,
			rootCA:         goodFile,
			clientRootCert: goodFile,
			expectedPanic:  "Failed to load server Certificate file 'does_not_exist' (open does_not_exist: no such file or directory)",
		},
		{
			name:           "BadPrivateKey",
			certificate:    goodFile,
			privateKey:     badFile,
			rootCA:         goodFile,
			clientRootCert: goodFile,
			expectedPanic:  "Failed to load PrivateKey file 'does_not_exist' (open does_not_exist: no such file or directory)",
		},
		{
			name:           "BadRootCA",
			certificate:    goodFile,
			privateKey:     goodFile,
			rootCA:         badFile,
			clientRootCert: goodFile,
			expectedPanic:  "Failed to load ServerRootCAs file 'open does_not_exist: no such file or directory' (does_not_exist)",
		},
		{
			name:           "BadClientRootCertificate",
			certificate:    goodFile,
			privateKey:     goodFile,
			rootCA:         goodFile,
			clientRootCert: badFile,
			expectedPanic:  "Failed to load ClientRootCAs file 'open does_not_exist: no such file or directory' (does_not_exist)",
		},
		{
			name:           "BadCertificate - cluster reuses server config",
			certificate:    badFile,
			privateKey:     goodFile,
			rootCA:         goodFile,
			clientRootCert: goodFile,
			clusterCert:    "",
			clusterKey:     "",
			clusterCA:      "",
			isCluster:      true,
			expectedPanic:  "Failed to load client TLS certificate file 'does_not_exist' (open does_not_exist: no such file or directory)",
		},
		{
			name:           "BadPrivateKey - cluster reuses server config",
			certificate:    goodFile,
			privateKey:     badFile,
			rootCA:         goodFile,
			clientRootCert: goodFile,
			clusterCert:    "",
			clusterKey:     "",
			clusterCA:      "",
			isCluster:      true,
			expectedPanic:  "Failed to load client TLS key file 'does_not_exist' (open does_not_exist: no such file or directory)",
		},
		{
			name:           "BadRootCA - cluster reuses server config",
			certificate:    goodFile,
			privateKey:     goodFile,
			rootCA:         badFile,
			clientRootCert: goodFile,
			clusterCert:    "",
			clusterKey:     "",
			clusterCA:      "",
			isCluster:      true,
			expectedPanic:  "Failed to load ServerRootCAs file '' (open : no such file or directory)",
		},
		{
			name:           "ClusterBadCertificate",
			certificate:    goodFile,
			privateKey:     goodFile,
			rootCA:         goodFile,
			clientRootCert: goodFile,
			clusterCert:    badFile,
			clusterKey:     goodFile,
			clusterCA:      goodFile,
			isCluster:      true,
			expectedPanic:  "Failed to load client TLS certificate file 'does_not_exist' (open does_not_exist: no such file or directory)",
		},
		{
			name:           "ClusterBadPrivateKey",
			certificate:    goodFile,
			privateKey:     goodFile,
			rootCA:         goodFile,
			clientRootCert: goodFile,
			clusterCert:    goodFile,
			clusterKey:     badFile,
			clusterCA:      goodFile,
			isCluster:      true,
			expectedPanic:  "Failed to load client TLS key file 'does_not_exist' (open does_not_exist: no such file or directory)",
		},
		{
			name:           "ClusterBadRootCA",
			certificate:    goodFile,
			privateKey:     goodFile,
			rootCA:         goodFile,
			clientRootCert: goodFile,
			clusterCert:    goodFile,
			clusterKey:     goodFile,
			clusterCA:      badFile,
			isCluster:      true,
			expectedPanic:  "Failed to load ServerRootCAs file 'does_not_exist' (open does_not_exist: no such file or directory)",
		},
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
			require.PanicsWithValue(t, tc.expectedPanic, func() {
				if !tc.isCluster {
					initializeServerConfig(conf, nil)
				} else {
					initializeClusterClientConfig(conf)
				}
			},
			)
		})
	}
}

func TestVerifyNoSystemChannelJoinBlock(t *testing.T) {
	configPathCleanup := configtest.SetDevFabricConfigPath(t)
	defer configPathCleanup()

	tmpDir := t.TempDir()
	copyYamlFiles("testdata", tmpDir)

	cryptoPath := generateCryptoMaterials(t, cryptogen, tmpDir)
	t.Logf("Generated crypto material to: %s", cryptoPath)
	genesisFile, _ := produceGenesisFileEtcdRaft(t, "testchannelid", tmpDir)

	var (
		config         *localconfig.TopLevel
		cryptoProvider bccsp.BCCSP
		fileRepo       *filerepo.Repo
		genesisBytes   []byte
	)

	setup := func() {
		var err error
		fileLedgerLocation := t.TempDir()

		config = &localconfig.TopLevel{
			General: localconfig.General{
				BootstrapMethod: "none",
			},
			FileLedger: localconfig.FileLedger{
				Location: fileLedgerLocation,
			},
			ChannelParticipation: localconfig.ChannelParticipation{Enabled: true},
		}

		cryptoProvider, err = sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)

		fileRepo, err = multichannel.InitJoinBlockFileRepo(config)
		require.NoError(t, err)
		require.NotNil(t, fileRepo)

		genesisBytes, err = ioutil.ReadFile(genesisFile)
		require.NoError(t, err)
		require.NotNil(t, genesisBytes)
	}

	t.Run("No join-block", func(t *testing.T) {
		setup()

		verifyNoSystemChannelJoinBlock(config, cryptoProvider)
	})

	t.Run("With genesis join-block", func(t *testing.T) {
		setup()

		err := fileRepo.Save("testchannelid", genesisBytes)
		require.NoError(t, err)
		require.Panics(t, func() { verifyNoSystemChannelJoinBlock(config, cryptoProvider) })
	})

	t.Run("With non-genesis join-block", func(t *testing.T) {
		setup()

		block := protoutil.UnmarshalBlockOrPanic(genesisBytes)
		block.Header.Number = 7
		configBlockBytes := protoutil.MarshalOrPanic(block)
		err := fileRepo.Save("testchannelid", configBlockBytes)
		require.NoError(t, err)
		require.Panics(t, func() { verifyNoSystemChannelJoinBlock(config, cryptoProvider) })
	})
}

func TestVerifyNoSystemChannel(t *testing.T) {
	cryptoProvider, _ := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())

	tmpdir := t.TempDir()

	rlf, err := fileledger.New(tmpdir, &disabled.Provider{})
	require.NoError(t, err)

	// no ledgers
	verifyNoSystemChannel(rlf, cryptoProvider)

	// skipping empty ledgers
	_, err = rlf.GetOrCreate("emptychannelid")
	require.NoError(t, err)
	verifyNoSystemChannel(rlf, cryptoProvider)

	// skipping app channel
	conf := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	conf.Consortiums = nil
	configBlock := encoder.New(conf).GenesisBlock()
	rl, err := rlf.GetOrCreate("appchannelid")
	require.NoError(t, err)
	err = rl.Append(configBlock)
	require.NoError(t, err)
	verifyNoSystemChannel(rlf, cryptoProvider)

	// detecting system channel genesis and panicking
	conf = genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	configBlock = encoder.New(conf).GenesisBlock()
	rl, err = rlf.GetOrCreate("testchannelid")
	require.NoError(t, err)
	err = rl.Append(configBlock)
	require.NoError(t, err)
	require.Panics(t, func() { verifyNoSystemChannel(rlf, cryptoProvider) })

	// Make and append the next config block, detecting system channel and panicking
	prevHash := protoutil.BlockHeaderHash(configBlock.Header)
	configBlock.Header.Number = 1
	configBlock.Header.PreviousHash = prevHash
	configBlock.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
		Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
			LastConfig: &common.LastConfig{Index: rl.Height()},
		}),
	})
	configBlock.Metadata.Metadata[common.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&common.Metadata{
		Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: rl.Height()}),
	})
	err = rl.Append(configBlock)
	require.NoError(t, err)
	require.Panics(t, func() { verifyNoSystemChannel(rlf, cryptoProvider) })
}

func TestLoadLocalMSP(t *testing.T) {
	t.Run("Happy", func(t *testing.T) {
		localMSPDir := configtest.GetDevMspDir()
		localMSP := loadLocalMSP(
			&localconfig.TopLevel{
				General: localconfig.General{
					LocalMSPDir: localMSPDir,
					LocalMSPID:  "SampleOrg",
					BCCSP: &factory.FactoryOpts{
						Default: "SW",
						SW: &factory.SwOpts{
							Hash:     "SHA2",
							Security: 256,
						},
					},
				},
			},
		)
		require.NotNil(t, localMSP)
		id, err := localMSP.GetIdentifier()
		require.NoError(t, err)
		require.Equal(t, id, "SampleOrg")
	})

	t.Run("Error", func(t *testing.T) {
		oldLogger := logger
		defer func() { logger = oldLogger }()
		logger, _ = floggingtest.NewTestLogger(t)

		require.Panics(t, func() {
			loadLocalMSP(
				&localconfig.TopLevel{
					General: localconfig.General{
						LocalMSPDir: "",
						LocalMSPID:  "",
					},
				},
			)
		})
	})
}

func TestInitializeMultichannelRegistrar(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	tmpDir := t.TempDir()
	copyYamlFiles("testdata", tmpDir)

	cryptoPath := generateCryptoMaterials(t, cryptogen, tmpDir)
	t.Logf("Generated crypto material to: %s", cryptoPath)
	genesisFile, _ := produceGenesisFileEtcdRaftAppChannel(t, "testchannelid", tmpDir)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	signer := &server_mocks.SignerSerializer{}

	t.Run("registrar without a system channel", func(t *testing.T) {
		conf := genesisConfig(t, genesisFile)
		srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{})
		require.NoError(t, err)
		lf, err := createLedgerFactory(conf, &disabled.Provider{})
		require.NoError(t, err)
		registrar := initializeMultichannelRegistrar(&cluster.PredicateDialer{}, comm.ServerConfig{}, srv, conf, signer, &disabled.Provider{}, lf, cryptoProvider)
		require.NotNil(t, registrar)
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
	require.NotPanics(t, func() {
		grpcServer := initializeGrpcServer(conf, initializeServerConfig(conf, nil))
		grpcServer.Listener().Close()
	})
}

// generateCryptoMaterials uses cryptogen to generate the necessary
// MSP files and TLS certificates
func generateCryptoMaterials(t *testing.T, cryptogen, tmpDir string) string {
	gt := NewGomegaWithT(t)
	cryptoPath := filepath.Join(tmpDir, "crypto")

	cmd := exec.Command(
		cryptogen,
		"generate",
		"--config", filepath.Join(tmpDir, "examplecom-config.yaml"),
		"--output", cryptoPath,
	)
	cryptogenProcess, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(cryptogenProcess, time.Minute).Should(gexec.Exit(0))

	return cryptoPath
}

func TestUpdateTrustedRoots(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	tmpDir := t.TempDir()
	copyYamlFiles("testdata", tmpDir)

	cryptoPath := generateCryptoMaterials(t, cryptogen, tmpDir)
	t.Logf("Generated crypto material to: %s", cryptoPath)

	// get a free random port
	listenAddr := func() string {
		l, _ := net.Listen("tcp", "localhost:0")
		l.Close()
		return l.Addr().String()
	}()
	port, _ := strconv.ParseUint(strings.Split(listenAddr, ":")[1], 10, 16)
	ledgerDir := path.Join(tmpDir, "ledger-dir")
	conf := &localconfig.TopLevel{
		General: localconfig.General{
			BootstrapMethod: "none",
			ListenAddress:   "localhost",
			ListenPort:      uint16(port),
			TLS: localconfig.TLS{
				Enabled:            false,
				ClientAuthRequired: false,
			},
		},
		FileLedger: localconfig.FileLedger{
			Location: ledgerDir,
		},
		Consensus: etcdraft.Config{
			WALDir:  path.Join(tmpDir, "etcdraft", "wal"),
			SnapDir: path.Join(tmpDir, "etcdraft", "snap"),
		},
	}
	grpcServer := initializeGrpcServer(conf, initializeServerConfig(conf, nil))

	caMgr := &caManager{
		appRootCAsByChain:     make(map[string][][]byte),
		ordererRootCAsByChain: make(map[string][][]byte),
	}
	callback := func(bundle *channelconfig.Bundle) {
		if grpcServer.MutualTLSRequired() {
			t.Log("callback called")
			caMgr.updateTrustedRoots(bundle, grpcServer)
		}
	}
	lf, err := createLedgerFactory(conf, &disabled.Provider{})
	require.NoError(t, err)

	genesisFile, serverCert := produceGenesisFileEtcdRaftAppChannel(t, "testchannelid", tmpDir)
	blockBytes, err := os.ReadFile(genesisFile)
	require.NoError(t, err)
	genesisBlock := protoutil.UnmarshalBlockOrPanic(blockBytes)
	initializeAppChannel(genesisBlock, lf)
	signer := &server_mocks.SignerSerializer{}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	srvConf := comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			Certificate: serverCert,
		},
	}

	r := initializeMultichannelRegistrar(&cluster.PredicateDialer{}, srvConf, grpcServer, conf, signer, &disabled.Provider{}, lf, cryptoProvider, callback)

	t.Logf("# app CAs: %d", len(caMgr.appRootCAsByChain["testchannelid"]))
	t.Logf("# orderer CAs: %d", len(caMgr.ordererRootCAsByChain["testchannelid"]))
	// mutual TLS not required so no updates should have occurred
	require.Equal(t, 0, len(caMgr.appRootCAsByChain["testchannelid"]))
	require.Equal(t, 0, len(caMgr.ordererRootCAsByChain["testchannelid"]))

	grpcServer.Listener().Close()
	cs := r.GetChain("testchannelid")
	cs.Halt()

	conf = &localconfig.TopLevel{
		General: localconfig.General{
			BootstrapMethod: "none",
			ListenAddress:   "localhost",
			ListenPort:      uint16(port),
			TLS: localconfig.TLS{
				Enabled:            true,
				ClientAuthRequired: true,
				PrivateKey:         filepath.Join(cryptoPath, "ordererOrganizations", "example.com", "orderers", "127.0.0.1.example.com", "tls", "server.key"),
				Certificate:        filepath.Join(cryptoPath, "ordererOrganizations", "example.com", "orderers", "127.0.0.1.example.com", "tls", "server.crt"),
			},
		},
		FileLedger: localconfig.FileLedger{
			Location: ledgerDir,
		},
		Consensus: etcdraft.Config{
			WALDir:  path.Join(tmpDir, "etcdraft", "wal"),
			SnapDir: path.Join(tmpDir, "etcdraft", "snap"),
		},
	}
	grpcServer = initializeGrpcServer(conf, initializeServerConfig(conf, nil))

	caMgr = &caManager{
		appRootCAsByChain:     make(map[string][][]byte),
		ordererRootCAsByChain: make(map[string][][]byte),
	}

	clusterConf, _ := initializeClusterClientConfig(conf)
	predDialer := &cluster.PredicateDialer{
		Config: clusterConf,
	}

	callback = func(bundle *channelconfig.Bundle) {
		if grpcServer.MutualTLSRequired() {
			t.Log("callback called")
			caMgr.updateTrustedRoots(bundle, grpcServer)
			caMgr.updateClusterDialer(predDialer, clusterConf.SecOpts.ServerRootCAs)
		}
	}

	r = initializeMultichannelRegistrar(predDialer, srvConf, grpcServer, conf, signer, &disabled.Provider{}, lf, cryptoProvider, callback)
	t.Logf("# app CAs: %d", len(caMgr.appRootCAsByChain["testchannelid"]))
	t.Logf("# orderer CAs: %d", len(caMgr.ordererRootCAsByChain["testchannelid"]))
	// mutual TLS is required so updates should have occurred
	// we do not expect an intermediate CA, only root CA for apps and orderers
	require.Equal(t, 1, len(caMgr.appRootCAsByChain["testchannelid"]))
	require.Equal(t, 1, len(caMgr.ordererRootCAsByChain["testchannelid"]))
	require.Len(t, predDialer.Config.SecOpts.ServerRootCAs, 1)
	grpcServer.Listener().Close()
	cs = r.GetChain("testchannelid")
	cs.Halt()
}

func TestRootServerCertAggregation(t *testing.T) {
	caMgr := &caManager{
		appRootCAsByChain:     make(map[string][][]byte),
		ordererRootCAsByChain: make(map[string][][]byte),
	}

	predDialer := &cluster.PredicateDialer{
		Config: comm.ClientConfig{},
	}

	ca1, err := tlsgen.NewCA()
	require.NoError(t, err)

	ca2, err := tlsgen.NewCA()
	require.NoError(t, err)

	caMgr.ordererRootCAsByChain["foo"] = [][]byte{ca1.CertBytes()}
	caMgr.ordererRootCAsByChain["bar"] = [][]byte{ca1.CertBytes()}

	caMgr.updateClusterDialer(predDialer, [][]byte{ca2.CertBytes(), ca2.CertBytes(), ca2.CertBytes()})

	require.Len(t, predDialer.Config.SecOpts.ServerRootCAs, 2)
	require.Contains(t, predDialer.Config.SecOpts.ServerRootCAs, ca1.CertBytes())
	require.Contains(t, predDialer.Config.SecOpts.ServerRootCAs, ca2.CertBytes())
}

func TestConfigureClusterListener(t *testing.T) {
	logEntries := make(chan string, 100)

	allocatePort := func() uint16 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		_, portStr, err := net.SplitHostPort(l.Addr().String())
		require.NoError(t, err)
		port, err := strconv.ParseInt(portStr, 10, 64)
		require.NoError(t, err)
		require.NoError(t, l.Close())
		t.Log("picked unused port", port)
		return uint16(port)
	}

	unUsedPort := allocatePort()

	backupLogger := logger
	logger = logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		logEntries <- entry.Message
		return nil
	}))

	defer func() {
		logger = backupLogger
	}()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

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
			expectedLogEntries: []string{"Failed to load cluster server key from 'bad' (I/O error)"},
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
			expectedPanic:      "Failed to load CA cert file 'bad' (I/O error)",
			generalSrv:         &comm.GRPCServer{},
			expectedLogEntries: []string{"Failed to load CA cert file 'bad' (I/O error)"},
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
				require.Equal(t, conf, testCase.generalConf)
				require.Equal(t, srv, testCase.generalSrv)
			}

			if testCase.expectedPanic != "" {
				f := func() {
					configureClusterListener(testCase.conf, testCase.generalConf, loadPEM)
				}
				require.Contains(t, panicMsg(f), testCase.expectedPanic)
			} else {
				configureClusterListener(testCase.conf, testCase.generalConf, loadPEM)
			}
			// Ensure logged messages that are expected were all logged
			var loggedMessages []string
			for len(logEntries) > 0 {
				logEntry := <-logEntries
				loggedMessages = append(loggedMessages, logEntry)
			}
			require.Subset(t, loggedMessages, testCase.expectedLogEntries)
		})
	}
}

func TestReuseListener(t *testing.T) {
	t.Run("good to reuse", func(t *testing.T) {
		top := &localconfig.TopLevel{General: localconfig.General{TLS: localconfig.TLS{Enabled: true}}}
		require.True(t, reuseListener(top))
	})

	t.Run("reuse tls disabled", func(t *testing.T) {
		top := &localconfig.TopLevel{}
		require.PanicsWithValue(
			t,
			"TLS is required for running ordering nodes of cluster type.",
			func() { reuseListener(top) },
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
		require.False(t, reuseListener(top))
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
			func() { reuseListener(top) },
		)
	})
}

func genesisConfig(t *testing.T, genesisFile string) *localconfig.TopLevel {
	t.Helper()
	localMSPDir := configtest.GetDevMspDir()
	ledgerDir := t.TempDir()

	return &localconfig.TopLevel{
		General: localconfig.General{
			BootstrapMethod: "none",
			LocalMSPDir:     localMSPDir,
			LocalMSPID:      "SampleOrg",
			BCCSP: &factory.FactoryOpts{
				Default: "SW",
				SW: &factory.SwOpts{
					Hash:     "SHA2",
					Security: 256,
				},
			},
		},
		FileLedger: localconfig.FileLedger{
			Location: ledgerDir,
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

// produces a system channel genesis file to make sure the server detects it and refuses to start
func produceGenesisFileEtcdRaft(t *testing.T, channelID string, tmpDir string) (string, []byte) {
	confRaft := genesisconfig.Load("SampleEtcdRaftSystemChannel", tmpDir)

	serverCert, err := ioutil.ReadFile(string(confRaft.Orderer.EtcdRaft.Consenters[0].ServerTlsCert))
	require.NoError(t, err)

	bootstrapper, err := encoder.NewBootstrapper(confRaft)
	require.NoError(t, err, "cannot create bootstrapper")

	joinBlockAppRaft := bootstrapper.GenesisBlockForChannel(channelID)
	f := path.Join(tmpDir, channelID+".block")
	err = os.WriteFile(f, protoutil.MarshalOrPanic(joinBlockAppRaft), 0o644)
	require.NoError(t, err)
	return f, serverCert
}

func produceGenesisFileEtcdRaftAppChannel(t *testing.T, channelID string, tmpDir string) (string, []byte) {
	confRaft := genesisconfig.Load("SampleOrgChannel", tmpDir)

	serverCert, err := ioutil.ReadFile(string(confRaft.Orderer.EtcdRaft.Consenters[0].ServerTlsCert))
	require.NoError(t, err)

	bootstrapper, err := encoder.NewBootstrapper(confRaft)
	require.NoError(t, err, "cannot create bootstrapper")

	joinBlockAppRaft := bootstrapper.GenesisBlockForChannel(channelID)

	f := path.Join(tmpDir, channelID+".block")
	err = os.WriteFile(f, protoutil.MarshalOrPanic(joinBlockAppRaft), 0o644)
	require.NoError(t, err)
	return f, serverCert
}

func initializeAppChannel(genesisBlock *common.Block, lf blockledger.Factory) {
	channelID, err := protoutil.GetChannelIDFromBlock(genesisBlock)
	if err != nil {
		logger.Fatal("Failed to parse channel ID from genesis block:", err)
	}
	gl, err := lf.GetOrCreate(channelID)
	if err != nil {
		logger.Fatal("Failed to create the system channel:", err)
	}
	if gl.Height() == 0 {
		if err := gl.Append(genesisBlock); err != nil {
			logger.Fatal("Could not write genesis block to ledger:", err)
		}
	}
	logger.Infof("Initialized the channel '%s' from genesis block", channelID)
}
