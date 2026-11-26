/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	gossip2 "github.com/hyperledger/fabric-protos-go-apiv2/gossip"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	mspproto "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-protos-go-apiv2/transientstore"
	"github.com/hyperledger/fabric/core/ledger"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

/*
Test checks following scenario, it tries to obtain private data for
given block sequence which is greater than available ledger height,
hence data should be looked up directly from transient store
*/
func TestNewDataRetriever_GetDataFromTransientStore(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"
	txID := "testTxID"

	committer.On("LedgerHeight").Return(uint64(1), nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).Return(newCollectionConfig(collectionName), nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	store.Persist(txID, 2, &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: &rwset.TxPvtReadWriteSet{
			DataModel: rwset.TxReadWriteSet_KV,
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				pvtReadWriteSet(namespace, collectionName, []byte{1, 2}),
				pvtReadWriteSet(namespace, collectionName, []byte{3, 4}),
			},
		},
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			namespace: {
				Config: []*peer.CollectionConfig{
					{
						Payload: &peer.CollectionConfig_StaticCollectionConfig{
							StaticCollectionConfig: &peer.StaticCollectionConfig{
								Name: collectionName,
							},
						},
					},
				},
			},
		},
	})

	// Request digest for private data which is greater than current ledger height
	// to make it query transient store for missed private data
	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   2,
		TxId:       txID,
		SeqInBlock: 1,
	}}, 2)

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotEmpty(rwSets)
	dig2pvtRWSet := rwSets[privdatacommon.DigKey{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   2,
		TxId:       txID,
		SeqInBlock: 1,
	}]
	assertion.NotNil(dig2pvtRWSet)
	pvtRWSets := dig2pvtRWSet.RWSet
	assertion.Len(pvtRWSets, 2)

	var mergedRWSet []byte
	for _, rws := range pvtRWSets {
		mergedRWSet = append(mergedRWSet, rws...)
	}

	assertion.Equal([]byte{1, 2, 3, 4}, mergedRWSet)
}

/*
Simple test case where available ledger height is greater than
requested block sequence and therefore private data will be retrieved
from the ledger rather than transient store as data being committed
*/
func TestNewDataRetriever_GetDataFromLedger(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"

	result := []*ledger.TxPvtData{{
		WriteSet: &rwset.TxPvtReadWriteSet{
			DataModel: rwset.TxReadWriteSet_KV,
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				pvtReadWriteSet(namespace, collectionName, []byte{1, 2}),
				pvtReadWriteSet(namespace, collectionName, []byte{3, 4}),
			},
		},
		SeqInBlock: 1,
	}}

	committer.On("LedgerHeight").Return(uint64(10), nil)
	committer.On("GetPvtDataByNum", uint64(5), mock.Anything).Return(result, nil)

	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).Return(newCollectionConfig(collectionName), nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	// Request digest for private data which is greater than current ledger height
	// to make it query ledger for missed private data
	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, uint64(5))

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotEmpty(rwSets)
	pvtRWSet := rwSets[privdatacommon.DigKey{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   5,
		TxId:       "testTxID",
		SeqInBlock: 1,
	}]
	assertion.NotEmpty(pvtRWSet)
	assertion.Len(pvtRWSet.RWSet, 2)

	var mergedRWSet []byte
	for _, rws := range pvtRWSet.RWSet {
		mergedRWSet = append(mergedRWSet, rws...)
	}

	assertion.Equal([]byte{1, 2, 3, 4}, mergedRWSet)
}

func TestNewDataRetriever_FailGetPvtDataFromLedger(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"

	committer.On("LedgerHeight").Return(uint64(10), nil)
	committer.On("GetPvtDataByNum", uint64(5), mock.Anything).
		Return(nil, errors.New("failing retrieving private data"))

	retriever := NewDataRetriever("testchannel", store.store, committer)

	// Request digest for private data which is greater than current ledger height
	// to make it query transient store for missed private data
	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, uint64(5))

	assertion := require.New(t)
	assertion.Error(err)
	assertion.Empty(rwSets)
}

