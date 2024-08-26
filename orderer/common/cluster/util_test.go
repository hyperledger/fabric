/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/metrics/disabled"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric/common/capabilities"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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

func TestVerifyBlockBFT(t *testing.T) {
	block, err := test.MakeGenesisBlock("mychannel")
	require.NoError(t, err)

	configTx := &common.Envelope{}
	err = proto.Unmarshal(block.Data.Data[0], configTx)
	require.NoError(t, err)

	configTx.Signature = []byte{1, 2, 3}
	block.Data.Data[0] = protoutil.MarshalOrPanic(configTx)
	block.Header.Number = 2
	block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)

	twoBlocks := createBlockChain(2, 3)
	twoBlocks[0] = block

	assignHashes(twoBlocks)

	var firstBlockVerified bool
	var secondBlockVerified bool

	err = cluster.VerifyBlocksBFT(twoBlocks, func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
		firstBlockVerified = true
		require.Equal(t, uint64(2), header.Number)
		return nil
	}, func(block *common.Block) protoutil.BlockVerifierFunc {
		return func(header *common.BlockHeader, metadata *common.BlockMetadata) error {
			require.Equal(t, uint64(3), header.Number)
			secondBlockVerified = true
			return nil
		}
	})

	require.NoError(t, err)
	require.True(t, firstBlockVerified)
	require.True(t, secondBlockVerified)
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
			errorContains: "computed hash of block (13) (6de668ac99645e179a4921b477d50df9295fa56cd44f8e5c94756b60ce32ce1c)" +
				" doesn't match claimed hash (07)",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[len(blockSequence)/2].Header.DataHash = []byte{7}
				return blockSequence
			},
		},
		{
			name: "prev hash mismatch",
			errorContains: "block [12]'s hash " +
				"(72cc7ddf4d8465da95115c0a906416d23d8c74bfcb731a5ab057c213d8db62e1) " +
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
			Signature: []byte{1, 2, 3},
			Payload: protoutil.MarshalOrPanic(&common.Payload{
				Header: &common.Header{},
			}),
		})
		block.Data.Data = append(block.Data.Data, txn)
		return block
	}
	var blockchain []*common.Block
	for seq := start; seq <= end; seq++ {
		block := newBlock(seq)
		block.Header.DataHash = protoutil.ComputeBlockDataHash(block.Data)
		blockchain = append(blockchain, block)
	}
	assignHashes(blockchain)
	return blockchain
}

func injectGlobalOrdererEndpoint(t *testing.T, block *common.Block, globalEndpoint, orgEndpoint string) {
	ordererAddresses := channelconfig.OrdererAddressesValue([]string{globalEndpoint})
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
	// Update the per org addresses
	ordererGrps := confEnv.Config.ChannelGroup.Groups[channelconfig.OrdererGroupKey].Groups
	for _, grp := range ordererGrps {
		if grp.Values[channelconfig.EndpointsKey] == nil {
			continue
		}
		if orgEndpoint == "" {
			grp.Values[channelconfig.EndpointsKey].Value = nil
			continue
		}
		// Inject the orgEndpoint
		ordererOrgProtos := &common.OrdererAddresses{
			Addresses: []string{orgEndpoint},
		}
		grp.Values[channelconfig.EndpointsKey].Value = protoutil.MarshalOrPanic(ordererOrgProtos)
	}
	// And put it back into the block
	payload.Data = protoutil.MarshalOrPanic(confEnv)
	env.Payload = protoutil.MarshalOrPanic(payload)
	block.Data.Data[0] = protoutil.MarshalOrPanic(env)
}

func setChannelCapability(t *testing.T, block *common.Block, capabiliity string) {
	env, err := protoutil.ExtractEnvelope(block, 0)
	require.NoError(t, err)
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	require.NoError(t, err)
	confEnv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	require.NoError(t, err)

	// Replace the orderer addresses
	topCapabilities := make(map[string]bool)
	topCapabilities[capabiliity] = true
	confEnv.Config.ChannelGroup.Values[channelconfig.CapabilitiesKey] = &common.ConfigValue{
		Value:     protoutil.MarshalOrPanic(channelconfig.CapabilitiesValue(topCapabilities).Value()),
		ModPolicy: channelconfig.AdminsPolicyKey,
	}

	payload.Data = protoutil.MarshalOrPanic(confEnv)
	env.Payload = protoutil.MarshalOrPanic(payload)
	block.Data.Data[0] = protoutil.MarshalOrPanic(env)
}

