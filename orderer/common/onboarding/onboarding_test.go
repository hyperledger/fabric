/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package onboarding

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	deliver_mocks "github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/internal/pkg/identity"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	cluster_mocks "github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	onboarding_mocks "github.com/hyperledger/fabric/orderer/common/onboarding/mocks"
	"github.com/hyperledger/fabric/protoutil"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// the paths to configtxgen and cryptogen, which can be used by tests to create
// genesis blocks and certificates
var configtxgen, cryptogen, tempDir string

func TestMain(m *testing.M) {
	var err error
	configtxgen, err = gexec.Build("github.com/hyperledger/fabric/cmd/configtxgen")
	if err != nil {
		fmt.Fprintf(os.Stderr, "configtxgen build failed: %v", err)
		os.Exit(-1)
	}
	cryptogen, err = gexec.Build("github.com/hyperledger/fabric/cmd/cryptogen")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryptogen build failed: %v", err)
		os.Exit(-1)
	}
	defer gexec.CleanupBuildArtifacts()

	tempDir, err = ioutil.TempDir("", "onboarding-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temporary directory: %v", err)
		os.Exit(-1)
	}
	defer os.RemoveAll(tempDir)

	copyYamlFiles("testdata", tempDir)

	os.Exit(m.Run())
}

//go:generate counterfeiter -o mocks/signer_serializer.go --fake-name SignerSerializer . signerSerializer
type signerSerializer interface {
	identity.SignerSerializer
}

//go:generate counterfeiter -o mocks/read_writer.go --fake-name ReadWriter . readWriter
type readWriter interface {
	blockledger.ReadWriter
}

//go:generate counterfeiter -o mocks/factory.go --fake-name Factory . factory
type factory interface {
	blockledger.Factory
}

func copyYamlFiles(src, dst string) {
	for _, file := range []string{"configtx.yaml", "examplecom-config.yaml"} {
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

func newServerNode(t *testing.T, key, cert []byte) *deliverServer {
	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			Key:         key,
			Certificate: cert,
			UseTLS:      true,
		},
	})
	if err != nil {
		panic(err)
	}
	ds := &deliverServer{
		t:              t,
		blockResponses: make(chan *orderer.DeliverResponse, 100),
		srv:            srv,
	}
	orderer.RegisterAtomicBroadcastServer(srv.Server(), ds)
	go srv.Start()
	return ds
}

type deliverServer struct {
	isConnected    int32
	t              *testing.T
	srv            *comm.GRPCServer
	blockResponses chan *orderer.DeliverResponse
}

func (*deliverServer) Broadcast(orderer.AtomicBroadcast_BroadcastServer) error {
	panic("implement me")
}

func (ds *deliverServer) Deliver(stream orderer.AtomicBroadcast_DeliverServer) error {
	atomic.StoreInt32(&ds.isConnected, 1)
	seekInfo, err := readSeekEnvelope(stream)
	if err != nil {
		panic(err)
	}
	if seekInfo.GetStart().GetSpecified() != nil {
		return ds.deliverBlocks(stream)
	}
	if seekInfo.GetStart().GetNewest() != nil {
		resp := <-ds.blockResponses
		if resp == nil {
			return nil
		}
		return stream.Send(resp)
	}
	panic(fmt.Sprintf("expected either specified or newest seek but got %v", seekInfo.GetStart()))
}

func readSeekEnvelope(stream orderer.AtomicBroadcast_DeliverServer) (*orderer.SeekInfo, error) {
	env, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, err
	}
	seekInfo := &orderer.SeekInfo{}
	if err = proto.Unmarshal(payload.Data, seekInfo); err != nil {
		return nil, err
	}
	return seekInfo, nil
}

func (ds *deliverServer) deliverBlocks(stream orderer.AtomicBroadcast_DeliverServer) error {
	for {
		blockChan := ds.blockResponses
		response := <-blockChan
		if response == nil {
			return nil
		}
		if err := stream.Send(response); err != nil {
			return err
		}
	}
}

func loadPEM(cryptoPath, suffix string, t *testing.T) []byte {
	ordererTLSPath := filepath.Join(cryptoPath, "ordererOrganizations", "example.com", "orderers", "127.0.0.1.example.com", "tls")
	b, err := ioutil.ReadFile(filepath.Join(ordererTLSPath, suffix))
	require.NoError(t, err)
	return b
}

func channelCreationBlock(systemChannel, applicationChannel string, prevBlock *common.Block) *common.Block {
	block := &common.Block{
		Header: &common.BlockHeader{
			Number:       prevBlock.Header.Number + 1,
			PreviousHash: protoutil.BlockHeaderHash(prevBlock.Header),
		},
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{{}, {}, {}, {}},
		},
		Data: &common.BlockData{
			Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
				Payload: protoutil.MarshalOrPanic(&common.Payload{
					Header: &common.Header{
						ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
							ChannelId: systemChannel,
							Type:      int32(common.HeaderType_ORDERER_TRANSACTION),
						}),
					},
					Data: protoutil.MarshalOrPanic(&common.Envelope{
						Payload: protoutil.MarshalOrPanic(&common.Payload{
							Header: &common.Header{
								ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
									Type:      int32(common.HeaderType_CONFIG),
									ChannelId: applicationChannel,
								}),
							},
						}),
					}),
				}),
			})},
		},
	}

	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
	return block
}