func TestNewDataRetriever_GetOnlyRelevantPvtData(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"

	result := []*ledger.TxPvtData{{
		WriteSet: &rwset.TxPvtReadWriteSet{
			DataModel: rwset.TxReadWriteSet_KV,
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				pvtReadWriteSet(namespace, collectionName, []byte{1}),
				pvtReadWriteSet(namespace, collectionName, []byte{2}),
				pvtReadWriteSet("invalidNamespace", collectionName, []byte{0, 0}),
				pvtReadWriteSet(namespace, "invalidCollectionName", []byte{0, 0}),
			},
		},
		SeqInBlock: 1,
	}}

	committer.On("LedgerHeight").Return(uint64(10), nil)
	committer.On("GetPvtDataByNum", uint64(5), mock.Anything).Return(result, nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).Return(newCollectionConfig(collectionName), nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	// Request digest for private data which is greater than current ledger height
	// to make it query transient store for missed private data
	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, 5)

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotEmpty(rwSets)
	pvtRWSet := rwSets[privdatacommon.DigKey{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   5,
		TxId:       "testTxID",
		SeqInBlock: 1,
	}]
	assertion.NotEmpty(pvtRWSet)
	assertion.Len(pvtRWSet.RWSet, 2)

	var mergedRWSet []byte
	for _, rws := range pvtRWSet.RWSet {
		mergedRWSet = append(mergedRWSet, rws...)
	}

	assertion.Equal([]byte{1, 2}, mergedRWSet)
}

func TestNewDataRetriever_GetMultipleDigests(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	ns1, ns2 := "testChaincodeName1", "testChaincodeName2"
	col1, col2 := "testCollectionName1", "testCollectionName2"

	result := []*ledger.TxPvtData{
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns1, col1, []byte{1}),
					pvtReadWriteSet(ns1, col1, []byte{2}),
					pvtReadWriteSet("invalidNamespace", col1, []byte{0, 0}),
					pvtReadWriteSet(ns1, "invalidCollectionName", []byte{0, 0}),
				},
			},
			SeqInBlock: 1,
		},
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns2, col2, []byte{3}),
					pvtReadWriteSet(ns2, col2, []byte{4}),
					pvtReadWriteSet("invalidNamespace", col2, []byte{0, 0}),
					pvtReadWriteSet(ns2, "invalidCollectionName", []byte{0, 0}),
				},
			},
			SeqInBlock: 2,
		},
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns1, col1, []byte{5}),
					pvtReadWriteSet(ns2, col2, []byte{6}),
					pvtReadWriteSet("invalidNamespace", col2, []byte{0, 0}),
					pvtReadWriteSet(ns2, "invalidCollectionName", []byte{0, 0}),
				},
			},
			SeqInBlock: 3,
		},
	}

	committer.On("LedgerHeight").Return(uint64(10), nil)
	committer.On("GetPvtDataByNum", uint64(5), mock.Anything).Return(result, nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, ns1).Return(newCollectionConfig(col1), nil)
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, ns2).Return(newCollectionConfig(col2), nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	// Request digest for private data which is greater than current ledger height
	// to make it query transient store for missed private data
	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  ns1,
		Collection: col1,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}, {
		Namespace:  ns2,
		Collection: col2,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 2,
	}}, 5)

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotEmpty(rwSets)
	assertion.Len(rwSets, 2)

	pvtRWSet := rwSets[privdatacommon.DigKey{
		Namespace:  ns1,
		Collection: col1,
		BlockSeq:   5,
		TxId:       "testTxID",
		SeqInBlock: 1,
	}]
	assertion.NotEmpty(pvtRWSet)
	assertion.Len(pvtRWSet.RWSet, 2)

	var mergedRWSet []byte
	for _, rws := range pvtRWSet.RWSet {
		mergedRWSet = append(mergedRWSet, rws...)
	}

	pvtRWSet = rwSets[privdatacommon.DigKey{
		Namespace:  ns2,
		Collection: col2,
		BlockSeq:   5,
		TxId:       "testTxID",
		SeqInBlock: 2,
	}]
	assertion.NotEmpty(pvtRWSet)
	assertion.Len(pvtRWSet.RWSet, 2)
	for _, rws := range pvtRWSet.RWSet {
		mergedRWSet = append(mergedRWSet, rws...)
	}

	assertion.Equal([]byte{1, 2, 3, 4}, mergedRWSet)
}

