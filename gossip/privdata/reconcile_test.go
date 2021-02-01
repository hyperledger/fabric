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

	gossip2 "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/gossip/metrics"
	gmetricsmocks "github.com/hyperledger/fabric/gossip/metrics/mocks"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNoItemsToReconcile(t *testing.T) {
	// Scenario: there is no missing private data to reconcile.
	// reconciler should identify that we don't have missing data and it doesn't need to call reconciliationFetcher to
	// fetch missing items.
	// reconciler shouldn't get an error.
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	missingInfo := ledger.MissingPvtDataInfo{}

	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(missingInfo, nil)
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	fetcher.On("FetchReconciledItems", mock.Anything).Return(nil, errors.New("this function shouldn't be called"))

	r := &Reconciler{
		channel:                "mychannel",
		logger:                 logger.With("channel", "mychannel"),
		metrics:                metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		ReconcileSleepInterval: time.Minute,
		ReconcileBatchSize:     1,
		ReconciliationFetcher:  fetcher, Committer: committer,
	}
	err := r.reconcile()

	require.NoError(t, err)
}

func TestNotReconcilingWhenCollectionConfigNotAvailable(t *testing.T) {
	// Scenario: reconciler gets an error when trying to read collection config for the missing private data.
	// as a result it removes the digest slice, and there are no digests to pull.
	// shouldn't get an error.
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	configHistoryRetriever := &mocks.ConfigHistoryRetriever{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}

	missingInfo := ledger.MissingPvtDataInfo{
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
		dig2CollectionConfig := args.Get(0).(privdatacommon.Dig2CollectionConfig)
		require.Equal(t, 0, len(dig2CollectionConfig))
		fetchCalled = true
	}).Return(nil, errors.New("called with no digests"))

	r := &Reconciler{
		channel:                "mychannel",
		logger:                 logger.With("channel", "mychannel"),
		metrics:                metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		ReconcileSleepInterval: time.Minute,
		ReconcileBatchSize:     1,
		ReconciliationFetcher:  fetcher, Committer: committer,
	}
	err := r.reconcile()

	require.Error(t, err)
	require.Equal(t, "called with no digests", err.Error())
	require.True(t, fetchCalled)
}

