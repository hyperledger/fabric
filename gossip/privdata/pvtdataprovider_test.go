/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"
	"sort"
	"testing"
	"time"

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
	seqInBlock    uint64
}

func init() {
	util.SetupTestLoggingWithLevel("INFO")
}

func TestRetrievePvtdata(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
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
		scenario                                                string
		storePvtdataOfInvalidTx, skipPullingInvalidTransactions bool
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer     []rwSet
		expectedDigKeys                                         []privdatacommon.DigKey
		pvtdataToRetrieve                                       []*ledger.TxPvtdataInfo
		expectedBlockPvtdata                                    *ledger.BlockPvtdata
	}{
		{
			// Scenario I
			scenario:                       "Scenario I: Only eligible private data in cache, no missing private data",
			storePvtdataOfInvalidTx:        true,
			skipPullingInvalidTransactions: false,
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
			expectedDigKeys:        []privdatacommon.DigKey{},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{},
			},
		},
		{
			// Scenario II
			scenario:                       "Scenario II: No eligible private data, skip ineligible private data from all sources even if found in cache",
			storePvtdataOfInvalidTx:        true,
			skipPullingInvalidTransactions: false,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
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
			expectedDigKeys: []privdatacommon.DigKey{},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{
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
			scenario:                       "Scenario III: Missing private data in cache, found in transient store",
			storePvtdataOfInvalidTx:        true,
			skipPullingInvalidTransactions: false,
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
			rwSetsInPeer:    []rwSet{},
			expectedDigKeys: []privdatacommon.DigKey{},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{},
			},
		},
		{
			// Scenario IV
			scenario:                       "Scenario IV: Missing private data in cache, found some in transient store and some in peer",
			storePvtdataOfInvalidTx:        true,
			skipPullingInvalidTransactions: false,
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
			expectedDigKeys: []privdatacommon.DigKey{
				{
					TxId:       "tx3",
					Namespace:  "ns1",
					Collection: "c1",
					BlockSeq:   ts.blockNum,
					SeqInBlock: 3,
				},
				{
					TxId:       "tx3",
					Namespace:  "ns1",
					Collection: "c2",
					BlockSeq:   ts.blockNum,
					SeqInBlock: 3,
				},
			},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{},
			},
		},
		{
			// Scenario V
			scenario:                       "Scenario V: Skip invalid txs when storePvtdataOfInvalidTx is false",
			storePvtdataOfInvalidTx:        false,
			skipPullingInvalidTransactions: false,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
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
			expectedDigKeys:        []privdatacommon.DigKey{},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{},
			},
		},
		{
			// Scenario VI
			scenario:                       "Scenario VI: Don't skip invalid txs when storePvtdataOfInvalidTx is true",
			storePvtdataOfInvalidTx:        true,
			skipPullingInvalidTransactions: false,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
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
			expectedDigKeys:        []privdatacommon.DigKey{},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{},
			},
		},
		{
			// Scenario VII
			scenario:                "Scenario VII: Can't find eligible tx from any source",
			storePvtdataOfInvalidTx: true,
			rwSetsInCache:           []rwSet{},
			rwSetsInTransientStore:  []rwSet{},
			rwSetsInPeer:            []rwSet{},
			expectedDigKeys: []privdatacommon.DigKey{
				{
					TxId:       "tx1",
					Namespace:  "ns1",
					Collection: "c1",
					BlockSeq:   ts.blockNum,
					SeqInBlock: 1,
				},
				{
					TxId:       "tx1",
					Namespace:  "ns1",
					Collection: "c2",
					BlockSeq:   ts.blockNum,
					SeqInBlock: 1,
				},
			},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{
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
			scenario:                       "Scenario VIII: Extra data not requested",
			storePvtdataOfInvalidTx:        true,
			skipPullingInvalidTransactions: false,
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
			expectedDigKeys: []privdatacommon.DigKey{
				{
					TxId:       "tx3",
					Namespace:  "ns1",
					Collection: "c1",
					BlockSeq:   ts.blockNum,
					SeqInBlock: 3,
				},
			},
			// Only requesting tx3, ns1, c1, should skip all extra data found in all sources
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
				MissingPvtData: ledger.TxMissingPvtData{},
			},
		},
		{
			// Scenario IX
			scenario:                       "Scenario IX: Skip pulling invalid txs when skipPullingInvalidTransactions is true",
			storePvtdataOfInvalidTx:        true,
			skipPullingInvalidTransactions: true,
			rwSetsInCache: []rwSet{
				{
					txID:        "tx1",
					namespace:   "ns1",
					collections: []string{"c1"},
					preHash:     ts.preHash,
					hash:        ts.hash,
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
					seqInBlock:  2,
				},
			},
			expectedDigKeys: []privdatacommon.DigKey{},
			pvtdataToRetrieve: []*ledger.TxPvtdataInfo{
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
					Invalid:    true,
					SeqInBlock: 2,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
					},
				},
				{
					TxID:       "tx3",
					Invalid:    true,
					SeqInBlock: 3,
					CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
						ns1c1,
					},
				},
			},
			// tx1 and tx2 are still fetched despite being invalid
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
				// Only tx3 is missing since we skip pulling invalid tx from peers
				MissingPvtData: ledger.TxMissingPvtData{
					3: []*ledger.MissingPvtData{
						{
							Namespace:  "ns1",
							Collection: "c1",
							IsEligible: true,
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			testRetrievePvtdataSuccess(t, test.scenario, ts, test.storePvtdataOfInvalidTx, test.skipPullingInvalidTransactions,
				test.rwSetsInCache, test.rwSetsInTransientStore, test.rwSetsInPeer, test.expectedDigKeys, test.pvtdataToRetrieve, test.expectedBlockPvtdata)
		})
	}
}

