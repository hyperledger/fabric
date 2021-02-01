/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//go:generate counterfeiter -o mocks/policy.go --fake-name Policy . policy

type policy interface {
	policies.Policy
}

//go:generate counterfeiter -o mocks/policy_manager.go --fake-name PolicyManager . policyManager

type policyManager interface {
	policies.Manager
}

func TestParallelStubActivation(t *testing.T) {
	// Scenario: Activate the stub from different goroutines in parallel.
	stub := &cluster.Stub{}
	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	instance := &cluster.RemoteContext{}
	var activationCount int
	maybeCreateInstance := func() (*cluster.RemoteContext, error) {
		activationCount++
		return instance, nil
	}

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			stub.Activate(maybeCreateInstance)
		}()
	}
	wg.Wait()
	activatedStub := stub.RemoteContext
	// Ensure the instance is the reference we stored
	// and not any other reference, i.e - it wasn't
	// copied by value.
	require.True(t, activatedStub == instance)
	// Ensure the method was invoked only once.
	require.Equal(t, activationCount, 1)
}

func TestDialerCustomKeepAliveOptions(t *testing.T) {
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)

	clientKeyPair, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	clientConfig := comm.ClientConfig{
		KaOpts: comm.KeepaliveOptions{
			ClientTimeout: time.Second * 12345,
		},
		Timeout: time.Millisecond * 100,
		SecOpts: comm.SecureOptions{
			RequireClientCert: true,
			Key:               clientKeyPair.Key,
			Certificate:       clientKeyPair.Cert,
			ServerRootCAs:     [][]byte{ca.CertBytes()},
			UseTLS:            true,
			ClientRootCAs:     [][]byte{ca.CertBytes()},
		},
	}

	dialer := &cluster.PredicateDialer{Config: clientConfig}
	timeout := dialer.Config.KaOpts.ClientTimeout
	require.Equal(t, time.Second*12345, timeout)
}

func TestPredicateDialerUpdateRootCAs(t *testing.T) {
	node1 := newTestNode(t)
	defer node1.stop()

	anotherTLSCA, err := tlsgen.NewCA()
	require.NoError(t, err)

	dialer := &cluster.PredicateDialer{
		Config: node1.clientConfig.Clone(),
	}
	dialer.Config.SecOpts.ServerRootCAs = [][]byte{anotherTLSCA.CertBytes()}
	dialer.Config.Timeout = time.Second
	dialer.Config.AsyncConnect = false

	_, err = dialer.Dial(node1.srv.Address(), nil)
	require.Error(t, err)

	// Update root TLS CAs asynchronously to make sure we don't have a data race.
	go func() {
		dialer.UpdateRootCAs(node1.clientConfig.SecOpts.ServerRootCAs)
	}()

	// Eventually we should succeed connecting.
	for i := 0; i < 10; i++ {
		conn, err := dialer.Dial(node1.srv.Address(), nil)
		if err == nil {
			conn.Close()
			return
		}
	}

	require.Fail(t, "could not connect after 10 attempts despite changing TLS CAs")
}

func TestDialerBadConfig(t *testing.T) {
	emptyCertificate := []byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----")
	dialer := &cluster.PredicateDialer{
		Config: comm.ClientConfig{
			SecOpts: comm.SecureOptions{
				UseTLS:        true,
				ServerRootCAs: [][]byte{emptyCertificate},
			},
		},
	}
	_, err := dialer.Dial("127.0.0.1:8080", func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		return nil
	})
	require.EqualError(t, err, "error adding root certificate: asn1: syntax error: sequence truncated")
}

func TestDERtoPEM(t *testing.T) {
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	keyPair, err := ca.NewServerCertKeyPair("localhost")
	require.NoError(t, err)
	require.Equal(t, cluster.DERtoPEM(keyPair.TLSCert.Raw), string(keyPair.Cert))
}

func TestStandardDialer(t *testing.T) {
	emptyCertificate := []byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----")
	certPool := [][]byte{emptyCertificate}
	config := comm.ClientConfig{SecOpts: comm.SecureOptions{UseTLS: true, ServerRootCAs: certPool}}
	standardDialer := &cluster.StandardDialer{
		Config: config,
	}
	_, err := standardDialer.Dial(cluster.EndpointCriteria{Endpoint: "127.0.0.1:8080", TLSRootCAs: certPool})
	require.EqualError(t,
		err,
		"failed creating gRPC client: error adding root certificate: asn1: syntax error: sequence truncated",
	)
}

