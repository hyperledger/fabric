/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"errors"
	"sync"
	"testing"
	"time"

	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/hyperledger/fabric/protos/common"
	gossip2 "github.com/hyperledger/fabric/protos/gossip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNoItemsToReconcile(t *testing.T) {
	// Scenario: there is no missing private data to reconcile.
	// reconciler should identify that we don't have missing data and it doesn't need to call reconciliationFetcher to
	// fetch missing items.
	// reconciler shouldn't get an error.
	committer := &committerMock{}
	fetcher := &mocks.ReconciliationFetcher{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	var missingInfo ledger.MissingPvtDataInfo
	missingInfo = make(map[uint64]ledger.MissingBlockPvtdataInfo)

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil)
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	fetcher.On("FetchReconciledItems", mock.Anything).Return(nil, errors.New("this function shouldn't be called"))

	r := &reconciler{config: ReconcilerConfig{sleepInterval: time.Minute, batchSize: 1}, ReconciliationFetcher: fetcher, Committer: committer}
	err := r.reconcile()

	assert.NoError(t, err)
}

func TestNotReconcilingWhenCollectionConfigNotAvailable(t *testing.T) {
	// Scenario: reconciler gets an error when trying to read collection config for the missing private data.
	// as a result it removes the digest slice, and there are no digests to pull.
	// shouldn't get an error.
	committer := &committerMock{}
	fetcher := &mocks.ReconciliationFetcher{}
	configHistoryRetriever := &mocks.ConfigHistoryRetriever{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	var missingInfo ledger.MissingPvtDataInfo

	missingInfo = map[uint64]ledger.MissingBlockPvtdataInfo{
		1: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			1: {{Collection: "col1", Namespace: "chain1"}},
		},
	}

	var collectionConfigInfo ledger.CollectionConfigInfo

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil)
	configHistoryRetriever.On("MostRecentCollectionConfigBelow", mock.Anything, mock.Anything).Return(&collectionConfigInfo, errors.New("fail to get collection config"))
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	committer.On("GetConfigHistoryRetriever").Return(configHistoryRetriever, nil)

	var fetchCalled bool
	fetcher.On("FetchReconciledItems", mock.Anything).Run(func(args mock.Arguments) {
		var dig2CollectionConfig = args.Get(0).(privdatacommon.Dig2CollectionConfig)
		assert.Equal(t, 0, len(dig2CollectionConfig))
		fetchCalled = true
	}).Return(nil, errors.New("called with no digests"))

	r := &reconciler{config: ReconcilerConfig{sleepInterval: time.Minute, batchSize: 1}, ReconciliationFetcher: fetcher, Committer: committer}
	err := r.reconcile()

	assert.Error(t, err)
	assert.Equal(t, "called with no digests", err.Error())
	assert.True(t, fetchCalled)
}

