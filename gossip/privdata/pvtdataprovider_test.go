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

	"github.com/hyperledger/fabric-protos-go/common"
	proto "github.com/hyperledger/fabric-protos-go/gossip"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	mspproto "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
	tspb "github.com/hyperledger/fabric-protos-go/transientstore"
	"github.com/hyperledger/fabric/bccsp/factory"
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
	preHash, hash      []byte
	channelID          string
	blockNum           uint64
	endorsers          []string
	peerSelfSignedData protoutil.SignedData
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

	identity := mspmgmt.GetLocalSigningIdentityOrPanic(factory.GetDefault())
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
	endorser := protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
		Mspid:   identity.GetMSPIdentifier(),
		IdBytes: []byte(fmt.Sprintf("p0%s", identity.GetMSPIdentifier())),
	})

	ts := testSupport{
		preHash:            []byte("rws-pre-image"),
		hash:               util2.ComputeSHA256([]byte("rws-pre-image")),
		channelID:          "testchannelid",
		blockNum:           uint64(1),
		endorsers:          []string{identity.GetMSPIdentifier()},
		peerSelfSignedData: peerSelfSignedData,
	}

	ns1c1 := collectionPvtdataInfoFromTemplate("ns1", "c1", identity.GetMSPIdentifier(), ts.hash, endorser, signature)
	ns1c2 := collectionPvtdataInfoFromTemplate("ns1", "c2", identity.GetMSPIdentifier(), ts.hash, endorser, signature)
	ineligiblens1c1 := collectionPvtdataInfoFromTemplate("ns1", "c1", "different-org", ts.hash, endorser, signature)

	tests := []struct {
		scenario                                                            string
		storePvtdataOfInvalidTx                                             bool
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer, rwSetsNotFound []rwSet
		txPvtdataQuery                                                      []*ledger.TxPvtdataInfo
		expectedBlockPvtdata                                                *ledger.BlockPvtdata
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
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx1",
					Invalid:    false,
					SeqInBlock: 1,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
						ns1c2,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{
					1: &ledger.TxPvtData{
						SeqInBlock: 1,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1", "c2"},
									}),
								},
							},
						},
					},
				},
				MissingPvtData: ledger.TxMissingPvtDataMap{},
			},
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
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					ineligible:  true,
					seqInBlock:  2,
				},
			},
			rwSetsInPeer: []rwSet{
				{
					txID:        "tx3",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					ineligible:  true,
					seqInBlock:  3,
				},
			},
			rwSetsNotFound: []rwSet{},
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx1",
					Invalid:    false,
					SeqInBlock: 1,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ineligiblens1c1,
					},
				},
				{
					TxID:       "tx2",
					Invalid:    false,
					SeqInBlock: 2,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ineligiblens1c1,
					},
				},
				{
					TxID:       "tx3",
					Invalid:    false,
					SeqInBlock: 3,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ineligiblens1c1,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{},
				MissingPvtData: ledger.TxMissingPvtDataMap{
					1: []*ledger.MissingPvtData{
						{
							Namespace:  "ns1",
							Collection: "c1",
							IsEligible: false,
						},
					},
					2: []*ledger.MissingPvtData{
						{
							Namespace:  "ns1",
							Collection: "c1",
							IsEligible: false,
						},
					},
					3: []*ledger.MissingPvtData{
						{
							Namespace:  "ns1",
							Collection: "c1",
							IsEligible: false,
						},
					},
				},
			},
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
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx1",
					Invalid:    false,
					SeqInBlock: 1,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
						ns1c2,
					},
				},
				{
					TxID:       "tx2",
					Invalid:    false,
					SeqInBlock: 2,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c2,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{
					1: &ledger.TxPvtData{
						SeqInBlock: 1,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1", "c2"},
									}),
								},
							},
						},
					},
					2: &ledger.TxPvtData{
						SeqInBlock: 2,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c2"},
									}),
								},
							},
						},
					},
				},
				MissingPvtData: ledger.TxMissingPvtDataMap{},
			},
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
					collections: []string{"c1", "c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  2,
				},
			},
			rwSetsInPeer: []rwSet{
				{
					txID:        "tx3",
					namespace:   "ns1",
					collections: []string{"c1", "c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  3,
				},
			},
			rwSetsNotFound: []rwSet{},
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx1",
					Invalid:    false,
					SeqInBlock: 1,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
						ns1c2,
					},
				},
				{
					TxID:       "tx2",
					Invalid:    false,
					SeqInBlock: 2,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
						ns1c2,
					},
				},
				{
					TxID:       "tx3",
					Invalid:    false,
					SeqInBlock: 3,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
						ns1c2,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{
					1: &ledger.TxPvtData{
						SeqInBlock: 1,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1", "c2"},
									}),
								},
							},
						},
					},
					2: &ledger.TxPvtData{
						SeqInBlock: 2,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1", "c2"},
									}),
								},
							},
						},
					},
					3: &ledger.TxPvtData{
						SeqInBlock: 3,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1", "c2"},
									}),
								},
							},
						},
					},
				},
				MissingPvtData: ledger.TxMissingPvtDataMap{},
			},
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
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx1",
					Invalid:    true,
					SeqInBlock: 1,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
					},
				},
				{
					TxID:       "tx2",
					Invalid:    false,
					SeqInBlock: 2,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{
					2: &ledger.TxPvtData{
						SeqInBlock: 2,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1"},
									}),
								},
							},
						},
					},
				},
				MissingPvtData: ledger.TxMissingPvtDataMap{},
			},
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
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx1",
					Invalid:    true,
					SeqInBlock: 1,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
					},
				},
				{
					TxID:       "tx2",
					Invalid:    false,
					SeqInBlock: 2,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{
					1: &ledger.TxPvtData{
						SeqInBlock: 1,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1"},
									}),
								},
							},
						},
					},
					2: &ledger.TxPvtData{
						SeqInBlock: 2,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1"},
									}),
								},
							},
						},
					},
				},
				MissingPvtData: ledger.TxMissingPvtDataMap{},
			},
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
					collections: []string{"c1", "c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					missing:     true,
					seqInBlock:  1,
				},
			},
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx1",
					Invalid:    false,
					SeqInBlock: 1,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
						ns1c2,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{},
				MissingPvtData: ledger.TxMissingPvtDataMap{
					1: []*ledger.MissingPvtData{
						{
							Namespace:  "ns1",
							Collection: "c1",
							IsEligible: true,
						},
						{
							Namespace:  "ns1",
							Collection: "c2",
							IsEligible: true,
						},
					},
				},
			},
		},
		{
			// Scenario VIII
			scenario:                "Scenario VIII: Extra data not requested",
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
					collections: []string{"c1", "c2"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  2,
				},
			},
			rwSetsInPeer: []rwSet{
				{
					txID:        "tx3",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
					seqInBlock:  3,
				},
			},
			rwSetsNotFound: []rwSet{},
			// Only requesting tx3, ns1, c1, should skip all extra data found in all sources
			txPvtdataQuery: []*ledger.TxPvtdataInfo{
				{
					TxID:       "tx3",
					Invalid:    false,
					SeqInBlock: 3,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
					},
				},
			},
			expectedBlockPvtdata: &ledger.BlockPvtdata{
				PvtData: ledger.TxPvtDataMap{
					3: &ledger.TxPvtData{
						SeqInBlock: 3,
						WriteSet: &rwset.TxPvtReadWriteSet{
							NsPvtRwset: []*rwset.NsPvtReadWriteSet{
								{
									Namespace: "ns1",
									CollectionPvtRwset: getCollectionPvtReadWriteSet(rwSet{
										preHash:     ts.preHash,
										collections: []string{"c1"},
									}),
								},
							},
						},
					},
				},
				MissingPvtData: ledger.TxMissingPvtDataMap{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			testRetrievePrivateDataSuccess(t, test.scenario, ts, test.storePvtdataOfInvalidTx,
				test.rwSetsInCache, test.rwSetsInTransientStore, test.rwSetsInPeer, test.rwSetsNotFound, test.txPvtdataQuery, test.expectedBlockPvtdata)
		})
	}
}

