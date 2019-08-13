/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"testing"

	pb "github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	transientstore2 "github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	util2 "github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/metrics"
	privdatacommon "github.com/hyperledger/fabric/gossip/privdata/common"
	"github.com/hyperledger/fabric/gossip/privdata/mocks"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testSupport struct {
	preHash, hash []byte
	channelID     string
	blockNum      uint64
	identity      msp.SigningIdentity
	endorsers     []string
}

type rwSet struct {
	txID          string
	namespace     string
	collections   []string
	preHash, hash []byte
	endorsers     []string
	ineligible    bool
	invalid       bool
	missing       bool
	seqInBlock    uint64
}

func init() {
	util.SetupTestLoggingWithLevel("INFO")
}

func TestRetrievePrivateData(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity := mspmgmt.GetLocalSigningIdentityOrPanic()

	ts := testSupport{
		preHash:   []byte("rws-pre-image"),
		hash:      util2.ComputeSHA256([]byte("rws-pre-image")),
		channelID: util2.GetTestChannelID(),
		blockNum:  uint64(1),
		identity:  identity,
		endorsers: []string{identity.GetMSPIdentifier()},
	}

	tests := []struct {
		scenario                                                            string
		storePvtdataOfInvalidTx                                             bool
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer, rwSetsNotFound []rwSet
	}{
		{
			// Scenario I
			scenario:                "Scenario I: Only eligible private data in cache, no missing private data",
			storePvtdataOfInvalidTx: true,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1", "c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  1,
				},
			},
			rwSetsInTransientStore: []rwSet{},
			rwSetsInPeer:           []rwSet{},
			rwSetsNotFound:         []rwSet{},
		},
		{
			// Scenario II
			scenario:                "Scenario II: No eligible private data, skip ineligible private data from all sources even if found in cache",
			storePvtdataOfInvalidTx: true,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					ineligible:  true,
					seqInBlock:  1,
				},
			},
			rwSetsInTransientStore: []rwSet{
				{
					txID:        "tx2",
					namespace:   "ns2",
					collections: []string{"c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					ineligible:  true,
					seqInBlock:  2,
				},
			},
			rwSetsInPeer: []rwSet{
				{
					txID:        "tx3",
					namespace:   "ns3",
					collections: []string{"c3"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					ineligible:  true,
					seqInBlock:  3,
				},
			},
			rwSetsNotFound: []rwSet{},
		},
		{
			// Scenario III
			scenario:                "Scenario III: Missing private data in cache, found in transient store",
			storePvtdataOfInvalidTx: true,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1", "c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  1,
				},
			},
			rwSetsInTransientStore: []rwSet{
				{
					txID:        "tx2",
					namespace:   "ns1",
					collections: []string{"c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  2,
				},
			},
			rwSetsInPeer:   []rwSet{},
			rwSetsNotFound: []rwSet{},
		},
		{
			// Scenario IV
			scenario:                "Scenario IV: Missing private data in cache, found some in transient store and some in peer",
			storePvtdataOfInvalidTx: true,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1", "c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  1,
				},
			},
			rwSetsInTransientStore: []rwSet{
				{
					txID:        "tx2",
					namespace:   "ns1",
					collections: []string{"c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  2,
				},
			},
			rwSetsInPeer: []rwSet{
				{
					txID:        "tx3",
					namespace:   "ns1",
					collections: []string{"c3"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  3,
				},
			},
			rwSetsNotFound: []rwSet{},
		},
		{
			// Scenario V
			scenario:                "Scenario V: Skip invalid txs when storePvtdataOfInvalidTx is false",
			storePvtdataOfInvalidTx: false,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					invalid:     true,
					seqInBlock:  1,
				},
				{
					txID:        "tx2",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  2,
				},
			},
			rwSetsInTransientStore: []rwSet{},
			rwSetsInPeer:           []rwSet{},
			rwSetsNotFound:         []rwSet{},
		},
		{
			// Scenario VI
			scenario:                "Scenario VI: Don't skip invalid txs when storePvtdataOfInvalidTx is true",
			storePvtdataOfInvalidTx: true,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					invalid:     true,
					seqInBlock:  1,
				},
				{
					txID:        "tx2",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  2,
				},
			},
			rwSetsInTransientStore: []rwSet{},
			rwSetsInPeer:           []rwSet{},
			rwSetsNotFound:         []rwSet{},
		},
		{
			// Scenario VII
			scenario:                "Scenario VII: Can't find eligible tx from any source",
			storePvtdataOfInvalidTx: true,
			rwSetsInCache:           []rwSet{},
			rwSetsInTransientStore:  []rwSet{},
			rwSetsInPeer:            []rwSet{},
			rwSetsNotFound: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1, c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					missing:     true,
					seqInBlock:  1,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			testRetrievePrivateDataSuccess(t, test.scenario, ts, test.storePvtdataOfInvalidTx,
				test.rwSetsInCache, test.rwSetsInTransientStore, test.rwSetsInPeer, test.rwSetsNotFound)
		})
	}
}