func TestReconciliationHappyPathWithoutScheduler(t *testing.T) {
	// Scenario: happy path when trying to reconcile missing private data.
	committer := &committerMock{}
	fetcher := &mocks.ReconciliationFetcher{}
	configHistoryRetriever := &mocks.ConfigHistoryRetriever{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	var missingInfo ledger.MissingPvtDataInfo

	missingInfo = map[uint64]ledger.MissingBlockPvtdataInfo{
		3: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			1: {{Collection: "col1", Namespace: "ns1"}},
		},
	}

	collectionConfigInfo := ledger.CollectionConfigInfo{
		CollectionConfig: &common.CollectionConfigPackage{
			Config: []*common.CollectionConfig{
				{Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: "col1",
					},
				}},
			},
		},
		CommittingBlockNum: 1,
	}

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil)
	configHistoryRetriever.On("MostRecentCollectionConfigBelow", mock.Anything, mock.Anything).Return(&collectionConfigInfo, nil)
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	committer.On("GetConfigHistoryRetriever").Return(configHistoryRetriever, nil)

	result := &privdatacommon.FetchedPvtDataContainer{}
	fetcher.On("FetchReconciledItems", mock.Anything).Run(func(args mock.Arguments) {
		var dig2CollectionConfig = args.Get(0).(privdatacommon.Dig2CollectionConfig)
		assert.Equal(t, 1, len(dig2CollectionConfig))
		for digest := range dig2CollectionConfig {
			hash := util2.ComputeSHA256([]byte("rws-pre-image"))
			element := &gossip2.PvtDataElement{
				Digest: &gossip2.PvtDataDigest{
					TxId:       digest.TxId,
					BlockSeq:   digest.BlockSeq,
					Collection: digest.Collection,
					Namespace:  digest.Namespace,
					SeqInBlock: digest.SeqInBlock,
				},
				Payload: [][]byte{hash},
			}
			result.AvailableElements = append(result.AvailableElements, element)
		}
	}).Return(result, nil)

	var commitPvtDataHappened bool
	var blockNum, seqInBlock uint64
	blockNum = 3
	seqInBlock = 1
	committer.On("CommitPvtData", mock.Anything).Run(func(args mock.Arguments) {
		var blockPvtData = args.Get(0).([]*ledger.BlockPvtData)
		assert.Equal(t, 1, len(blockPvtData))
		assert.Equal(t, blockNum, blockPvtData[0].BlockNum)
		assert.Equal(t, seqInBlock, blockPvtData[0].WriteSets[1].SeqInBlock)
		assert.Equal(t, "ns1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
		assert.Equal(t, "col1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
		commitPvtDataHappened = true
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	r := &reconciler{config: ReconcilerConfig{sleepInterval: time.Minute, batchSize: 1}, ReconciliationFetcher: fetcher, Committer: committer}
	err := r.reconcile()

	assert.NoError(t, err)
	assert.True(t, commitPvtDataHappened)
}

func TestReconciliationHappyPathWithScheduler(t *testing.T) {
	// Scenario: happy path when trying to reconcile missing private data.
	committer := &committerMock{}
	fetcher := &mocks.ReconciliationFetcher{}
	configHistoryRetriever := &mocks.ConfigHistoryRetriever{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	var missingInfo ledger.MissingPvtDataInfo

	missingInfo = map[uint64]ledger.MissingBlockPvtdataInfo{
		3: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			1: {{Collection: "col1", Namespace: "ns1"}},
		},
	}

	collectionConfigInfo := ledger.CollectionConfigInfo{
		CollectionConfig: &common.CollectionConfigPackage{
			Config: []*common.CollectionConfig{
				{Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: "col1",
					},
				}},
			},
		},
		CommittingBlockNum: 1,
	}

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil)
	configHistoryRetriever.On("MostRecentCollectionConfigBelow", mock.Anything, mock.Anything).Return(&collectionConfigInfo, nil)
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	committer.On("GetConfigHistoryRetriever").Return(configHistoryRetriever, nil)

	result := &privdatacommon.FetchedPvtDataContainer{}
	fetcher.On("FetchReconciledItems", mock.Anything).Run(func(args mock.Arguments) {
		var dig2CollectionConfig = args.Get(0).(privdatacommon.Dig2CollectionConfig)
		assert.Equal(t, 1, len(dig2CollectionConfig))
		for digest := range dig2CollectionConfig {
			hash := util2.ComputeSHA256([]byte("rws-pre-image"))
			element := &gossip2.PvtDataElement{
				Digest: &gossip2.PvtDataDigest{
					TxId:       digest.TxId,
					BlockSeq:   digest.BlockSeq,
					Collection: digest.Collection,
					Namespace:  digest.Namespace,
					SeqInBlock: digest.SeqInBlock,
				},
				Payload: [][]byte{hash},
			}
			result.AvailableElements = append(result.AvailableElements, element)
		}
	}).Return(result, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	var commitPvtDataHappened bool
	var blockNum, seqInBlock uint64
	blockNum = 3
	seqInBlock = 1
	committer.On("CommitPvtData", mock.Anything).Run(func(args mock.Arguments) {
		var blockPvtData = args.Get(0).([]*ledger.BlockPvtData)
		assert.Equal(t, 1, len(blockPvtData))
		assert.Equal(t, blockNum, blockPvtData[0].BlockNum)
		assert.Equal(t, seqInBlock, blockPvtData[0].WriteSets[1].SeqInBlock)
		assert.Equal(t, "ns1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
		assert.Equal(t, "col1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
		commitPvtDataHappened = true
		wg.Done()
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	r := NewReconciler(committer, fetcher, ReconcilerConfig{sleepInterval: time.Millisecond * 100, batchSize: 1})
	r.Start()
	wg.Wait()
	r.Stop()

	assert.True(t, commitPvtDataHappened)
}