func TestOnboardingChannelUnavailable(t *testing.T) {
	// Scenario: During the probing phase of the onboarding,
	// a channel is deemed relevant and we try to pull it during the
	// second phase, but alas - precisely at that time - it becomes
	// unavailable.
	// The onboarding code is expected to skip the replication after
	// the maximum attempt number is exhausted, and not to try to replicate
	// the channel indefinitely.
	cryptoPath := generateCryptoMaterials(t, cryptogen)
	defer os.RemoveAll(cryptoPath)

	caCert := loadPEM(cryptoPath, "ca.crt", t)
	key := loadPEM(cryptoPath, "server.key", t)
	cert := loadPEM(cryptoPath, "server.crt", t)

	deliverServer := newServerNode(t, key, cert)
	defer deliverServer.srv.Stop()

	systemChannelBlockPath := generateBootstrapBlock(t, tempDir, configtxgen, "system", "SampleSoloSystemChannel")
	systemChannelBlockBytes, err := ioutil.ReadFile(systemChannelBlockPath)
	require.NoError(t, err)

	applicationChannelBlockPath := generateBootstrapBlock(t, tempDir, configtxgen, "testchannel", "SampleOrgChannel")
	applicationChannelBlockBytes, err := ioutil.ReadFile(applicationChannelBlockPath)
	require.NoError(t, err)

	testchannelGB := &common.Block{}
	require.NoError(t, proto.Unmarshal(applicationChannelBlockBytes, testchannelGB))
	testchannelGB.Header.Number = 0

	systemChannelGenesisBlock := &common.Block{
		Header: &common.BlockHeader{
			Number: 0,
		},
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{{}, {}, {}},
		},
		Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{},
			}),
		})}},
	}
	systemChannelGenesisBlock.Header.DataHash = protoutil.BlockDataHash(systemChannelGenesisBlock.Data)

	channelCreationBlock := channelCreationBlock("system", "testchannel", systemChannelGenesisBlock)

	bootBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(systemChannelBlockBytes, bootBlock))
	bootBlock.Header.Number = 2
	bootBlock.Header.PreviousHash = protoutil.BlockHeaderHash(channelCreationBlock.Header)
	injectOrdererEndpoint(t, bootBlock, deliverServer.srv.Address())
	injectConsenterCertificate(t, testchannelGB, cert)

	blocksCommittedToSystemLedger := make(chan uint64, 3)
	blocksCommittedToApplicationLedger := make(chan uint64, 1)

	systemLedger := &cluster_mocks.LedgerWriter{}
	systemLedger.On("Height").Return(uint64(0))
	systemLedger.On("Append", mock.Anything).Return(nil).Run(func(arguments mock.Arguments) {
		seq := arguments.Get(0).(*common.Block).Header.Number
		blocksCommittedToSystemLedger <- seq
	})

	appLedger := &cluster_mocks.LedgerWriter{}
	appLedger.On("Height").Return(uint64(0))
	appLedger.On("Append", mock.Anything).Return(nil).Run(func(arguments mock.Arguments) {
		seq := arguments.Get(0).(*common.Block).Header.Number
		blocksCommittedToApplicationLedger <- seq
	})

	lf := &cluster_mocks.LedgerFactory{}
	lf.On("GetOrCreate", "system").Return(systemLedger, nil)
	lf.On("GetOrCreate", "testchannel").Return(appLedger, nil)
	lf.On("Close")

	config := &localconfig.TopLevel{
		General: localconfig.General{
			Cluster: localconfig.Cluster{
				ReplicationPullTimeout:  time.Hour,
				DialTimeout:             time.Hour,
				RPCTimeout:              time.Hour,
				ReplicationRetryTimeout: time.Millisecond,
				ReplicationBufferSize:   1,
				ReplicationMaxRetries:   5,
			},
		},
	}

	secConfig := comm.SecureOptions{
		Certificate:   cert,
		Key:           key,
		UseTLS:        true,
		ServerRootCAs: [][]byte{caCert},
	}

	verifier := &cluster_mocks.BlockVerifier{}
	verifier.On("VerifyBlockSignature", mock.Anything, mock.Anything).Return(nil)
	vr := &cluster_mocks.VerifierRetriever{}
	vr.On("RetrieveVerifier", mock.Anything).Return(verifier)

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	r := &ReplicationInitiator{
		verifierRetriever: vr,
		lf:                lf,
		logger:            flogging.MustGetLogger("testOnboarding"),
		conf:              config,
		secOpts:           secConfig,
		cryptoProvider:    cryptoProvider,
	}

	type event struct {
		expectedLog  string
		responseFunc func(chan *orderer.DeliverResponse)
	}

	var probe, pullSystemChannel, pullAppChannel, failedPulling bool

	var loggerHooks []zap.Option

	for _, e := range []event{
		{
			expectedLog: "Probing whether I should pull channel testchannel",
			responseFunc: func(blockResponses chan *orderer.DeliverResponse) {
				probe = true

				// At this point the client will re-connect, so close the stream.
				blockResponses <- nil
				// And send the genesis block of the application channel 'testchannel'
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: testchannelGB,
					},
				}
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: testchannelGB,
					},
				}
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: testchannelGB,
					},
				}
				blockResponses <- nil
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: testchannelGB,
					},
				}
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: testchannelGB,
					},
				}
				blockResponses <- nil
			},
		},
		{
			expectedLog: "Pulling channel system",
			responseFunc: func(blockResponses chan *orderer.DeliverResponse) {
				pullSystemChannel = true

				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: bootBlock,
					},
				}
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: bootBlock,
					},
				}
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: systemChannelGenesisBlock,
					},
				}
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: channelCreationBlock,
					},
				}
				blockResponses <- &orderer.DeliverResponse{
					Type: &orderer.DeliverResponse_Block{
						Block: bootBlock,
					},
				}
			},
		},
		{
			expectedLog: "Pulling channel testchannel",
			responseFunc: func(blockResponses chan *orderer.DeliverResponse) {
				pullAppChannel = true

				for i := 0; i < config.General.Cluster.ReplicationMaxRetries+1; i++ {
					// Send once the genesis block, to make the client think this is a valid OSN endpoint
					deliverServer.blockResponses <- &orderer.DeliverResponse{
						Type: &orderer.DeliverResponse_Block{
							Block: testchannelGB,
						},
					}
					// Send EOF to make the client abort and retry again
					deliverServer.blockResponses <- nil
				}
			},
		},
		{
			expectedLog: "Failed pulling channel testchannel: retry attempts exhausted",
			responseFunc: func(blockResponses chan *orderer.DeliverResponse) {
				failedPulling = true
			},
		},
	} {
		event := e
		loggerHooks = append(loggerHooks, zap.Hooks(func(entry zapcore.Entry) error {
			if strings.Contains(entry.Message, event.expectedLog) {
				event.responseFunc(deliverServer.blockResponses)
			}
			return nil
		}))
	}

	// Program the logger to intercept the event
	r.logger = r.logger.WithOptions(loggerHooks...)

	// Send the latest system channel block (sequence 2) that is identical to the bootstrap block
	deliverServer.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{
			Block: bootBlock,
		},
	}
	// Send the bootstrap block of the system channel
	deliverServer.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{
			Block: systemChannelGenesisBlock,
		},
	}
	// Send a channel creation block (sequence 1) that denotes creation of 'testchannel'
	deliverServer.blockResponses <- &orderer.DeliverResponse{
		Type: &orderer.DeliverResponse_Block{
			Block: channelCreationBlock,
		},
	}

	r.ReplicateIfNeeded(bootBlock)

	// Ensure all events were invoked
	require.True(t, probe)
	require.True(t, pullSystemChannel)
	require.True(t, pullAppChannel)
	require.True(t, failedPulling)

	// Ensure system channel was fully pulled
	require.Len(t, blocksCommittedToSystemLedger, 3)
	// But the application channel only contains 1 block (the genesis block)
	require.Len(t, blocksCommittedToApplicationLedger, 1)
}

