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

package statedb

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
)

func TestUpdateBatchIterator(t *testing.T) {
	batch := NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))

	batch.Put("ns2", "key6", []byte("value6"), version.NewHeight(2, 3))
	batch.Put("ns2", "key5", []byte("value5"), version.NewHeight(2, 2))
	batch.Put("ns2", "key4", []byte("value4"), version.NewHeight(2, 1))

	checkItrResults(t, batch.GetRangeScanIterator("ns1", "key2", "key3"), []*VersionedKV{
		&VersionedKV{CompositeKey{"ns1", "key2"}, VersionedValue{[]byte("value2"), version.NewHeight(1, 2)}},
	})

	checkItrResults(t, batch.GetRangeScanIterator("ns2", "key0", "key8"), []*VersionedKV{
		&VersionedKV{CompositeKey{"ns2", "key4"}, VersionedValue{[]byte("value4"), version.NewHeight(2, 1)}},
		&VersionedKV{CompositeKey{"ns2", "key5"}, VersionedValue{[]byte("value5"), version.NewHeight(2, 2)}},
		&VersionedKV{CompositeKey{"ns2", "key6"}, VersionedValue{[]byte("value6"), version.NewHeight(2, 3)}},
	})

	checkItrResults(t, batch.GetRangeScanIterator("ns2", "", ""), []*VersionedKV{
		&VersionedKV{CompositeKey{"ns2", "key4"}, VersionedValue{[]byte("value4"), version.NewHeight(2, 1)}},
		&VersionedKV{CompositeKey{"ns2", "key5"}, VersionedValue{[]byte("value5"), version.NewHeight(2, 2)}},
		&VersionedKV{CompositeKey{"ns2", "key6"}, VersionedValue{[]byte("value6"), version.NewHeight(2, 3)}},
	})

	checkItrResults(t, batch.GetRangeScanIterator("non-existing-ns", "", ""), nil)
}

func checkItrResults(t *testing.T, itr ResultsIterator, expectedResults []*VersionedKV) {
	for i := 0; i < len(expectedResults); i++ {
		res, _ := itr.Next()
		testutil.AssertEquals(t, res, expectedResults[i])
	}
	lastRes, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, lastRes)
}
