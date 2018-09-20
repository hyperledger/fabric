/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package statebasedval

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
)

func TestRangeQueryBoundaryConditions(t *testing.T) {
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 0))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 1))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 2))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 3))
	batch.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 4))

	testcase1 := "NoResults"
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "key7", EndKey: "key10", ItrExhausted: true}
	rqi1.SetRawReads([]*kvrwset.KVRead{})
	testRangeQuery(t, testcase1, batch, version.NewHeight(1, 4), "ns1", rqi1, true)

	testcase2 := "NoResultsDuringValidation"
	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "key7", EndKey: "key10", ItrExhausted: true}
	rqi2.SetRawReads([]*kvrwset.KVRead{rwsetutil.NewKVRead("key8", version.NewHeight(1, 8))})
	testRangeQuery(t, testcase2, batch, version.NewHeight(1, 4), "ns1", rqi2, false)

	testcase3 := "OneExtraTailingResultsDuringValidation"
	rqi3 := &kvrwset.RangeQueryInfo{StartKey: "key1", EndKey: "key4", ItrExhausted: true}
	rqi3.SetRawReads([]*kvrwset.KVRead{
		rwsetutil.NewKVRead("key1", version.NewHeight(1, 0)),
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
	})
	testRangeQuery(t, testcase3, batch, version.NewHeight(1, 4), "ns1", rqi3, false)

	testcase4 := "TwoExtraTailingResultsDuringValidation"
	rqi4 := &kvrwset.RangeQueryInfo{StartKey: "key1", EndKey: "key5", ItrExhausted: true}
	rqi4.SetRawReads([]*kvrwset.KVRead{
		rwsetutil.NewKVRead("key1", version.NewHeight(1, 0)),
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
	})
	testRangeQuery(t, testcase4, batch, version.NewHeight(1, 4), "ns1", rqi4, false)
}

func testRangeQuery(t *testing.T, testcase string, stateData *statedb.UpdateBatch, savepoint *version.Height,
	ns string, rqi *kvrwset.RangeQueryInfo, expectedResult bool) {
	t.Run(testcase, func(t *testing.T) {
		testDBEnv := stateleveldb.NewTestVDBEnv(t)
		defer testDBEnv.Cleanup()
		db, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
		assert.NoError(t, err)
		if stateData != nil {
			db.ApplyUpdates(stateData, savepoint)
		}

		itr, err := db.GetStateRangeScanIterator(ns, rqi.StartKey, rqi.EndKey)
		assert.NoError(t, err)
		validator := &rangeQueryResultsValidator{}
		validator.init(rqi, itr)
		isValid, err := validator.validate()
		assert.NoError(t, err)
		assert.Equal(t, expectedResult, isValid)
	})
}
