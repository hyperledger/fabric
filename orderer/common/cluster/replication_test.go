/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// participant returns whether the caller participates in the chain.
// It receives a ChainPuller that should already be calibrated for the chain,
// and a SelfMembershipPredicate that is used to detect whether the caller should service the chain.
// It returns nil if the caller participates in the chain.
// It may return:
// ErrNotInChannel in case the caller doesn't participate in the chain.
// ErrForbidden in case the caller is forbidden from pulling the block.
// ErrServiceUnavailable in case all orderers reachable cannot complete the request.
// ErrRetryCountExhausted in case no orderer is reachable.
func participant(puller cluster.ChainPuller, analyzeLastConfBlock cluster.SelfMembershipPredicate) error {
	lastConfigBlock, err := cluster.PullLastConfigBlock(puller)
	if err != nil {
		return err
	}
	return analyzeLastConfBlock(lastConfigBlock)
}

func TestParticipant(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		heightsByEndpoints    map[string]uint64
		heightsByEndpointsErr error
		latestBlockSeq        uint64
		latestBlock           *common.Block
		latestConfigBlockSeq  uint64
		latestConfigBlock     *common.Block
		expectedError         string
		predicateReturns      error
	}{
		{
			name:          "No available orderer",
			expectedError: cluster.ErrRetryCountExhausted.Error(),
		},
		{
			name:                  "Unauthorized for the channel",
			expectedError:         cluster.ErrForbidden.Error(),
			heightsByEndpointsErr: cluster.ErrForbidden,
		},
		{
			name:                  "No OSN services the channel",
			expectedError:         cluster.ErrServiceUnavailable.Error(),
			heightsByEndpointsErr: cluster.ErrServiceUnavailable,
		},
		{
			name: "Pulled block has no metadata",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock:    &common.Block{},
			expectedError:  "failed to retrieve metadata: no metadata in block",
		},
		{
			name: "Pulled block has no last config sequence in metadata",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: nil,
				},
			},
			expectedError: "failed to retrieve metadata: no metadata at index [SIGNATURES]",
		},
		{
			name: "Pulled block's SIGNATURES metadata is malformed",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{1, 2, 3}},
				},
			},
			expectedError: "failed to retrieve metadata: error unmarshalling metadata at index [SIGNATURES]",
		},
		{
			name: "Pulled block's LAST_CONFIG metadata is malformed",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, {1, 2, 3}},
				},
			},
			expectedError: "failed to retrieve metadata: error unmarshalling metadata at index [LAST_CONFIG]",
		},
		{
			name: "Pulled block's metadata is valid and has a last config",
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{protoutil.MarshalOrPanic(&common.Metadata{
						Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
							LastConfig: &common.LastConfig{Index: 42},
						}),
					})},
				},
			},
			latestConfigBlockSeq: 42,
			latestConfigBlock:    &common.Block{Header: &common.BlockHeader{Number: 42}},
			predicateReturns:     cluster.ErrNotInChannel,
		},
		{
			name:          "Failed pulling last block",
			expectedError: cluster.ErrRetryCountExhausted.Error(),
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock:    nil,
		},
		{
			name:          "Failed pulling last config block",
			expectedError: cluster.ErrRetryCountExhausted.Error(),
			heightsByEndpoints: map[string]uint64{
				"orderer.example.com:7050": 100,
			},
			latestBlockSeq: uint64(99),
			latestBlock: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{protoutil.MarshalOrPanic(&common.Metadata{
						Value: protoutil.MarshalOrPanic(&common.OrdererBlockMetadata{
							LastConfig: &common.LastConfig{Index: 42},
						}),
					})},
				},
			},
			latestConfigBlockSeq: 42,
			latestConfigBlock:    nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			configBlocks := make(chan *common.Block, 1)
			predicate := func(configBlock *common.Block) error {
				configBlocks <- configBlock
				return testCase.predicateReturns
			}
			puller := &mocks.ChainPuller{}
			puller.On("HeightsByEndpoints").Return(testCase.heightsByEndpoints, testCase.heightsByEndpointsErr)
			puller.On("PullBlock", testCase.latestBlockSeq).Return(testCase.latestBlock)
			puller.On("PullBlock", testCase.latestConfigBlockSeq).Return(testCase.latestConfigBlock)
			puller.On("Close")

			err := participant(puller, predicate)
			if testCase.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.expectedError)
				require.Len(t, configBlocks, 0)
			} else {
				require.Len(t, configBlocks, 1)
				require.Equal(t, testCase.predicateReturns, err)
			}
		})
	}
}