func TestRetrievePrivateDataFailure(t *testing.T) {
	var (
		scenario                                            string
		storePvtdataOfInvalidTx                             bool
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet
		rwSetsNotFound                                      []rwSet
		rwSetsQuery                                         []rwSet
		txPvtdataInfo                                       []*ledger.TxPvtdataInfo
	)

	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity := mspmgmt.GetLocalSigningIdentityOrPanic()

	ts := testSupport{
		preHash:   []byte("rws-pre-image"),
		hash:      util2.ComputeSHA256([]byte("rws-pre-image")),
		channelID: util2.GetTestChannelID(),
		blockNum:  uint64(1),
		identity:  identity,
		endorsers: []string{identity.GetMSPIdentifier()},
	}

	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))

	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))

	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	scenario = "\nScenario I: Invalid collection config policy"
	storePvtdataOfInvalidTx = true
	rwSetsInCache = []rwSet{}
	rwSetsInTransientStore = []rwSet{}
	rwSetsInPeer = []rwSet{}
	rwSetsNotFound = []rwSet{
		{
			txID:        "tx1",
			namespace:   "ns1",
			collections: []string{"c1"},
			preHash:     ts.preHash,
			hash:        ts.hash,
			missing:     true,
			seqInBlock:  1,
		},
	}
	rwSetsQuery = append(rwSetsInCache, rwSetsInTransientStore...)
	rwSetsQuery = append(rwSetsQuery, rwSetsInPeer...)
	rwSetsQuery = append(rwSetsQuery, rwSetsNotFound...)

	txPvtdataInfo = formatTxPvtdataInfo(rwSetsQuery, peerSelfSignedData.Signature, ts)
	txPvtdataInfo[0].CollectionPvtdataInfo[0].CollectionConfig.MemberOrgsPolicy = nil
	expectedErr := "Collection config policy is nil"

	testRetrievePrivateDataFailure(t, scenario, ts,
		peerSelfSignedData, storePvtdataOfInvalidTx,
		txPvtdataInfo,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound,
		expectedErr)
}

func TestRetryFetchFromPeer(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))
	identity := mspmgmt.GetLocalSigningIdentityOrPanic()

	ts := testSupport{
		preHash:   []byte("rws-pre-image"),
		hash:      util2.ComputeSHA256([]byte("rws-pre-image")),
		channelID: util2.GetTestChannelID(),
		blockNum:  uint64(1),
		identity:  identity,
		endorsers: []string{identity.GetMSPIdentifier()},
	}

	serializedID, err := identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))

	data := []byte{1, 2, 3}
	signature, err := identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))

	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	tempdir, err := ioutil.TempDir("", "ts")
	require.NoError(t, err, fmt.Sprintf("Failed to create test directory, got err %s", err))
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))

	defer storeProvider.Close()
	defer os.RemoveAll(tempdir)

	storePvtdataOfInvalidTx := true
	rwSetsInCache := []rwSet{}
	rwSetsInTransientStore := []rwSet{}
	rwSetsInPeer := []rwSet{}
	rwSetsNotFound := []rwSet{
		{
			txID:        "tx1",
			namespace:   "ns1",
			collections: []string{"c1, c2"},
			preHash:     ts.preHash,
			hash:        ts.hash,
			missing:     true,
			seqInBlock:  1,
		},
	}
	pdp := setupPrivateDataProvider(t, ts, peerSelfSignedData,
		storePvtdataOfInvalidTx, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound)
	require.NotNil(t, pdp)

	fakeSleeper := &mocks.Sleeper{}
	SetSleeper(pdp, fakeSleeper)

	rwSetsQuery := append(rwSetsInCache, rwSetsInTransientStore...)
	rwSetsQuery = append(rwSetsQuery, rwSetsInPeer...)
	rwSetsQuery = append(rwSetsQuery, rwSetsNotFound...)

	txPvtdataQuery := formatTxPvtdataInfo(rwSetsQuery, signature, ts)

	_, err = pdp.RetrievePrivatedata(txPvtdataQuery)
	assert.NoError(t, err)
	var maxRetries int

	maxRetries = int(testConfig.PullRetryThreshold / pullRetrySleepInterval)
	assert.Equal(t, fakeSleeper.SleepCallCount() <= maxRetries, true)
	assert.Equal(t, fakeSleeper.SleepArgsForCall(0), pullRetrySleepInterval)
}

