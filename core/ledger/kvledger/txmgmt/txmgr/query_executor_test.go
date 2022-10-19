/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"crypto/sha256"
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

var testHashFunc = func(data []byte) ([]byte, error) {
	h := sha256.New()
	if _, err := h.Write(data); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func TestPvtdataResultsItr(t *testing.T) {
	testEnv := testEnvsMap[levelDBtestEnvName]
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns1", "coll1"}: 0,
			{"ns2", "coll1"}: 0,
			{"ns3", "coll1"}: 0,
		},
	)
	testEnv.init(t, "test-pvtdata-range-queries", btlPolicy)
	defer testEnv.cleanup()

	txMgr := testEnv.getTxMgr()
	populateCollConfigForTest(t, txMgr, []collConfigkey{
		{"ns1", "coll1"}, {"ns2", "coll1"}, {"ns3", "coll1"}, {"ns4", "coll1"},
	},
		version.NewHeight(1, 0),
	)

	updates := privacyenabledstate.NewUpdateBatch()
	putPvtUpdates(t, updates, "ns1", "coll1", "key1", []byte("pvt_value1"), version.NewHeight(1, 1))
	putPvtUpdates(t, updates, "ns1", "coll1", "key2", []byte("pvt_value2"), version.NewHeight(1, 2))
	putPvtUpdates(t, updates, "ns1", "coll1", "key3", []byte("pvt_value3"), version.NewHeight(1, 3))
	putPvtUpdates(t, updates, "ns1", "coll1", "key4", []byte("pvt_value4"), version.NewHeight(1, 4))
	putPvtUpdates(t, updates, "ns2", "coll1", "key5", []byte("pvt_value5"), version.NewHeight(1, 5))
	putPvtUpdates(t, updates, "ns2", "coll1", "key6", []byte("pvt_value6"), version.NewHeight(1, 6))
	putPvtUpdates(t, updates, "ns3", "coll1", "key7", []byte("pvt_value7"), version.NewHeight(1, 7))
	require.NoError(t, txMgr.db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 7)))
	qe := newQueryExecutor(txMgr, "", nil, true, testHashFunc)

	resItr, err := qe.GetPrivateDataRangeScanIterator("ns1", "coll1", "key1", "key3")
	require.NoError(t, err)
	testItr(t, resItr, "ns1", "coll1", []string{"key1", "key2"})

	resItr, err = qe.GetPrivateDataRangeScanIterator("ns4", "coll1", "key1", "key3")
	require.NoError(t, err)
	testItr(t, resItr, "ns4", "coll1", []string{})
}

func testItr(t *testing.T, itr commonledger.ResultsIterator, expectedNs string, expectedColl string, expectedKeys []string) {
	t.Logf("Testing itr for [%d] keys", len(expectedKeys))
	defer itr.Close()
	for _, expectedKey := range expectedKeys {
		queryResult, _ := itr.Next()
		pvtdataKV := queryResult.(*queryresult.KV)
		ns := pvtdataKV.Namespace
		key := pvtdataKV.Key
		require.Equal(t, expectedNs, ns)
		require.Equal(t, expectedKey, key)
	}
	last, err := itr.Next()
	require.NoError(t, err)
	require.Nil(t, last)
}

func TestPrivateDataMetadataRetrievalByHash(t *testing.T) {
	for _, testEnv := range testEnvs {
		testPrivateDataMetadataRetrievalByHash(t, testEnv)
	}
}

