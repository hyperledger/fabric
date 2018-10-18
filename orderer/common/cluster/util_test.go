/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cluster_test

import (
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/cluster/mocks"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestParallelStubActivation(t *testing.T) {
	t.Parallel()
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
	assert.True(t, activatedStub == instance)
	// Ensure the method was invoked only once.
	assert.Equal(t, activationCount, 1)
}

func TestDialerCustomKeepAliveOptions(t *testing.T) {
	t.Parallel()
	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)

	clientKeyPair, err := ca.NewClientCertKeyPair()
	clientConfig := comm.ClientConfig{
		KaOpts: &comm.KeepaliveOptions{
			ClientTimeout: time.Second * 12345,
		},
		Timeout: time.Millisecond * 100,
		SecOpts: &comm.SecureOptions{
			RequireClientCert: true,
			Key:               clientKeyPair.Key,
			Certificate:       clientKeyPair.Cert,
			ServerRootCAs:     [][]byte{ca.CertBytes()},
			UseTLS:            true,
			ClientRootCAs:     [][]byte{ca.CertBytes()},
		},
	}

	dialer := cluster.NewTLSPinningDialer(clientConfig)
	timeout := dialer.Config.Load().(comm.ClientConfig).KaOpts.ClientTimeout
	assert.Equal(t, time.Second*12345, timeout)
}

func TestDialerBadConfig(t *testing.T) {
	t.Parallel()
	emptyCertificate := []byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----")
	dialer := cluster.NewTLSPinningDialer(comm.ClientConfig{SecOpts: &comm.SecureOptions{UseTLS: true, ServerRootCAs: [][]byte{emptyCertificate}}})
	_, err := dialer.Dial("127.0.0.1:8080", func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		return nil
	})
	assert.EqualError(t, err, "error adding root certificate: asn1: syntax error: sequence truncated")
}

func TestDERtoPEM(t *testing.T) {
	t.Parallel()
	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)
	keyPair, err := ca.NewServerCertKeyPair("localhost")
	assert.NoError(t, err)
	assert.Equal(t, cluster.DERtoPEM(keyPair.TLSCert.Raw), string(keyPair.Cert))
}

func TestStandardDialerDialer(t *testing.T) {
	t.Parallel()
	emptyCertificate := []byte("-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----")
	dialer := cluster.NewTLSPinningDialer(comm.ClientConfig{SecOpts: &comm.SecureOptions{UseTLS: true, ServerRootCAs: [][]byte{emptyCertificate}}})
	standardDialer := &cluster.StandardDialerDialer{Dialer: dialer}
	_, err := standardDialer.Dial("127.0.0.1:8080")
	assert.EqualError(t, err, "error adding root certificate: asn1: syntax error: sequence truncated")
}

func TestVerifyBlockSignature(t *testing.T) {
	verifier := &mocks.BlockVerifier{}
	verifier.On("VerifyBlockSignature", mock.Anything).Return(nil)

	block := createBlockChain(3, 3)[0]

	// The block should have a valid structure
	err := cluster.VerifyBlockSignature(block, verifier)
	assert.NoError(t, err)

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
				metadata := utils.GetMetadataFromBlockOrPanic(block, common.BlockMetadataIndex_SIGNATURES)
				metadata.Signatures[0].SignatureHeader = []byte{1, 2, 3}
				block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = utils.MarshalOrPanic(metadata)
				return block
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			// Create a copy of the block
			blockCopy := &common.Block{}
			err := proto.Unmarshal(utils.MarshalOrPanic(block), blockCopy)
			assert.NoError(t, err)
			// Mutate the block to sabotage it
			blockCopy = testCase.mutateBlock(blockCopy)
			err = cluster.VerifyBlockSignature(blockCopy, verifier)
			assert.Contains(t, err.Error(), testCase.errorContains)
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
	assert.NoError(t, verify(createBlockChain(start, end)))

	twoBlocks := createBlockChain(2, 3)
	twoBlocks[0].Header = nil
	assert.EqualError(t, cluster.VerifyBlockHash(1, twoBlocks), "previous block header is nil")

	// Index out of bounds
	blockchain := createBlockChain(start, end)
	err := cluster.VerifyBlockHash(100, blockchain)
	assert.EqualError(t, err, "index 100 out of bounds (total 21 blocks)")

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
			errorContains: "computed hash of block (13) (cd00e292c5970d3c5e2f0ffa5171e555bc46bfc4faddfb4a418b6840b86e79a3)" +
				" doesn't match claimed hash (07)",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				blockSequence[len(blockSequence)/2].Header.DataHash = []byte{7}
				return blockSequence
			},
		},
		{
			name: "prev hash mismatch",
			errorContains: "block 13's hash " +
				"(89c561fb407ef3e7feb5f0140d83cec7f6c8f9ace2e38b59ae6373c72304a386) " +
				"mismatches 12's prev block hash (07)",
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
			assert.EqualError(t, err, testCase.errorContains)
		})
	}
}