func testRetrievePrivateDataSuccess(t *testing.T,
	scenario string,
	ts testSupport,
	storePvtdataOfInvalidTx bool,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	rwSetsNotFound []rwSet) {

	fmt.Println("\n" + scenario)

	serializedID, err := ts.identity.Serialize()
	require.NoError(t, err, fmt.Sprintf("Serialize should have succeeded, got err %s", err))

	data := []byte{1, 2, 3}
	signature, err := ts.identity.Sign(data)
	require.NoError(t, err, fmt.Sprintf("Could not sign identity, got err %s", err))

	peerSelfSignedData := protoutil.SignedData{
		Identity:  serializedID,
		Signature: signature,
		Data:      data,
	}

	tempdir, err := ioutil.TempDir("", "ts")
	require.NoError(t, err, fmt.Sprintf("Failed to create test directory, got err %s", err))
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))
	defer storeProvider.Close()
	defer os.RemoveAll(tempdir)

	pdp := setupPrivateDataProvider(t, ts, peerSelfSignedData,
		storePvtdataOfInvalidTx, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound)
	require.NotNil(t, pdp, scenario)

	rwSetsQuery := append(rwSetsInCache, rwSetsInTransientStore...)
	rwSetsQuery = append(rwSetsQuery, rwSetsInPeer...)
	rwSetsQuery = append(rwSetsQuery, rwSetsNotFound...)

	txPvtdataQuery := formatTxPvtdataInfo(rwSetsQuery, signature, ts)

	retrievedPvtdata, err := pdp.RetrievePrivatedata(txPvtdataQuery)
	assert.NoError(t, err, scenario)

	expectedRwSets := []rwSet{}
	for _, rws := range rwSetsQuery {
		if !rws.invalid || storePvtdataOfInvalidTx {
			expectedRwSets = append(expectedRwSets, rws)
		}
	}

	expectedBlockPvtdata := formatExpectedBlockPvtdata(expectedRwSets)
	// sometimes the collection private write sets are added out of order
	// so we need to sort it to check equality with expected
	blockPvtdata := sortBlockPvtdata(retrievedPvtdata.blockPvtdata)
	assert.Equal(t, expectedBlockPvtdata, blockPvtdata, scenario)

	// Test pvtdata is purged from store on Done() call
	testPurged(t, scenario, retrievedPvtdata, store, txPvtdataQuery)
}

func testRetrievePrivateDataFailure(t *testing.T,
	scenario string,
	ts testSupport,
	peerSelfSignedData protoutil.SignedData,
	storePvtdataOfInvalidTx bool,
	txPvtdataInfo []*ledger.TxPvtdataInfo,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	rwSetsNotFound []rwSet,
	expectedErr string) {

	fmt.Println("\n" + scenario)

	tempdir, err := ioutil.TempDir("", "ts")
	require.NoError(t, err, fmt.Sprintf("Failed to create test directory, got err %s", err))
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))
	defer storeProvider.Close()
	defer os.RemoveAll(tempdir)

	pdp := setupPrivateDataProvider(t, ts, peerSelfSignedData,
		storePvtdataOfInvalidTx, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound)
	require.NotNil(t, pdp, scenario)

	_, err = pdp.RetrievePrivatedata(txPvtdataInfo)
	assert.EqualError(t, err, expectedErr, scenario)
}

func setupPrivateDataProvider(t *testing.T,
	ts testSupport,
	peerSelfSignedData protoutil.SignedData,
	storePvtdataOfInvalidTx bool, store *transientstore.Store,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	rwSetsNotFound []rwSet) *PvtdataProvider {

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain(ts.channelID)
	})

	// set up data in cache
	cachedPvtdata := storePvtdataInCache(rwSetsInCache)
	// set up data in transient store
	err := storePvtdataInTransientStore(rwSetsInTransientStore, store)
	require.NoError(t, err, fmt.Sprintf("Failed to store private data in transient store: got err %s", err))

	// set up data in peer
	fetcher := &fetcherMock{t: t}
	storePvtdataInPeer(rwSetsInPeer, rwSetsNotFound, fetcher, ts)

	pdp := &PvtdataProvider{
		selfSignedData:                          peerSelfSignedData,
		logger:                                  logger,
		listMissingPrivateDataDurationHistogram: metrics.ListMissingPrivateDataDuration.With("channel", ts.channelID),
		fetchDurationHistogram:                  metrics.FetchDuration.With("channel", ts.channelID),
		purgeDurationHistogram:                  metrics.PurgeDuration.With("channel", ts.channelID),
		transientStore:                          store,
		pullRetryThreshold:                      testConfig.PullRetryThreshold,
		cachedPvtdata:                           cachedPvtdata,
		transientBlockRetention:                 testConfig.TransientBlockRetention,
		channelID:                               ts.channelID,
		blockNum:                                ts.blockNum,
		storePvtdataOfInvalidTx:                 storePvtdataOfInvalidTx,
		fetcher:                                 fetcher,
		idDeserializerFactory:                   idDeserializerFactory,
	}

	return pdp
}