func TestRetrievePvtdataFailure(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
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
	skipPullingInvalidTransactions := false
	rwSetsInCache := []rwSet{}
	rwSetsInTransientStore := []rwSet{}
	rwSetsInPeer := []rwSet{}
	expectedDigKeys := []privdatacommon.DigKey{}
	pvtdataToRetrieve := []*ledger.TxPvtdataInfo{
		{
			TxID:       "tx1",
			Invalid:    false,
			SeqInBlock: 1,
			CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
				invalidns1c1,
			},
		},
	}

	expectedErr := "Collection config policy is nil"

	testRetrievePvtdataFailure(t, scenario, ts,
		peerSelfSignedData, storePvtdataOfInvalidTx, skipPullingInvalidTransactions,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		expectedDigKeys, pvtdataToRetrieve,
		expectedErr)
}

func TestRetryFetchFromPeer(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
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

	tempdir := t.TempDir()
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))

	defer storeProvider.Close()

	storePvtdataOfInvalidTx := true
	skipPullingInvalidTransactions := false
	rwSetsInCache := []rwSet{}
	rwSetsInTransientStore := []rwSet{}
	rwSetsInPeer := []rwSet{}
	expectedDigKeys := []privdatacommon.DigKey{
		{
			TxId:       "tx1",
			Namespace:  "ns1",
			Collection: "c1",
			BlockSeq:   ts.blockNum,
			SeqInBlock: 1,
		},
		{
			TxId:       "tx1",
			Namespace:  "ns1",
			Collection: "c2",
			BlockSeq:   ts.blockNum,
			SeqInBlock: 1,
		},
	}
	pvtdataToRetrieve := []*ledger.TxPvtdataInfo{
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
		storePvtdataOfInvalidTx, skipPullingInvalidTransactions, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		expectedDigKeys)
	require.NotNil(t, pdp)

	fakeSleeper := &mocks.Sleeper{}
	SetSleeper(pdp, fakeSleeper)
	fakeSleeper.SleepStub = func(sleepDur time.Duration) {
		time.Sleep(sleepDur)
	}

	_, err = pdp.RetrievePvtdata(pvtdataToRetrieve)
	require.NoError(t, err)

	maxRetries := int(testConfig.PullRetryThreshold / pullRetrySleepInterval)
	require.Equal(t, fakeSleeper.SleepCallCount() <= maxRetries, true)
	require.Equal(t, fakeSleeper.SleepArgsForCall(0), pullRetrySleepInterval)
}