func testPrivateDataMetadataRetrievalByHash(t *testing.T, env testEnv) {
	ledgerid := "test-privatedata-metadata-retrieval-byhash"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 0,
		},
	)
	env.init(t, ledgerid, btlPolicy)
	defer env.cleanup()

	txMgr := env.getTxMgr()
	bg, _ := testutil.NewBlockGenerator(t, ledgerid, false)
	populateCollConfigForTest(t, txMgr, []collConfigkey{{"ns", "coll"}}, version.NewHeight(1, 1))
	// Simulate and commit tx1 - set val and metadata for key1
	key1, value1, metadata1 := "key1", []byte("value1"), map[string][]byte{"entry1": []byte("meatadata1-entry1")}
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	require.NoError(t, s1.SetPrivateData("ns", "coll", key1, value1))
	require.NoError(t, s1.SetPrivateDataMetadata("ns", "coll", key1, metadata1))
	s1.Done()
	blkAndPvtdata1, _ := prepareNextBlockForTestFromSimulator(t, bg, s1)
	_, _, _, err := txMgr.ValidateAndPrepare(blkAndPvtdata1, true)
	require.NoError(t, err)
	require.NoError(t, txMgr.Commit())

	t.Run("query-helper-for-queryexecutor", func(t *testing.T) {
		qe := newQueryExecutor(txMgr, "", nil, true, testHashFunc)
		metadataRetrieved, err := qe.GetPrivateDataMetadataByHash("ns", "coll", util.ComputeStringHash("key1"))
		require.NoError(t, err)
		require.Equal(t, metadata1, metadataRetrieved)
	})

	t.Run("query-helper-for-txsimulator", func(t *testing.T) {
		qe := newQueryExecutor(txMgr, "txid-1", rwsetutil.NewRWSetBuilder(), true, testHashFunc)
		_, err = qe.GetPrivateDataMetadataByHash("ns", "coll", util.ComputeStringHash("key1"))
		require.EqualError(t, err, "retrieving private data metadata by keyhash is not supported in simulation. This function is only available for query as yet")
	})
}

func TestGetPvtdataHash(t *testing.T) {
	for _, testEnv := range testEnvs {
		testGetPvtdataHash(t, testEnv)
	}
}

func testGetPvtdataHash(t *testing.T, env testEnv) {
	ledgerid := "test-get-pvtdata-hash"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 0,
		},
	)
	env.init(t, ledgerid, btlPolicy)
	defer env.cleanup()
	txMgr := env.getTxMgr()
	populateCollConfigForTest(t, txMgr, []collConfigkey{{"ns", "coll"}}, version.NewHeight(1, 1))

	batch := privacyenabledstate.NewUpdateBatch()
	batch.HashUpdates.Put(
		"ns", "coll",
		util.ComputeStringHash("existing-key"),
		util.ComputeStringHash("existing-value"),
		version.NewHeight(1, 1),
	)
	require.NoError(t, txMgr.db.ApplyPrivacyAwareUpdates(batch, version.NewHeight(1, 5)))

	s, _ := txMgr.NewTxSimulator("test_tx1")
	simulator := s.(*txSimulator)
	hash, err := simulator.GetPrivateDataHash("ns", "coll", "non-existing-key")
	require.NoError(t, err)
	require.Nil(t, hash)

	hash, err = simulator.GetPrivateDataHash("ns", "coll", "existing-key")
	require.NoError(t, err)
	require.Equal(t, util.ComputeStringHash("existing-value"), hash)
	simulator.Done()

	simRes, err := simulator.GetTxSimulationResults()
	require.NoError(t, err)
	require.False(t, simRes.ContainsPvtWrites())
	txrwset, _ := rwsetutil.TxRwSetFromProtoMsg(simRes.PubSimulationResults)

	expectedRwSet := &rwsetutil.TxRwSet{
		NsRwSets: []*rwsetutil.NsRwSet{
			{
				NameSpace: "ns",
				KvRwSet:   &kvrwset.KVRWSet{},
				CollHashedRwSets: []*rwsetutil.CollHashedRwSet{
					{
						CollectionName: "coll",
						HashedRwSet: &kvrwset.HashedRWSet{
							HashedReads: []*kvrwset.KVReadHash{
								{
									KeyHash: util.ComputeStringHash("existing-key"),
									Version: &kvrwset.Version{BlockNum: 1, TxNum: 1},
								},
								{
									KeyHash: util.ComputeStringHash("non-existing-key"),
									Version: nil,
								},
							},
						},
					},
				},
			},
		},
	}
	require.Equal(t, expectedRwSet, txrwset)
}

func putPvtUpdates(t *testing.T, updates *privacyenabledstate.UpdateBatch, ns, coll, key string, value []byte, ver *version.Height) {
	updates.PvtUpdates.Put(ns, coll, key, value, ver)
	updates.HashUpdates.Put(ns, coll, util.ComputeStringHash(key), util.ComputeHash(value), ver)
}