func testPurged(t *testing.T,
	scenario string,
	retrievedPvtdata *RetrievedPvtdata,
	store *transientstore.Store,
	txPvtdataInfo []*ledger.TxPvtdataInfo) {

	retrievedPvtdata.Purge()
	for _, pvtdata := range retrievedPvtdata.blockPvtdata.PvtData {
		func() {
			txID := getTxIDBySeqInBlock(pvtdata.SeqInBlock, txPvtdataInfo)
			require.NotEqual(t, txID, "", fmt.Sprintf("Could not find txID for SeqInBlock %d", pvtdata.SeqInBlock), scenario)

			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			defer iterator.Close()
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))

			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))

			assert.Nil(t, res, scenario)
		}()
	}
}

func storePvtdataInCache(rwsets []rwSet) util.PvtDataCollections {
	res := []*ledger.TxPvtData{}
	for _, rws := range rwsets {
		set := &rwset.TxPvtReadWriteSet{
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{
				{
					Namespace:          rws.namespace,
					CollectionPvtRwset: getCollectionPvtReadWriteSet(rws),
				},
			},
		}

		res = append(res, &ledger.TxPvtData{
			SeqInBlock: rws.seqInBlock,
			WriteSet:   set,
		})
	}

	return res
}

func storePvtdataInTransientStore(rwsets []rwSet, store *transientstore.Store) error {
	for _, rws := range rwsets {
		set := &transientstore2.TxPvtReadWriteSetWithConfigInfo{
			PvtRwset: &rwset.TxPvtReadWriteSet{
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace:          rws.namespace,
						CollectionPvtRwset: getCollectionPvtReadWriteSet(rws),
					},
				},
			},
			CollectionConfigs: make(map[string]*common.CollectionConfigPackage),
		}

		err := store.Persist(rws.txID, 1, set)
		if err != nil {
			return err
		}
	}
	return nil
}

func storePvtdataInPeer(rwSets, missing []rwSet, fetcher *fetcherMock, ts testSupport) {
	digKeys := []privdatacommon.DigKey{}
	availableElements := []*proto.PvtDataElement{}
	for _, rws := range rwSets {
		if !rws.ineligible {
			for _, c := range rws.collections {
				digKeys = append(digKeys, privdatacommon.DigKey{
					TxId:       rws.txID,
					Namespace:  rws.namespace,
					Collection: c,
					BlockSeq:   ts.blockNum,
					SeqInBlock: rws.seqInBlock,
				})
				availableElements = append(availableElements, &proto.PvtDataElement{
					Digest: &proto.PvtDataDigest{
						TxId:       rws.txID,
						Namespace:  rws.namespace,
						Collection: c,
						BlockSeq:   ts.blockNum,
						SeqInBlock: rws.seqInBlock,
					},
					Payload: [][]byte{ts.preHash},
				})
			}
		}
	}
	for _, rws := range missing {
		if !rws.ineligible {
			for _, c := range rws.collections {
				digKeys = append(digKeys, privdatacommon.DigKey{
					TxId:       rws.txID,
					Namespace:  rws.namespace,
					Collection: c,
					BlockSeq:   ts.blockNum,
					SeqInBlock: rws.seqInBlock,
				})
			}
		}
	}

	fetcher.On("fetch", mock.Anything).expectingDigests(digKeys).expectingEndorsers(ts.endorsers...).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: availableElements,
	}, nil)
}

func formatExpectedBlockPvtdata(rwsets []rwSet) *ledger.BlockPvtdata {
	expectedBlockPvtdata := &ledger.BlockPvtdata{
		PvtData:        make(ledger.TxPvtDataMap),
		MissingPvtData: make(ledger.TxMissingPvtDataMap),
	}
	for _, rws := range rwsets {
		if rws.missing || rws.ineligible {
			expectedBlockPvtdata.MissingPvtData[rws.seqInBlock] = getMissingPvtdata(rws)
		} else {
			expectedBlockPvtdata.PvtData[rws.seqInBlock] = &ledger.TxPvtData{
				SeqInBlock: rws.seqInBlock,
				WriteSet: &rwset.TxPvtReadWriteSet{
					NsPvtRwset: []*rwset.NsPvtReadWriteSet{
						{
							Namespace:          rws.namespace,
							CollectionPvtRwset: getCollectionPvtReadWriteSet(rws),
						},
					},
				},
			}
		}
	}
	return expectedBlockPvtdata
}

