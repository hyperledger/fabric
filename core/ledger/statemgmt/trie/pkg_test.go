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
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/util"
	"github.com/tecbot/gorocksdb"
)

var testDBWrapper = db.NewTestDBWrapper()

type stateTrieTestWrapper struct {
	stateTrie *StateTrie
	t         *testing.T
}

func newStateTrieTestWrapper(t *testing.T) *stateTrieTestWrapper {
	return &stateTrieTestWrapper{NewStateTrie(), t}
}

func (stateTrieTestWrapper *stateTrieTestWrapper) Get(chaincodeID string, key string) []byte {
	value, err := stateTrieTestWrapper.stateTrie.Get(chaincodeID, key)
	testutil.AssertNoError(stateTrieTestWrapper.t, err, "Error while getting value")
	stateTrieTestWrapper.t.Logf("state value for chaincodeID,key=[%s,%s] = [%s], ", chaincodeID, key, string(value))
	return value
}

func (stateTrieTestWrapper *stateTrieTestWrapper) PrepareWorkingSetAndComputeCryptoHash(stateDelta *statemgmt.StateDelta) []byte {
	stateTrieTestWrapper.stateTrie.PrepareWorkingSet(stateDelta)
	cryptoHash, err := stateTrieTestWrapper.stateTrie.ComputeCryptoHash()
	testutil.AssertNoError(stateTrieTestWrapper.t, err, "Error while computing crypto hash")
	stateTrieTestWrapper.t.Logf("Cryptohash = [%x]", cryptoHash)
	return cryptoHash
}

func (stateTrieTestWrapper *stateTrieTestWrapper) AddChangesForPersistence(writeBatch *gorocksdb.WriteBatch) {
	err := stateTrieTestWrapper.stateTrie.AddChangesForPersistence(writeBatch)
	testutil.AssertNoError(stateTrieTestWrapper.t, err, "Error while adding changes to db write-batch")
}

func (stateTrieTestWrapper *stateTrieTestWrapper) PersistChangesAndResetInMemoryChanges() {
	writeBatch := gorocksdb.NewWriteBatch()
	defer writeBatch.Destroy()
	stateTrieTestWrapper.AddChangesForPersistence(writeBatch)
	testDBWrapper.WriteToDB(stateTrieTestWrapper.t, writeBatch)
	stateTrieTestWrapper.stateTrie.ClearWorkingSet(true)
}

type trieNodeTestWrapper struct {
	trieNode *trieNode
	t        *testing.T
}

func (trieNodeTestWrapper *trieNodeTestWrapper) marshal() []byte {
	serializedContent, err := trieNodeTestWrapper.trieNode.marshal()
	testutil.AssertNoError(trieNodeTestWrapper.t, err, "Error while marshalling trieNode")
	return serializedContent
}

func (trieNodeTestWrapper *trieNodeTestWrapper) unmarshal(key *trieKey, serializedContent []byte) *trieNode {
	trieNode, err := unmarshalTrieNode(key, serializedContent)
	testutil.AssertNoError(trieNodeTestWrapper.t, err, "Error while unmarshalling trieNode")
	return trieNode
}

func TestMain(m *testing.M) {
	testutil.SetupTestConfig()
	os.Exit(m.Run())
}

func expectedCryptoHashForTest(key *trieKey, value []byte, childrenHashes ...[]byte) []byte {
	expectedHash := []byte{}
	if key != nil {
		keyBytes := key.getEncodedBytes()
		expectedHash = append(expectedHash, proto.EncodeVarint(uint64(len(keyBytes)))...)
		expectedHash = append(expectedHash, keyBytes...)
		expectedHash = append(expectedHash, value...)
	}
	for _, b := range childrenHashes {
		expectedHash = append(expectedHash, b...)
	}
	return util.ComputeCryptoHash(expectedHash)
}
