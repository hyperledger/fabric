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

package commontests

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

// TestBasicRW tests basic read-write
func TestBasicRW(t *testing.T, db statedb.VersionedDB) {
	db.Open()
	defer db.Close()
	val, err := db.GetState("ns", "key1")
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, val)

	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value3"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte{}, Version: version.NewHeight(1, 4)}
	batch.Put("ns1", "key1", vv1.Value, vv1.Version)
	batch.Put("ns1", "key2", vv2.Value, vv2.Version)
	batch.Put("ns2", "key3", vv3.Value, vv3.Version)
	batch.Put("ns2", "key4", vv4.Value, vv4.Version)
	savePoint := version.NewHeight(2, 5)
	db.ApplyUpdates(batch, savePoint)

	vv, _ := db.GetState("ns1", "key1")
	testutil.AssertEquals(t, vv, &vv1)

	vv, _ = db.GetState("ns2", "key4")
	testutil.AssertEquals(t, vv, &vv4)

	sp, err := db.GetLatestSavePoint()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, sp, savePoint)
}

// TestDeletes tests deteles
func TestDeletes(t *testing.T, db statedb.VersionedDB) {
	db.Open()
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	vv1 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	vv2 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	vv3 := statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 3)}
	vv4 := statedb.VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 4)}

	batch.Put("ns", "key1", vv1.Value, vv1.Version)
	batch.Put("ns", "key2", vv2.Value, vv2.Version)
	batch.Put("ns", "key3", vv2.Value, vv3.Version)
	batch.Put("ns", "key4", vv2.Value, vv4.Version)
	batch.Delete("ns", "key3", version.NewHeight(1, 5))
	savePoint := version.NewHeight(1, 5)
	err := db.ApplyUpdates(batch, savePoint)
	testutil.AssertNoError(t, err, "")
	vv, _ := db.GetState("ns", "key2")
	testutil.AssertEquals(t, vv, &vv2)

	vv, err = db.GetState("ns", "key3")
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, vv)

	batch = statedb.NewUpdateBatch()
	batch.Delete("ns", "key2", version.NewHeight(1, 6))
	err = db.ApplyUpdates(batch, savePoint)
	testutil.AssertNoError(t, err, "")
	vv, err = db.GetState("ns", "key2")
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, vv)
}

// TestIterator tests the iterator
func TestIterator(t *testing.T, db statedb.VersionedDB) {
	db.Open()
	defer db.Close()
	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))
	batch.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 4))
	batch.Put("ns2", "key5", []byte("value5"), version.NewHeight(1, 5))
	batch.Put("ns2", "key6", []byte("value6"), version.NewHeight(1, 6))
	batch.Put("ns3", "key7", []byte("value7"), version.NewHeight(1, 7))
	savePoint := version.NewHeight(2, 5)
	db.ApplyUpdates(batch, savePoint)

	itr1, _ := db.GetStateRangeScanIterator("ns1", "key1", "")
	testItr(t, itr1, []string{"key1", "key2", "key3", "key4"})

	itr2, _ := db.GetStateRangeScanIterator("ns1", "key2", "key3")
	testItr(t, itr2, []string{"key2"})

	itr3, _ := db.GetStateRangeScanIterator("ns1", "", "")
	testItr(t, itr3, []string{"key1", "key2", "key3", "key4"})

	itr4, _ := db.GetStateRangeScanIterator("ns2", "", "")
	testItr(t, itr4, []string{"key5", "key6"})
}

func testItr(t *testing.T, itr statedb.ResultsIterator, expectedKeys []string) {
	defer itr.Close()
	for _, expectedKey := range expectedKeys {
		vkv, _ := itr.Next()
		key := vkv.Key
		testutil.AssertEquals(t, key, expectedKey)
	}
	last, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, last)
}