func TestBlockPullerFromConfigBlockFailures(t *testing.T) {
	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	require.NoError(t, err)

	validBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(blockBytes, validBlock))

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	for _, testCase := range []struct {
		name         string
		expectedErr  string
		pullerConfig cluster.PullerConfig
		block        *common.Block
	}{
		{
			name:        "nil block",
			expectedErr: "nil block",
		},
		{
			name:        "invalid block",
			expectedErr: "block data is nil",
			block:       &common.Block{},
		},
		{
			name:        "bad envelope inside block",
			expectedErr: "failed extracting bundle from envelope: failed to unmarshal payload from envelope: error unmarshalling Payload",
			block: &common.Block{
				Data: &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
						Payload: []byte{1, 2, 3},
					})},
				},
			},
		},
		{
			name:        "invalid TLS certificate",
			expectedErr: "unable to decode TLS certificate PEM: ////",
			block:       validBlock,
			pullerConfig: cluster.PullerConfig{
				TLSCert: []byte{255, 255, 255},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			verifier := func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
				return nil
			}
			verifierRetriever := &mocks.VerifierRetriever{}
			verifierRetriever.On("RetrieveVerifier", mock.Anything).Return(verifier)
			bp, err := cluster.BlockPullerFromConfigBlock(testCase.pullerConfig, testCase.block, verifierRetriever, cryptoProvider)
			require.Error(t, err)
			require.Contains(t, err.Error(), testCase.expectedErr)
			require.Nil(t, bp)
		})
	}
}

func testBlockPullerFromConfig(t *testing.T, blockVerifiers []protoutil.BlockVerifierFunc, expectedLogMsg string, iterations int) {
	verifierRetriever := &mocks.VerifierRetriever{}
	for _, blockVerifier := range blockVerifiers {
		verifierRetriever.On("RetrieveVerifier", mock.Anything).Return(blockVerifier).Once()
	}

	caCert, err := ioutil.ReadFile(filepath.Join("testdata", "ca.crt"))
	require.NoError(t, err)

	tlsCert, err := ioutil.ReadFile(filepath.Join("testdata", "server.crt"))
	require.NoError(t, err)

	tlsKey, err := ioutil.ReadFile(filepath.Join("testdata", "server.key"))
	require.NoError(t, err)

	osn := newClusterNode(t)
	osn.srv.Stop()
	// Replace the gRPC server with a TLS one
	osn.srv, err = comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			Key:               tlsKey,
			RequireClientCert: true,
			Certificate:       tlsCert,
			ClientRootCAs:     [][]byte{caCert},
			UseTLS:            true,
		},
	})
	require.NoError(t, err)
	orderer.RegisterAtomicBroadcastServer(osn.srv.Server(), osn)
	// And start it
	go osn.srv.Start()
	defer osn.stop()

	// Start from a valid configuration block
	blockBytes, err := ioutil.ReadFile(filepath.Join("testdata", "mychannel.block"))
	require.NoError(t, err)

	validBlock := &common.Block{}
	require.NoError(t, proto.Unmarshal(blockBytes, validBlock))

	// And inject into it a 127.0.0.1 orderer endpoint and a new TLS CA certificate.
	injectTLSCACert(t, validBlock, caCert)
	injectGlobalOrdererEndpoint(t, validBlock, osn.srv.Address())
	validBlock.Header.DataHash = protoutil.BlockDataHash(validBlock.Data)

	for attempt := 0; attempt < iterations; attempt++ {
		blockMsg := &orderer.DeliverResponse_Block{
			Block: validBlock,
		}

		osn.blockResponses <- &orderer.DeliverResponse{
			Type: blockMsg,
		}

		osn.blockResponses <- &orderer.DeliverResponse{
			Type: blockMsg,
		}

		osn.blockResponses <- nil

		osn.addExpectProbeAssert()
		osn.addExpectPullAssert(0)
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	bp, err := cluster.BlockPullerFromConfigBlock(cluster.PullerConfig{
		TLSCert:             tlsCert,
		TLSKey:              tlsKey,
		MaxTotalBufferBytes: 1,
		Channel:             "mychannel",
		Signer:              &mocks.SignerSerializer{},
		Timeout:             time.Hour,
	}, validBlock, verifierRetriever, cryptoProvider)
	require.NoError(t, err)
	bp.RetryTimeout = time.Millisecond * 10
	defer bp.Close()

	var seenExpectedLogMsg bool
	bp.Logger = bp.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if strings.Contains(entry.Message, expectedLogMsg) {
			seenExpectedLogMsg = true
		}
		return nil
	}))

	block := bp.PullBlock(0)
	require.Equal(t, uint64(0), block.Header.Number)
	require.True(t, seenExpectedLogMsg)
}