func formatTxPvtdataInfo(rwsets []rwSet, signature []byte, ts testSupport) []*ledger.TxPvtdataInfo {
	txPvtdataInfo := []*ledger.TxPvtdataInfo{}
	for _, rws := range rwsets {
		mspID := "different-org"
		if !rws.ineligible {
			mspID = ts.identity.GetMSPIdentifier()
		}
		colPvtdataInfo := []*ledger.CollectionPvtdataInfo{}
		endorsements := []*peer.Endorsement{}
		for _, e := range ts.endorsers {
			sID := &mspproto.SerializedIdentity{Mspid: e, IdBytes: []byte(fmt.Sprintf("p0%s", e))}
			b, _ := pb.Marshal(sID)
			endorsements = append(endorsements, &peer.Endorsement{
				Endorser:  b,
				Signature: signature,
			})
		}
		for _, c := range rws.collections {
			pvtdata := &ledger.CollectionPvtdataInfo{
				Collection:   c,
				Namespace:    rws.namespace,
				ExpectedHash: rws.hash,
				Endorsers:    endorsements,
				CollectionConfig: &common.StaticCollectionConfig{
					Name:           c,
					MemberOnlyRead: true,
					MemberOrgsPolicy: &common.CollectionPolicyConfig{
						Payload: &common.CollectionPolicyConfig_SignaturePolicy{
							SignaturePolicy: &common.SignaturePolicyEnvelope{
								Rule: &common.SignaturePolicy{
									Type: &common.SignaturePolicy_SignedBy{
										SignedBy: 0,
									},
								},
								Identities: []*mspproto.MSPPrincipal{
									{
										PrincipalClassification: mspproto.MSPPrincipal_ROLE,
										Principal: protoutil.MarshalOrPanic(&mspproto.MSPRole{
											MspIdentifier: mspID,
											Role:          mspproto.MSPRole_MEMBER,
										}),
									},
								},
							},
						},
					},
				},
			}
			colPvtdataInfo = append(colPvtdataInfo, pvtdata)
		}
		info := &ledger.TxPvtdataInfo{
			TxID:                  rws.txID,
			Invalid:               rws.invalid,
			SeqInBlock:            rws.seqInBlock,
			CollectionPvtdataInfo: colPvtdataInfo,
		}
		txPvtdataInfo = append(txPvtdataInfo, info)
	}
	return txPvtdataInfo
}

func getMissingPvtdata(rws rwSet) []*ledger.MissingPvtData {
	missing := []*ledger.MissingPvtData{}
	for _, c := range rws.collections {
		missing = append(missing, &ledger.MissingPvtData{
			Namespace:  rws.namespace,
			Collection: c,
			IsEligible: !rws.ineligible,
		})
	}

	sort.Slice(missing, func(i, j int) bool {
		return missing[i].Collection < missing[j].Collection
	})

	return missing
}
func getCollectionPvtReadWriteSet(rws rwSet) []*rwset.CollectionPvtReadWriteSet {
	colPvtRwSet := []*rwset.CollectionPvtReadWriteSet{}
	for _, c := range rws.collections {
		colPvtRwSet = append(colPvtRwSet, &rwset.CollectionPvtReadWriteSet{
			CollectionName: c,
			Rwset:          rws.preHash,
		})
	}

	sort.Slice(colPvtRwSet, func(i, j int) bool {
		return colPvtRwSet[i].CollectionName < colPvtRwSet[j].CollectionName
	})

	return colPvtRwSet
}

func sortBlockPvtdata(blockPvtdata *ledger.BlockPvtdata) *ledger.BlockPvtdata {
	for _, pvtdata := range blockPvtdata.PvtData {
		for _, ws := range pvtdata.WriteSet.NsPvtRwset {
			sort.Slice(ws.CollectionPvtRwset, func(i, j int) bool {
				return ws.CollectionPvtRwset[i].CollectionName < ws.CollectionPvtRwset[j].CollectionName
			})
		}
	}
	for _, missingPvtdata := range blockPvtdata.MissingPvtData {
		sort.Slice(missingPvtdata, func(i, j int) bool {
			return missingPvtdata[i].Collection < missingPvtdata[j].Collection
		})
	}
	return blockPvtdata
}