func TestSkipPullingAllInvalidTransactions(t *testing.T) {
	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
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

	tempdir := t.TempDir()
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))

	defer storeProvider.Close()

	storePvtdataOfInvalidTx := true
	skipPullingInvalidTransactions := true
	rwSetsInCache := []rwSet{}
	rwSetsInTransientStore := []rwSet{}
	rwSetsInPeer := []rwSet{}
	expectedDigKeys := []privdatacommon.DigKey{}
	expectedBlockPvtdata := &ledger.BlockPvtdata{
		PvtData: ledger.TxPvtDataMap{},
		MissingPvtData: ledger.TxMissingPvtData{
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
	}
	pvtdataToRetrieve := []*ledger.TxPvtdataInfo{
		{
			TxID:       "tx1",
			Invalid:    true,
			SeqInBlock: 1,
			CollectionPvtdataInfo: []*ledger.CollectionPvtdataInfo{
				ns1c1,
				ns1c2,
			},
		},
	}
	pdp := setupPrivateDataProvider(t, ts, testConfig,
		storePvtdataOfInvalidTx, skipPullingInvalidTransactions, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		expectedDigKeys)
	require.NotNil(t, pdp)

	fakeSleeper := &mocks.Sleeper{}
	SetSleeper(pdp, fakeSleeper)
	newFetcher := &fetcherMock{t: t}
	pdp.fetcher = newFetcher

	retrievedPvtdata, err := pdp.RetrievePvtdata(pvtdataToRetrieve)
	require.NoError(t, err)

	blockPvtdata := sortBlockPvtdata(retrievedPvtdata.GetBlockPvtdata())
	require.Equal(t, expectedBlockPvtdata, blockPvtdata)

	// Check sleep and fetch were never called
	require.Equal(t, fakeSleeper.SleepCallCount(), 0)
	require.Len(t, newFetcher.Calls, 0)
}

func TestRetrievedPvtdataPurgeBelowHeight(t *testing.T) {
	conf := testConfig
	conf.TransientBlockRetention = 5

	err := msptesttools.LoadMSPSetupForTesting()
	require.NoError(t, err, fmt.Sprintf("Failed to setup local msp for testing, got err %s", err))

	identity, err := mspmgmt.GetLocalMSP(factory.GetDefault()).GetDefaultSigningIdentity()
	require.NoError(t, err)
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

	tempdir := t.TempDir()
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))

	defer storeProvider.Close()

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
			CollectionConfigs: make(map[string]*peer.CollectionConfigPackage),
		})
	}

	// test that the initial data shows up in the store
	for i := 1; i < 9; i++ {
		func() {
			txID := fmt.Sprintf("tx%d", i)
			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))
			defer iterator.Close()
			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))
			require.NotNil(t, res)
		}()
	}

	storePvtdataOfInvalidTx := true
	skipPullingInvalidTransactions := false
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
	expectedDigKeys := []privdatacommon.DigKey{}
	// request tx9 which is found in both the cache and transient store
	pvtdataToRetrieve := []*ledger.TxPvtdataInfo{
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
		storePvtdataOfInvalidTx, skipPullingInvalidTransactions, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer, expectedDigKeys)
	require.NotNil(t, pdp)

	retrievedPvtdata, err := pdp.RetrievePvtdata(pvtdataToRetrieve)
	require.NoError(t, err)

	retrievedPvtdata.Purge()

	for i := 1; i <= 9; i++ {
		func() {
			txID := fmt.Sprintf("tx%d", i)
			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))
			defer iterator.Close()
			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))
			// Check that only the fetched private write set was purged because we haven't reached a blockNum that's a multiple of 5 yet
			if i == 9 {
				require.Nil(t, res)
			} else {
				require.NotNil(t, res)
			}
		}()
	}

	// increment blockNum to a multiple of transientBlockRetention
	pdp.blockNum = 10
	retrievedPvtdata, err = pdp.RetrievePvtdata(pvtdataToRetrieve)
	require.NoError(t, err)

	retrievedPvtdata.Purge()

	for i := 1; i <= 9; i++ {
		func() {
			txID := fmt.Sprintf("tx%d", i)
			iterator, err := store.GetTxPvtRWSetByTxid(txID, nil)
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))
			defer iterator.Close()
			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))
			// Check that the first 5 sets have been purged alongside the 9th set purged earlier
			if i < 6 || i == 9 {
				require.Nil(t, res)
			} else {
				require.NotNil(t, res)
			}
		}()
	}
}

func TestFetchStats(t *testing.T) {
	fetchStats := fetchStats{
		fromLocalCache:     1,
		fromTransientStore: 2,
		fromRemotePeer:     3,
	}
	require.Equal(t, "(1 from local cache, 2 from transient store, 3 from other peers)", fetchStats.String())
}

