/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/metrics/disabled"
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
		DialTimeout: time.Millisecond * 100,
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
		Config: node1.clientConfig,
	}
	dialer.Config.SecOpts.ServerRootCAs = [][]byte{anotherTLSCA.CertBytes()}
	dialer.Config.DialTimeout = time.Second
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
	require.ErrorContains(t, err, "error adding root certificate")
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
	require.ErrorContains(t, err, "error adding root certificate")
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
			expectedError: "error unmarshalling Envelope",
			block:         &common.Block{Data: &common.BlockData{Data: [][]byte{{1, 2, 3}}}},
		},
		{
			name:          "bad genesis block",
			expectedError: "invalid config envelope",
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
			expectedError: "error unmarshalling Envelope",
			block:         &common.Block{Data: &common.BlockData{Data: [][]byte{{1, 2, 3}}}},
		},
		{
			name:          "invalid payload in block envelope",
			expectedError: "error unmarshalling Payload",
			block: &common.Block{Data: &common.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(&common.Envelope{
				Payload: []byte{1, 2, 3},
			})}}},
		},
		{
			name:          "invalid channel header",
			expectedError: "error unmarshalling ChannelHeader",
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
			expectedError: "invalid config envelope",
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
			require.Error(t, err)
			require.Contains(t, err.Error(), testCase.expectedError)
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

		require.Error(t, verifier(nil, nil))
	})

	t.Run("Bad config envelope", func(t *testing.T) {
		bva := &cluster.BlockVerifierAssembler{BCCSP: cryptoProvider}
		_, err := bva.VerifierFromConfig(&common.ConfigEnvelope{}, "mychannel")
		require.EqualError(t, err, "channelconfig Config cannot be nil")
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

	mockErr := errors.New("Mock error")
	verifier := func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
		return mockErr
	}

	verifierFactory := &mocks.VerifierFactory{}
	verifierFactory.On("VerifierFromConfig",
		mock.Anything, "mychannel").Return(verifier, nil)

	registry := &cluster.VerificationRegistry{
		Logger:             flogging.MustGetLogger("test"),
		VerifiersByChannel: make(map[string]protoutil.BlockVerifierFunc),
		VerifierFactory:    verifierFactory,
	}

	var loadCount int
	registry.LoadVerifier = func(chain string) protoutil.BlockVerifierFunc {
		require.Equal(t, "mychannel", chain)
		loadCount++
		return verifier
	}

	v := registry.RetrieveVerifier("mychannel")
	require.Nil(t, v)

	registry.RegisterVerifier("mychannel")
	v = registry.RetrieveVerifier("mychannel")
	require.Same(t, verifier(nil, nil), v(nil, nil))
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

	mockErr := errors.New("Mock error")
	verifier := func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
		return mockErr
	}

	for _, testCase := range []struct {
		description           string
		verifiersByChannel    map[string]protoutil.BlockVerifierFunc
		blockCommitted        *common.Block
		channelCommitted      string
		channelRetrieved      string
		expectedVerifier      protoutil.BlockVerifierFunc
		verifierFromConfig    protoutil.BlockVerifierFunc
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
			verifiersByChannel: make(map[string]protoutil.BlockVerifierFunc),
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
			verifiersByChannel: make(map[string]protoutil.BlockVerifierFunc),
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
			if testCase.expectedVerifier == nil {
				require.Nil(t, verifier)
			} else {
				require.NotNil(t, verifier)
				require.Same(t, testCase.expectedVerifier(nil, nil), verifier(nil, nil))
			}
		})
	}
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

//go:generate counterfeiter -o mocks/bccsp.go --fake-name BCCSP . iBCCSP

type iBCCSP interface {
	bccsp.BCCSP
}

func TestBlockVerifierBuilderEmptyBlock(t *testing.T) {
	bvfunc := cluster.BlockVerifierBuilder(&mocks.BCCSP{})
	block := &common.Block{}
	verifier := bvfunc(block)
	require.ErrorContains(t, verifier(nil, nil), "initialized with an invalid config block: block contains no data")
}

func TestBlockVerifierBuilderNoConfigBlock(t *testing.T) {
	bvfunc := cluster.BlockVerifierBuilder(&mocks.BCCSP{})
	block := createBlockChain(3, 3)[0]
	verifier := bvfunc(block)
	md := &common.BlockMetadata{}
	require.ErrorContains(t, verifier(nil, md), "initialized with an invalid config block: channelconfig Config cannot be nil")
}

func TestBlockVerifierFunc(t *testing.T) {
	block := sampleConfigBlock()
	bvfunc := cluster.BlockVerifierBuilder(&mocks.BCCSP{})

	verifier := bvfunc(block)

	header := &common.BlockHeader{}
	md := &common.BlockMetadata{
		Metadata: [][]byte{
			protoutil.MarshalOrPanic(&common.Metadata{Signatures: []*common.MetadataSignature{
				{
					Signature:        []byte{},
					IdentifierHeader: protoutil.MarshalOrPanic(&common.IdentifierHeader{Identifier: 1}),
				},
			}}),
		},
	}

	err := verifier(header, md)
	require.NoError(t, err)
}

