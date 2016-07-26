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

package buckettree

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/op/go-logging"
)

func TestBucketCache(t *testing.T) {
	testutil.SetLogLevel(logging.INFO, "buckettree")
	rootHash1, rootHash2, rootHash3, rootHash4 := testGetRootHashes(t, false)
	rootHash5, rootHash6, rootHash7, rootHash8 := testGetRootHashes(t, true)
	testutil.AssertEquals(t, rootHash1, rootHash5)
	testutil.AssertEquals(t, rootHash2, rootHash6)
	testutil.AssertEquals(t, rootHash3, rootHash7)
	testutil.AssertEquals(t, rootHash4, rootHash8)
}

func testGetRootHashes(t *testing.T, enableBlockCache bool) ([]byte, []byte, []byte, []byte) {
	// number of buckets at each level 26,9,3,1
	testHasher, stateImplTestWrapper, stateDelta := createFreshDBAndInitTestStateImplWithCustomHasher(t, 26, 3)
	// populate hash function such that they intersect at higher level buckets
	testHasher.populate("chaincodeID1", "key1", 1)
	testHasher.populate("chaincodeID2", "key2", 15)
	testHasher.populate("chaincodeID3", "key3", 26)

	if !enableBlockCache {
		stateImplTestWrapper.stateImpl.bucketCache = newBucketCache(0)
	}
	stateDelta.Set("chaincodeID1", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID2", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID3", "key3", []byte("value3"), nil)
	rootHash1 := stateImplTestWrapper.prepareWorkingSetAndComputeCryptoHash(stateDelta)
	stateImplTestWrapper.persistChangesAndResetInMemoryChanges()

	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID1", "key1", []byte("value1_new"), nil)
	rootHash2 := stateImplTestWrapper.prepareWorkingSetAndComputeCryptoHash(stateDelta)
	stateImplTestWrapper.persistChangesAndResetInMemoryChanges()

	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Delete("chaincodeID2", "key2", nil)
	rootHash3 := stateImplTestWrapper.prepareWorkingSetAndComputeCryptoHash(stateDelta)
	stateImplTestWrapper.persistChangesAndResetInMemoryChanges()

	if enableBlockCache {
		stateImplTestWrapper.stateImpl.bucketCache = newBucketCache(20)
		stateImplTestWrapper.stateImpl.bucketCache.loadAllBucketNodesFromDB()
	}
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID3", "key3", []byte("value3_new"), nil)
	rootHash4 := stateImplTestWrapper.prepareWorkingSetAndComputeCryptoHash(stateDelta)
	stateImplTestWrapper.persistChangesAndResetInMemoryChanges()
	return rootHash1, rootHash2, rootHash3, rootHash4
}
