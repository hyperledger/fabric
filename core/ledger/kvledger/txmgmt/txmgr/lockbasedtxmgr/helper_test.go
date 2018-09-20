/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package lockbasedtxmgr

import (
	"testing"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
	"github.com/stretchr/testify/assert"
)

func TestPvtdataResultsItr(t *testing.T) {
	testEnv := testEnvs[0]
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns1", "coll1", 0)
	cs.SetBTL("ns2", "coll1", 0)
	cs.SetBTL("ns3", "coll1", 0)
	testEnv.init(t, "test-pvtdata-range-queries", pvtdatapolicy.ConstructBTLPolicy(cs))
	defer testEnv.cleanup()

	txMgr := testEnv.getTxMgr().(*LockBasedTxMgr)
	populateCollConfigForTest(t, txMgr, []collConfigkey{
		{"ns1", "coll1"}, {"ns2", "coll1"}, {"ns3", "coll1"}, {"ns4", "coll1"}},
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
	txMgr.db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 7))
	queryHelper := newQueryHelper(txMgr, nil)

	resItr, err := queryHelper.getPrivateDataRangeScanIterator("ns1", "coll1", "key1", "key3")
	assert.NoError(t, err)
	testItr(t, resItr, "ns1", "coll1", []string{"key1", "key2"})

	resItr, err = queryHelper.getPrivateDataRangeScanIterator("ns4", "coll1", "key1", "key3")
	assert.NoError(t, err)
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
		assert.Equal(t, expectedNs, ns)
		assert.Equal(t, expectedKey, key)
	}
	last, err := itr.Next()
	assert.NoError(t, err)
	assert.Nil(t, last)
}

func TestPrivateDataMetadataRetrievalByHash(t *testing.T) {
	for _, testEnv := range testEnvs {
		testPrivateDataMetadataRetrievalByHash(t, testEnv)
	}
}

func testPrivateDataMetadataRetrievalByHash(t *testing.T, env testEnv) {
	ledgerid := "test-privatedata-metadata-retrieval-byhash"
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns", "coll", 0)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)
	env.init(t, ledgerid, btlPolicy)
	defer env.cleanup()

	txMgr := env.getTxMgr()
	bg, _ := testutil.NewBlockGenerator(t, ledgerid, false)
	populateCollConfigForTest(t, txMgr.(*LockBasedTxMgr), []collConfigkey{{"ns", "coll"}}, version.NewHeight(1, 1))
	// Simulate and commit tx1 - set val and metadata for key1
	key1, value1, metadata1 := "key1", []byte("value1"), map[string][]byte{"entry1": []byte("meatadata1-entry1")}
	s1, _ := txMgr.NewTxSimulator("test_tx1")
	s1.SetPrivateData("ns", "coll", key1, value1)
	s1.SetPrivateDataMetadata("ns", "coll", key1, metadata1)
	s1.Done()
	blkAndPvtdata1 := prepareNextBlockForTestFromSimulator(t, bg, s1)
	assert.NoError(t, txMgr.ValidateAndPrepare(blkAndPvtdata1, true))
	assert.NoError(t, txMgr.Commit())

	t.Run("query-helper-for-queryexecutor", func(t *testing.T) {
		queryHelper := newQueryHelper(txMgr.(*LockBasedTxMgr), nil)
		metadataRetrieved, err := queryHelper.getPrivateDataMetadataByHash("ns", "coll", util.ComputeStringHash("key1"))
		assert.NoError(t, err)
		assert.Equal(t, metadata1, metadataRetrieved)
	})

	t.Run("query-helper-for-txsimulator", func(t *testing.T) {
		queryHelper := newQueryHelper(txMgr.(*LockBasedTxMgr), rwsetutil.NewRWSetBuilder())
		_, err := queryHelper.getPrivateDataMetadataByHash("ns", "coll", util.ComputeStringHash("key1"))
		assert.EqualError(t, err, "retrieving private data metadata by keyhash is not supported in simulation. This function is only available for query as yet")
	})
}

func putPvtUpdates(t *testing.T, updates *privacyenabledstate.UpdateBatch, ns, coll, key string, value []byte, ver *version.Height) {
	updates.PvtUpdates.Put(ns, coll, key, value, ver)
	updates.HashUpdates.Put(ns, coll, util.ComputeStringHash(key), util.ComputeHash(value), ver)
}
