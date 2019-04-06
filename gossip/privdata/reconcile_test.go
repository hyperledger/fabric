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

	"github.com/hyperledger/fabric/common/metrics/disabled"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/gossip/metrics"
	gmetricsmocks "github.com/hyperledger/fabric/gossip/metrics/mocks"
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
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	var missingInfo ledger.MissingPvtDataInfo
	missingInfo = make(map[uint64]ledger.MissingBlockPvtdataInfo)

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil)
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	fetcher.On("FetchReconciledItems", mock.Anything).Return(nil, errors.New("this function shouldn't be called"))

	r := &Reconciler{channel: "", metrics: metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		config:                &ReconcilerConfig{SleepInterval: time.Minute, BatchSize: 1, IsEnabled: true},
		ReconciliationFetcher: fetcher, Committer: committer}
	err := r.reconcile()

	assert.NoError(t, err)
}

func TestNotReconcilingWhenCollectionConfigNotAvailable(t *testing.T) {
	// Scenario: reconciler gets an error when trying to read collection config for the missing private data.
	// as a result it removes the digest slice, and there are no digests to pull.
	// shouldn't get an error.
	committer := &mocks.Committer{}
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

	r := &Reconciler{channel: "", metrics: metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		config:                &ReconcilerConfig{SleepInterval: time.Minute, BatchSize: 1, IsEnabled: true},
		ReconciliationFetcher: fetcher, Committer: committer}
	err := r.reconcile()

	assert.Error(t, err)
	assert.Equal(t, "called with no digests", err.Error())
	assert.True(t, fetchCalled)
}

