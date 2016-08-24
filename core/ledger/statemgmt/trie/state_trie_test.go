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

package trie

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestStateTrie_ComputeHash_AllInMemory_NoContents(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateTrie := NewStateImpl()
	stateTrieTestWrapper := &stateTrieTestWrapper{stateTrie, t}
	hash := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(statemgmt.NewStateDelta())
	testutil.AssertEquals(t, hash, nil)
}

func TestStateTrie_ComputeHash_AllInMemory(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateTrie := NewStateImpl()
	stateTrieTestWrapper := &stateTrieTestWrapper{stateTrie, t}
	stateDelta := statemgmt.NewStateDelta()

	// Test1 - Add a few keys
	stateDelta.Set("chaincodeID1", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID1", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID2", "key3", []byte("value3"), nil)
	stateDelta.Set("chaincodeID2", "key4", []byte("value4"), nil)
	stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	rootHash1 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)

	hash1 := expectedCryptoHashForTest(newTrieKey("chaincodeID1", "key1"), []byte("value1"))
	hash2 := expectedCryptoHashForTest(newTrieKey("chaincodeID1", "key2"), []byte("value2"))
	hash3 := expectedCryptoHashForTest(newTrieKey("chaincodeID2", "key3"), []byte("value3"))
	hash4 := expectedCryptoHashForTest(newTrieKey("chaincodeID2", "key4"), []byte("value4"))

	hash1Hash2 := expectedCryptoHashForTest(nil, nil, hash1, hash2)
	hash3Hash4 := expectedCryptoHashForTest(nil, nil, hash3, hash4)
	expectedRootHash1 := expectedCryptoHashForTest(nil, nil, hash1Hash2, hash3Hash4)
	testutil.AssertEquals(t, rootHash1, expectedRootHash1)
	stateTrie.ClearWorkingSet(true)

	//Test2 - Add one more key
	t.Logf("-- Add one more key exiting key --- ")
	stateDelta.Set("chaincodeID3", "key5", []byte("value5"), nil)
	rootHash2 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	hash5 := expectedCryptoHashForTest(newTrieKey("chaincodeID3", "key5"), []byte("value5"))
	expectedRootHash2 := expectedCryptoHashForTest(nil, nil, hash1Hash2, hash3Hash4, hash5)
	testutil.AssertEquals(t, rootHash2, expectedRootHash2)
	stateTrie.ClearWorkingSet(true)

	// Test3 - Remove one of the existing keys
	t.Logf("-- Remove an exiting key --- ")
	stateDelta.Delete("chaincodeID2", "key4", nil)
	rootHash3 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	expectedRootHash3 := expectedCryptoHashForTest(nil, nil, hash1Hash2, hash3, hash5)
	testutil.AssertEquals(t, rootHash3, expectedRootHash3)
	stateTrie.ClearWorkingSet(true)
}

func TestStateTrie_GetSet_WithDB(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateTrie := NewStateImpl()
	stateTrieTestWrapper := &stateTrieTestWrapper{stateTrie, t}
	stateDelta := statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID1", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID1", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID2", "key3", []byte("value3"), nil)
	stateDelta.Set("chaincodeID2", "key4", []byte("value4"), nil)
	stateDelta.Set("chaincodeID3", "key5", []byte{}, nil)
	stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()
	testutil.AssertEquals(t, stateTrieTestWrapper.Get("chaincodeID1", "key1"), []byte("value1"))

	emptyBytes := stateTrieTestWrapper.Get("chaincodeID3", "key5")
	if emptyBytes == nil || len(emptyBytes) != 0 {
		t.Fatalf("Expected an empty byte array. found = %#v", emptyBytes)
	}
	nilVal := stateTrieTestWrapper.Get("chaincodeID3", "non-existing-key")
	if nilVal != nil {
		t.Fatalf("Expected a nil. found = %#v", nilVal)
	}
}