func TestReconciliationHappyPathWithoutScheduler(t *testing.T) {
	// Scenario: happy path when trying to reconcile missing private data.
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	configHistoryRetriever := &mocks.ConfigHistoryRetriever{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}

	missingInfo := ledger.MissingPvtDataInfo{
		3: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			1: {{Collection: "col1", Namespace: "ns1"}},
		},
		4: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			4: {{Collection: "col1", Namespace: "ns1"}},
			5: {{Collection: "col1", Namespace: "ns1"}},
		},
	}

	collectionConfigInfo := ledger.CollectionConfigInfo{
		CollectionConfig: &peer.CollectionConfigPackage{
			Config: []*peer.CollectionConfig{
				{Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
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
		dig2CollectionConfig := args.Get(0).(privdatacommon.Dig2CollectionConfig)
		require.Equal(t, 3, len(dig2CollectionConfig))
		for digest := range dig2CollectionConfig {
			if digest.BlockSeq != 3 {
				// fetch private data only for block 3. Assume that the other
				// block's private data could not be fetched
				continue
			}
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

	expectedUnreconciledMissingData := ledger.MissingPvtDataInfo{
		4: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			4: {{Collection: "col1", Namespace: "ns1"}},
			5: {{Collection: "col1", Namespace: "ns1"}},
		},
	}

	var commitPvtDataOfOldBlocksHappened bool
	var blockNum, seqInBlock uint64
	blockNum = 3
	seqInBlock = 1
	committer.On("CommitPvtDataOfOldBlocks", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		require.Len(t, args, 2)
		reconciledPvtdata := args.Get(0).([]*ledger.ReconciledPvtdata)
		require.Equal(t, 1, len(reconciledPvtdata))
		require.Equal(t, blockNum, reconciledPvtdata[0].BlockNum)
		require.Equal(t, seqInBlock, reconciledPvtdata[0].WriteSets[1].SeqInBlock)
		require.Equal(t, "ns1", reconciledPvtdata[0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
		require.Equal(t, "col1", reconciledPvtdata[0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
		commitPvtDataOfOldBlocksHappened = true

		unreconciledPvtdata := args.Get(1).(ledger.MissingPvtDataInfo)
		require.Equal(t, expectedUnreconciledMissingData, unreconciledPvtdata)
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	testMetricProvider := gmetricsmocks.TestUtilConstructMetricProvider()
	metrics := metrics.NewGossipMetrics(testMetricProvider.FakeProvider).PrivdataMetrics

	r := &Reconciler{
		channel:                "mychannel",
		logger:                 logger.With("channel", "mychannel"),
		metrics:                metrics,
		ReconcileSleepInterval: time.Minute,
		ReconcileBatchSize:     1,
		ReconciliationFetcher:  fetcher, Committer: committer,
	}
	err := r.reconcile()

	require.NoError(t, err)
	require.True(t, commitPvtDataOfOldBlocksHappened)

	require.Equal(t,
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

	missingInfo := ledger.MissingPvtDataInfo{
		3: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			1: {{Collection: "col1", Namespace: "ns1"}},
		},
	}

	collectionConfigInfo := ledger.CollectionConfigInfo{
		CollectionConfig: &peer.CollectionConfigPackage{
			Config: []*peer.CollectionConfig{
				{Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
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
		dig2CollectionConfig := args.Get(0).(privdatacommon.Dig2CollectionConfig)
		require.Equal(t, 1, len(dig2CollectionConfig))
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
	committer.On("CommitPvtDataOfOldBlocks", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		reconciledPvtdata := args.Get(0).([]*ledger.ReconciledPvtdata)
		require.Equal(t, 1, len(reconciledPvtdata))
		require.Equal(t, blockNum, reconciledPvtdata[0].BlockNum)
		require.Equal(t, seqInBlock, reconciledPvtdata[0].WriteSets[1].SeqInBlock)
		require.Equal(t, "ns1", reconciledPvtdata[0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
		require.Equal(t, "col1", reconciledPvtdata[0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
		commitPvtDataOfOldBlocksHappened = true

		require.Nil(t, args.Get(1))
		wg.Done()
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	r := NewReconciler(
		"",
		metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		committer,
		fetcher,
		&PrivdataConfig{
			ReconcileSleepInterval: time.Millisecond * 100,
			ReconcileBatchSize:     1,
			ReconciliationEnabled:  true,
		})
	r.Start()
	wg.Wait()
	r.Stop()

	require.True(t, commitPvtDataOfOldBlocksHappened)
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
		CollectionConfig: &peer.CollectionConfigPackage{
			Config: []*peer.CollectionConfig{
				{Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
						Name: "col1",
					},
				}},
				{Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
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
		dig2CollectionConfig := args.Get(0).(privdatacommon.Dig2CollectionConfig)
		require.Equal(t, 1, len(dig2CollectionConfig))
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
	pvtDataStore := make([][]*ledger.ReconciledPvtdata, 0)
	committer.On("CommitPvtDataOfOldBlocks", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		reconciledPvtdata := args.Get(0).([]*ledger.ReconciledPvtdata)
		require.Equal(t, 1, len(reconciledPvtdata))
		pvtDataStore = append(pvtDataStore, reconciledPvtdata)
		commitPvtDataOfOldBlocksHappened = true

		require.Nil(t, args.Get(1))
		wg.Done()
	}).Return([]*ledger.PvtdataHashMismatch{}, nil)

	r := NewReconciler(
		"",
		metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		committer,
		fetcher,
		&PrivdataConfig{
			ReconcileSleepInterval: time.Millisecond * 100,
			ReconcileBatchSize:     1,
			ReconciliationEnabled:  true,
		})
	r.Start()
	<-stopC
	r.Stop()
	nextC <- struct{}{}
	wg.Wait()

	require.Equal(t, 2, len(pvtDataStore))
	require.Equal(t, uint64(4), pvtDataStore[0][0].BlockNum)
	require.Equal(t, uint64(3), pvtDataStore[1][0].BlockNum)

	require.Equal(t, uint64(1), pvtDataStore[0][0].WriteSets[1].SeqInBlock)
	require.Equal(t, uint64(2), pvtDataStore[1][0].WriteSets[2].SeqInBlock)

	require.Equal(t, "ns1", pvtDataStore[0][0].WriteSets[1].WriteSet.NsPvtRwset[0].Namespace)
	require.Equal(t, "ns2", pvtDataStore[1][0].WriteSets[2].WriteSet.NsPvtRwset[0].Namespace)

	require.Equal(t, "col1", pvtDataStore[0][0].WriteSets[1].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)
	require.Equal(t, "col2", pvtDataStore[1][0].WriteSets[2].WriteSet.NsPvtRwset[0].CollectionPvtRwset[0].CollectionName)

	require.True(t, commitPvtDataOfOldBlocksHappened)
}

func TestReconciliationFailedToCommit(t *testing.T) {
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	configHistoryRetriever := &mocks.ConfigHistoryRetriever{}
	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}

	missingInfo := ledger.MissingPvtDataInfo{
		3: map[uint64][]*ledger.MissingCollectionPvtDataInfo{
			1: {{Collection: "col1", Namespace: "ns1"}},
		},
	}

	collectionConfigInfo := ledger.CollectionConfigInfo{
		CollectionConfig: &peer.CollectionConfigPackage{
			Config: []*peer.CollectionConfig{
				{Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
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
		dig2CollectionConfig := args.Get(0).(privdatacommon.Dig2CollectionConfig)
		require.Equal(t, 1, len(dig2CollectionConfig))
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

	committer.On("CommitPvtDataOfOldBlocks", mock.Anything, mock.Anything).Return(nil, errors.New("failed to commit"))

	r := &Reconciler{
		channel:                "mychannel",
		logger:                 logger.With("channel", "mychannel"),
		metrics:                metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics,
		ReconcileSleepInterval: time.Minute,
		ReconcileBatchSize:     1,
		ReconciliationFetcher:  fetcher, Committer: committer,
	}
	err := r.reconcile()

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to commit")
}

func TestFailuresWhileReconcilingMissingPvtData(t *testing.T) {
	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics
	committer := &mocks.Committer{}
	fetcher := &mocks.ReconciliationFetcher{}
	committer.On("GetMissingPvtDataTracker").Return(nil, errors.New("failed to obtain missing pvt data tracker"))

	r := NewReconciler(
		"",
		metrics,
		committer,
		fetcher,
		&PrivdataConfig{
			ReconcileSleepInterval: time.Millisecond * 100,
			ReconcileBatchSize:     1,
			ReconciliationEnabled:  true,
		})
	err := r.reconcile()
	require.Error(t, err)
	require.Contains(t, "failed to obtain missing pvt data tracker", err.Error())

	committer.Mock = mock.Mock{}
	committer.On("GetMissingPvtDataTracker").Return(nil, nil)
	r = NewReconciler("", metrics, committer, fetcher,
		&PrivdataConfig{ReconcileSleepInterval: time.Millisecond * 100, ReconcileBatchSize: 1, ReconciliationEnabled: true})
	err = r.reconcile()
	require.Error(t, err)
	require.Contains(t, "got nil as MissingPvtDataTracker, exiting...", err.Error())

	missingPvtDataTracker := &mocks.MissingPvtDataTracker{}
	missingPvtDataTracker.On("GetMissingPvtDataInfoForMostRecentBlocks", mock.Anything).Return(nil, errors.New("failed get missing pvt data for recent blocks"))

	committer.Mock = mock.Mock{}
	committer.On("GetMissingPvtDataTracker").Return(missingPvtDataTracker, nil)
	r = NewReconciler("", metrics, committer, fetcher,
		&PrivdataConfig{ReconcileSleepInterval: time.Millisecond * 100, ReconcileBatchSize: 1, ReconciliationEnabled: true})
	err = r.reconcile()
	require.Error(t, err)
	require.Contains(t, "failed get missing pvt data for recent blocks", err.Error())
}

func TestConstructUnreconciledMissingData(t *testing.T) {
	requestedMissingData := privdatacommon.Dig2CollectionConfig{
		privdatacommon.DigKey{
			TxId:       "tx1",
			Namespace:  "ns1",
			Collection: "coll1",
			BlockSeq:   1,
			SeqInBlock: 1,
		}: nil,
		privdatacommon.DigKey{
			TxId:       "tx1",
			Namespace:  "ns2",
			Collection: "coll2",
			BlockSeq:   1,
			SeqInBlock: 1,
		}: nil,
		privdatacommon.DigKey{
			TxId:       "tx1",
			Namespace:  "ns3",
			Collection: "coll3",
			BlockSeq:   1,
			SeqInBlock: 3,
		}: nil,
		privdatacommon.DigKey{
			TxId:       "tx2",
			Namespace:  "ns4",
			Collection: "coll4",
			BlockSeq:   4,
			SeqInBlock: 4,
		}: nil,
	}

	testCases := []struct {
		description                     string
		fetchedData                     []*gossip2.PvtDataElement
		expectedUnreconciledMissingData ledger.MissingPvtDataInfo
	}{
		{
			description: "none-reconciled",
			fetchedData: nil,
			expectedUnreconciledMissingData: ledger.MissingPvtDataInfo{
				1: ledger.MissingBlockPvtdataInfo{
					1: []*ledger.MissingCollectionPvtDataInfo{
						{
							Namespace:  "ns1",
							Collection: "coll1",
						},
						{
							Namespace:  "ns2",
							Collection: "coll2",
						},
					},
					3: []*ledger.MissingCollectionPvtDataInfo{
						{
							Namespace:  "ns3",
							Collection: "coll3",
						},
					},
				},
				4: ledger.MissingBlockPvtdataInfo{
					4: []*ledger.MissingCollectionPvtDataInfo{
						{
							Namespace:  "ns4",
							Collection: "coll4",
						},
					},
				},
			},
		},
		{
			description: "all-reconciled",
			fetchedData: []*gossip2.PvtDataElement{
				{
					Digest: &gossip2.PvtDataDigest{
						TxId:       "tx1",
						Namespace:  "ns1",
						Collection: "coll1",
						BlockSeq:   1,
						SeqInBlock: 1,
					},
				},
				{
					Digest: &gossip2.PvtDataDigest{
						TxId:       "tx1",
						Namespace:  "ns2",
						Collection: "coll2",
						BlockSeq:   1,
						SeqInBlock: 1,
					},
				},
				{
					Digest: &gossip2.PvtDataDigest{
						TxId:       "tx1",
						Namespace:  "ns3",
						Collection: "coll3",
						BlockSeq:   1,
						SeqInBlock: 3,
					},
				},
				{
					Digest: &gossip2.PvtDataDigest{
						TxId:       "tx2",
						Namespace:  "ns4",
						Collection: "coll4",
						BlockSeq:   4,
						SeqInBlock: 4,
					},
				},
			},
			expectedUnreconciledMissingData: nil,
		},
		{
			description: "some-unreconciled",
			fetchedData: []*gossip2.PvtDataElement{
				{
					Digest: &gossip2.PvtDataDigest{
						TxId:       "tx1",
						Namespace:  "ns1",
						Collection: "coll1",
						BlockSeq:   1,
						SeqInBlock: 1,
					},
				},
				{
					Digest: &gossip2.PvtDataDigest{
						TxId:       "tx1",
						Namespace:  "ns3",
						Collection: "coll3",
						BlockSeq:   1,
						SeqInBlock: 3,
					},
				},
				{
					Digest: &gossip2.PvtDataDigest{
						TxId:       "tx2",
						Namespace:  "ns4",
						Collection: "coll4",
						BlockSeq:   4,
						SeqInBlock: 4,
					},
				},
			},
			expectedUnreconciledMissingData: ledger.MissingPvtDataInfo{
				1: ledger.MissingBlockPvtdataInfo{
					1: []*ledger.MissingCollectionPvtDataInfo{
						{
							Namespace:  "ns2",
							Collection: "coll2",
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			unreconciledData := constructUnreconciledMissingData(requestedMissingData, testCase.fetchedData)
			require.Equal(t, len(testCase.expectedUnreconciledMissingData), len(unreconciledData))
			for blkNum, txsMissingData := range testCase.expectedUnreconciledMissingData {
				for txNum, expectedUnreconciledData := range txsMissingData {
					require.ElementsMatch(t, expectedUnreconciledData, unreconciledData[blkNum][txNum])
				}
			}
		})
	}
}
