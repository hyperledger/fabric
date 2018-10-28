/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"io/ioutil"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/crypto/tlsgen"
	"github.com/hyperledger/fabric/core/comm"
	"github.com/hyperledger/fabric/orderer/common/cluster"
	"github.com/hyperledger/fabric/orderer/common/localconfig"
	"github.com/hyperledger/fabric/orderer/consensus"
	"github.com/hyperledger/fabric/orderer/mocks/common/multichannel"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestLastConfigBlock(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		block         *common.Block
		support       consensus.ConsenterSupport
		expectedError string
	}{
		{
			name:          "nil block",
			expectedError: "nil block",
		},
		{
			name:          "nil support",
			expectedError: "nil support",
			block:         &common.Block{},
		},
		{
			name:          "nil metadata",
			expectedError: "no metadata in block",
			block:         &common.Block{},
			support:       &multichannel.ConsenterSupport{},
		},
		{
			name:          "no last config block metadata",
			expectedError: "no metadata in block",
			block: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}},
				},
			},
			support: &multichannel.ConsenterSupport{},
		},
		{
			name: "bad metadata in block",
			expectedError: "error unmarshaling metadata from block at index " +
				"[LAST_CONFIG]: proto: common.Metadata: illegal tag 0 (wire type 1)",
			block: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, {1, 2, 3}},
				},
			},
			support: &multichannel.ConsenterSupport{},
		},
		{
			name: "no block with index",
			block: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
						Value: utils.MarshalOrPanic(&common.LastConfig{Index: 666}),
					})},
				},
			},
			expectedError: "unable to retrieve last config block 666",
			support:       &multichannel.ConsenterSupport{},
		},
		{
			name: "valid last config block",
			block: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
						Value: utils.MarshalOrPanic(&common.LastConfig{Index: 42}),
					})},
				},
			},
			support: &multichannel.ConsenterSupport{
				BlockByIndex: map[uint64]*common.Block{42: {}},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			block, err := LastConfigBlock(testCase.block, testCase.support)
			if testCase.expectedError == "" {
				assert.NoError(t, err)
				assert.NotNil(t, block)
				return
			}
			assert.EqualError(t, err, testCase.expectedError)
			assert.Nil(t, block)
		})
	}
}

func TestEndpointconfigFromFromSupport(t *testing.T) {
	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	assert.NoError(t, err)

	goodConfigBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, goodConfigBlock))

	for _, testCase := range []struct {
		name            string
		height          uint64
		blockAtHeight   *common.Block
		lastConfigBlock *common.Block
		expectedError   string
	}{
		{
			name:          "Block returns nil",
			expectedError: "unable to retrieve block 99",
			height:        100,
		},
		{
			name:          "Last config block number cannot be retrieved from last block",
			blockAtHeight: &common.Block{},
			expectedError: "no metadata in block",
			height:        100,
		},
		{
			name: "Last config block cannot be retrieved",
			blockAtHeight: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
						Value: utils.MarshalOrPanic(&common.LastConfig{Index: 42}),
					})},
				},
			},
			expectedError: "unable to retrieve last config block 42",
			height:        100,
		},
		{
			name: "Last config block is retrieved but it is invalid",
			blockAtHeight: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
						Value: utils.MarshalOrPanic(&common.LastConfig{Index: 42}),
					})},
				},
			},
			lastConfigBlock: &common.Block{},
			expectedError:   "block data is nil",
			height:          100,
		},
		{
			name: "Last config block is retrieved and is valid",
			blockAtHeight: &common.Block{
				Metadata: &common.BlockMetadata{
					Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
						Value: utils.MarshalOrPanic(&common.LastConfig{Index: 42}),
					})},
				},
			},
			lastConfigBlock: goodConfigBlock,
			height:          100,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cs := &multichannel.ConsenterSupport{
				BlockByIndex: make(map[uint64]*common.Block),
			}
			cs.HeightVal = testCase.height
			cs.BlockByIndex[cs.HeightVal-1] = testCase.blockAtHeight
			cs.BlockByIndex[42] = testCase.lastConfigBlock

			certs, err := EndpointconfigFromFromSupport(cs)
			if testCase.expectedError == "" {
				assert.NotNil(t, certs)
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, testCase.expectedError)
			assert.Nil(t, certs)
		})
	}
}

func TestNewBlockPuller(t *testing.T) {
	ca, err := tlsgen.NewCA()
	assert.NoError(t, err)

	blockBytes, err := ioutil.ReadFile("testdata/mychannel.block")
	assert.NoError(t, err)

	goodConfigBlock := &common.Block{}
	assert.NoError(t, proto.Unmarshal(blockBytes, goodConfigBlock))

	lastBlock := &common.Block{
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{{}, utils.MarshalOrPanic(&common.Metadata{
				Value: utils.MarshalOrPanic(&common.LastConfig{Index: 42}),
			})},
		},
	}

	cs := &multichannel.ConsenterSupport{
		HeightVal: 100,
		BlockByIndex: map[uint64]*common.Block{
			42: goodConfigBlock,
			99: lastBlock,
		},
	}

	dialer := cluster.NewTLSPinningDialer(comm.ClientConfig{
		SecOpts: &comm.SecureOptions{
			Certificate: ca.CertBytes(),
		},
	})

	bp, err := newBlockPuller(cs, dialer, localconfig.Cluster{})
	assert.NoError(t, err)
	assert.NotNil(t, bp)

	// From here on, we test failures.
	for _, testCase := range []struct {
		name          string
		expectedError string
		cs            consensus.ConsenterSupport
		dialer        *cluster.PredicateDialer
		certificate   []byte
	}{
		{
			name:          "UnInitialized dialer",
			certificate:   ca.CertBytes(),
			cs:            cs,
			expectedError: "client config not initialized",
			dialer:        &cluster.PredicateDialer{},
		},
		{
			name: "UnInitialized dialer",

			cs: &multichannel.ConsenterSupport{
				HeightVal: 100,
			},
			certificate:   ca.CertBytes(),
			expectedError: "unable to retrieve block 99",
			dialer:        dialer,
		},
		{
			name:          "Certificate is invalid",
			cs:            cs,
			certificate:   []byte{1, 2, 3},
			expectedError: "client certificate isn't in PEM format: \x01\x02\x03",
			dialer:        dialer,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cc, err := testCase.dialer.ClientConfig()
			if err == nil {
				cc.SecOpts.Certificate = testCase.certificate
				testCase.dialer.SetConfig(cc)
			}
			bp, err := newBlockPuller(testCase.cs, testCase.dialer, localconfig.Cluster{})
			assert.Nil(t, bp)
			assert.EqualError(t, err, testCase.expectedError)
		})
	}
}
