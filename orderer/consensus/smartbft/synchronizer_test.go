/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package smartbft_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft"
	"github.com/hyperledger/fabric/orderer/consensus/smartbft/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func noopUpdateLastHash(_ *cb.Block) types.Reconfig { return types.Reconfig{} }

func TestSynchronizerSync(t *testing.T) {
	blockBytes, err := os.ReadFile("testdata/mychannel.block")
	require.NoError(t, err)

	goodConfigBlock := &cb.Block{}
	require.NoError(t, proto.Unmarshal(blockBytes, goodConfigBlock))

	b99 := makeBlockWithMetadata(99, 42, &smartbftprotos.ViewMetadata{ViewId: 1, LatestSequence: 12})
	b100 := makeBlockWithMetadata(100, 42, &smartbftprotos.ViewMetadata{ViewId: 1, LatestSequence: 13})
	b101 := makeConfigBlockWithMetadata(goodConfigBlock, 101, &smartbftprotos.ViewMetadata{ViewId: 2, LatestSequence: 1})
	b102 := makeBlockWithMetadata(102, 101, &smartbftprotos.ViewMetadata{ViewId: 2, LatestSequence: 3})

	blockNum2configSqn := map[uint64]uint64{
		99:  7,
		100: 7,
		101: 8,
		102: 8,
	}

	t.Run("no remotes", func(t *testing.T) {
		bp := mocks.NewBlockPuller(t)
		bp.EXPECT().Close()
		bp.EXPECT().HeightsByEndpoints().Return(
			map[string]uint64{},
			nil,
		)
		fakeLedger := mocks.NewLedger(t)
		fakeLedger.EXPECT().Height().Return(100)
		fakeLedger.EXPECT().Block(mock.Anything).Return(b99)
		fakeSequencer := mocks.NewSequencer(t)

		l := flogging.NewFabricLogger(zap.NewExample())

		decision := &types.SyncResponse{
			Latest: types.Decision{},
		}
		syn := &smartbft.Synchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{}, nil
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b99 {
					return &decision.Latest
				}
				return nil
			},
			Logger:      l,
			BlockPuller: bp,
			ClusterSize: 4,
			OnCommit:    noopUpdateLastHash,
			Ledger:      fakeLedger,
			Sequencer:   fakeSequencer,
		}

		d := syn.Sync()
		assert.Equal(t, *decision, d)
	})

	t.Run("all nodes present", func(t *testing.T) {
		bp := mocks.NewBlockPuller(t)
		bp.EXPECT().Close()
		bp.EXPECT().HeightsByEndpoints().Return(
			map[string]uint64{
				"example.com:1": 100,
				"example.com:2": 102,
				"example.com:3": 103,
				"example.com:4": 200, // byzantine, lying
			},
			nil,
		)
		bp.EXPECT().PullBlock(uint64(100)).Return(b100)
		bp.EXPECT().PullBlock(uint64(101)).Return(b101)
		bp.EXPECT().PullBlock(uint64(102)).Return(b102)

		height := uint64(100)
		ledger := map[uint64]*cb.Block{99: b99}

		mockLegder := mocks.NewLedger(t)
		mockLegder.EXPECT().Height().RunAndReturn(func() uint64 { return height })
		mockSequencer := mocks.NewSequencer(t)
		mockSequencer.EXPECT().Sequence().RunAndReturn(func() uint64 { return blockNum2configSqn[height-1] })
		writeBlockHandler := func(b *cb.Block, m []byte) {
			ledger[height] = b
			height++
		}
		mockLegder.EXPECT().WriteConfigBlock(
			mock.AnythingOfType("*common.Block"), mock.Anything).RunAndReturn(writeBlockHandler)
		mockLegder.EXPECT().WriteBlock(
			mock.AnythingOfType("*common.Block"), mock.Anything).RunAndReturn(writeBlockHandler)

		decision := &types.SyncResponse{
			Latest: types.Decision{},
		}
		syn := &smartbft.Synchronizer{
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b102 {
					return &decision.Latest
				}
				return nil
			},
			Logger:      flogging.NewFabricLogger(zap.NewExample()),
			BlockPuller: bp,
			ClusterSize: 4,
			OnCommit:    noopUpdateLastHash,
			Ledger:      mockLegder,
			Sequencer:   mockSequencer,
		}

		d := syn.Sync()
		assert.Equal(t, *decision, d)
	})

	t.Run("3/4 nodes present", func(t *testing.T) {
		bp := mocks.NewBlockPuller(t)
		bp.EXPECT().Close()
		bp.EXPECT().HeightsByEndpoints().Return(
			map[string]uint64{
				"example.com:1": 100,
				"example.com:2": 102,
				"example.com:4": 200, // byzantine, lying
			},
			nil,
		)
		bp.EXPECT().PullBlock(uint64(100)).Return(b100)
		bp.EXPECT().PullBlock(uint64(101)).Return(b101)

		height := uint64(100)
		ledger := map[uint64]*cb.Block{99: b99}

		fakeLedger := mocks.NewLedger(t)
		fakeLedger.EXPECT().Height().RunAndReturn(func() uint64 { return height })
		writeBlockHandler := func(b *cb.Block, m []byte) {
			ledger[height] = b
			height++
		}
		fakeLedger.EXPECT().WriteConfigBlock(
			mock.AnythingOfType("*common.Block"), mock.Anything).RunAndReturn(writeBlockHandler)
		fakeLedger.EXPECT().WriteBlock(
			mock.AnythingOfType("*common.Block"), mock.Anything).RunAndReturn(writeBlockHandler)

		fakeSequencer := mocks.NewSequencer(t)
		fakeSequencer.EXPECT().Sequence().RunAndReturn(
			func() uint64 { return blockNum2configSqn[height-1] })

		decision := &types.SyncResponse{
			Latest: types.Decision{},
		}
		syn := &smartbft.Synchronizer{
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b101 {
					return &decision.Latest
				}
				return nil
			},
			Logger:      flogging.NewFabricLogger(zap.NewExample()),
			BlockPuller: bp,
			ClusterSize: 4,
			OnCommit:    noopUpdateLastHash,
			Ledger:      fakeLedger,
			Sequencer:   fakeSequencer,
		}

		d := syn.Sync()
		assert.Equal(t, *decision, d)
	})

	t.Run("2/4 nodes present", func(t *testing.T) {
		bp := mocks.NewBlockPuller(t)
		bp.EXPECT().Close()
		bp.EXPECT().HeightsByEndpoints().Return(
			map[string]uint64{
				"example.com:1": 101,
				"example.com:4": 200, // byzantine, lying
			},
			nil,
		)
		bp.EXPECT().PullBlock(uint64(100)).Return(b100)

		height := uint64(100)
		ledger := map[uint64]*cb.Block{99: b99}

		fakeLedger := mocks.NewLedger(t)
		fakeLedger.EXPECT().Height().RunAndReturn(func() uint64 { return height })
		writeBlockHandler := func(b *cb.Block, m []byte) {
			ledger[height] = b
			height++
		}
		fakeLedger.EXPECT().WriteBlock(
			mock.AnythingOfType("*common.Block"), mock.Anything).RunAndReturn(writeBlockHandler)

		fakeSequencer := mocks.NewSequencer(t)
		fakeSequencer.EXPECT().Sequence().RunAndReturn(
			func() uint64 { return blockNum2configSqn[height-1] })

		decision := &types.SyncResponse{
			Latest: types.Decision{},
		}
		syn := &smartbft.Synchronizer{
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b100 {
					return &decision.Latest
				}
				return nil
			},
			Logger:      flogging.NewFabricLogger(zap.NewExample()),
			BlockPuller: bp,
			ClusterSize: 4,
			OnCommit:    noopUpdateLastHash,
			Ledger:      fakeLedger,
			Sequencer:   fakeSequencer,
		}

		d := syn.Sync()
		assert.Equal(t, *decision, d)
	})

	t.Run("1/4 nodes present", func(t *testing.T) {
		bp := mocks.NewBlockPuller(t)
		bp.EXPECT().Close()
		bp.EXPECT().HeightsByEndpoints().Return(
			map[string]uint64{
				"example.com:1": 100,
			},
			nil,
		)

		height := uint64(100)
		ledger := map[uint64]*cb.Block{99: b99}

		fakeLedger := mocks.NewLedger(t)
		fakeLedger.EXPECT().Height().RunAndReturn(func() uint64 { return height })
		fakeLedger.EXPECT().Block(mock.AnythingOfType("uint64")).RunAndReturn(
			func(sqn uint64) *cb.Block {
				return ledger[sqn]
			})

		fakeSequencer := mocks.NewSequencer(t)

		decision := &types.SyncResponse{
			Latest: types.Decision{},
		}
		syn := &smartbft.Synchronizer{
			LatestConfig: func() (types.Configuration, []uint64) {
				return types.Configuration{}, nil
			},
			BlockToDecision: func(block *cb.Block) *types.Decision {
				if block == b99 {
					return &decision.Latest
				}
				return nil
			},
			Logger:      flogging.NewFabricLogger(zap.NewExample()),
			BlockPuller: bp,
			ClusterSize: 4,
			OnCommit:    noopUpdateLastHash,
			Ledger:      fakeLedger,
			Sequencer:   fakeSequencer,
		}

		d := syn.Sync()
		assert.Equal(t, *decision, d)
	})
}

func makeBlockWithMetadata(sqnNum, lastConfigIndex uint64, viewMetadata *smartbftprotos.ViewMetadata) *cb.Block {
	block := protoutil.NewBlock(sqnNum, nil)
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(
		&cb.Metadata{
			Value: protoutil.MarshalOrPanic(&cb.LastConfig{Index: lastConfigIndex}),
		},
	)
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			ConsenterMetadata: protoutil.MarshalOrPanic(viewMetadata),
			LastConfig: &cb.LastConfig{
				Index: sqnNum,
			},
		}),
	})
	return block
}

func makeConfigBlockWithMetadata(configBlock *cb.Block, sqnNum uint64, viewMetadata *smartbftprotos.ViewMetadata) *cb.Block {
	block := proto.Clone(configBlock).(*cb.Block)
	block.Header.Number = sqnNum

	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(
		&cb.Metadata{
			Value: protoutil.MarshalOrPanic(&cb.LastConfig{Index: sqnNum}),
		},
	)
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			ConsenterMetadata: protoutil.MarshalOrPanic(viewMetadata),
			LastConfig: &cb.LastConfig{
				Index: sqnNum,
			},
		}),
	})
	return block
}
