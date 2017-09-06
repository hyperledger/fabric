/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockTransientStore struct {
}

func (*mockTransientStore) Persist(txid string, endorserid string, endorsementBlkHt uint64, privateSimulationResults *rwset.TxPvtReadWriteSet) error {
	panic("implement me")
}

func (*mockTransientStore) GetSelfSimulatedTxPvtRWSetByTxid(txid string) (*transientstore.EndorserPvtSimulationResults, error) {
	panic("implement me")
}

type committerMock struct {
	mock.Mock
}

func (mock *committerMock) CommitWithPvtData(blockAndPvtData *ledger.BlockAndPvtData) error {
	args := mock.Called(blockAndPvtData)
	return args.Error(0)
}

func (mock *committerMock) GetPvtDataAndBlockByNum(seqNum uint64) (*ledger.BlockAndPvtData, error) {
	args := mock.Called(seqNum)
	return args.Get(0).(*ledger.BlockAndPvtData), args.Error(1)
}

func (mock *committerMock) Commit(block *common.Block) error {
	args := mock.Called(block)
	return args.Error(0)
}

func (mock *committerMock) LedgerHeight() (uint64, error) {
	args := mock.Called()
	return args.Get(0).(uint64), args.Error(1)
}

func (mock *committerMock) GetBlocks(blockSeqs []uint64) []*common.Block {
	args := mock.Called(blockSeqs)
	seqs := args.Get(0)
	if seqs == nil {
		return nil
	}
	return seqs.([]*common.Block)
}

func (mock *committerMock) Close() {
	mock.Called()
}

func TestPvtDataCollections_FailOnEmptyPayload(t *testing.T) {
	collection := &util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{1, 2, 3, 4, 5, 6, 7},
							},
						},
					},
				},
			},
		},

		nil,
	}

	_, err := collection.Marshal()
	assertion := assert.New(t)
	assertion.Error(err, "Expected to fail since second item has nil payload")
	assertion.Equal("Mallformed private data payload, rwset index 1 is nil", fmt.Sprintf("%s", err))
}

func TestPvtDataCollections_FailMarshalingWriteSet(t *testing.T) {
	collection := &util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet:   nil,
		},
	}

	_, err := collection.Marshal()
	assertion := assert.New(t)
	assertion.Error(err, "Expected to fail since first item has nil writeset")
	assertion.Contains(fmt.Sprintf("%s", err), "Could not marshal private rwset index 0")
}

func TestPvtDataCollections_Marshal(t *testing.T) {
	collection := &util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{1, 2, 3, 4, 5, 6, 7},
							},
						},
					},
				},
			},
		},

		&ledger.TxPvtData{
			SeqInBlock: uint64(2),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{42, 42, 42, 42, 42, 42, 42},
							},
						},
					},
					{
						Namespace: "ns2",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "otherCollection",
								Rwset:          []byte{10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
							},
						},
					},
				},
			},
		},
	}

	bytes, err := collection.Marshal()

	assertion := assert.New(t)
	assertion.NoError(err)
	assertion.NotNil(bytes)
	assertion.Equal(2, len(bytes))
}

func TestPvtDataCollections_Unmarshal(t *testing.T) {
	collection := util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "secretCollection",
								Rwset:          []byte{1, 2, 3, 4, 5, 6, 7},
							},
						},
					},
				},
			},
		},
	}

	bytes, err := collection.Marshal()

	assertion := assert.New(t)
	assertion.NoError(err)
	assertion.NotNil(bytes)
	assertion.Equal(1, len(bytes))

	var newCol util.PvtDataCollections

	err = newCol.Unmarshal(bytes)
	assertion.NoError(err)
	assertion.Equal(newCol, collection)
}

func TestNewCoordinator(t *testing.T) {
	assertion := assert.New(t)

	committer := new(committerMock)

	block := &common.Block{
		Header: &common.BlockHeader{
			Number:       1,
			PreviousHash: []byte{0, 0, 0},
			DataHash:     []byte{1, 1, 1},
		},
		Data: &common.BlockData{
			Data: [][]byte{},
		},
	}

	blockToCommit := &common.Block{
		Header: &common.BlockHeader{
			Number:       2,
			PreviousHash: []byte{1, 1, 1},
			DataHash:     []byte{2, 2, 2},
		},
		Data: &common.BlockData{
			Data: [][]byte{},
		},
	}

	pvtData := util.PvtDataCollections{
		&ledger.TxPvtData{
			SeqInBlock: uint64(1),
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "mySecretCollection",
								Rwset:          []byte{1, 1, 1, 1, 1},
							},
						},
					},
				},
			},
		},
	}

	committer.On("GetBlocks", []uint64{1}).Return([]*common.Block{block})
	committer.On("GetBlocks", []uint64{2}).Return(nil)

	committer.On("LedgerHeight").Return(uint64(1), nil)
	committer.On("Commit", blockToCommit).Return(nil)

	committer.On("CommitWithPvtData", mock.Anything).Run(func(args mock.Arguments) {
		// Check that block and private data correctly wrapped up
		blockAndPvtData := args.Get(0).(*ledger.BlockAndPvtData)
		assertion.Equal(blockToCommit, blockAndPvtData.Block)
		assertion.Equal(1, len(blockAndPvtData.BlockPvtData))
		assertion.Equal(pvtData[0], blockAndPvtData.BlockPvtData[1])

	}).Return(nil)

	coord := NewCoordinator(committer, &mockTransientStore{})

	b, err := coord.GetBlockByNum(1)

	assertion.NoError(err)
	assertion.Equal(block, b)

	b, err = coord.GetBlockByNum(2)

	assertion.Error(err)
	assertion.Nil(b)

	height, err := coord.LedgerHeight()
	assertion.NoError(err)
	assertion.Equal(uint64(1), height)

	missingPvtTx, err := coord.StoreBlock(blockToCommit, pvtData)

	assertion.NoError(err)
	assertion.Empty(missingPvtTx)
}
