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

package statebasedval

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

func TestCombinedIterator(t *testing.T) {
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	defer testDBEnv.Cleanup()

	db, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	// populate db with initial data
	batch := statedb.NewUpdateBatch()
	batch.Put("ns", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns", "key4", []byte("value4"), version.NewHeight(1, 1))
	batch.Put("ns", "key6", []byte("value6"), version.NewHeight(1, 1))
	batch.Put("ns", "key8", []byte("value8"), version.NewHeight(1, 1))
	db.ApplyUpdates(batch, version.NewHeight(1, 5))

	// prepare batch1
	batch1 := statedb.NewUpdateBatch()
	batch1.Put("ns", "key3", []byte("value3"), version.NewHeight(1, 1))
	batch1.Delete("ns", "key5", version.NewHeight(1, 1))
	batch1.Put("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1))
	batch1.Put("ns", "key7", []byte("value7"), version.NewHeight(1, 1))

	// prepare batch2 (empty)
	batch2 := statedb.NewUpdateBatch()

	// Test db + batch1 updates (exclude endKey)
	itr1, _ := newCombinedIterator(db, batch1, "ns", "key2", "key8", false)
	defer itr1.Close()
	checkItrResults(t, "ExcludeEndKey", itr1, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
	})

	// Test db + batch1 updates (include endKey)
	itr1WithEndKey, _ := newCombinedIterator(db, batch1, "ns", "key2", "key8", true)
	defer itr1WithEndKey.Close()
	checkItrResults(t, "IncludeEndKey", itr1WithEndKey, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key8", []byte("value8"), version.NewHeight(1, 1)),
	})

	// Test db + batch1 updates (include endKey) for extra range
	itr1WithEndKeyExtraRange, _ := newCombinedIterator(db, batch1, "ns", "key0", "key9", true)
	defer itr1WithEndKeyExtraRange.Close()
	checkItrResults(t, "IncludeEndKey_ExtraRange", itr1WithEndKeyExtraRange, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key1", []byte("value1"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key8", []byte("value8"), version.NewHeight(1, 1)),
	})

	// Test db + batch1 updates with full range query
	itr3, _ := newCombinedIterator(db, batch1, "ns", "", "", false)
	defer itr3.Close()
	checkItrResults(t, "ExcludeEndKey_FullRange", itr3, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key1", []byte("value1"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key8", []byte("value8"), version.NewHeight(1, 1)),
	})

	// Test db + batch2 updates
	itr2, _ := newCombinedIterator(db, batch2, "ns", "key2", "key8", false)
	defer itr2.Close()
	checkItrResults(t, "ExcludeEndKey_EmptyUpdates", itr2, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6"), version.NewHeight(1, 1)),
	})
}

func checkItrResults(t *testing.T, testName string, itr statedb.ResultsIterator, expectedResults []*statedb.VersionedKV) {
	t.Run(testName, func(t *testing.T) {
		for i := 0; i < len(expectedResults); i++ {
			res, _ := itr.Next()
			testutil.AssertEquals(t, res, expectedResults[i])
		}
		lastRes, err := itr.Next()
		testutil.AssertNoError(t, err, "")
		testutil.AssertNil(t, lastRes)
	})
}

func constructVersionedKV(ns string, key string, value []byte, version *version.Height) *statedb.VersionedKV {
	return &statedb.VersionedKV{
		CompositeKey:   statedb.CompositeKey{Namespace: ns, Key: key},
		VersionedValue: statedb.VersionedValue{Value: value, Version: version}}
}