func TestVerifyBlockSignature(t *testing.T) {
	verifier := &mocks.BlockVerifier{}
	var nilConfigEnvelope *common.ConfigEnvelope
	verifier.On("VerifyBlockSignature", mock.Anything, nilConfigEnvelope).Return(nil)

	block := createBlockChain(3, 3)[0]

	// The block should have a valid structure
	err := cluster.VerifyBlockSignature(block, verifier, nil)
	require.NoError(t, err)

	for _, testCase := range []struct {
		name          string
		mutateBlock   func(*common.Block) *common.Block
		errorContains string
	}{
		{
			name:          "nil metadata",
			errorContains: "no metadata in block",
			mutateBlock: func(block *common.Block) *common.Block {
				block.Metadata = nil
				return block
			},
		},
		{
			name:          "zero metadata slice",
			errorContains: "no metadata in block",
			mutateBlock: func(block *common.Block) *common.Block {
				block.Metadata.Metadata = nil
				return block
			},
		},
		{
			name:          "nil metadata",
			errorContains: "failed unmarshaling medatata for signatures",
			mutateBlock: func(block *common.Block) *common.Block {
				block.Metadata.Metadata[0] = []byte{1, 2, 3}
				return block
			},
		},
		{
			name:          "bad signature header",
			errorContains: "failed unmarshaling signature header",
			mutateBlock: func(block *common.Block) *common.Block {
				metadata := protoutil.GetMetadataFromBlockOrPanic(block, common.BlockMetadataIndex_SIGNATURES)
				metadata.Signatures[0].SignatureHeader = []byte{1, 2, 3}
				block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(metadata)
				return block
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			// Create a copy of the block
			blockCopy := &common.Block{}
			err := proto.Unmarshal(protoutil.MarshalOrPanic(block), blockCopy)
			require.NoError(t, err)
			// Mutate the block to sabotage it
			blockCopy = testCase.mutateBlock(blockCopy)
			err = cluster.VerifyBlockSignature(blockCopy, verifier, nil)
			require.Contains(t, err.Error(), testCase.errorContains)
		})
	}
}

func TestVerifyBlockHash(t *testing.T) {
	var start uint64 = 3
	var end uint64 = 23

	verify := func(blockchain []*common.Block) error {
		for i := 0; i < len(blockchain); i++ {
			err := cluster.VerifyBlockHash(i, blockchain)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// Verify that createBlockChain() creates a valid blockchain
	require.NoError(t, verify(createBlockChain(start, end)))

	twoBlocks := createBlockChain(2, 3)
	twoBlocks[0].Header = nil
	require.EqualError(t, cluster.VerifyBlockHash(1, twoBlocks), "previous block header is nil")

	// Index out of bounds
	blockchain := createBlockChain(start, end)
	err := cluster.VerifyBlockHash(100, blockchain)
	require.EqualError(t, err, "index 100 out of bounds (total 21 blocks)")

	for _, testCase := range []struct {
		name                string
		mutateBlockSequence func([]*common.Block) []*common.Block
		errorContains       string
	}{
		{
			name:          "non consecutive sequences",
			errorContains: "sequences 12 and 666 were received consecutively",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[len(blockSequence)/2].Header.Number = 666
				assignHashes(blockSequence)
				return blockSequence
			},
		},
		{
			name: "data hash mismatch",
			errorContains: "computed hash of block (13) (dcb2ec1c5e482e4914cb953ff8eedd12774b244b12912afbe6001ba5de9ff800)" +
				" doesn't match claimed hash (07)",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[len(blockSequence)/2].Header.DataHash = []byte{7}
				return blockSequence
			},
		},
		{
			name: "prev hash mismatch",
			errorContains: "block [12]'s hash " +
				"(866351705f1c2f13e10d52ead9d0ca3b80689ede8cc8bf70a6d60c67578323f4) " +
				"mismatches block [13]'s prev block hash (07)",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[len(blockSequence)/2].Header.PreviousHash = []byte{7}
				return blockSequence
			},
		},
		{
			name:          "nil block header",
			errorContains: "missing block header",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[0].Header = nil
				return blockSequence
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			blockchain := createBlockChain(start, end)
			blockchain = testCase.mutateBlockSequence(blockchain)
			err := verify(blockchain)
			require.EqualError(t, err, testCase.errorContains)
		})
	}
}