func TestRetrievePrivateDataFailure(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity := mspmgmt.GetLocalSigningIdentityOrPanic(factory.GetDefault())
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
	endorser := protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
		Mspid:   identity.GetMSPIdentifier(),
		IdBytes: []byte(fmt.Sprintf("p0%s", identity.GetMSPIdentifier())),
	})

	ts := testSupport{
		preHash:            []byte("rws-pre-image"),
		hash:               util2.ComputeSHA256([]byte("rws-pre-image")),
		channelID:          "testchannelid",
		blockNum:           uint64(1),
		endorsers:          []string{identity.GetMSPIdentifier()},
		peerSelfSignedData: peerSelfSignedData,
	}

	invalidns1c1 := collectionPvtdataInfoFromTemplate("ns1", "c1", identity.GetMSPIdentifier(), ts.hash, endorser, signature)
	invalidns1c1.CollectionConfig.MemberOrgsPolicy = nil

	scenario := "Scenario I: Invalid collection config policy"
	storePvtdataOfInvalidTx := true
	rwSetsInCache := []rwSet{}
	rwSetsInTransientStore := []rwSet{}
	rwSetsInPeer := []rwSet{}
	rwSetsNotFound := []rwSet{
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
	txPvtdataQuery := []*ledger.TxPvtdataInfo{
		{
			TxID:       "tx3",
			Invalid:    false,
			SeqInBlock: 3,
			CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
				invalidns1c1,
			},
		},
	}

	expectedErr := "Collection config policy is nil"

	testRetrievePrivateDataFailure(t, scenario, ts,
		peerSelfSignedData, storePvtdataOfInvalidTx,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound,
		txPvtdataQuery,
		expectedErr)
}

