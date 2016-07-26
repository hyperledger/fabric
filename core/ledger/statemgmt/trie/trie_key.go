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
	"bytes"
	"fmt"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
)

type trieKeyEncoder interface {
	newTrieKey(originalBytes []byte) trieKeyInterface
	getMaxTrieWidth() int
	decodeTrieKeyBytes(encodedBytes []byte) (originalBytes []byte)
}

type trieKeyInterface interface {
	getLevel() int
	getParentTrieKey() trieKeyInterface
	getIndexInParent() int
	getEncodedBytes() []byte
}

var trieKeyEncoderImpl trieKeyEncoder = newByteTrieKeyEncoder()
var rootTrieKeyBytes = []byte{}
var rootTrieKeyStr = string(rootTrieKeyBytes)
var rootTrieKey = newTrieKeyFromCompositeKey(rootTrieKeyBytes)

type trieKey struct {
	trieKeyImpl trieKeyInterface
}

func newTrieKey(chaincodeID string, key string) *trieKey {
	compositeKey := statemgmt.ConstructCompositeKey(chaincodeID, key)
	return newTrieKeyFromCompositeKey(compositeKey)
}

func newTrieKeyFromCompositeKey(compositeKey []byte) *trieKey {
	return &trieKey{trieKeyEncoderImpl.newTrieKey(compositeKey)}
}

func decodeTrieKeyBytes(encodedBytes []byte) []byte {
	return trieKeyEncoderImpl.decodeTrieKeyBytes(encodedBytes)
}

func (key *trieKey) getEncodedBytes() []byte {
	return key.trieKeyImpl.getEncodedBytes()
}

func (key *trieKey) getLevel() int {
	return key.trieKeyImpl.getLevel()
}

func (key *trieKey) getIndexInParent() int {
	if key.isRootKey() {
		panic(fmt.Errorf("Parent for Trie root shoould not be asked for"))
	}
	return key.trieKeyImpl.getIndexInParent()
}

func (key *trieKey) getParentTrieKey() *trieKey {
	if key.isRootKey() {
		panic(fmt.Errorf("Parent for Trie root shoould not be asked for"))
	}
	return &trieKey{key.trieKeyImpl.getParentTrieKey()}
}

func (key *trieKey) getEncodedBytesAsStr() string {
	return string(key.trieKeyImpl.getEncodedBytes())
}

func (key *trieKey) isRootKey() bool {
	return len(key.getEncodedBytes()) == 0
}

func (key *trieKey) getParentLevel() int {
	if key.isRootKey() {
		panic(fmt.Errorf("Parent for Trie root shoould not be asked for"))
	}
	return key.getLevel() - 1
}

func (key *trieKey) assertIsChildOf(parentTrieKey *trieKey) {
	if !bytes.Equal(key.getParentTrieKey().getEncodedBytes(), parentTrieKey.getEncodedBytes()) {
		panic(fmt.Errorf("trie key [%s] is not a child of trie key [%s]", key, parentTrieKey))
	}
}