func TestVerifyBlocks(t *testing.T) {
	var sigSet1 []*protoutil.SignedData
	var sigSet2 []*protoutil.SignedData

	configEnvelope1 := &common.ConfigEnvelope{
		Config: &common.Config{
			Sequence: 1,
		},
	}
	configEnvelope2 := &common.ConfigEnvelope{
		Config: &common.Config{
			Sequence: 2,
		},
	}
	configTransaction := func(envelope *common.ConfigEnvelope) *common.Envelope {
		return &common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Data: protoutil.MarshalOrPanic(envelope),
				Header: &common.Header{
					ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
						Type: int32(common.HeaderType_CONFIG),
					}),
				},
			}),
		}
	}

	for _, testCase := range []struct {
		name                  string
		configureVerifier     func(*mocks.BlockVerifier)
		mutateBlockSequence   func([]*common.Block) []*common.Block
		expectedError         string
		verifierExpectedCalls int
	}{
		{
			name: "empty sequence",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				return nil
			},
			expectedError: "buffer is empty",
		},
		{
			name: "prev hash mismatch",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[len(blockSequence)/2].Header.PreviousHash = []byte{7}
				return blockSequence
			},
			expectedError: "block [74]'s hash " +
				"(5cb4bd1b6a73f81afafd96387bb7ff4473c2425929d0862586f5fbfa12d762dd) " +
				"mismatches block [75]'s prev block hash (07)",
		},
		{
			name: "bad signature",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				return blockSequence
			},
			configureVerifier: func(verifier *mocks.BlockVerifier) {
				var nilEnvelope *common.ConfigEnvelope
				verifier.On("VerifyBlockSignature", mock.Anything, nilEnvelope).Return(errors.New("bad signature"))
			},
			expectedError:         "bad signature",
			verifierExpectedCalls: 1,
		},
		{
			name: "block that its type cannot be classified",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[len(blockSequence)/2].Data = &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{})},
				}
				blockSequence[len(blockSequence)/2].Header.DataHash = protoutil.BlockDataHash(blockSequence[len(blockSequence)/2].Data)
				assignHashes(blockSequence)
				return blockSequence
			},
			expectedError: "nil header in payload",
		},
		{
			name: "config blocks in the sequence need to be verified and one of them is improperly signed",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				var err error
				// Put a config transaction in block n / 4
				blockSequence[len(blockSequence)/4].Data = &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(configTransaction(configEnvelope1))},
				}
				blockSequence[len(blockSequence)/4].Header.DataHash = protoutil.BlockDataHash(blockSequence[len(blockSequence)/4].Data)

				// Put a config transaction in block n / 2
				blockSequence[len(blockSequence)/2].Data = &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(configTransaction(configEnvelope2))},
				}
				blockSequence[len(blockSequence)/2].Header.DataHash = protoutil.BlockDataHash(blockSequence[len(blockSequence)/2].Data)

				assignHashes(blockSequence)

				sigSet1, err = cluster.SignatureSetFromBlock(blockSequence[len(blockSequence)/4])
				require.NoError(t, err)
				sigSet2, err = cluster.SignatureSetFromBlock(blockSequence[len(blockSequence)/2])
				require.NoError(t, err)

				return blockSequence
			},
			configureVerifier: func(verifier *mocks.BlockVerifier) {
				var nilEnvelope *common.ConfigEnvelope
				// The first config block, validates correctly.
				verifier.On("VerifyBlockSignature", sigSet1, nilEnvelope).Return(nil).Once()
				// However, the second config block - validates incorrectly.
				confEnv1 := &common.ConfigEnvelope{}
				proto.Unmarshal(protoutil.MarshalOrPanic(configEnvelope1), confEnv1)
				verifier.On("VerifyBlockSignature", sigSet2, confEnv1).Return(errors.New("bad signature")).Once()
			},
			expectedError:         "bad signature",
			verifierExpectedCalls: 2,
		},
		{
			name: "config block in the sequence needs to be verified, and it is properly signed",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				var err error
				// Put a config transaction in block n / 4
				blockSequence[len(blockSequence)/4].Data = &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(configTransaction(configEnvelope1))},
				}
				blockSequence[len(blockSequence)/4].Header.DataHash = protoutil.BlockDataHash(blockSequence[len(blockSequence)/4].Data)

				assignHashes(blockSequence)

				sigSet1, err = cluster.SignatureSetFromBlock(blockSequence[len(blockSequence)/4])
				require.NoError(t, err)

				sigSet2, err = cluster.SignatureSetFromBlock(blockSequence[len(blockSequence)-1])
				require.NoError(t, err)

				return blockSequence
			},
			configureVerifier: func(verifier *mocks.BlockVerifier) {
				var nilEnvelope *common.ConfigEnvelope
				confEnv1 := &common.ConfigEnvelope{}
				proto.Unmarshal(protoutil.MarshalOrPanic(configEnvelope1), confEnv1)
				verifier.On("VerifyBlockSignature", sigSet1, nilEnvelope).Return(nil).Once()
				verifier.On("VerifyBlockSignature", sigSet2, confEnv1).Return(nil).Once()
			},
			// We have a single config block in the 'middle' of the chain, so we have 2 verifications total:
			// The last block, and the config block.
			verifierExpectedCalls: 2,
		},
		{
			name: "last two blocks are config blocks, last block only verified once",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				var err error
				// Put a config transaction in block n-2 and in n-1
				blockSequence[len(blockSequence)-2].Data = &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(configTransaction(configEnvelope1))},
				}
				blockSequence[len(blockSequence)-2].Header.DataHash = protoutil.BlockDataHash(blockSequence[len(blockSequence)-2].Data)

				blockSequence[len(blockSequence)-1].Data = &common.BlockData{
					Data: [][]byte{protoutil.MarshalOrPanic(configTransaction(configEnvelope2))},
				}
				blockSequence[len(blockSequence)-1].Header.DataHash = protoutil.BlockDataHash(blockSequence[len(blockSequence)-1].Data)

				assignHashes(blockSequence)

				sigSet1, err = cluster.SignatureSetFromBlock(blockSequence[len(blockSequence)-2])
				require.NoError(t, err)

				sigSet2, err = cluster.SignatureSetFromBlock(blockSequence[len(blockSequence)-1])
				require.NoError(t, err)

				return blockSequence
			},
			configureVerifier: func(verifier *mocks.BlockVerifier) {
				var nilEnvelope *common.ConfigEnvelope
				confEnv1 := &common.ConfigEnvelope{}
				proto.Unmarshal(protoutil.MarshalOrPanic(configEnvelope1), confEnv1)
				verifier.On("VerifyBlockSignature", sigSet1, nilEnvelope).Return(nil).Once()
				// We ensure that the signature set of the last block is verified using the config envelope of the block
				// before it.
				verifier.On("VerifyBlockSignature", sigSet2, confEnv1).Return(nil).Once()
				// Note that we do not record a call to verify the last block, with the config envelope extracted from the block itself.
			},
			// We have 2 config blocks, yet we only verify twice- the first config block, and the next config block, but no more,
			// since the last block is a config block.
			verifierExpectedCalls: 2,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			blockchain := createBlockChain(50, 100)
			blockchain = testCase.mutateBlockSequence(blockchain)
			verifier := &mocks.BlockVerifier{}
			if testCase.configureVerifier != nil {
				testCase.configureVerifier(verifier)
			}
			err := cluster.VerifyBlocks(blockchain, verifier)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func assignHashes(blockchain []*common.Block) {
	for i := 1; i < len(blockchain); i++ {
		blockchain[i].Header.PreviousHash = protoutil.BlockHeaderHash(blockchain[i-1].Header)
	}
}