func TestStateTrie_ComputeHash_WithDB_Spread_Keys(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateTrie := NewStateImpl()
	stateTrieTestWrapper := &stateTrieTestWrapper{stateTrie, t}

	// Add a few keys and write to DB
	stateDelta := statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID1", "key1", []byte("value1"), nil)
	stateDelta.Set("chaincodeID1", "key2", []byte("value2"), nil)
	stateDelta.Set("chaincodeID2", "key3", []byte("value3"), nil)
	stateDelta.Set("chaincodeID2", "key4", []byte("value4"), nil)
	stateTrie.PrepareWorkingSet(stateDelta)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	/////////////////////////////////////////////////////////
	// Test1 - Add a non-existing key
	/////////////////////////////////////////////////////////
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID3", "key5", []byte("value5"), nil)
	rootHash1 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	expectedHash1 := expectedCryptoHashForTest(newTrieKey("chaincodeID1", "key1"), []byte("value1"))
	expectedHash2 := expectedCryptoHashForTest(newTrieKey("chaincodeID1", "key2"), []byte("value2"))
	expectedHash3 := expectedCryptoHashForTest(newTrieKey("chaincodeID2", "key3"), []byte("value3"))
	expectedHash4 := expectedCryptoHashForTest(newTrieKey("chaincodeID2", "key4"), []byte("value4"))
	expectedHash1Hash2 := expectedCryptoHashForTest(nil, nil, expectedHash1, expectedHash2)
	expectedHash3Hash4 := expectedCryptoHashForTest(nil, nil, expectedHash3, expectedHash4)
	expectedHash5 := expectedCryptoHashForTest(newTrieKey("chaincodeID3", "key5"), []byte("value5"))
	expectedRootHash1 := expectedCryptoHashForTest(nil, nil, expectedHash1Hash2, expectedHash3Hash4, expectedHash5)
	testutil.AssertEquals(t, rootHash1, expectedRootHash1)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	/////////////////////////////////////////////////////////
	// Test2 - Change value of an existing key
	/////////////////////////////////////////////////////////
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID2", "key4", []byte("value4-new"), nil)
	rootHash2 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	expectedHash4 = expectedCryptoHashForTest(newTrieKey("chaincodeID2", "key4"), []byte("value4-new"))
	expectedHash3Hash4 = expectedCryptoHashForTest(nil, nil, expectedHash3, expectedHash4)
	expectedRootHash2 := expectedCryptoHashForTest(nil, nil, expectedHash1Hash2, expectedHash3Hash4, expectedHash5)
	testutil.AssertEquals(t, rootHash2, expectedRootHash2)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	/////////////////////////////////////////////////////////
	// Test3 - Change value of another existing key
	/////////////////////////////////////////////////////////
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID1", "key1", []byte("value1-new"), nil)
	rootHash3 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	expectedHash1 = expectedCryptoHashForTest(newTrieKey("chaincodeID1", "key1"), []byte("value1-new"))
	expectedHash1Hash2 = expectedCryptoHashForTest(nil, nil, expectedHash1, expectedHash2)
	expectedRootHash3 := expectedCryptoHashForTest(nil, nil, expectedHash1Hash2, expectedHash3Hash4, expectedHash5)
	testutil.AssertEquals(t, rootHash3, expectedRootHash3)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	/////////////////////////////////////////////////////////
	// Test4 - Delete an existing existing key
	/////////////////////////////////////////////////////////
	t.Logf("-- Delete an existing key ---")
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Delete("chaincodeID3", "key5", nil)
	rootHash4 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	expectedRootHash4 := expectedCryptoHashForTest(nil, nil, expectedHash1Hash2, expectedHash3Hash4)
	testutil.AssertEquals(t, rootHash4, expectedRootHash4)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()
	// Delete should remove the key from db because, this key has no value and no children
	testutil.AssertNil(t, testDBWrapper.GetFromStateCF(t, newTrieKey("chaincodeID3", "key5").getEncodedBytes()))

	/////////////////////////////////////////////////////////
	// Test5 - Delete another existing existing key
	/////////////////////////////////////////////////////////
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Delete("chaincodeID2", "key4", nil)
	rootHash5 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	expectedRootHash5 := expectedCryptoHashForTest(nil, nil, expectedHash1Hash2, expectedHash3)
	testutil.AssertEquals(t, rootHash5, expectedRootHash5)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()
	testutil.AssertNil(t, testDBWrapper.GetFromStateCF(t, newTrieKey("chaincodeID2", "key4").getEncodedBytes()))
}