func TestReplicate(t *testing.T) {
	clusterConfig := localconfig.Cluster{
		ReplicationPullTimeout:  time.Hour,
		DialTimeout:             time.Hour,
		RPCTimeout:              time.Hour,
		ReplicationRetryTimeout: time.Hour,
		ReplicationBufferSize:   1,
	}

	var bootBlock common.Block
	var bootBlockWithCorruptedPayload common.Block

	flogging.ActivateSpec("testReplicateIfNeeded=debug")

	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	cryptoPath := generateCryptoMaterials(t, cryptogen)
	defer os.RemoveAll(cryptoPath)

	applicationChannelBlockPath := generateBootstrapBlock(t, tempDir, configtxgen, "testchannel", "SampleOrgChannel")
	applicationChannelBlockBytes, err := ioutil.ReadFile(applicationChannelBlockPath)
	require.NoError(t, err)

	caCert := loadPEM(cryptoPath, "ca.crt", t)
	key := loadPEM(cryptoPath, "server.key", t)
	cert := loadPEM(cryptoPath, "server.crt", t)

	prepareTestCase := func() *deliverServer {
		deliverServer := newServerNode(t, key, cert)

		require.NoError(t, proto.Unmarshal(applicationChannelBlockBytes, &bootBlock))
		bootBlock.Header.Number = 10
		injectOrdererEndpoint(t, &bootBlock, deliverServer.srv.Address())

		copyBlock := func(block *common.Block, seq uint64) common.Block {
			res := common.Block{}
			require.NoError(t, proto.Unmarshal(protoutil.MarshalOrPanic(block), &res))
			res.Header.Number = seq
			return res
		}

		bootBlockWithCorruptedPayload = copyBlock(&bootBlock, 100)
		env := &common.Envelope{}
		require.NoError(t, proto.Unmarshal(bootBlockWithCorruptedPayload.Data.Data[0], env))
		payload := &common.Payload{}
		require.NoError(t, proto.Unmarshal(env.Payload, payload))
		payload.Data = []byte{1, 2, 3}

		deliverServer.blockResponses <- &orderer.DeliverResponse{
			Type: &orderer.DeliverResponse_Block{Block: &bootBlock},
		}

		blocks := make([]*common.Block, 11)
		for seq := uint64(0); seq <= uint64(10); seq++ {
			block := copyBlock(&bootBlock, seq)
			if seq > 0 {
				block.Header.PreviousHash = protoutil.BlockHeaderHash(blocks[seq-1].Header)
			}
			blocks[seq] = &block
			deliverServer.blockResponses <- &orderer.DeliverResponse{
				Type: &orderer.DeliverResponse_Block{Block: &block},
			}
		}
		// We close the block responses to mark the server side to return from
		// the method dispatch.
		close(deliverServer.blockResponses)

		// We need to ensure the hash chain is valid with respect to the bootstrap block.
		// Validating the hash chain itself when we traverse channels will be taken care
		// of in FAB-12926.
		bootBlock.Header.PreviousHash = protoutil.BlockHeaderHash(blocks[9].Header)
		return deliverServer
	}

	var hooksActivated bool

	for _, testCase := range []struct {
		name               string
		panicValue         string
		systemLedgerHeight uint64
		bootBlock          *common.Block
		secOpts            comm.SecureOptions
		conf               *localconfig.TopLevel
		ledgerFactoryErr   error
		signer             identity.SignerSerializer
		zapHooks           []func(zapcore.Entry) error
		shouldConnect      bool
		replicateFunc      func(*ReplicationInitiator, *common.Block)
		verificationCount  int
	}{
		{
			name:               "Genesis block makes replication be skipped",
			bootBlock:          &common.Block{Header: &common.BlockHeader{Number: 0}},
			systemLedgerHeight: 10,
			zapHooks: []func(entry zapcore.Entry) error{
				func(entry zapcore.Entry) error {
					hooksActivated = true
					require.Equal(t, entry.Message, "Booted with a genesis block, replication isn't an option")
					return nil
				},
			},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateIfNeeded(bootstrapBlock)
			},
		},
		{
			name:               "Block puller initialization failure panics",
			systemLedgerHeight: 10,
			panicValue:         "Failed creating puller config from bootstrap block: unable to decode TLS certificate PEM: ",
			bootBlock:          &bootBlockWithCorruptedPayload,
			conf:               &localconfig.TopLevel{},
			secOpts:            comm.SecureOptions{},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateIfNeeded(bootstrapBlock)
			},
		},
		{
			name:               "Extraction of system channel name fails",
			systemLedgerHeight: 10,
			panicValue:         "Failed extracting system channel name from bootstrap block: failed to retrieve channel id - block is empty",
			bootBlock:          &common.Block{Header: &common.BlockHeader{Number: 100}},
			conf:               &localconfig.TopLevel{},
			secOpts:            comm.SecureOptions{},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateIfNeeded(bootstrapBlock)
			},
		},
		{
			name:               "Is Replication needed fails",
			systemLedgerHeight: 10,
			ledgerFactoryErr:   errors.New("I/O error"),
			panicValue:         "Failed determining whether replication is needed: I/O error",
			bootBlock:          &bootBlock,
			conf:               &localconfig.TopLevel{},
			secOpts: comm.SecureOptions{
				Certificate: cert,
				Key:         key,
			},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateIfNeeded(bootstrapBlock)
			},
		},
		{
			name:               "Replication isn't needed",
			systemLedgerHeight: 11,
			bootBlock:          &bootBlock,
			conf:               &localconfig.TopLevel{},
			secOpts: comm.SecureOptions{
				Certificate: cert,
				Key:         key,
			},
			zapHooks: []func(entry zapcore.Entry) error{
				func(entry zapcore.Entry) error {
					hooksActivated = true
					require.Equal(t, entry.Message, "Replication isn't needed")
					return nil
				},
			},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateIfNeeded(bootstrapBlock)
			},
		},
		{
			name: "Replication is needed, but pulling fails",
			panicValue: "Failed pulling system channel: " +
				"failed obtaining the latest block for channel testchannel",
			shouldConnect:      true,
			systemLedgerHeight: 10,
			bootBlock:          &bootBlock,
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: clusterConfig,
				},
			},
			secOpts: comm.SecureOptions{
				Certificate:   cert,
				Key:           key,
				UseTLS:        true,
				ServerRootCAs: [][]byte{caCert},
			},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateIfNeeded(bootstrapBlock)
			},
		},
		{
			name:               "Explicit replication is requested, but the channel shouldn't be pulled",
			verificationCount:  10,
			shouldConnect:      true,
			systemLedgerHeight: 10,
			bootBlock:          &bootBlock,
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: clusterConfig,
				},
			},
			secOpts: comm.SecureOptions{
				Certificate:   cert,
				Key:           key,
				UseTLS:        true,
				ServerRootCAs: [][]byte{caCert},
			},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateChains(bootstrapBlock, []string{"foo"})
			},
			zapHooks: []func(entry zapcore.Entry) error{
				func(entry zapcore.Entry) error {
					possibleLogs := []string{
						"Will now replicate chains [foo]",
						"Channel testchannel shouldn't be pulled. Skipping it",
					}
					for _, possibleLog := range possibleLogs {
						if entry.Message == possibleLog {
							hooksActivated = true
						}
					}
					return nil
				},
			},
		},
		{
			name: "Explicit replication is requested, but the channel cannot be pulled",
			panicValue: "Failed pulling system channel: " +
				"failed obtaining the latest block for channel testchannel",
			shouldConnect:      true,
			systemLedgerHeight: 10,
			bootBlock:          &bootBlock,
			conf: &localconfig.TopLevel{
				General: localconfig.General{
					Cluster: clusterConfig,
				},
			},
			secOpts: comm.SecureOptions{
				Certificate:   cert,
				Key:           key,
				UseTLS:        true,
				ServerRootCAs: [][]byte{caCert},
			},
			replicateFunc: func(ri *ReplicationInitiator, bootstrapBlock *common.Block) {
				ri.ReplicateChains(bootstrapBlock, []string{"testchannel"})
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			deliverServer := prepareTestCase()
			defer deliverServer.srv.Stop()

			lw := &cluster_mocks.LedgerWriter{}
			lw.On("Height").Return(testCase.systemLedgerHeight).Once()

			lf := &cluster_mocks.LedgerFactory{}
			lf.On("GetOrCreate", mock.Anything).Return(lw, testCase.ledgerFactoryErr).Twice()
			lf.On("Close")

			verifier := &cluster_mocks.BlockVerifier{}
			verifier.On("VerifyBlockSignature", mock.Anything, mock.Anything).Return(nil)
			vr := &cluster_mocks.VerifierRetriever{}
			vr.On("RetrieveVerifier", mock.Anything).Return(verifier)

			cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
			require.NoError(t, err)

			r := &ReplicationInitiator{
				verifierRetriever: vr,
				lf:                lf,
				logger:            flogging.MustGetLogger("testReplicateIfNeeded"),
				signer:            testCase.signer,

				conf:           testCase.conf,
				secOpts:        testCase.secOpts,
				cryptoProvider: cryptoProvider,
			}

			if testCase.panicValue != "" {
				require.PanicsWithValue(t, testCase.panicValue, func() {
					testCase.replicateFunc(r, testCase.bootBlock)
				})
				return
			}

			// Else, we are not expected to panic.
			r.logger = r.logger.WithOptions(zap.Hooks(testCase.zapHooks...))

			// This is the method we're testing.
			testCase.replicateFunc(r, testCase.bootBlock)

			// Ensure we ran the hooks for a test case that doesn't panic
			require.True(t, hooksActivated)
			// Restore the flag for the next iteration
			defer func() {
				hooksActivated = false
			}()

			require.Equal(t, testCase.shouldConnect, atomic.LoadInt32(&deliverServer.isConnected) == int32(1))
			verifier.AssertNumberOfCalls(t, "VerifyBlockSignature", testCase.verificationCount)
		})
	}
}

