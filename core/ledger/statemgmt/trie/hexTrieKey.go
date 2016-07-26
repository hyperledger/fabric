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
	"encoding/hex"
	"fmt"
)

var charIndexMap = map[hexTrieKey]int{
	"0": 0,
	"1": 1,
	"2": 2,
	"3": 3,
	"4": 4,
	"5": 5,
	"6": 6,
	"7": 7,
	"8": 8,
	"9": 9,
	"a": 10,
	"b": 11,
	"c": 12,
	"d": 13,
	"e": 14,
	"f": 15,
}

type hexTrieKeyEncoder struct {
}

func newHexTrieKeyEncoder() trieKeyEncoder {
	return &hexTrieKeyEncoder{}
}

func (encoder *hexTrieKeyEncoder) newTrieKey(originalBytes []byte) trieKeyInterface {
	return hexTrieKey(hex.EncodeToString(originalBytes))
}

func (encoder *hexTrieKeyEncoder) decodeTrieKeyBytes(encodedBytes []byte) []byte {
	originalBytes, err := hex.DecodeString(string(encodedBytes))
	if err != nil {
		panic(fmt.Errorf("Invalid input: input bytes=[%x], error:%s", encodedBytes, err))
	}
	return originalBytes
}

func (encoder *hexTrieKeyEncoder) getMaxTrieWidth() int {
	return len(charIndexMap)
}

type hexTrieKey string

func (key hexTrieKey) getLevel() int {
	return len(key)
}

func (key hexTrieKey) getParentTrieKey() trieKeyInterface {
	if key.isRootKey() {
		panic(fmt.Errorf("Parent for Trie root shoould not be asked for"))
	}
	return key[:len(key)-1]
}

func (key hexTrieKey) getIndexInParent() int {
	if key.isRootKey() {
		panic(fmt.Errorf("Parent for Trie root shoould not be asked for"))
	}
	return charIndexMap[key[len(key)-1:]]
}

func (key hexTrieKey) getEncodedBytes() []byte {
	return []byte(key)
}

func (key hexTrieKey) isRootKey() bool {
	return len(key) == 0
}