func testRetrievePvtdataSuccess(t *testing.T,
	scenario string,
	ts testSupport,
	storePvtdataOfInvalidTx, skipPullingInvalidTransactions bool,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	expectedDigKeys []privdatacommon.DigKey,
	pvtdataToRetrieve []*ledger.TxPvtdataInfo,
	expectedBlockPvtdata *ledger.BlockPvtdata) {
	fmt.Println("\n" + scenario)

	tempdir := t.TempDir()
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))
	defer storeProvider.Close()

	pdp := setupPrivateDataProvider(t, ts, testConfig,
		storePvtdataOfInvalidTx, skipPullingInvalidTransactions, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		expectedDigKeys)
	require.NotNil(t, pdp, scenario)

	retrievedPvtdata, err := pdp.RetrievePvtdata(pvtdataToRetrieve)
	require.NoError(t, err, scenario)

	// sometimes the collection private write sets are added out of order
	// so we need to sort it to check equality with expected
	blockPvtdata := sortBlockPvtdata(retrievedPvtdata.GetBlockPvtdata())
	require.Equal(t, expectedBlockPvtdata, blockPvtdata, scenario)

	// Test pvtdata is purged from store on Done() call
	testPurged(t, scenario, retrievedPvtdata, store, pvtdataToRetrieve)
}

func testRetrievePvtdataFailure(t *testing.T,
	scenario string,
	ts testSupport,
	peerSelfSignedData protoutil.SignedData,
	storePvtdataOfInvalidTx, skipPullingInvalidTransactions bool,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	expectedDigKeys []privdatacommon.DigKey,
	pvtdataToRetrieve []*ledger.TxPvtdataInfo,
	expectedErr string) {
	fmt.Println("\n" + scenario)

	tempdir := t.TempDir()
	storeProvider, err := transientstore.NewStoreProvider(tempdir)
	require.NoError(t, err, fmt.Sprintf("Failed to create store provider, got err %s", err))
	store, err := storeProvider.OpenStore(ts.channelID)
	require.NoError(t, err, fmt.Sprintf("Failed to open store, got err %s", err))
	defer storeProvider.Close()

	pdp := setupPrivateDataProvider(t, ts, testConfig,
		storePvtdataOfInvalidTx, skipPullingInvalidTransactions, store,
		rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer,
		expectedDigKeys)
	require.NotNil(t, pdp, scenario)

	_, err = pdp.RetrievePvtdata(pvtdataToRetrieve)
	require.EqualError(t, err, expectedErr, scenario)
}

func setupPrivateDataProvider(t *testing.T,
	ts testSupport,
	config CoordinatorConfig,
	storePvtdataOfInvalidTx, skipPullingInvalidTransactions bool, store *transientstore.Store,
	rwSetsInCache, rwSetsInTransientStore, rwSetsInPeer []rwSet,
	expectedDigKeys []privdatacommon.DigKey) *PvtdataProvider {
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
	storePvtdataInPeer(rwSetsInPeer, expectedDigKeys, fetcher, ts, skipPullingInvalidTransactions)

	pdp := &PvtdataProvider{
		mspID:                                   "Org1MSP",
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
		skipPullingInvalidTransactions:          skipPullingInvalidTransactions,
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
			require.NoError(t, err, fmt.Sprintf("Failed obtaining iterator from transient store, got err %s", err))
			defer iterator.Close()

			res, err := iterator.Next()
			require.NoError(t, err, fmt.Sprintf("Failed iterating, got err %s", err))

			require.Nil(t, res, scenario)
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
			CollectionConfigs: make(map[string]*peer.CollectionConfigPackage),
		}

		err := store.Persist(rws.txID, 1, set)
		if err != nil {
			return err
		}
	}
	return nil
}

func storePvtdataInPeer(rwSets []rwSet, expectedDigKeys []privdatacommon.DigKey, fetcher *fetcherMock, ts testSupport, skipPullingInvalidTransactions bool) {
	availableElements := []*proto.PvtDataElement{}
	for _, rws := range rwSets {
		for _, c := range rws.collections {
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

	endorsers := []string{}
	if len(expectedDigKeys) > 0 {
		endorsers = ts.endorsers
	}
	fetcher.On("fetch", mock.Anything).expectingDigests(expectedDigKeys).expectingEndorsers(endorsers...).Return(&privdatacommon.FetchedPvtDataContainer{
		AvailableElements: availableElements,
	}, nil)
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
		CollectionConfig: &peer.StaticCollectionConfig{
			Name:           collection,
			MemberOnlyRead: true,
			MemberOrgsPolicy: &peer.CollectionPolicyConfig{
				Payload: &peer.CollectionPolicyConfig_SignaturePolicy{
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