func createBlockChain(start, end uint64) []*common.Block {
	newBlock := func(seq uint64) *common.Block {
		sHdr := &common.SignatureHeader{
			Creator: []byte{1, 2, 3},
			Nonce:   []byte{9, 5, 42, 66},
		}
		block := protoutil.NewBlock(seq, nil)
		blockSignature := &common.MetadataSignature{
			SignatureHeader: protoutil.MarshalOrPanic(sHdr),
		}
		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&common.Metadata{
			Value: nil,
			Signatures: []*common.MetadataSignature{
				blockSignature,
			},
		})

		txn := protoutil.MarshalOrPanic(&common.Envelope{
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{},
			}),
		})
		block.Data.Data = append(block.Data.Data, txn)
		return block
	}
	var blockchain []*common.Block
	for seq := uint64(start); seq <= uint64(end); seq++ {
		block := newBlock(seq)
		block.Data.Data = append(block.Data.Data, make([]byte, 100))
		block.Header.DataHash = protoutil.BlockDataHash(block.Data)
		blockchain = append(blockchain, block)
	}
	assignHashes(blockchain)
	return blockchain
}

func TestEndpointconfigFromConfigBlockGreenPath(t *testing.T) {
	t.Run("global endpoints", func(t *testing.T) {
		block, err := test.MakeGenesisBlock("mychannel")
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		// For a block that doesn't have per org endpoints,
		// we take the global endpoints
		injectGlobalOrdererEndpoint(t, block, "globalEndpoint")
		endpointConfig, err := cluster.EndpointconfigFromConfigBlock(block, cryptoProvider)
		require.NoError(t, err)
		require.Len(t, endpointConfig, 1)
		require.Equal(t, "globalEndpoint", endpointConfig[0].Endpoint)

		bl, _ := pem.Decode(endpointConfig[0].TLSRootCAs[0])
		cert, err := x509.ParseCertificate(bl.Bytes)
		require.NoError(t, err)

		require.True(t, cert.IsCA)
	})

	t.Run("per org endpoints", func(t *testing.T) {
		block, err := test.MakeGenesisBlock("mychannel")
		require.NoError(t, err)

		// Make a second config.
		gConf := genesisconfig.Load(genesisconfig.SampleSingleMSPSoloProfile, configtest.GetDevConfigDir())
		gConf.Orderer.Capabilities = map[string]bool{
			capabilities.OrdererV2_0: true,
		}
		channelGroup, err := encoder.NewChannelGroup(gConf)
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		bundle, err := channelconfig.NewBundle("mychannel", &common.Config{ChannelGroup: channelGroup}, cryptoProvider)
		require.NoError(t, err)

		msps, err := bundle.MSPManager().GetMSPs()
		require.NoError(t, err)
		caBytes := msps["SampleOrg"].GetTLSRootCerts()[0]

		require.NoError(t, err)
		injectAdditionalTLSCAEndpointPair(t, block, "anotherEndpoint", caBytes, "fooOrg")
		endpointConfig, err := cluster.EndpointconfigFromConfigBlock(block, cryptoProvider)
		require.NoError(t, err)
		// And ensure that the endpoints that are taken, are the per org ones.
		require.Len(t, endpointConfig, 2)
		for _, endpoint := range endpointConfig {
			// If this is the original organization (and not the clone),
			// the TLS CA is 'caBytes' read from the second block.
			if endpoint.Endpoint == "anotherEndpoint" {
				require.Len(t, endpoint.TLSRootCAs, 1)
				require.Equal(t, caBytes, endpoint.TLSRootCAs[0])
				continue
			}
			// Else, our endpoints are from the original org, and the TLS CA is something else.
			require.NotEqual(t, caBytes, endpoint.TLSRootCAs[0])
			// The endpoints we expect to see are something else.
			require.Equal(t, 0, strings.Index(endpoint.Endpoint, "127.0.0.1:"))
			bl, _ := pem.Decode(endpoint.TLSRootCAs[0])
			cert, err := x509.ParseCertificate(bl.Bytes)
			require.NoError(t, err)
			require.True(t, cert.IsCA)
		}
	})
}