func sampleConfigBlock() *common.Block {
	return &common.Block{
		Header: &common.BlockHeader{
			PreviousHash: []byte("foo"),
		},
		Data: &common.BlockData{
			Data: [][]byte{
				protoutil.MarshalOrPanic(&common.Envelope{
					Payload: protoutil.MarshalOrPanic(&common.Payload{
						Header: &common.Header{
							ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
								Type:      int32(common.HeaderType_CONFIG),
								ChannelId: "mychannel",
							}),
						},
						Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
							Config: &common.Config{
								ChannelGroup: &common.ConfigGroup{
									Values: map[string]*common.ConfigValue{
										"Capabilities": {
											Value: protoutil.MarshalOrPanic(&common.Capabilities{
												Capabilities: map[string]*common.Capability{"V3_0": {}},
											}),
										},
										"HashingAlgorithm": {
											Value: protoutil.MarshalOrPanic(&common.HashingAlgorithm{Name: "SHA256"}),
										},
										"BlockDataHashingStructure": {
											Value: protoutil.MarshalOrPanic(&common.BlockDataHashingStructure{Width: math.MaxUint32}),
										},
									},
									Groups: map[string]*common.ConfigGroup{
										"Orderer": {
											Policies: map[string]*common.ConfigPolicy{
												"BlockValidation": {
													Policy: &common.Policy{
														Type: 3,
													},
												},
											},
											Values: map[string]*common.ConfigValue{
												"BatchSize": {
													Value: protoutil.MarshalOrPanic(&orderer.BatchSize{
														MaxMessageCount:   500,
														AbsoluteMaxBytes:  10485760,
														PreferredMaxBytes: 2097152,
													}),
												},
												"BatchTimeout": {
													Value: protoutil.MarshalOrPanic(&orderer.BatchTimeout{
														Timeout: "2s",
													}),
												},
												"Capabilities": {
													Value: protoutil.MarshalOrPanic(&common.Capabilities{
														Capabilities: map[string]*common.Capability{"V3_0": {}},
													}),
												},
												"ConsensusType": {
													Value: protoutil.MarshalOrPanic(&common.BlockData{Data: [][]byte{[]byte("BFT")}}),
												},
												"Orderers": {
													Value: protoutil.MarshalOrPanic(&common.Orderers{
														ConsenterMapping: []*common.Consenter{
															{
																Id:       1,
																Host:     "host1",
																Port:     8001,
																MspId:    "msp1",
																Identity: []byte("identity1"),
															},
														},
													}),
												},
											},
										},
									},
								},
							},
						}),
					}),
					Signature: []byte("bar"),
				}),
			},
		},
	}
}

func TestGetTLSSessionBinding(t *testing.T) {
	serverCert, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	srv, err := comm.NewGRPCServer("127.0.0.1:0", comm.ServerConfig{
		SecOpts: comm.SecureOptions{
			Certificate: serverCert.Cert,
			Key:         serverCert.Key,
			UseTLS:      true,
		},
	})
	require.NoError(t, err)

	handler := &mocks.Handler{}

	svc := &cluster.ClusterService{
		MinimumExpirationWarningInterval: time.Second * 2,
		StreamCountReporter: &cluster.StreamCountReporter{
			Metrics: cluster.NewMetrics(&disabled.Provider{}),
		},
		Logger:              flogging.MustGetLogger("test"),
		StepLogger:          flogging.MustGetLogger("test"),
		RequestHandler:      handler,
		MembershipByChannel: make(map[string]*cluster.ChannelMembersConfig),
	}

	orderer.RegisterClusterNodeServiceServer(srv.Server(), svc)
	go srv.Start()
	defer srv.Stop()

	clientConf := comm.ClientConfig{
		DialTimeout: time.Second * 3,
		SecOpts: comm.SecureOptions{
			ServerRootCAs: [][]byte{ca.CertBytes()},
			UseTLS:        true,
		},
	}
	conn, err := clientConf.Dial(srv.Address())
	require.NoError(t, err)

	cl := orderer.NewClusterNodeServiceClient(conn)
	stream, err := cl.Step(context.Background())
	require.NoError(t, err)

	binding, err := cluster.GetTLSSessionBinding(stream.Context(), []byte{1, 2, 3, 4, 5})
	require.NoError(t, err)
	require.Len(t, binding, 32)
}

func TestChainParticipant(t *testing.T) {
	for _, testCase := range []struct {
		name                  string
		heightsByEndpoints    map[string]uint64
		heightsByEndpointsErr error
		latestBlockSeq        uint64
		latestBlock           *common.Block
		latestConfigBlockSeq  uint64
		latestConfigBlock     *common.Block
		expectedError         string
		participantError      error
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
			participantError:     cluster.ErrNotInChannel,
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
			puller := &mocks.ChainPuller{}
			puller.On("HeightsByEndpoints").Return(testCase.heightsByEndpoints, testCase.heightsByEndpointsErr)
			puller.On("PullBlock", testCase.latestBlockSeq).Return(testCase.latestBlock)
			puller.On("PullBlock", testCase.latestConfigBlockSeq).Return(testCase.latestConfigBlock)
			puller.On("Close")

			// Checks whether the caller participates in the chain.
			// The ChainPuller should already be calibrated for the chain,
			// participantError is used to detect whether the caller should service the chain.
			// err is nil if the caller participates in the chain.
			// err may be:
			// ErrNotInChannel in case the caller doesn't participate in the chain.
			// ErrForbidden in case the caller is forbidden from pulling the block.
			// ErrServiceUnavailable in case all orderers reachable cannot complete the request.
			// ErrRetryCountExhausted in case no orderer is reachable.
			lastConfigBlock, err := cluster.PullLastConfigBlock(puller)
			configBlocks := make(chan *common.Block, 1)
			if err == nil {
				configBlocks <- lastConfigBlock
				err = testCase.participantError
			}

			if testCase.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), testCase.expectedError)
				require.Len(t, configBlocks, 0)
			} else {
				require.Len(t, configBlocks, 1)
				require.Equal(t, testCase.participantError, err)
			}
		})
	}
}