func TestEndpointconfigFromConfigBlockGreenPath(t *testing.T) {
	t.Run("global endpoints V2", func(t *testing.T) {
		block, err := test.MakeGenesisBlock("mychannel")
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		// For a block that doesn't have per org endpoints,
		// we take the global endpoints
		injectGlobalOrdererEndpoint(t, block, "globalEndpoint", "")
		setChannelCapability(t, block, capabilities.ChannelV2_0)
		endpointConfig, err := cluster.EndpointconfigFromConfigBlock(block, cryptoProvider)
		require.NoError(t, err)
		require.Len(t, endpointConfig, 1)
		require.Equal(t, "globalEndpoint", endpointConfig[0].Endpoint)

		bl, _ := pem.Decode(endpointConfig[0].TLSRootCAs[0])
		cert, err := x509.ParseCertificate(bl.Bytes)
		require.NoError(t, err)

		require.True(t, cert.IsCA)
	})

	t.Run("global endpoints and org endpoints V2", func(t *testing.T) {
		block, err := test.MakeGenesisBlock("mychannel")
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		// For a block that has both global and per org endpoints,
		// we take the per org endpoints
		injectGlobalOrdererEndpoint(t, block, "globalEndpoint", "orgEndpoint")
		setChannelCapability(t, block, capabilities.ChannelV2_0)
		endpointConfig, err := cluster.EndpointconfigFromConfigBlock(block, cryptoProvider)
		require.NoError(t, err)
		require.Len(t, endpointConfig, 1)
		require.Equal(t, "orgEndpoint", endpointConfig[0].Endpoint)

		bl, _ := pem.Decode(endpointConfig[0].TLSRootCAs[0])
		cert, err := x509.ParseCertificate(bl.Bytes)
		require.NoError(t, err)

		require.True(t, cert.IsCA)
	})

	t.Run("global endpoints V3", func(t *testing.T) {
		block, err := test.MakeGenesisBlock("mychannel")
		require.NoError(t, err)

		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		// In V3, we do not allow global endpoints
		injectGlobalOrdererEndpoint(t, block, "globalEndpoint", "orgEndpoint")
		endpointConfig, err := cluster.EndpointconfigFromConfigBlock(block, cryptoProvider)
		require.EqualError(t, err, "failed extracting bundle from envelope: initializing channelconfig failed: global OrdererAddresses are not allowed with V3_0 capability, use org specific addresses only")
		require.Nil(t, endpointConfig)
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

func TestBlockValidationPolicyVerifier(t *testing.T) {
	dir := t.TempDir()

	cryptogen, err := gexec.Build("github.com/hyperledger/fabric/cmd/cryptogen")
	require.NoError(t, err)
	defer os.Remove(cryptogen)

	cryptoConfigDir := filepath.Join(dir, "crypto-config")
	b, err := exec.Command(cryptogen, "generate", fmt.Sprintf("--output=%s", cryptoConfigDir)).CombinedOutput()
	require.NoError(t, err, string(b))

	config := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
	config.Orderer.Organizations = append(config.Orderer.Organizations, &genesisconfig.Organization{
		MSPDir:           filepath.Join(cryptoConfigDir, "ordererOrganizations", "example.com", "msp"),
		OrdererEndpoints: []string{"foo:7050", "bar:8050"},
		MSPType:          "bccsp",
		ID:               "SampleMSP",
		Name:             "SampleOrg",
		Policies: map[string]*genesisconfig.Policy{
			"Admins":  {Type: "ImplicitMeta", Rule: "ANY Admins"},
			"Readers": {Type: "ImplicitMeta", Rule: "ANY Readers"},
			"Writers": {Type: "ImplicitMeta", Rule: "ANY Writers"},
		},
	})

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

func generateCertificatesSmartBFT(confAppSmartBFT *genesisconfig.Profile, certDir string, certs ...string) error {
	for i, c := range confAppSmartBFT.Orderer.ConsenterMapping {
		c.MSPID = "SampleOrg"
		cert := filepath.Join(certDir, certs[i])
		c.Identity = cert
		c.ServerTLSCert = cert
		c.ClientTLSCert = cert
	}

	return nil
}

func TestBlockVerifierFunc(t *testing.T) {
	certPath := filepath.Join("testdata", "blockverification", "msp", "signcerts")

	conf := genesisconfig.Load(genesisconfig.SampleAppChannelSmartBftProfile, filepath.Join("testdata", "blockverification"))
	err := generateCertificatesSmartBFT(conf, certPath, "peer.pem", "orderer.example.com-cert.pem", "peer0.org1.example.com-cert.pem", "peer0.org2.example.com-cert.pem")
	require.NoError(t, err)

	flogging.ActivateSpec("debug")

	gb := encoder.New(conf).GenesisBlockForChannel("foo")

	bc := &mocks.BCCSP{}
	bc.VerifyReturns(true, nil)
	bc.GetHashReturns(sha256.New(), nil)
	bc.HashStub = func(msg []byte, _ bccsp.HashOpts) ([]byte, error) {
		dig := sha256.Sum256(msg)
		return dig[:], nil
	}
	bvfunc := cluster.BlockVerifierBuilder(bc)

	verifier := bvfunc(gb)

	header := &common.BlockHeader{}
	md := &common.BlockMetadata{
		Metadata: [][]byte{
			protoutil.MarshalOrPanic(&common.Metadata{Signatures: []*common.MetadataSignature{
				{
					Signature:        []byte{1},
					IdentifierHeader: protoutil.MarshalOrPanic(&common.IdentifierHeader{Identifier: 1}),
				},
				{
					Signature:        []byte{2},
					IdentifierHeader: protoutil.MarshalOrPanic(&common.IdentifierHeader{Identifier: 2}),
				},
				{
					Signature:        []byte{3},
					IdentifierHeader: protoutil.MarshalOrPanic(&common.IdentifierHeader{Identifier: 3}),
				},
			}}),
		},
	}

	err = verifier(header, md)
	require.NoError(t, err)
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
			puller.On("HeightsByEndpoints").Return(testCase.heightsByEndpoints, "", testCase.heightsByEndpointsErr)
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