func TestEndpointconfigFromConfigBlockFailures(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("nil block", func(t *testing.T) {
		certs, err := cluster.EndpointconfigFromConfigBlock(nil, cryptoProvider)
		require.Nil(t, certs)
		require.EqualError(t, err, "nil block")
	})

	t.Run("nil block data", func(t *testing.T) {
		certs, err := cluster.EndpointconfigFromConfigBlock(&common.Block{}, cryptoProvider)
		require.Nil(t, certs)
		require.EqualError(t, err, "block data is nil")
	})

	t.Run("no envelope", func(t *testing.T) {
		certs, err := cluster.EndpointconfigFromConfigBlock(&common.Block{
			Data: &common.BlockData{},
		}, cryptoProvider)
		require.Nil(t, certs)
		require.EqualError(t, err, "envelope index out of bounds")
	})

	t.Run("bad envelope", func(t *testing.T) {
		certs, err := cluster.EndpointconfigFromConfigBlock(&common.Block{
			Data: &common.BlockData{
				Data: [][]byte{{}},
			},
		}, cryptoProvider)
		require.Nil(t, certs)
		require.EqualError(t, err, "failed extracting bundle from envelope: envelope header cannot be nil")
	})
}

func TestConfigFromBlockBadInput(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		block         *common.Block
		expectedError string
	}{
		{
			name:          "nil block",
			expectedError: "empty block",
			block:         nil,
		},
		{
			name:          "nil block data",
			expectedError: "empty block",
			block:         &common.Block{},
		},
		{
			name:          "no data in block",
			expectedError: "empty block",
			block:         &common.Block{Data: &common.BlockData{}},
		},
		{
			name:          "invalid payload",
			expectedError: "error unmarshaling Envelope: proto: common.Envelope: illegal tag 0 (wire type 1)",
			block:         &common.Block{Data: &common.BlockData{Data: [][]byte{{1, 2, 3}}}},
		},
		{
			name:          "bad genesis block",
			expectedError: "invalid config envelope: proto: common.ConfigEnvelope: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Header: &common.BlockHeader{}, Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
					Payload: protoutil.MarshalOrPanic(&common.Payload{
						Data: []byte{1, 2, 3},
					}),
				})}},
			},
		},
		{
			name:          "invalid envelope in block",
			expectedError: "error unmarshaling Envelope: proto: common.Envelope: illegal tag 0 (wire type 1)",
			block:         &common.Block{Data: &common.BlockData{Data: [][]byte{{1, 2, 3}}}},
		},
		{
			name:          "invalid payload in block envelope",
			expectedError: "error unmarshaling Payload: proto: common.Payload: illegal tag 0 (wire type 1)",
			block: &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
				Payload: []byte{1, 2, 3},
			})}}},
		},
		{
			name:          "invalid channel header",
			expectedError: "error unmarshaling ChannelHeader: proto: common.ChannelHeader: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Header: &common.BlockHeader{Number: 1},
				Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
					Payload: protoutil.MarshalOrPanic(&common.Payload{
						Header: &common.Header{
							ChannelHeader: []byte{1, 2, 3},
						},
					}),
				})}},
			},
		},
		{
			name:          "invalid config block",
			expectedError: "invalid config envelope: proto: common.ConfigEnvelope: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Header: &common.BlockHeader{},
				Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
					Payload: protoutil.MarshalOrPanic(&common.Payload{
						Data: []byte{1, 2, 3},
						Header: &common.Header{
							ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
								Type: int32(common.HeaderType_CONFIG),
							}),
						},
					}),
				})}},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			conf, err := cluster.ConfigFromBlock(testCase.block)
			require.Nil(t, conf)
			require.EqualError(t, err, testCase.expectedError)
		})
	}
}