func TestInactiveChainReplicator(t *testing.T) {
	for _, testCase := range []struct {
		description                          string
		chainsTracked                        []string
		ReplicateChainsExpectedInput1        []string
		ReplicateChainsExpectedInput1Reverse []string
		ReplicateChainsExpectedInput2        []string
		ReplicateChainsOutput1               []string
		ReplicateChainsOutput2               []string
		chainsExpectedToBeReplicated         []string
		expectedRegisteredChains             map[string]struct{}
		ReplicateChainsExpectedCallCount     int
		genesisBlock                         *common.Block
	}{
		{
			description:              "no chains tracked",
			expectedRegisteredChains: map[string]struct{}{},
		},
		{
			description:                          "some chains tracked, but not all succeed replication",
			chainsTracked:                        []string{"foo", "bar"},
			ReplicateChainsExpectedInput1:        []string{"foo", "bar"},
			ReplicateChainsExpectedInput1Reverse: []string{"bar", "foo"},
			ReplicateChainsExpectedInput2:        []string{"bar"},
			ReplicateChainsOutput1:               []string{"foo"},
			ReplicateChainsOutput2:               []string{},
			chainsExpectedToBeReplicated:         []string{"foo"},
			ReplicateChainsExpectedCallCount:     2,
			genesisBlock:                         &common.Block{},
			expectedRegisteredChains: map[string]struct{}{
				"foo": {},
				"bar": {},
			},
		},
		{
			description:                          "some chains tracked, and all succeed replication but on 2nd pass",
			chainsTracked:                        []string{"foo", "bar"},
			ReplicateChainsExpectedInput1:        []string{"foo", "bar"},
			ReplicateChainsExpectedInput1Reverse: []string{"bar", "foo"},
			ReplicateChainsExpectedInput2:        []string{"bar"},
			ReplicateChainsOutput1:               []string{"foo"},
			ReplicateChainsOutput2:               []string{"bar"},
			chainsExpectedToBeReplicated:         []string{"foo", "bar"},
			ReplicateChainsExpectedCallCount:     2,
			genesisBlock:                         &common.Block{},
			expectedRegisteredChains: map[string]struct{}{
				"foo": {},
				"bar": {},
			},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			registeredChains := make(map[string]struct{})
			registerChain := func(chain string) {
				registeredChains[chain] = struct{}{}
			}
			scheduler := make(chan time.Time)
			replicator := &onboarding_mocks.ChainReplicator{}
			icr := &InactiveChainReplicator{
				registerChain:            registerChain,
				logger:                   flogging.MustGetLogger("test"),
				replicator:               replicator,
				chains2CreationCallbacks: make(map[string]chainCreation),
				retrieveLastSysChannelConfigBlock: func() *common.Block {
					return nil
				},
				quitChan:     make(chan struct{}),
				doneChan:     make(chan struct{}),
				scheduleChan: scheduler,
			}

			trackedChains := make(chan string, 10)
			// Simulate starting of a chain by simply adding it to a channel
			for _, trackedChain := range testCase.chainsTracked {
				trackedChain := trackedChain
				icr.TrackChain(trackedChain, testCase.genesisBlock, func() {
					trackedChains <- trackedChain
				})
			}

			// First pass
			input := testCase.ReplicateChainsExpectedInput1
			output := testCase.ReplicateChainsOutput1
			replicator.On("ReplicateChains", mock.Anything, input).Return(output).Once()
			// the input might be called in reverse order
			input = testCase.ReplicateChainsExpectedInput1Reverse
			replicator.On("ReplicateChains", mock.Anything, input).Return(output).Once()

			// Second pass
			input = testCase.ReplicateChainsExpectedInput2
			output = testCase.ReplicateChainsOutput2
			replicator.On("ReplicateChains", mock.Anything, input).Return(output).Once()

			var replicatorStopped sync.WaitGroup
			replicatorStopped.Add(1)
			go func() {
				defer replicatorStopped.Done()
				// trigger to replicate the first time
				scheduler <- time.Time{}
				// trigger to replicate a second time
				scheduler <- time.Time{}
				icr.Stop()
			}()
			icr.Run()
			replicatorStopped.Wait()
			close(trackedChains)

			var replicatedChains []string
			for chain := range trackedChains {
				replicatedChains = append(replicatedChains, chain)
			}
			require.Equal(t, testCase.chainsExpectedToBeReplicated, replicatedChains)
			replicator.AssertNumberOfCalls(t, "ReplicateChains", testCase.ReplicateChainsExpectedCallCount)
			require.Equal(t, testCase.expectedRegisteredChains, registeredChains)
		})
	}
}

