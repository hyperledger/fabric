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
	"github.com/hyperledger/fabric/core/ledger/statemgmt"
)

type levelDeltaMap map[string]*trieNode

type trieDelta struct {
	lowestLevel int
	deltaMap    map[int]levelDeltaMap
}

func newLevelDeltaMap() levelDeltaMap {
	return levelDeltaMap(make(map[string]*trieNode))
}

func newTrieDelta(stateDelta *statemgmt.StateDelta) *trieDelta {
	trieDelta := &trieDelta{0, make(map[int]levelDeltaMap)}
	chaincodes := stateDelta.GetUpdatedChaincodeIds(false)
	for _, chaincodeID := range chaincodes {
		updates := stateDelta.GetUpdates(chaincodeID)
		for key, updatedvalue := range updates {
			if updatedvalue.IsDeleted() {
				trieDelta.delete(chaincodeID, key)
			} else {
				if stateDelta.RollBackwards {
					trieDelta.set(chaincodeID, key, updatedvalue.GetPreviousValue())
				} else {
					trieDelta.set(chaincodeID, key, updatedvalue.GetValue())
				}
			}
		}
	}
	return trieDelta
}

func (trieDelta *trieDelta) getLowestLevel() int {
	return trieDelta.lowestLevel
}

func (trieDelta *trieDelta) getChangesAtLevel(level int) []*trieNode {
	levelDelta := trieDelta.deltaMap[level]
	changedNodes := make([]*trieNode, len(levelDelta))
	for _, v := range levelDelta {
		changedNodes = append(changedNodes, v)
	}
	return changedNodes
}

func (trieDelta *trieDelta) getParentOf(trieNode *trieNode) *trieNode {
	parentLevel := trieNode.getParentLevel()
	parentTrieKey := trieNode.getParentTrieKey()
	levelDeltaMap := trieDelta.deltaMap[parentLevel]
	if levelDeltaMap == nil {
		return nil
	}
	return levelDeltaMap[parentTrieKey.getEncodedBytesAsStr()]
}

func (trieDelta *trieDelta) addTrieNode(trieNode *trieNode) {
	level := trieNode.getLevel()
	levelDeltaMap := trieDelta.deltaMap[level]
	if levelDeltaMap == nil {
		levelDeltaMap = newLevelDeltaMap()
		trieDelta.deltaMap[level] = levelDeltaMap
	}
	levelDeltaMap[trieNode.trieKey.getEncodedBytesAsStr()] = trieNode
	if level > trieDelta.lowestLevel {
		trieDelta.lowestLevel = level
	}
}

func (trieDelta *trieDelta) getTrieRootNode() *trieNode {
	levelZeroMap := trieDelta.deltaMap[0]
	if levelZeroMap == nil {
		return nil
	}
	return levelZeroMap[rootTrieKeyStr]
}

func (trieDelta *trieDelta) set(chaincodeId string, key string, value []byte) {
	trieNode := newTrieNode(newTrieKey(chaincodeId, key), value, true)
	trieDelta.addTrieNode(trieNode)
}

func (trieDelta *trieDelta) delete(chaincodeId string, key string) {
	trieDelta.set(chaincodeId, key, nil)
}