func TestBlockValidationPolicyVerifier(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	group, err := encoder.NewChannelGroup(config)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	validConfigEnvelope := &common.ConfigEnvelope{
		Config: &common.Config{
			ChannelGroup: group,
		},
	}

	for _, testCase := range []struct {
		description   string
		expectedError string
		envelope      *common.ConfigEnvelope
		policyMap     map[string]policies.Policy
		policy        policies.Policy
	}{
		/**
		{
			description:   "policy not found",
			expectedError: "policy /Channel/Orderer/BlockValidation wasn't found",
		},
		*/
		{
			description:   "policy evaluation fails",
			expectedError: "invalid signature",
			policy: &mocks.Policy{
				EvaluateSignedDataStub: func([]*protoutil.SignedData) error {
					return errors.New("invalid signature")
				},
			},
		},
		{
			description:   "bad config envelope",
			expectedError: "config must contain a channel group",
			policy:        &mocks.Policy{},
			envelope:      &common.ConfigEnvelope{Config: &common.Config{}},
		},
		{
			description: "good config envelope overrides custom policy manager",
			policy: &mocks.Policy{
				EvaluateSignedDataStub: func([]*protoutil.SignedData) error {
					return errors.New("invalid signature")
				},
			},
			envelope: validConfigEnvelope,
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			mockPolicyManager := &mocks.PolicyManager{}
			if testCase.policy != nil {
				mockPolicyManager.GetPolicyReturns(testCase.policy, true)
			} else {
				mockPolicyManager.GetPolicyReturns(nil, false)
			}
			mockPolicyManager.GetPolicyReturns(testCase.policy, true)
			verifier := &cluster.BlockValidationPolicyVerifier{
				Logger:    flogging.MustGetLogger("test"),
				Channel:   "mychannel",
				PolicyMgr: mockPolicyManager,
				BCCSP:     cryptoProvider,
			}

			err := verifier.VerifyBlockSignature(nil, testCase.envelope)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBlockVerifierAssembler(t *testing.T) {
	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	group, err := encoder.NewChannelGroup(config)
	require.NoError(t, err)
	require.NotNil(t, group)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("Good config envelope", func(t *testing.T) {
		bva := &cluster.BlockVerifierAssembler{BCCSP: cryptoProvider}
		verifier, err := bva.VerifierFromConfig(&common.ConfigEnvelope{
			Config: &common.Config{
				ChannelGroup: group,
			},
		}, "mychannel")
		require.NoError(t, err)

		require.NoError(t, verifier.VerifyBlockSignature(nil, nil))
	})

	t.Run("Bad config envelope", func(t *testing.T) {
		bva := &cluster.BlockVerifierAssembler{BCCSP: cryptoProvider}
		_, err := bva.VerifierFromConfig(&common.ConfigEnvelope{}, "mychannel")
		require.EqualError(t, err, "failed extracting bundle from envelope: channelconfig Config cannot be nil")
	})
}

func TestLastConfigBlock(t *testing.T) {
	blockRetriever := &mocks.BlockRetriever{}
	blockRetriever.On("Block", uint64(42)).Return(&common.Block{})
	blockRetriever.On("Block", uint64(666)).Return(nil)

	for _, testCase := range []struct {
		name           string
		block          *common.Block
		blockRetriever cluster.BlockRetriever
		expectedError  string
	}{
		{
			name:           "nil block",
			expectedError:  "nil block",
			blockRetriever: blockRetriever,
		},
		{
			name:          "nil support",
			expectedError: "nil blockRetriever",
			block:         &common.Block{},
		},
		{
			name:           "nil metadata",
			expectedError:  "failed to retrieve metadata: no metadata in block",
			blockRetriever: blockRetriever,
			block:          &common.Block{},
		},
		{
			name: "no block with index",
			block: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, protoutil.MarshalOrPanic(&common.Metadata{
						Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 666}),
					})},
				},
			},
			expectedError:  "unable to retrieve last config block [666]",
			blockRetriever: blockRetriever,
		},
		{
			name: "valid last config block",
			block: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, protoutil.MarshalOrPanic(&common.Metadata{
						Value: protoutil.MarshalOrPanic(&common.LastConfig{Index: 42}),
					})},
				},
			},
			blockRetriever: blockRetriever,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			block, err := cluster.LastConfigBlock(testCase.block, testCase.blockRetriever)
			if testCase.expectedError == "" {
				require.NoError(t, err)
				require.NotNil(t, block)
				return
			}
			require.EqualError(t, err, testCase.expectedError)
			require.Nil(t, block)
		})
	}
}

func TestVerificationRegistryRegisterVerifier(t *testing.T) {
	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	require.NoError(t, err)

	block := &common.Block{}
	require.NoError(t, proto.Unmarshal(blockBytes, block))

	verifier := &mocks.BlockVerifier{}

	verifierFactory := &mocks.VerifierFactory{}
	verifierFactory.On("VerifierFromConfig",
		mock.Anything, "mychannel").Return(verifier, nil)

	registry := &cluster.VerificationRegistry{
		Logger:             flogging.MustGetLogger("test"),
		VerifiersByChannel: make(map[string]cluster.BlockVerifier),
		VerifierFactory:    verifierFactory,
	}

	var loadCount int
	registry.LoadVerifier = func(chain string) cluster.BlockVerifier {
		require.Equal(t, "mychannel", chain)
		loadCount++
		return verifier
	}

	v := registry.RetrieveVerifier("mychannel")
	require.Nil(t, v)

	registry.RegisterVerifier("mychannel")
	v = registry.RetrieveVerifier("mychannel")
	require.Equal(t, verifier, v)
	require.Equal(t, 1, loadCount)

	// If the verifier exists, this is a no-op
	registry.RegisterVerifier("mychannel")
	require.Equal(t, 1, loadCount)
}