func TestInactiveChainReplicatorChannels(t *testing.T) {
	icr := &InactiveChainReplicator{
		logger:                   flogging.MustGetLogger("test"),
		chains2CreationCallbacks: make(map[string]chainCreation),
	}
	icr.TrackChain("foo", &common.Block{}, func() {})
	require.Contains(t, icr.Channels(), cluster.ChannelGenesisBlock{ChannelName: "foo", GenesisBlock: &common.Block{}})

	icr.TrackChain("bar", nil, func() {})
	require.Contains(t, icr.Channels(), cluster.ChannelGenesisBlock{ChannelName: "bar", GenesisBlock: nil})

	icr.Close()
}

func injectConsenterCertificate(t *testing.T, block *common.Block, tlsCert []byte) {
	env, err := protoutil.ExtractEnvelope(block, 0)
	require.NoError(t, err)
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)
	consensus := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Values[channelconfig.ConsensusTypeKey]
	consensus.Value = protoutil.MarshalOrPanic(&orderer.ConsensusType{
		Type: "etcdraft",
		Metadata: protoutil.MarshalOrPanic(&etcdraft.ConfigMetadata{
			Consenters: []*etcdraft.Consenter{
				{
					ServerTlsCert: tlsCert,
					ClientTlsCert: tlsCert,
				},
			},
		}),
	})

	payload.Data = protoutil.MarshalOrPanic(confEnv)
	env.Payload = protoutil.MarshalOrPanic(payload)
	block.Data.Data[0] = protoutil.MarshalOrPanic(env)
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
}