func TestBlockPullerFromConfigBlockGreenPath(t *testing.T) {
	t.Skipf("This test is disabled, file is going away")
	noopVerifier := func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
		return nil
	}
	for _, testCase := range []struct {
		description        string
		blockVerifiers     []protoutil.BlockVerifierFunc
		expectedLogMessage string
		iterations         int
	}{
		{
			description:        "Success",
			blockVerifiers:     []protoutil.BlockVerifierFunc{noopVerifier},
			expectedLogMessage: "Got block [0] of size",
			iterations:         1,
		},
		{
			description: "Failure",
			iterations:  2,
			// First time it returns nil, second time returns like the success case
			blockVerifiers: []protoutil.BlockVerifierFunc{nil, noopVerifier},
			expectedLogMessage: "Failed verifying received blocks: " +
				"couldn't acquire verifier for channel mychannel",
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			testBlockPullerFromConfig(t, testCase.blockVerifiers,
				testCase.expectedLogMessage, testCase.iterations)
		})
	}
}

func injectGlobalOrdererEndpoint(t *testing.T, block *common.Block, endpoint string) {
	ordererAddresses := channelconfig.OrdererAddressesValue([]string{endpoint})
	// Unwrap the layers until we reach the orderer addresses
	env, err := protoutil.ExtractEnvelope(block, 0)
	require.NoError(t, err)
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)
	// Replace the orderer addresses
	confEnv.Config.ChannelGroup.Values[ordererAddresses.Key()] = &common.ConfigValue{
		Value:     protoutil.MarshalOrPanic(ordererAddresses.Value()),
		ModPolicy: "/Channel/Orderer/Admins",
	}
	// Remove the per org addresses, if applicable
	ordererGrps := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	for _, grp := range ordererGrps {
		if grp.Values[channelconfig.EndpointsKey] == nil {
			continue
		}
		grp.Values[channelconfig.EndpointsKey].Value = nil
	}
	// And put it back into the block
	payload.Data = protoutil.MarshalOrPanic(confEnv)
	env.Payload = protoutil.MarshalOrPanic(payload)
	block.Data.Data[0] = protoutil.MarshalOrPanic(env)
}

func injectTLSCACert(t *testing.T, block *common.Block, tlsCA []byte) {
	// Unwrap the layers until we reach the TLS CA certificates
	env, err := protoutil.ExtractEnvelope(block, 0)
	require.NoError(t, err)
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)
	mspKey := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups["OrdererOrg"].Values[channelconfig.MSPKey]
	rawMSPConfig := mspKey.Value
	mspConf := &msp.MSPConfig{}
	err = proto.Unmarshal(rawMSPConfig, mspConf)
	require.NoError(t, err)
	fabricMSPConf := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConf.Config, fabricMSPConf)
	require.NoError(t, err)
	// Replace the TLS root certs with the given ones
	fabricMSPConf.TlsRootCerts = [][]byte{tlsCA}
	// And put it back into the block
	mspConf.Config = protoutil.MarshalOrPanic(fabricMSPConf)
	mspKey.Value = protoutil.MarshalOrPanic(mspConf)
	payload.Data = protoutil.MarshalOrPanic(confEnv)
	env.Payload = protoutil.MarshalOrPanic(payload)
	block.Data.Data[0] = protoutil.MarshalOrPanic(env)
}