func TestVerificationRegistry(t *testing.T) {
	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	require.NoError(t, err)

	block := &common.Block{}
	require.NoError(t, proto.Unmarshal(blockBytes, block))

	flogging.ActivateSpec("test=DEBUG")
	defer flogging.Reset()

	verifier := &mocks.BlockVerifier{}

	for _, testCase := range []struct {
		description           string
		verifiersByChannel    map[string]cluster.BlockVerifier
		blockCommitted        *common.Block
		channelCommitted      string
		channelRetrieved      string
		expectedVerifier      cluster.BlockVerifier
		verifierFromConfig    cluster.BlockVerifier
		verifierFromConfigErr error
		loggedMessages        map[string]struct{}
	}{
		{
			description:      "bad block",
			blockCommitted:   &common.Block{},
			channelRetrieved: "foo",
			channelCommitted: "foo",
			loggedMessages: map[string]struct{}{
				"Failed parsing block of channel foo: empty block, content: " +
					"{\n\t\"data\": null,\n\t\"header\": null,\n\t\"metadata\": null\n}\n": {},
				"No verifier for channel foo exists": {},
			},
			expectedVerifier: nil,
		},
		{
			description:      "not a config block",
			blockCommitted:   createBlockChain(5, 5)[0],
			channelRetrieved: "foo",
			channelCommitted: "foo",
			loggedMessages: map[string]struct{}{
				"No verifier for channel foo exists":                             {},
				"Committed block [5] for channel foo that is not a config block": {},
			},
			expectedVerifier: nil,
		},
		{
			description:           "valid block but verifier from config fails",
			blockCommitted:        block,
			verifierFromConfigErr: errors.New("invalid MSP config"),
			channelRetrieved:      "bar",
			channelCommitted:      "bar",
			loggedMessages: map[string]struct{}{
				"Failed creating a verifier from a " +
					"config block for channel bar: invalid MSP config, " +
					"content: " + cluster.BlockToString(block): {},
				"No verifier for channel bar exists": {},
			},
			expectedVerifier: nil,
		},
		{
			description:        "valid block and verifier from config succeeds but wrong channel retrieved",
			blockCommitted:     block,
			verifierFromConfig: verifier,
			channelRetrieved:   "foo",
			channelCommitted:   "bar",
			loggedMessages: map[string]struct{}{
				"No verifier for channel foo exists":         {},
				"Committed config block [0] for channel bar": {},
			},
			expectedVerifier:   nil,
			verifiersByChannel: make(map[string]cluster.BlockVerifier),
		},
		{
			description:        "valid block and verifier from config succeeds",
			blockCommitted:     block,
			verifierFromConfig: verifier,
			channelRetrieved:   "bar",
			channelCommitted:   "bar",
			loggedMessages: map[string]struct{}{
				"Committed config block [0] for channel bar": {},
			},
			expectedVerifier:   verifier,
			verifiersByChannel: make(map[string]cluster.BlockVerifier),
		},
	} {
		t.Run(testCase.description, func(t *testing.T) {
			verifierFactory := &mocks.VerifierFactory{}
			verifierFactory.On("VerifierFromConfig",
				mock.Anything, testCase.channelCommitted).Return(testCase.verifierFromConfig, testCase.verifierFromConfigErr)

			registry := &cluster.VerificationRegistry{
				Logger:             flogging.MustGetLogger("test"),
				VerifiersByChannel: testCase.verifiersByChannel,
				VerifierFactory:    verifierFactory,
			}

			loggedEntriesByMethods := make(map[string]struct{})
			// Configure the logger to collect the message logged
			registry.Logger = registry.Logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
				loggedEntriesByMethods[entry.Message] = struct{}{}
				return nil
			}))

			registry.BlockCommitted(testCase.blockCommitted, testCase.channelCommitted)
			verifier := registry.RetrieveVerifier(testCase.channelRetrieved)

			require.Equal(t, testCase.loggedMessages, loggedEntriesByMethods)
			require.Equal(t, testCase.expectedVerifier, verifier)
		})
	}
}

func TestLedgerInterceptor(t *testing.T) {
	block := &common.Block{}

	ledger := &mocks.LedgerWriter{}
	ledger.On("Append", block).Return(nil).Once()

	var intercepted bool

	var interceptedLedger cluster.LedgerWriter = &cluster.LedgerInterceptor{
		Channel:      "mychannel",
		LedgerWriter: ledger,
		InterceptBlockCommit: func(b *common.Block, channel string) {
			require.Equal(t, block, b)
			require.Equal(t, "mychannel", channel)
			intercepted = true
		},
	}

	err := interceptedLedger.Append(block)
	require.NoError(t, err)
	require.True(t, intercepted)
	ledger.AssertCalled(t, "Append", block)
}

func injectAdditionalTLSCAEndpointPair(t *testing.T, block *common.Block, endpoint string, tlsCA []byte, orgName string) {
	// Unwrap the layers until we reach the orderer addresses
	env, err := protoutil.ExtractEnvelope(block, 0)
	require.NoError(t, err)
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)
	ordererGrp := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	// Get the first orderer org config
	var firstOrdererConfig *common.ConfigGroup
	for _, grp := range ordererGrp {
		firstOrdererConfig = grp
		break
	}
	// Duplicate it.
	secondOrdererConfig := proto.Clone(firstOrdererConfig).(*common.ConfigGroup)
	ordererGrp[orgName] = secondOrdererConfig
	// Reach the FabricMSPConfig buried in it.
	mspConfig := &msp.MSPConfig{}
	err = proto.Unmarshal(secondOrdererConfig.Values[channelconfig.MSPKey].Value, mspConfig)
	require.NoError(t, err)

	fabricConfig := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConfig.Config, fabricConfig)
	require.NoError(t, err)

	// Plant the given TLS CA in it.
	fabricConfig.TlsRootCerts = [][]byte{tlsCA}
	// No intermediate root CAs, to make the test simpler.
	fabricConfig.TlsIntermediateCerts = nil
	// Rename it.
	fabricConfig.Name = orgName

	// Pack the MSP config back into the config
	secondOrdererConfig.Values[channelconfig.MSPKey].Value = protoutil.MarshalOrPanic(&msp.MSPConfig{
		Config: protoutil.MarshalOrPanic(fabricConfig),
		Type:   mspConfig.Type,
	})

	// Inject the endpoint
	ordererOrgProtos := &common.OrdererAddresses{
		Addresses: []string{endpoint},
	}
	secondOrdererConfig.Values[channelconfig.EndpointsKey].Value = protoutil.MarshalOrPanic(ordererOrgProtos)

	// Fold everything back into the block
	payload.Data = protoutil.MarshalOrPanic(confEnv)
	env.Payload = protoutil.MarshalOrPanic(payload)
	block.Data.Data[0] = protoutil.MarshalOrPanic(env)
}

