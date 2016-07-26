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

import "github.com/hyperledger/fabric/core/db"

func fetchTrieNodeFromDB(key *trieKey) (*trieNode, error) {
	stateTrieLogger.Debugf("Enter fetchTrieNodeFromDB() for trieKey [%s]", key)
	openchainDB := db.GetDBHandle()
	trieNodeBytes, err := openchainDB.GetFromStateCF(key.getEncodedBytes())
	if err != nil {
		stateTrieLogger.Errorf("Error in retrieving trie node from DB for triekey [%s]. Error:%s", key, err)
		return nil, err
	}

	if trieNodeBytes == nil {
		return nil, nil
	}

	trieNode, err := unmarshalTrieNode(key, trieNodeBytes)
	if err != nil {
		stateTrieLogger.Errorf("Error in unmarshalling trie node for triekey [%s]. Error:%s", key, err)
		return nil, err
	}
	stateTrieLogger.Debugf("Exit fetchTrieNodeFromDB() for trieKey [%s]", key)
	return trieNode, nil
}