func injectOrdererEndpoint(t *testing.T, block *common.Block, endpoint string) {
	ordererAddresses := channelconfig.OrdererAddressesValue([]string{endpoint})
	// Unwrap the layers until we reach the orderer addresses
	env, err := protoutil.ExtractEnvelope(block, 0)
	require.NoError(t, err)
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)
	// Replace the orderer addresses
	confEnv.Config.ChannelGroup.Values[ordererAddresses.Key()].Value = protoutil.MarshalOrPanic(ordererAddresses.Value())
	// And put it back into the block
	payload.Data = protoutil.MarshalOrPanic(confEnv)
	env.Payload = protoutil.MarshalOrPanic(payload)
	block.Data.Data[0] = protoutil.MarshalOrPanic(env)
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
}

func TestVerifierLoader(t *testing.T) {
	cryptoPath := generateCryptoMaterials(t, cryptogen)
	defer os.RemoveAll(cryptoPath)

	systemChannelBlockPath := generateBootstrapBlock(t, tempDir, configtxgen, "system", "SampleSoloSystemChannel")
	systemChannelBlockBytes, err := ioutil.ReadFile(systemChannelBlockPath)
	require.NoError(t, err)

	configBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(systemChannelBlockBytes, configBlock))

	verifier := &cluster_mocks.BlockVerifier{}

	for _, testCase := range []struct {
		description               string
		ledgerGetOrCreateErr      error
		ledgerHeight              uint64
		lastBlock                 *common.Block
		lastConfigBlock           *common.Block
		verifierFromConfigReturns cluster.BlockVerifier
		verifierFromConfigErr     error
		onFailureInvoked          bool
		expectedPanic             string
		expectedLoggedMessages    map[string]struct{}
		expectedResult            verifiersByChannel
	}{
		{
			description:          "obtaining ledger fails",
			ledgerGetOrCreateErr: errors.New("oops"),
			expectedPanic:        "Failed obtaining ledger for channel mychannel",
		},
		{
			description: "empty ledger",
			expectedLoggedMessages: map[string]struct{}{
				"Channel mychannel has no blocks, skipping it": {},
			},
			expectedResult: make(verifiersByChannel),
		},
		{
			description:   "block retrieval fails",
			ledgerHeight:  100,
			expectedPanic: "Failed retrieving block [99] for channel mychannel",
		},
		{
			description:   "block retrieval succeeds but the block is bad",
			ledgerHeight:  100,
			lastBlock:     &common.Block{},
			expectedPanic: "Failed retrieving config block [99] for channel mychannel",
		},
		{
			description:  "config block retrieved is bad",
			ledgerHeight: 100,
			lastBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, protoutil.MarshalOrPanic(&common.Metadata{
						Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 21}),
					}), {}, {}},
				},
			},
			lastConfigBlock:  &common.Block{Header: &common.BlockHeader{Number: 21}},
			expectedPanic:    "Failed extracting configuration for channel mychannel from block [21]: empty block",
			onFailureInvoked: true,
		},
		{
			description:  "VerifierFromConfig fails",
			ledgerHeight: 100,
			lastBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, protoutil.MarshalOrPanic(&common.Metadata{
						Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 21}),
					}), {}, {}},
				},
			},
			lastConfigBlock:       configBlock,
			verifierFromConfigErr: errors.New("failed initializing MSP"),
			expectedPanic:         "Failed creating verifier for channel mychannel from block [99]: failed initializing MSP",
			onFailureInvoked:      true,
		},
		{
			description:  "VerifierFromConfig succeeds",
			ledgerHeight: 100,
			lastBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, protoutil.MarshalOrPanic(&common.Metadata{
						Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 21}),
					}), {}, {}},
				},
			},
			lastConfigBlock: configBlock,
			expectedLoggedMessages: map[string]struct{}{
				"Loaded verifier for channel mychannel from config block at index 99": {},
			},
			expectedResult: verifiersByChannel{
				"mychannel": verifier,
			},
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			iterator := &deliver_mocks.BlockIterator{}
			iterator.NextReturnsOnCall(0, testCase.lastBlock, common.Status_SUCCESS)
			iterator.NextReturnsOnCall(1, testCase.lastConfigBlock, common.Status_SUCCESS)

			ledger := &onboarding_mocks.ReadWriter{}
			ledger.HeightReturns(testCase.ledgerHeight)
			ledger.IteratorReturns(iterator, 1)

			ledgerFactory := &onboarding_mocks.Factory{}
			ledgerFactory.GetOrCreateReturns(ledger, testCase.ledgerGetOrCreateErr)
			ledgerFactory.ChannelIDsReturns([]string{"mychannel"})

			verifierFactory := &cluster_mocks.VerifierFactory{}
			verifierFactory.On("VerifierFromConfig", mock.Anything, "mychannel").Return(verifier, testCase.verifierFromConfigErr)

			var onFailureInvoked bool
			onFailure := func(_ *common.Block) {
				onFailureInvoked = true
			}

			verifierLoader := &verifierLoader{
				onFailure:       onFailure,
				ledgerFactory:   ledgerFactory,
				verifierFactory: verifierFactory,
				logger:          flogging.MustGetLogger("test"),
			}

			verifierLoader.logger = verifierLoader.logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
				delete(testCase.expectedLoggedMessages, entry.Message)
				return nil
			}))

			if testCase.expectedPanic != "" {
				f := func() {
					verifierLoader.loadVerifiers()
				}
				require.PanicsWithValue(t, testCase.expectedPanic, f)
			} else {
				res := verifierLoader.loadVerifiers()
				require.Equal(t, testCase.expectedResult, res)
			}

			require.Equal(t, testCase.onFailureInvoked, onFailureInvoked)
			require.Empty(t, testCase.expectedLoggedMessages)
		})
	}
}