func TestRetryFetchFromPeer(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity := mspmgmt.GetLocalSigningIdentityOrPanic(factory.GetDefault())
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
	endorser := protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
		Mspid:   identity.GetMSPIdentifier(),
		IdBytes: []byte(fmt.Sprintf("p0%s", identity.GetMSPIdentifier())),
	})

	ts := testSupport{
		preHash:            []byte("rws-pre-image"),
		hash:               util2.ComputeSHA256([]byte("rws-pre-image")),
		channelID:          "testchannelid",
		blockNum:           uint64(1),
		endorsers:          []string{identity.GetMSPIdentifier()},
		peerSelfSignedData: peerSelfSignedData,
	}

	ns1c1 := collectionPvtdataInfoFromTemplate("ns1", "c1", identity.GetMSPIdentifier(), ts.hash, endorser, signature)
	ns1c2 := collectionPvtdataInfoFromTemplate("ns1", "c2", identity.GetMSPIdentifier(), ts.hash, endorser, signature)

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
			collections: []string{"c1", "c2"},
			preHash:     ts.preHash,
			hash:        ts.hash,
			missing:     true,
			seqInBlock:  1,
		},
	}
	txPvtdataQuery := []*ledger.TxPvtdataInfo{
		{
			TxID:       "tx1",
			Invalid:    false,
			SeqInBlock: 1,
			CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
				ns1c1,
				ns1c2,
			},
		},
	}
	pdp := setupPrivateDataProvider(t, ts, testConfig,
		storePvtdataOfInvalidTx, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound)
	require.NotNil(t, pdp)

	fakeSleeper := &mocks.Sleeper{}
	SetSleeper(pdp, fakeSleeper)

	_, err = pdp.RetrievePrivatedata(txPvtdataQuery)
	assert.NoError(t, err)
	var maxRetries int

	maxRetries = int(testConfig.PullRetryThreshold / pullRetrySleepInterval)
	assert.Equal(t, fakeSleeper.SleepCallCount() <= maxRetries, true)
	assert.Equal(t, fakeSleeper.SleepArgsForCall(0), pullRetrySleepInterval)
}

