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
	"fmt"

	"github.com/hyperledger/fabric/core/db"
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/op/go-logging"
	"github.com/tecbot/gorocksdb"
)

var stateTrieLogger = logging.MustGetLogger("stateTrie")
var logHashOfEveryNode = false

// StateTrie defines the trie for the state, a merkle tree where keys
// and values are stored for fast hash computation.
type StateTrie struct {
	trieDelta              *trieDelta
	persistedStateHash     []byte
	lastComputedCryptoHash []byte
	recomputeCryptoHash    bool
}

// NewStateTrie contructs a new empty StateTrie
func NewStateTrie() *StateTrie {
	return &StateTrie{}
}

// Initialize the state trie with the root key
func (stateTrie *StateTrie) Initialize(configs map[string]interface{}) error {
	rootNode, err := fetchTrieNodeFromDB(rootTrieKey)
	if err != nil {
		panic(fmt.Errorf("Error in fetching root node from DB while initializing state trie: %s", err))
	}
	if rootNode != nil {
		stateTrie.persistedStateHash = rootNode.computeCryptoHash()
		stateTrie.lastComputedCryptoHash = stateTrie.persistedStateHash
	}
	return nil
}

// Get the value for a given chaincode ID and key
func (stateTrie *StateTrie) Get(chaincodeID string, key string) ([]byte, error) {
	trieNode, err := fetchTrieNodeFromDB(newTrieKey(chaincodeID, key))
	if err != nil {
		return nil, err
	}
	if trieNode == nil {
		return nil, nil
	}
	return trieNode.value, nil
}

// PrepareWorkingSet creates the start of a new delta
func (stateTrie *StateTrie) PrepareWorkingSet(stateDelta *statemgmt.StateDelta) error {
	stateTrie.trieDelta = newTrieDelta(stateDelta)
	stateTrie.recomputeCryptoHash = true
	return nil
}

// ClearWorkingSet clears the existing delta
func (stateTrie *StateTrie) ClearWorkingSet(changesPersisted bool) {
	stateTrie.trieDelta = nil
	stateTrie.recomputeCryptoHash = false

	if changesPersisted {
		stateTrie.persistedStateHash = stateTrie.lastComputedCryptoHash
	} else {
		stateTrie.lastComputedCryptoHash = stateTrie.persistedStateHash
	}
}

// ComputeCryptoHash returns the hash of the current state trie
func (stateTrie *StateTrie) ComputeCryptoHash() ([]byte, error) {
	stateTrieLogger.Debug("Enter - ComputeCryptoHash()")
	if !stateTrie.recomputeCryptoHash {
		stateTrieLogger.Debug("No change since last time crypto-hash was computed. Returning result from last computation")
		return stateTrie.lastComputedCryptoHash, nil
	}
	lowestLevel := stateTrie.trieDelta.getLowestLevel()
	stateTrieLogger.Debugf("Lowest level in trieDelta = [%d]", lowestLevel)
	for level := lowestLevel; level > 0; level-- {
		changedNodes := stateTrie.trieDelta.deltaMap[level]
		for _, changedNode := range changedNodes {
			err := stateTrie.processChangedNode(changedNode)
			if err != nil {
				return nil, err
			}
		}
	}
	trieRootNode := stateTrie.trieDelta.getTrieRootNode()
	if trieRootNode == nil {
		return stateTrie.lastComputedCryptoHash, nil
	}
	stateTrie.lastComputedCryptoHash = trieRootNode.computeCryptoHash()
	stateTrie.recomputeCryptoHash = false
	hash := stateTrie.lastComputedCryptoHash
	stateTrieLogger.Debug("Exit - ComputeCryptoHash()")
	return hash, nil
}

func (stateTrie *StateTrie) processChangedNode(changedNode *trieNode) error {
	stateTrieLogger.Debugf("Enter - processChangedNode() for node [%s]", changedNode)
	dbNode, err := fetchTrieNodeFromDB(changedNode.trieKey)
	if err != nil {
		return err
	}
	if dbNode != nil {
		stateTrieLogger.Debugf("processChangedNode() - merging attributes from db node [%s]", dbNode)
		changedNode.mergeMissingAttributesFrom(dbNode)
	}
	newCryptoHash := changedNode.computeCryptoHash()
	parentNode := stateTrie.trieDelta.getParentOf(changedNode)
	if parentNode == nil {
		parentNode = newTrieNode(changedNode.getParentTrieKey(), nil, false)
		stateTrie.trieDelta.addTrieNode(parentNode)
	}
	parentNode.setChildCryptoHash(changedNode.getIndexInParent(), newCryptoHash)
	if logHashOfEveryNode {
		stateTrieLogger.Debugf("Hash for changedNode[%s]", changedNode)
		stateTrieLogger.Debugf("%#v", newCryptoHash)
	}
	stateTrieLogger.Debugf("Exit - processChangedNode() for node [%s]", changedNode)
	return nil
}

// AddChangesForPersistence commits current changes to the database
func (stateTrie *StateTrie) AddChangesForPersistence(writeBatch *gorocksdb.WriteBatch) error {
	if stateTrie.recomputeCryptoHash {
		_, err := stateTrie.ComputeCryptoHash()
		if err != nil {
			return err
		}
	}

	if stateTrie.trieDelta == nil {
		stateTrieLogger.Info("trieDelta is nil. Not writing anything to DB")
		return nil
	}

	openchainDB := db.GetDBHandle()
	lowestLevel := stateTrie.trieDelta.getLowestLevel()
	for level := lowestLevel; level >= 0; level-- {
		changedNodes := stateTrie.trieDelta.deltaMap[level]
		for _, changedNode := range changedNodes {
			if changedNode.markedForDeletion {
				writeBatch.DeleteCF(openchainDB.StateCF, changedNode.trieKey.getEncodedBytes())
				continue
			}
			serializedContent, err := changedNode.marshal()
			if err != nil {
				return err
			}
			writeBatch.PutCF(openchainDB.StateCF, changedNode.trieKey.getEncodedBytes(), serializedContent)
		}
	}
	stateTrieLogger.Debug("Added changes to DB")
	return nil
}

// PerfHintKeyChanged is currently a no-op. Can perform pre-fetching of relevant data from db here.
func (stateTrie *StateTrie) PerfHintKeyChanged(chaincodeID string, key string) {
	// nothing for now. Can perform pre-fetching of relevant data from db here.
}

// GetStateSnapshotIterator - method implementation for interface 'statemgmt.HashableState'
func (stateTrie *StateTrie) GetStateSnapshotIterator(snapshot *gorocksdb.Snapshot) (statemgmt.StateSnapshotIterator, error) {
	return newStateSnapshotIterator(snapshot)
}

// GetRangeScanIterator returns an iterator for performing a range scan between the start and end keys
func (stateTrie *StateTrie) GetRangeScanIterator(chaincodeID string, startKey string, endKey string) (statemgmt.RangeScanIterator, error) {
	return newRangeScanIterator(chaincodeID, startKey, endKey)
}