func TestVerifyBlocks(t *testing.T) {
	for _, testCase := range []struct {
		name                string
		verifierReturns     error
		mutateBlockSequence func([]*common.Block) []*common.Block
		expectedError       string
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
			expectedError: "block 75's hash " +
				"(0515ada3e221589634db8be7f3b614e955f7af283cbcdc4fed6c9cbbe9984c98) " +
				"mismatches 74's prev block hash (07)",
		},
		{
			name: "bad signature",
			mutateBlockSequence: func(blockSequence []*common.Block) []*common.Block {
				return blockSequence
			},
			verifierReturns: errors.New("bad signature"),
			expectedError:   "bad signature",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			verifier := &mocks.BlockVerifier{}
			verifier.On("VerifyBlockSignature", mock.Anything).Return(testCase.verifierReturns)
			blockchain := createBlockChain(50, 100)
			blockchain = testCase.mutateBlockSequence(blockchain)
			err := cluster.VerifyBlocks(blockchain, verifier)
			assert.EqualError(t, err, testCase.expectedError)
		})
	}
}

func assignHashes(blockchain []*common.Block) {
	for i := 1; i < len(blockchain); i++ {
		blockchain[i].Header.PreviousHash = blockchain[i-1].Header.Hash()
	}
}

func createBlockChain(start, end uint64) []*common.Block {
	newBlock := func(seq uint64) *common.Block {
		sHdr := &common.SignatureHeader{
			Creator: []byte{1, 2, 3},
			Nonce:   []byte{9, 5, 42, 66},
		}
		block := common.NewBlock(seq, nil)
		blockSignature := &common.MetadataSignature{
			SignatureHeader: utils.MarshalOrPanic(sHdr),
		}
		block.Metadata.Metadata[common.BlockMetadataIndex_SIGNATURES] = utils.MarshalOrPanic(&common.Metadata{
			Value: nil,
			Signatures: []*common.MetadataSignature{
				blockSignature,
			},
		})
		return block
	}
	var blockchain []*common.Block
	for seq := uint64(start); seq <= uint64(end); seq++ {
		block := newBlock(seq)
		block.Data.Data = append(block.Data.Data, make([]byte, 100))
		block.Header.DataHash = block.Data.Hash()
		blockchain = append(blockchain, block)
	}
	assignHashes(blockchain)
	return blockchain
}

func TestTLSCACertsFromConfigBlockGreenPath(t *testing.T) {
	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	assert.NoError(t, err)

	block := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, block))

	certs, err := cluster.TLSCACertsFromConfigBlock(block)
	assert.NoError(t, err)
	assert.Len(t, certs, 1)

	bl, _ := pem.Decode(certs[0])
	cert, err := x509.ParseCertificate(bl.Bytes)
	assert.NoError(t, err)

	assert.True(t, cert.IsCA)
	assert.Equal(t, "tlsca.example.com", cert.Subject.CommonName)
}

func TestTLSCACertsFromConfigBlockFailures(t *testing.T) {
	t.Run("nil block", func(t *testing.T) {
		certs, err := cluster.TLSCACertsFromConfigBlock(nil)
		assert.Nil(t, certs)
		assert.EqualError(t, err, "nil block")
	})

	t.Run("nil block data", func(t *testing.T) {
		certs, err := cluster.TLSCACertsFromConfigBlock(&common.Block{})
		assert.Nil(t, certs)
		assert.EqualError(t, err, "block data is nil")
	})

	t.Run("no envelope", func(t *testing.T) {
		certs, err := cluster.TLSCACertsFromConfigBlock(&common.Block{
			Data: &common.BlockData{},
		})
		assert.Nil(t, certs)
		assert.EqualError(t, err, "envelope index out of bounds")
	})

	t.Run("bad envelope", func(t *testing.T) {
		certs, err := cluster.TLSCACertsFromConfigBlock(&common.Block{
			Data: &common.BlockData{
				Data: [][]byte{{}},
			},
		})
		assert.Nil(t, certs)
		assert.EqualError(t, err, "failed extracting bundle from envelope: envelope header cannot be nil")
	})
}