func TestEndpointCriteriaString(t *testing.T) {
	// The top cert is the issuer of the bottom cert
	certs := `-----BEGIN CERTIFICATE-----
MIIBozCCAUigAwIBAgIQMXmzUnikiAZDr4VsrBL+rzAKBggqhkjOPQQDAjAxMS8w
LQYDVQQFEyY2NTc2NDA3Njc5ODcwOTA3OTEwNDM5NzkxMTAwNzA0Mzk3Njg3OTAe
Fw0xOTExMTEyMDM5MDRaFw0yOTExMDkyMDM5MDRaMDExLzAtBgNVBAUTJjY1NzY0
MDc2Nzk4NzA5MDc5MTA0Mzk3OTExMDA3MDQzOTc2ODc5MFkwEwYHKoZIzj0CAQYI
KoZIzj0DAQcDQgAEzBBkRvWgasCKf1pejwpOu+1Fv9FffOZMHnna/7lfMrAqOs8d
HMDVU7mSexu7YNTpAwm4vkdHXi35H8zlVABTxaNCMEAwDgYDVR0PAQH/BAQDAgGm
MB0GA1UdJQQWMBQGCCsGAQUFBwMCBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH/
MAoGCCqGSM49BAMCA0kAMEYCIQCXqXoYLAJN9diIdGxPlRQJgJLju4brWXZfyt3s
E9TjFwIhAOuUJjcOchdP6UA9WLnVWciEo1Omf59NgfHL1gUPb/t6
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIBpDCCAUqgAwIBAgIRAIyvtL0z1xQ+NecXeH1HmmAwCgYIKoZIzj0EAwIwMTEv
MC0GA1UEBRMmNjU3NjQwNzY3OTg3MDkwNzkxMDQzOTc5MTEwMDcwNDM5NzY4Nzkw
HhcNMTkxMTExMjAzOTA0WhcNMTkxMTEyMjAzOTA0WjAyMTAwLgYDVQQFEycxODcw
MDQyMzcxODQwMjY5Mzk2ODUxNzk1NzM3MzIyMTc2OTA3MjAwWTATBgcqhkjOPQIB
BggqhkjOPQMBBwNCAARZBFDBOfC7T9RbsX+PgyE6sM7ocuwn6krIGjc00ICivFgQ
qdHMU7hiswiYwSvwh9MDHlprCRW3ycSgEYQgKU5to0IwQDAOBgNVHQ8BAf8EBAMC
BaAwHQYDVR0lBBYwFAYIKwYBBQUHAwIGCCsGAQUFBwMBMA8GA1UdEQQIMAaHBH8A
AAEwCgYIKoZIzj0EAwIDSAAwRQIhAK6G7qr/ClszCFP25gsflA31+7eoss5vi3o4
qz8bY+s6AiBvO0aOfE8M4ibjmRE4vSXo0+gkOIJKqZcmiRdnJSr8Xw==
-----END CERTIFICATE-----`

	epc := cluster.EndpointCriteria{
		Endpoint:   "orderer.example.com:7050",
		TLSRootCAs: [][]byte{[]byte(certs)},
	}

	actual := fmt.Sprint(epc)
	expected := `{"CAs":[{"Expired":false,"Issuer":"self","Subject":"SERIALNUMBER=65764076798709079104397911007043976879"},{"Expired":true,"Issuer":"SERIALNUMBER=65764076798709079104397911007043976879","Subject":"SERIALNUMBER=187004237184026939685179573732217690720"}],"Endpoint":"orderer.example.com:7050"}`
	require.Equal(t, expected, actual)
}

func TestComparisonMemoizer(t *testing.T) {
	var invocations int

	m := &cluster.ComparisonMemoizer{
		MaxEntries: 5,
		F: func(a, b []byte) bool {
			invocations++
			return bytes.Equal(a, b)
		},
	}

	// Warm-up cache
	for i := 0; i < 5; i++ {
		notSame := m.Compare([]byte{byte(i)}, []byte{1, 2, 3})
		require.False(t, notSame)
		require.Equal(t, i+1, invocations)
	}

	// Ensure lookups are cached
	for i := 0; i < 5; i++ {
		notSame := m.Compare([]byte{byte(i)}, []byte{1, 2, 3})
		require.False(t, notSame)
		require.Equal(t, 5, invocations)
	}

	// Put a new entry which will cause a cache miss
	same := m.Compare([]byte{5}, []byte{5})
	require.True(t, same)
	require.Equal(t, 6, invocations)

	// Keep adding more and more elements to the cache and ensure it stays smaller than its size
	for i := 0; i < 20; i++ {
		odd := m.Compare([]byte{byte(1)}, []byte{byte(i % 2)})
		require.Equal(t, i%2 != 0, odd)
		require.LessOrEqual(t, m.Size(), int(m.MaxEntries))
	}
}