func TestValidateBootstrapBlock(t *testing.T) {
	cryptoPath := generateCryptoMaterials(t, cryptogen)
	defer os.RemoveAll(cryptoPath)

	systemChannelBlockPath := generateBootstrapBlock(t, tempDir, configtxgen, "system", "SampleSoloSystemChannel")
	systemChannelBlockBytes, err := ioutil.ReadFile(systemChannelBlockPath)
	require.NoError(t, err)

	applicationChannelBlockPath := generateBootstrapBlock(t, tempDir, configtxgen, "mychannel", "SampleOrgChannel")
	applicationChannelBlockBytes, err := ioutil.ReadFile(applicationChannelBlockPath)
	require.NoError(t, err)

	appBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(applicationChannelBlockBytes, appBlock))

	systemBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(systemChannelBlockBytes, systemBlock))

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	for _, testCase := range []struct {
		description   string
		block         *common.Block
		expectedError string
	}{
		{
			description:   "nil block",
			expectedError: "nil block",
		},
		{
			description:   "empty block",
			block:         &common.Block{},
			expectedError: "empty block data",
		},
		{
			description: "bad envelope",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{{1, 2, 3}},
				},
			},
			expectedError: "failed extracting envelope from block: " +
				"proto: common.Envelope: illegal tag 0 (wire type 1)",
		},
		{
			description:   "application channel block",
			block:         appBlock,
			expectedError: "the block isn't a system channel block because it lacks ConsortiumsConfig",
		},
		{
			description: "system channel block",
			block:       systemBlock,
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			err := ValidateBootstrapBlock(testCase.block, cryptoProvider)
			if testCase.expectedError == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, testCase.expectedError)
		})
	}
}

