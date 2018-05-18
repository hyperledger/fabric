/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package lockbasedtxmgr

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"

	commonledger "github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/ledger/queryresult"
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
	testutil.AssertNoError(t, err, "")
	testItr(t, resItr, "ns1", "coll1", []string{"key1", "key2"})

	resItr, err = queryHelper.getPrivateDataRangeScanIterator("ns4", "coll1", "key1", "key3")
	testutil.AssertNoError(t, err, "")
	testItr(t, resItr, "ns4", "coll1", []string{})
}

func putPvtUpdates(t *testing.T, updates *privacyenabledstate.UpdateBatch, ns, coll, key string, value []byte, ver *version.Height) {
	updates.PvtUpdates.Put(ns, coll, key, value, ver)
	updates.HashUpdates.Put(ns, coll, util.ComputeStringHash(key), util.ComputeHash(value), ver)
}

func testItr(t *testing.T, itr commonledger.ResultsIterator, expectedNs string, expectedColl string, expectedKeys []string) {
	t.Logf("Testing itr for [%d] keys", len(expectedKeys))
	defer itr.Close()
	for _, expectedKey := range expectedKeys {
		queryResult, _ := itr.Next()
		pvtdataKV := queryResult.(*queryresult.KV)
		ns := pvtdataKV.Namespace
		key := pvtdataKV.Key
		testutil.AssertEquals(t, ns, expectedNs)
		testutil.AssertEquals(t, key, expectedKey)
	}
	last, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, last)
}