func TestRetrievedPvtdataPurgeBelowHeight(t *testing.T) {
	conf := testConfig
	conf.TransientBlockRetention = 5

	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity := mspmgmt.GetLocalSigningIdentityOrPanic(factory.GetDefault())
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
	endorser := protoutil.MarshalOrPanic(&mspproto.SerializedIdentity{
		Mspid:   identity.GetMSPIdentifier(),
		IdBytes: []byte(fmt.Sprintf("p0%s", identity.GetMSPIdentifier())),
	})

	ts := testSupport{
		preHash:            []byte("rws-pre-image"),
		hash:               util2.ComputeSHA256([]byte("rws-pre-image")),
		channelID:          "testchannelid",
		blockNum:           uint64(9),
		endorsers:          []string{identity.GetMSPIdentifier()},
		peerSelfSignedData: peerSelfSignedData,
	}

	ns1c1 := collectionPvtdataInfoFromTemplate("ns1", "c1", identity.GetMSPIdentifier(), ts.hash, endorser, signature)

	tempdir, err := ioutil.TempDir("", "ts")
	require.NoError(t, err, fmt.Sprintf("Failed to create test directory, got err %s", err))
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))

	defer storeProvider.Close()
	defer os.RemoveAll(tempdir)

	// set up store with 9 existing private data write sets
	for i := 0; i < 9; i++ {
		txID := fmt.Sprintf("tx%d", i+1)
		store.Persist(txID, uint64(i), &tspb.TxPvtReadWriteSetWithConfigInfo{
			PvtRwset: &rwset.TxPvtReadWriteSet{
				NsPvtRwset: []*rwset.NsPvtReadWriteSet{
					{
						Namespace: "ns1",
						CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
							{
								CollectionName: "c1",
								Rwset:          []byte("rws-pre-image"),
							},
						},
					},
				},
			},
			CollectionConfigs: make(map[string]*common.CollectionConfigPackage),
		})
	}

	// test that the initial data shows up in the store
	for i := 1; i < 9; i++ {
		func() {
			txID := fmt.Sprintf("tx%d", i)
			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			defer iterator.Close()
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))
			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))
			assert.NotNil(t, res)
		}()
	}

	storePvtdataOfInvalidTx := true
	rwSetsInCache := []rwSet{
		{
			txID:        "tx9",
			namespace:   "ns1",
			collections: []string{"c1"},
			preHash:     ts.preHash,
			hash:        ts.hash,
			seqInBlock:  1,
		},
	}
	rwSetsInTransientStore := []rwSet{}
	rwSetsInPeer := []rwSet{}
	rwSetsNotFound := []rwSet{}
	// request tx9 which is found in both the cache and transient store
	txPvtdataQuery := []*ledger.TxPvtdataInfo{
		{
			TxID:       "tx9",
			Invalid:    false,
			SeqInBlock: 1,
			CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
				ns1c1,
			},
		},
	}
	pdp := setupPrivateDataProvider(t, ts, conf,
		storePvtdataOfInvalidTx, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound)
	require.NotNil(t, pdp)

	retrievedPvtdata, err := pdp.RetrievePrivatedata(txPvtdataQuery)
	require.NoError(t, err)

	retrievedPvtdata.Purge()

	for i := 1; i <= 9; i++ {
		func() {
			txID := fmt.Sprintf("tx%d", i)
			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			defer iterator.Close()
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))
			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))
			// Check that only the fetched private write set was purged because we haven't reached a blockNum that's a multiple of 5 yet
			if i == 9 {
				assert.Nil(t, res)
			} else {
				assert.NotNil(t, res)
			}
		}()
	}

	// increment blockNum to a multiple of transientBlockRetention
	pdp.blockNum = 10
	retrievedPvtdata, err = pdp.RetrievePrivatedata(txPvtdataQuery)
	require.NoError(t, err)

	retrievedPvtdata.Purge()

	for i := 1; i <= 9; i++ {
		func() {
			txID := fmt.Sprintf("tx%d", i)
			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			defer iterator.Close()
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))
			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))
			// Check that the first 5 sets have been purged alongside the 9th set purged earlier
			if i < 6 || i == 9 {
				assert.Nil(t, res)
			} else {
				assert.NotNil(t, res)
			}
		}()
	}
}