func generateBootstrapBlock(t *testing.T, tempDir, configtxgen, channel, profile string) string {
	gt := NewGomegaWithT(t)
	// create a genesis block for the specified channel and profile
	genesisBlockPath := filepath.Join(tempDir, channel+".block")
	cmd := exec.Command(
		configtxgen,
		"-channelID", channel,
		"-profile", profile,
		"-outputBlock", genesisBlockPath,
		"--configPath", tempDir,
	)
	configtxgenProcess, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(configtxgenProcess, time.Minute).Should(gexec.Exit(0))
	gt.Expect(configtxgenProcess.Err).To(gbytes.Say("Writing genesis block"))

	return genesisBlockPath
}

// generateCryptoMaterials uses cryptogen to generate the necessary
// MSP files and TLS certificates
func generateCryptoMaterials(t *testing.T, cryptogen string) string {
	gt := NewGomegaWithT(t)
	cryptoPath := filepath.Join(tempDir, "crypto")

	cmd := exec.Command(
		cryptogen,
		"generate",
		"--config", filepath.Join(tempDir, "examplecom-config.yaml"),
		"--output", cryptoPath,
	)
	cryptogenProcess, err := gexec.Start(cmd, nil, nil)
	gt.Expect(err).NotTo(HaveOccurred())
	gt.Eventually(cryptogenProcess, time.Minute).Should(gexec.Exit(0))

	return cryptoPath
}

func TestCreateReplicator(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	bootBlock := encoder.New(genesisconfig.Load(genesisconfig.SampleDevModeSoloProfile)).GenesisBlockForChannel("system")

	iterator := &deliver_mocks.BlockIterator{}
	iterator.NextReturnsOnCall(0, bootBlock, common.Status_SUCCESS)
	iterator.NextReturnsOnCall(1, bootBlock, common.Status_SUCCESS)

	ledger := &onboarding_mocks.ReadWriter{}
	ledger.HeightReturns(1)
	ledger.IteratorReturns(iterator, 1)

	ledgerFactory := &onboarding_mocks.Factory{}
	ledgerFactory.GetOrCreateReturns(ledger, nil)
	ledgerFactory.ChannelIDsReturns([]string{"mychannel"})

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	signer := &onboarding_mocks.SignerSerializer{}
	r := NewReplicationInitiator(ledgerFactory, bootBlock, &localconfig.TopLevel{}, comm.SecureOptions{}, signer, cryptoProvider)

	err = r.verifierRetriever.RetrieveVerifier("mychannel").VerifyBlockSignature(nil, nil)
	require.EqualError(t, err, "implicit policy evaluation failed - 0 sub-policies were satisfied, but this policy requires 1 of the 'Writers' sub-policies to be satisfied")

	err = r.verifierRetriever.RetrieveVerifier("system").VerifyBlockSignature(nil, nil)
	require.NoError(t, err)
}
