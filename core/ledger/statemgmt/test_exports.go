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

package statemgmt

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/ledger/testutil"
)

// AssertIteratorContains - tests wether the iterator (itr) contains expected results (provided in map)
func AssertIteratorContains(t *testing.T, itr RangeScanIterator, expected map[string][]byte) {
	count := 0
	actual := make(map[string][]byte)
	for itr.Next() {
		count++
		k, v := itr.GetKeyValue()
		actual[k] = v
	}

	t.Logf("Results from iterator: %s", actual)
	testutil.AssertEquals(t, count, len(expected))
	for k, v := range expected {
		testutil.AssertEquals(t, actual[k], v)
	}
}

// ConstructRandomStateDelta creates a random state delta for testing
func ConstructRandomStateDelta(
	t testing.TB,
	chaincodeIDPrefix string,
	numChaincodes int,
	maxKeySuffix int,
	numKeysToInsert int,
	kvSize int) *StateDelta {
	delta := NewStateDelta()
	s2 := rand.NewSource(time.Now().UnixNano())
	r2 := rand.New(s2)

	for i := 0; i < numKeysToInsert; i++ {
		chaincodeID := chaincodeIDPrefix + "_" + strconv.Itoa(r2.Intn(numChaincodes))
		key := "key_" + strconv.Itoa(r2.Intn(maxKeySuffix))
		valueSize := kvSize - len(key)
		if valueSize < 1 {
			panic(fmt.Errorf("valueSize cannot be less than one. ValueSize=%d", valueSize))
		}
		value := testutil.ConstructRandomBytes(t, valueSize)
		delta.Set(chaincodeID, key, value, nil)
	}

	for _, chaincodeDelta := range delta.ChaincodeStateDeltas {
		sortedKeys := chaincodeDelta.getSortedKeys()
		smallestKey := sortedKeys[0]
		largestKey := sortedKeys[len(sortedKeys)-1]
		t.Logf("chaincode=%s, numKeys=%d, smallestKey=%s, largestKey=%s", chaincodeDelta.ChaincodeID, len(sortedKeys), smallestKey, largestKey)
	}
	return delta
}