func testRetrievePrivateDataSuccess(t *testing.T,
	scenario string,
	ts testSupport,
	storePvtdataOfInvalidTx bool,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	rwSetsNotFound []rwSet,
	txPvtdataQuery []*ledger.TxPvtdataInfo,
	expectedBlockPvtdata *ledger.BlockPvtdata) {

	fmt.Println("\n" + scenario)

	tempdir, err := ioutil.TempDir("", "ts")
	require.NoError(t, err, fmt.Sprintf("Failed to create test directory, got err %s", err))
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))
	defer storeProvider.Close()
	defer os.RemoveAll(tempdir)

	pdp := setupPrivateDataProvider(t, ts, testConfig,
		storePvtdataOfInvalidTx, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound)
	require.NotNil(t, pdp, scenario)

	retrievedPvtdata, err := pdp.RetrievePrivatedata(txPvtdataQuery)
	assert.NoError(t, err, scenario)

	// sometimes the collection private write sets are added out of order
	// so we need to sort it to check equality with expected
	blockPvtdata := sortBlockPvtdata(retrievedPvtdata.GetBlockPvtdata())
	assert.Equal(t, expectedBlockPvtdata, blockPvtdata, scenario)

	// Test pvtdata is purged from store on Done() call
	testPurged(t, scenario, retrievedPvtdata, store, txPvtdataQuery)
}

func testRetrievePrivateDataFailure(t *testing.T,
	scenario string,
	ts testSupport,
	peerSelfSignedData protoutil.SignedData,
	storePvtdataOfInvalidTx bool,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	rwSetsNotFound []rwSet,
	txPvtdataQuery []*ledger.TxPvtdataInfo,
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

	pdp := setupPrivateDataProvider(t, ts, testConfig,
		storePvtdataOfInvalidTx, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		rwSetsNotFound)
	require.NotNil(t, pdp, scenario)

	_, err = pdp.RetrievePrivatedata(txPvtdataQuery)
	assert.EqualError(t, err, expectedErr, scenario)
}

func setupPrivateDataProvider(t *testing.T,
	ts testSupport,
	config CoordinatorConfig,
	storePvtdataOfInvalidTx bool, store *transientstore.Store,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	rwSetsNotFound []rwSet) *PvtdataProvider {

	metrics := metrics.NewGossipMetrics(&disabled.Provider{}).PrivdataMetrics

	idDeserializerFactory := IdentityDeserializerFactoryFunc(func(chainID string) msp.IdentityDeserializer {
		return mspmgmt.GetManagerForChain(ts.channelID)
	})

	// set up data in cache
	prefetchedPvtdata := storePvtdataInCache(rwSetsInCache)
	// set up data in transient store
	err := storePvtdataInTransientStore(rwSetsInTransientStore, store)
	require.NoError(t, err, fmt.Sprintf("Failed to store private data in transient store: got err %s", err))

	// set up data in peer
	fetcher := &fetcherMock{t: t}
	storePvtdataInPeer(rwSetsInPeer, rwSetsNotFound, fetcher, ts)

	pdp := &PvtdataProvider{
		selfSignedData:                          ts.peerSelfSignedData,
		logger:                                  logger,
		listMissingPrivateDataDurationHistogram: metrics.ListMissingPrivateDataDuration.With("channel", ts.channelID),
		fetchDurationHistogram:                  metrics.FetchDuration.With("channel", ts.channelID),
		purgeDurationHistogram:                  metrics.PurgeDuration.With("channel", ts.channelID),
		transientStore:                          store,
		pullRetryThreshold:                      config.PullRetryThreshold,
		prefetchedPvtdata:                       prefetchedPvtdata,
		transientBlockRetention:                 config.TransientBlockRetention,
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
	retrievedPvtdata ledger.RetrievedPvtdata,
	store *transientstore.Store,
	txPvtdataInfo []*ledger.TxPvtdataInfo) {

	retrievedPvtdata.Purge()
	for _, pvtdata := range retrievedPvtdata.GetBlockPvtdata().PvtData {
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
		set := &tspb.TxPvtReadWriteSetWithConfigInfo{
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

func collectionPvtdataInfoFromTemplate(namespace, collection, mspIdentifier string, hash, endorser, signature []byte) *ledger.CollectionPvtdataInfo {
	return &ledger.CollectionPvtdataInfo{
		Collection:   collection,
		Namespace:    namespace,
		ExpectedHash: hash,
		Endorsers: []*peer.Endorsement{
			{
				Endorser:  endorser,
				Signature: signature,
			},
		},
		CollectionConfig: &common.StaticCollectionConfig{
			Name:           collection,
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
									MspIdentifier: mspIdentifier,
									Role:          mspproto.MSPRole_MEMBER,
								}),
							},
						},
					},
				},
			},
		},
	}
}