func TestReconciliationHappyPathWithoutScheduler(t *testing.T) {
	// Scenario: happy path when trying to reconcile missing private data.
	committer := &mocks.Committer{}
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

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil).Run(func(_ mock.Arguments) {
		missingPvtDataTracker.Mock = mock.Mock{}
		missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(nil, nil)
	})
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

	var commitPvtDataOfOldBlocksHappened bool
	var blockNum, seqInBlock uint64
	blockNum = 3
	seqInBlock = 1
	committer.On("CommitPvtDataOfOldBlocks", mock.Anything).Run(func(args mock.Arguments) {
		var blockPvtData = args.Get(0).([]*ledger.BlockPvtData)
		assert.Equal(t, 1, len(blockPvtData))
		assert.Equal(t, blockNum, blockPvtData[0].BlockNum)
		assert.Equal(t, seqInBlock, blockPvtData[0].WriteSets[1].SeqInBlock)
		assert.Equal(t, "ns1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
		assert.Equal(t, "col1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
		commitPvtDataOfOldBlocksHappened = true
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	testMetricProvider := gmetricsmocks.TestUtilConstructMetricProvider()
	metrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).PrivdataMetrics

	r := &Reconciler{channel: "mychannel", metrics: metrics,
		config:                &ReconcilerConfig{SleepInterval: time.Minute, BatchSize: 1, IsEnabled: true},
		ReconciliationFetcher: fetcher, Committer: committer}
	err := r.reconcile()

	assert.NoError(t, err)
	assert.True(t, commitPvtDataOfOldBlocksHappened)

	assert.Equal(t,
		[]string{"channel", "mychannel"},
		testMetricProvider.FakeReconciliationDuration.WithArgsForCall(0),
	)
}

func TestReconciliationHappyPathWithScheduler(t *testing.T) {
	// Scenario: happy path when trying to reconcile missing private data.
	committer := &mocks.Committer{}
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

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil).Run(func(_ mock.Arguments) {
		missingPvtDataTracker.Mock = mock.Mock{}
		missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(nil, nil)
	})
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

	var commitPvtDataOfOldBlocksHappened bool
	var blockNum, seqInBlock uint64
	blockNum = 3
	seqInBlock = 1
	committer.On("CommitPvtDataOfOldBlocks", mock.Anything).Run(func(args mock.Arguments) {
		var blockPvtData = args.Get(0).([]*ledger.BlockPvtData)
		assert.Equal(t, 1, len(blockPvtData))
		assert.Equal(t, blockNum, blockPvtData[0].BlockNum)
		assert.Equal(t, seqInBlock, blockPvtData[0].WriteSets[1].SeqInBlock)
		assert.Equal(t, "ns1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
		assert.Equal(t, "col1", blockPvtData[0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
		commitPvtDataOfOldBlocksHappened = true
		wg.Done()
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	r := NewReconciler("", metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics, committer, fetcher,
		&ReconcilerConfig{SleepInterval: time.Millisecond * 100, BatchSize: 1, IsEnabled: true})
	r.Start()
	wg.Wait()
	r.Stop()

	assert.True(t, commitPvtDataOfOldBlocksHappened)
}

func TestReconciliationPullingMissingPrivateDataAtOnePass(t *testing.T) {
	// Scenario: define batch size to retrieve missing private data to 1
	// and make sure that even though there are missing data for two blocks
	// they will be reconciled with one shot.
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	configHistoryRetriever := &mocks.ConfigHistoryRetriever{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}

	missingInfo := ledger.MissingPvtDataInfo{
		4: ledger.MissingBlockPvtdataInfo{
			1: {{Collection: "col1", Namespace: "ns1"}},
		},
	}

	collectionConfigInfo := &ledger.CollectionConfigInfo{
		CollectionConfig: &common.CollectionConfigPackage{
			Config: []*common.CollectionConfig{
				{Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: "col1",
					},
				}},
				{Payload: &common.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &common.StaticCollectionConfig{
						Name: "col2",
					},
				}},
			},
		},
		CommittingBlockNum: 1,
	}

	stopC := make(chan struct{})
	nextC := make(chan struct{})

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).
		Return(missingInfo, nil).Run(func(_ mock.Arguments) {
		missingInfo := ledger.MissingPvtDataInfo{
			3: ledger.MissingBlockPvtdataInfo{
				2: {{Collection: "col2", Namespace: "ns2"}},
			},
		}
		missingPvtDataTracker.Mock = mock.Mock{}
		missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).
			Return(missingInfo, nil).Run(func(_ mock.Arguments) {
			// here we are making sure that we will first stop
			// reconciliation so next call to GetMissingPvtDataInfoForMostRecentBlocks
			// will go into same round
			<-nextC
			missingPvtDataTracker.Mock = mock.Mock{}
			missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).
				Return(nil, nil)
		})
		// make sure we calling stop reconciliation loop, so for sure
		// in this test we won't get to second round though making sure
		// we are retrieving on single pass
		stopC <- struct{}{}
	})

	configHistoryRetriever.
		On("MostRecentCollectionConfigBelow", mock.Anything, mock.Anything).
		Return(collectionConfigInfo, nil)

	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	committer.On("GetConfigHistoryRetriever").Return(configHistoryRetriever, nil)

	result := &privdatacommon.FetchedPvtDataContainer{}
	fetcher.On("FetchReconciledItems", mock.Anything).Run(func(args mock.Arguments) {
		result.AvailableElements = make([]*gossip2.PvtDataElement, 0)
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
	wg.Add(2)

	var commitPvtDataOfOldBlocksHappened bool
	pvtDataStore := make([][]*ledger.BlockPvtData, 0)
	committer.On("CommitPvtDataOfOldBlocks", mock.Anything).Run(func(args mock.Arguments) {
		var blockPvtData = args.Get(0).([]*ledger.BlockPvtData)
		assert.Equal(t, 1, len(blockPvtData))
		pvtDataStore = append(pvtDataStore, blockPvtData)
		commitPvtDataOfOldBlocksHappened = true
		wg.Done()
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	r := NewReconciler("", metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics, committer, fetcher,
		&ReconcilerConfig{SleepInterval: time.Millisecond * 100, BatchSize: 1, IsEnabled: true})
	r.Start()
	<-stopC
	r.Stop()
	nextC <- struct{}{}
	wg.Wait()

	assert.Equal(t, 2, len(pvtDataStore))
	assert.Equal(t, uint64(4), pvtDataStore[0][0].BlockNum)
	assert.Equal(t, uint64(3), pvtDataStore[1][0].BlockNum)

	assert.Equal(t, uint64(1), pvtDataStore[0][0].WriteSets[1].SeqInBlock)
	assert.Equal(t, uint64(2), pvtDataStore[1][0].WriteSets[2].SeqInBlock)

	assert.Equal(t, "ns1", pvtDataStore[0][0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
	assert.Equal(t, "ns2", pvtDataStore[1][0].WriteSets[2].WriteSet.NsPvtRwset[0].Namespace)

	assert.Equal(t, "col1", pvtDataStore[0][0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
	assert.Equal(t, "col2", pvtDataStore[1][0].WriteSets[2].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)

	assert.True(t, commitPvtDataOfOldBlocksHappened)
}

func TestReconciliationFailedToCommit(t *testing.T) {
	committer := &mocks.Committer{}
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

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil).Run(func(_ mock.Arguments) {
		missingPvtDataTracker.Mock = mock.Mock{}
		missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(nil, nil)
	})
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

	committer.On("CommitPvtDataOfOldBlocks", mock.Anything).Return(nil, errors.New("failed to commit"))

	r := &Reconciler{channel: "", metrics: metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		config:                &ReconcilerConfig{SleepInterval: time.Minute, BatchSize: 1, IsEnabled: true},
		ReconciliationFetcher: fetcher, Committer: committer}
	err := r.reconcile()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to commit")
}

func TestFailuresWhileReconcilingMissingPvtData(t *testing.T) {
	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	committer.On("GetMissingPvtDataTracker").Return(nil, errors.New("failed to obtain missing pvt data tracker"))

	r := NewReconciler("", metrics, committer, fetcher,
		&ReconcilerConfig{SleepInterval: time.Millisecond * 100, BatchSize: 1, IsEnabled: true})
	err := r.reconcile()
	assert.Error(t, err)
	assert.Contains(t, "failed to obtain missing pvt data tracker", err.Error())

	committer.Mock = mock.Mock{}
	committer.On("GetMissingPvtDataTracker").Return(nil, nil)
	r = NewReconciler("", metrics, committer, fetcher,
		&ReconcilerConfig{SleepInterval: time.Millisecond * 100, BatchSize: 1, IsEnabled: true})
	err = r.reconcile()
	assert.Error(t, err)
	assert.Contains(t, "got nil as MissingPvtDataTracker, exiting...", err.Error())

	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(nil, errors.New("failed get missing pvt data for recent blocks"))

	committer.Mock = mock.Mock{}
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	r = NewReconciler("", metrics, committer, fetcher,
		&ReconcilerConfig{SleepInterval: time.Millisecond * 100, BatchSize: 1, IsEnabled: true})
	err = r.reconcile()
	assert.Error(t, err)
	assert.Contains(t, "failed get missing pvt data for recent blocks", err.Error())
}