func TestNewDataRetriever_EmptyWriteSet(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	ns1 := "testChaincodeName1"
	col1 := "testCollectionName1"

	result := []*ledger.TxPvtData{
		{
			SeqInBlock: 1,
		},
	}

	committer.On("LedgerHeight").Return(uint64(10), nil)
	committer.On("GetPvtDataByNum", uint64(5), mock.Anything).Return(result, nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, ns1).Return(newCollectionConfig(col1), nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  ns1,
		Collection: col1,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, 5)

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotEmpty(rwSets)

	pvtRWSet := rwSets[privdatacommon.DigKey{
		Namespace:  ns1,
		Collection: col1,
		BlockSeq:   5,
		TxId:       "testTxID",
		SeqInBlock: 1,
	}]
	assertion.NotEmpty(pvtRWSet)
	assertion.Empty(pvtRWSet.RWSet)
}

func TestNewDataRetriever_FailedObtainConfigHistoryRetriever(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	ns1 := "testChaincodeName1"
	col1 := "testCollectionName1"

	result := []*ledger.TxPvtData{
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns1, col1, []byte{1}),
					pvtReadWriteSet(ns1, col1, []byte{2}),
				},
			},
			SeqInBlock: 1,
		},
	}

	committer.On("LedgerHeight").Return(uint64(10), nil)
	committer.On("GetPvtDataByNum", uint64(5), mock.Anything).Return(result, nil)
	committer.On("GetConfigHistoryRetriever").Return(nil, errors.New("failed to obtain ConfigHistoryRetriever"))

	retriever := NewDataRetriever("testchannel", store.store, committer)

	_, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  ns1,
		Collection: col1,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, 5)

	assertion := require.New(t)
	assertion.Contains(err.Error(), "failed to obtain ConfigHistoryRetriever")
}

func TestNewDataRetriever_NoCollectionConfig(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	ns1, ns2 := "testChaincodeName1", "testChaincodeName2"
	col1, col2 := "testCollectionName1", "testCollectionName2"

	result := []*ledger.TxPvtData{
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns1, col1, []byte{1}),
					pvtReadWriteSet(ns1, col1, []byte{2}),
				},
			},
			SeqInBlock: 1,
		},
		{
			WriteSet: &rwset.TxPvtReadWriteSet{
				DataModel: rwset.TxReadWriteSet_KV,
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					pvtReadWriteSet(ns2, col2, []byte{3}),
					pvtReadWriteSet(ns2, col2, []byte{4}),
				},
			},
			SeqInBlock: 2,
		},
	}

	committer.On("LedgerHeight").Return(uint64(10), nil)
	committer.On("GetPvtDataByNum", uint64(5), mock.Anything).Return(result, nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, ns1).
		Return(newCollectionConfig(col1), errors.New("failed to obtain collection config"))
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, ns2).
		Return(nil, nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)
	assertion := require.New(t)

	_, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  ns1,
		Collection: col1,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, 5)
	assertion.Error(err)
	assertion.Contains(err.Error(), "cannot find recent collection config update below block sequence")

	_, _, err = retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  ns2,
		Collection: col2,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 2,
	}}, 5)
	assertion.Error(err)
	assertion.Contains(err.Error(), "no collection config update below block sequence")
}

func TestNewDataRetriever_FailedGetLedgerHeight(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	ns1 := "testChaincodeName1"
	col1 := "testCollectionName1"

	committer.On("LedgerHeight").Return(uint64(0), errors.New("failed to read ledger height"))
	retriever := NewDataRetriever("testchannel", store.store, committer)

	_, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  ns1,
		Collection: col1,
		BlockSeq:   uint64(5),
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, 5)

	assertion := require.New(t)
	assertion.Error(err)
	assertion.Contains(err.Error(), "failed to read ledger height")
}

