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
	"sort"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/stretchr/testify/assert"
)

func TestPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("Nil value to Put() did not panic\n")
		}
	}()

	batch := NewUpdateBatch()
	// The following call to Put() should result in panic
	batch.Put("ns1", "key1", nil, nil)
}

//Test Put(), Get(), and Delete()
func TestPutGetDeleteExistsGetUpdates(t *testing.T) {
	batch := NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))

	//Get() should return above inserted <k,v> pair
	actualVersionedValue := batch.Get("ns1", "key1")
	assert.Equal(t, &VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}, actualVersionedValue)
	//Exists() should return false as key2 does not exist
	actualResult := batch.Exists("ns1", "key2")
	expectedResult := false
	assert.Equal(t, expectedResult, actualResult)

	//Exists() should return false as ns3 does not exist
	actualResult = batch.Exists("ns3", "key2")
	expectedResult = false
	assert.Equal(t, expectedResult, actualResult)

	//Get() should return nill as key2 does not exist
	actualVersionedValue = batch.Get("ns1", "key2")
	assert.Nil(t, actualVersionedValue)
	//Get() should return nill as ns3 does not exist
	actualVersionedValue = batch.Get("ns3", "key2")
	assert.Nil(t, actualVersionedValue)

	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	//Exists() should return true as key2 exists
	actualResult = batch.Exists("ns1", "key2")
	expectedResult = true
	assert.Equal(t, expectedResult, actualResult)

	//GetUpdatedNamespaces should return 3 namespaces
	batch.Put("ns2", "key2", []byte("value2"), version.NewHeight(1, 2))
	batch.Put("ns3", "key2", []byte("value2"), version.NewHeight(1, 2))
	actualNamespaces := batch.GetUpdatedNamespaces()
	sort.Strings(actualNamespaces)
	expectedNamespaces := []string{"ns1", "ns2", "ns3"}
	assert.Equal(t, expectedNamespaces, actualNamespaces)

	//GetUpdates should return two VersionedValues for the namespace ns1
	expectedUpdates := make(map[string]*VersionedValue)
	expectedUpdates["key1"] = &VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}
	expectedUpdates["key2"] = &VersionedValue{Value: []byte("value2"), Version: version.NewHeight(1, 2)}
	actualUpdates := batch.GetUpdates("ns1")
	assert.Equal(t, expectedUpdates, actualUpdates)

	actualUpdates = batch.GetUpdates("ns4")
	assert.Nil(t, actualUpdates)

	//Delete the above inserted <k,v> pair
	batch.Delete("ns1", "key2", version.NewHeight(1, 2))
	//Exists() should return true after deleting key2
	//Exists() should return true iff the key has action(Put/Delete) in this batch
	actualResult = batch.Exists("ns1", "key2")
	expectedResult = true
	assert.Equal(t, expectedResult, actualResult)

}

func TestUpdateBatchIterator(t *testing.T) {
	batch := NewUpdateBatch()
	batch.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	batch.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))

	batch.Put("ns2", "key6", []byte("value6"), version.NewHeight(2, 3))
	batch.Put("ns2", "key5", []byte("value5"), version.NewHeight(2, 2))
	batch.Put("ns2", "key4", []byte("value4"), version.NewHeight(2, 1))

	checkItrResults(t, batch.GetRangeScanIterator("ns1", "key2", "key3"), []*VersionedKV{
		{CompositeKey{"ns1", "key2"}, VersionedValue{[]byte("value2"), nil, version.NewHeight(1, 2)}},
	})

	checkItrResults(t, batch.GetRangeScanIterator("ns2", "key0", "key8"), []*VersionedKV{
		{CompositeKey{"ns2", "key4"}, VersionedValue{[]byte("value4"), nil, version.NewHeight(2, 1)}},
		{CompositeKey{"ns2", "key5"}, VersionedValue{[]byte("value5"), nil, version.NewHeight(2, 2)}},
		{CompositeKey{"ns2", "key6"}, VersionedValue{[]byte("value6"), nil, version.NewHeight(2, 3)}},
	})

	checkItrResults(t, batch.GetRangeScanIterator("ns2", "", ""), []*VersionedKV{
		{CompositeKey{"ns2", "key4"}, VersionedValue{[]byte("value4"), nil, version.NewHeight(2, 1)}},
		{CompositeKey{"ns2", "key5"}, VersionedValue{[]byte("value5"), nil, version.NewHeight(2, 2)}},
		{CompositeKey{"ns2", "key6"}, VersionedValue{[]byte("value6"), nil, version.NewHeight(2, 3)}},
	})

	checkItrResults(t, batch.GetRangeScanIterator("non-existing-ns", "", ""), nil)
}

func checkItrResults(t *testing.T, itr QueryResultsIterator, expectedResults []*VersionedKV) {
	for i := 0; i < len(expectedResults); i++ {
		res, _ := itr.Next()
		assert.Equal(t, expectedResults[i], res)
	}
	lastRes, err := itr.Next()
	assert.NoError(t, err)
	assert.Nil(t, lastRes)
	itr.Close()
}

// TestPaginatedRangeValidation tests queries with pagination
func TestPaginatedRangeValidation(t *testing.T) {

	queryOptions := make(map[string]interface{})
	queryOptions["limit"] = int32(10)

	err := ValidateRangeMetadata(queryOptions)
	assert.NoError(t, err, "An error was thrown for a valid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit"] = float32(10.2)

	err = ValidateRangeMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit"] = "10"

	err = ValidateRangeMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid option")

	queryOptions = make(map[string]interface{})
	queryOptions["limit1"] = int32(10)

	err = ValidateRangeMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid option")

}