func TestStateTrie_ComputeHash_WithDB_Staggered_Keys(t *testing.T) {
	testDBWrapper.CleanDB(t)
	stateTrie := NewStateImpl()
	stateTrieTestWrapper := &stateTrieTestWrapper{stateTrie, t}

	/////////////////////////////////////////////////////////
	// Test1 - Add a few staggered keys
	/////////////////////////////////////////////////////////
	stateDelta := statemgmt.NewStateDelta()
	stateDelta.Set("ID", "key1", []byte("value_key1"), nil)
	stateDelta.Set("ID", "key", []byte("value_key"), nil)
	stateDelta.Set("ID", "k", []byte("value_k"), nil)
	expectedHashKey1 := expectedCryptoHashForTest(newTrieKey("ID", "key1"), []byte("value_key1"))
	expectedHashKey := expectedCryptoHashForTest(newTrieKey("ID", "key"), []byte("value_key"), expectedHashKey1)
	expectedHashK := expectedCryptoHashForTest(newTrieKey("ID", "k"), []byte("value_k"), expectedHashKey)
	rootHash1 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	testutil.AssertEquals(t, rootHash1, expectedHashK)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	/////////////////////////////////////////////////////////
	// Test2 - Add a new key in path of existing staggered keys
	/////////////////////////////////////////////////////////
	t.Logf("- Add a new key in path of existing staggered keys -")
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("ID", "ke", []byte("value_ke"), nil)
	expectedHashKe := expectedCryptoHashForTest(newTrieKey("ID", "ke"), []byte("value_ke"), expectedHashKey)
	expectedHashK = expectedCryptoHashForTest(newTrieKey("ID", "k"), []byte("value_k"), expectedHashKe)
	rootHash2 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	testutil.AssertEquals(t, rootHash2, expectedHashK)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	/////////////////////////////////////////////////////////
	// Test3 - Change value of one of the existing keys
	/////////////////////////////////////////////////////////
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("ID", "ke", []byte("value_ke_new"), nil)
	expectedHashKe = expectedCryptoHashForTest(newTrieKey("ID", "ke"), []byte("value_ke_new"), expectedHashKey)
	expectedHashK = expectedCryptoHashForTest(newTrieKey("ID", "k"), []byte("value_k"), expectedHashKe)
	rootHash3 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	testutil.AssertEquals(t, rootHash3, expectedHashK)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()

	/////////////////////////////////////////////////////////
	// Test4 - delete one of the existing keys
	/////////////////////////////////////////////////////////
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Delete("ID", "ke", nil)
	expectedHashK = expectedCryptoHashForTest(newTrieKey("ID", "k"), []byte("value_k"), expectedHashKey)
	rootHash4 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	testutil.AssertEquals(t, rootHash4, expectedHashK)
	stateTrieTestWrapper.PersistChangesAndResetInMemoryChanges()
	// Delete should not remove the key from db because, this key has children
	testutil.AssertNotNil(t, testDBWrapper.GetFromStateCF(t, newTrieKey("ID", "ke").getEncodedBytes()))
	testutil.AssertEquals(t, rootHash1, rootHash4)

	//////////////////////////////////////////////////////////////
	// Test4 -  Add one more key as a sibling of an intermediate node
	//////////////////////////////////////////////////////////////
	stateDelta = statemgmt.NewStateDelta()
	stateDelta.Set("ID", "kez", []byte("value_kez"), nil)
	expectedHashKez := expectedCryptoHashForTest(newTrieKey("ID", "kez"), []byte("value_kez"))
	expectedHashKe = expectedCryptoHashForTest(nil, nil, expectedHashKey, expectedHashKez)
	expectedHashK = expectedCryptoHashForTest(newTrieKey("ID", "k"), []byte("value_k"), expectedHashKe)
	rootHash5 := stateTrieTestWrapper.PrepareWorkingSetAndComputeCryptoHash(stateDelta)
	testutil.AssertEquals(t, rootHash5, expectedHashK)
}