func TestNewDataRetriever_EmptyPvtRWSetInTransientStore(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"

	committer.On("LedgerHeight").Return(uint64(1), nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   2,
		TxId:       "testTxID",
		SeqInBlock: 1,
	}}, 2)

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.NotEmpty(rwSets)
	assertion.Empty(rwSets[privdatacommon.DigKey{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   2,
		TxId:       "testTxID",
		SeqInBlock: 1,
	}])
}

/*
Test checks that all write sets the transient store holds for a txID are returned
under the collection config committed to the ledger, whatever collection config the
entries themselves were stored with. The stored config is supplied by whichever peer
wrote the entry, so it decides neither who the data is released to nor what is served.
*/
func TestNewDataRetriever_TransientStoreEntriesWithDifferentConfigs(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"
	txID := "testTxID"

	ledgerConfig := collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org1"), 3)

	committer.On("LedgerHeight").Return(uint64(1), nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).Return(&ledger.CollectionConfigInfo{
		CollectionConfig: &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{ledgerConfig}},
	}, nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	// Two entries endorsed under the ledger's member orgs policy, with a different MaximumPeerCount.
	store.Persist(txID, 2, transientStoreEntry(namespace, collectionName, []byte{1, 2},
		collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org1"), 1)))
	store.Persist(txID, 2, transientStoreEntry(namespace, collectionName, []byte{3, 4},
		collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org1"), 2)))
	// An entry pushed with a member orgs policy that differs from the ledger's.
	store.Persist(txID, 2, transientStoreEntry(namespace, collectionName, []byte{5, 6},
		collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org2"), 1)))

	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   2,
		TxId:       txID,
		SeqInBlock: 1,
	}}, 2)

	assertion := require.New(t)
	assertion.NoError(err)
	dig2pvtRWSet := rwSets[privdatacommon.DigKey{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   2,
		TxId:       txID,
		SeqInBlock: 1,
	}]
	assertion.NotNil(dig2pvtRWSet)
	assertion.True(proto.Equal(ledgerConfig, dig2pvtRWSet.CollectionConfig))
	assertion.ElementsMatch([]util.PrivateRWSet{{1, 2}, {3, 4}, {5, 6}}, dig2pvtRWSet.RWSet)
}

/*
Test checks that an entry whose member orgs policy is not the one the ledger holds for
the collection is still served under the ledger config. This happens in both directions
around a collection config update: a transaction endorsed under the previous policy, and
an entry endorsed under the new policy arriving at a peer that has not committed the
update yet. Eligibility comes from the ledger config either way, so the entry is served
rather than withheld until the block that carries the transaction commits.
*/
func TestNewDataRetriever_TransientStoreEntryUnderPreviousPolicy(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"
	txID := "testTxID"

	// The collection was updated so that org2 replaced org1 as its member; the
	// transaction was endorsed before the update under the previous policy.
	ledgerConfig := collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org2"), 1)

	committer.On("LedgerHeight").Return(uint64(1), nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).Return(&ledger.CollectionConfigInfo{
		CollectionConfig: &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{ledgerConfig}},
	}, nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	store.Persist(txID, 2, transientStoreEntry(namespace, collectionName, []byte{1, 2},
		collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org1"), 1)))

	digest := &gossip2.PvtDataDigest{
		Namespace:  namespace,
		Collection: collectionName,
		BlockSeq:   2,
		TxId:       txID,
		SeqInBlock: 1,
	}
	rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{digest}, 2)

	assertion := require.New(t)
	assertion.NoError(err)
	dig2pvtRWSet := rwSets[*toDigKey(digest)]
	assertion.NotNil(dig2pvtRWSet)
	assertion.True(proto.Equal(ledgerConfig, dig2pvtRWSet.CollectionConfig))
	assertion.Equal([]util.PrivateRWSet{{1, 2}}, dig2pvtRWSet.RWSet)
}

/*
Test checks that no private data is returned from the transient store when the
collection config cannot be obtained from the ledger, since there is no trustworthy
config to evaluate the requester against. This covers a ledger with no config below
the digest's block, a config package without the digest's collection, and a failure
of either step of the lookup.
*/
func TestNewDataRetriever_TransientStoreNoLedgerCollectionConfig(t *testing.T) {
	namespace := "testChaincodeName1"
	collectionName := "testCollectionName"
	txID := "testTxID"

	tests := []struct {
		name          string
		configHistory func() (ledger.ConfigHistoryRetriever, error)
	}{
		{
			name: "no collection config below the block",
			configHistory: func() (ledger.ConfigHistoryRetriever, error) {
				historyRetreiver := &mocks.ConfigHistoryRetriever{}
				historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).Return(nil, nil)
				return historyRetreiver, nil
			},
		},
		{
			name: "the collection is not in the ledger config",
			configHistory: func() (ledger.ConfigHistoryRetriever, error) {
				historyRetreiver := &mocks.ConfigHistoryRetriever{}
				historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).Return(&ledger.CollectionConfigInfo{
					CollectionConfig: &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{
						collectionConfigWithPolicy("anotherCollectionName", memberOrgsPolicy("org1"), 1),
					}},
				}, nil)
				return historyRetreiver, nil
			},
		},
		{
			name: "the config history retriever cannot be obtained",
			configHistory: func() (ledger.ConfigHistoryRetriever, error) {
				return nil, errors.New("failed to obtain ConfigHistoryRetriever")
			},
		},
		{
			name: "the collection config lookup fails",
			configHistory: func() (ledger.ConfigHistoryRetriever, error) {
				historyRetreiver := &mocks.ConfigHistoryRetriever{}
				historyRetreiver.On("MostRecentCollectionConfigBelow", mock.Anything, namespace).
					Return(nil, errors.New("failed to obtain collection config"))
				return historyRetreiver, nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newTransientStore(t)
			defer store.tearDown()

			committer := &mocks.Committer{}
			committer.On("LedgerHeight").Return(uint64(1), nil)
			committer.On("GetConfigHistoryRetriever").Return(test.configHistory())

			retriever := NewDataRetriever("testchannel", store.store, committer)

			store.Persist(txID, 2, transientStoreEntry(namespace, collectionName, []byte{1, 2},
				collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org1"), 1)))

			digest := &gossip2.PvtDataDigest{
				Namespace:  namespace,
				Collection: collectionName,
				BlockSeq:   2,
				TxId:       txID,
				SeqInBlock: 1,
			}
			rwSets, _, err := retriever.CollectionRWSet([]*gossip2.PvtDataDigest{digest}, 2)

			assertion := require.New(t)
			assertion.NoError(err)
			dig2pvtRWSet := rwSets[*toDigKey(digest)]
			assertion.NotNil(dig2pvtRWSet)
			assertion.Nil(dig2pvtRWSet.CollectionConfig)
			assertion.Empty(dig2pvtRWSet.RWSet)
		})
	}
}

/*
Test checks that the collection config is read from the ledger once per namespace and
block sequence within a single CollectionRWSet call, rather than once per digest. The
lookup creates a query executor and reads the state DB for implicit collections, and a
pull request for one block commonly carries a digest per collection.
*/
func TestNewDataRetriever_TransientStoreLedgerConfigLookupCached(t *testing.T) {
	committer := &mocks.Committer{}

	store := newTransientStore(t)
	defer store.tearDown()

	namespace := "testChaincodeName1"
	collections := []string{"testCollectionName1", "testCollectionName2", "testCollectionName3"}
	txID := "testTxID"

	configPackage := &peer.CollectionConfigPackage{}
	for _, collectionName := range collections {
		configPackage.Config = append(configPackage.Config,
			collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org1"), 1))
	}

	committer.On("LedgerHeight").Return(uint64(1), nil)
	historyRetreiver := &mocks.ConfigHistoryRetriever{}
	historyRetreiver.On("MostRecentCollectionConfigBelow", uint64(2), namespace).
		Return(&ledger.CollectionConfigInfo{CollectionConfig: configPackage}, nil)
	committer.On("GetConfigHistoryRetriever").Return(historyRetreiver, nil)

	retriever := NewDataRetriever("testchannel", store.store, committer)

	var digests []*gossip2.PvtDataDigest
	for _, collectionName := range collections {
		store.Persist(txID, 2, transientStoreEntry(namespace, collectionName, []byte{1, 2},
			collectionConfigWithPolicy(collectionName, memberOrgsPolicy("org1"), 1)))
		digests = append(digests, &gossip2.PvtDataDigest{
			Namespace:  namespace,
			Collection: collectionName,
			BlockSeq:   2,
			TxId:       txID,
			SeqInBlock: 1,
		})
	}

	rwSets, _, err := retriever.CollectionRWSet(digests, 2)

	assertion := require.New(t)
	assertion.NoError(err)
	assertion.Len(rwSets, len(collections))
	for _, digest := range digests {
		dig2pvtRWSet := rwSets[*toDigKey(digest)]
		assertion.NotNil(dig2pvtRWSet)
		assertion.Equal([]util.PrivateRWSet{{1, 2}}, dig2pvtRWSet.RWSet)
	}
	committer.AssertNumberOfCalls(t, "GetConfigHistoryRetriever", 1)
	historyRetreiver.AssertNumberOfCalls(t, "MostRecentCollectionConfigBelow", 1)
}

func memberOrgsPolicy(mspID string) *peer.CollectionPolicyConfig {
	return &peer.CollectionPolicyConfig{
		Payload: &peer.CollectionPolicyConfig_SignaturePolicy{
			SignaturePolicy: &common.SignaturePolicyEnvelope{
				Rule: &common.SignaturePolicy{
					Type: &common.SignaturePolicy_SignedBy{SignedBy: 0},
				},
				Identities: []*mspproto.MSPPrincipal{{
					PrincipalClassification: mspproto.MSPPrincipal_ROLE,
					Principal: protoutil.MarshalOrPanic(&mspproto.MSPRole{
						MspIdentifier: mspID,
						Role:          mspproto.MSPRole_MEMBER,
					}),
				}},
			},
		},
	}
}

func collectionConfigWithPolicy(collectionName string, policy *peer.CollectionPolicyConfig, maxPeers int32) *peer.CollectionConfig {
	return &peer.CollectionConfig{
		Payload: &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name:             collectionName,
				MemberOrgsPolicy: policy,
				MaximumPeerCount: maxPeers,
			},
		},
	}
}

func transientStoreEntry(namespace, collectionName string, data []byte, config *peer.CollectionConfig) *transientstore.TxPvtReadWriteSetWithConfigInfo {
	return &transientstore.TxPvtReadWriteSetWithConfigInfo{
		PvtRwset: &rwset.TxPvtReadWriteSet{
			DataModel:  rwset.TxReadWriteSet_KV,
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{pvtReadWriteSet(namespace, collectionName, data)},
		},
		CollectionConfigs: map[string]*peer.CollectionConfigPackage{
			namespace: {Config: []*peer.CollectionConfig{config}},
		},
	}
}

func newCollectionConfig(collectionName string) *ledger.CollectionConfigInfo {
	return &ledger.CollectionConfigInfo{
		CollectionConfig: &peer.CollectionConfigPackage{
			Config: []*peer.CollectionConfig{
				{
					Payload: &peer.CollectionConfig_StaticCollectionConfig{
						StaticCollectionConfig: &peer.StaticCollectionConfig{
							Name: collectionName,
						},
					},
				},
			},
		},
	}
}

func pvtReadWriteSet(ns string, collectionName string, data []byte) *rwset.NsPvtReadWriteSet {
	return &rwset.NsPvtReadWriteSet{
		Namespace: ns,
		CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{{
			CollectionName: collectionName,
			Rwset:          data,
		}},
	}
}
