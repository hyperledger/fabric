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
	"encoding/binary"
	"fmt"
	"math"
)

var numBytesAtEachLevel = 1

type byteTrieKeyEncoder struct {
}

func newByteTrieKeyEncoder() trieKeyEncoder {
	return &byteTrieKeyEncoder{}
}

func (encoder *byteTrieKeyEncoder) newTrieKey(originalBytes []byte) trieKeyInterface {
	len := len(originalBytes)
	remainingBytes := len % numBytesAtEachLevel
	bytesToAppend := 0
	if remainingBytes != 0 {
		bytesToAppend = numBytesAtEachLevel - remainingBytes
	}
	for i := 0; i < bytesToAppend; i++ {
		originalBytes = append(originalBytes, byte(0))
	}
	return byteTrieKey(originalBytes)
}

func (encoder *byteTrieKeyEncoder) decodeTrieKeyBytes(encodedBytes []byte) []byte {
	return encodedBytes
}

func (encoder *byteTrieKeyEncoder) getMaxTrieWidth() int {
	return int(math.Pow(2, float64(8*numBytesAtEachLevel)))
}

type byteTrieKey string

func (key byteTrieKey) getLevel() int {
	return len(key) / numBytesAtEachLevel
}

func (key byteTrieKey) getParentTrieKey() trieKeyInterface {
	if key.isRootKey() {
		panic(fmt.Errorf("Parent for Trie root shoould not be asked for"))
	}
	return key[:len(key)-numBytesAtEachLevel]
}

func (key byteTrieKey) getIndexInParent() int {
	if key.isRootKey() {
		panic(fmt.Errorf("Parent for Trie root should not be asked for"))
	}
	indexBytes := []byte{}
	for i := 0; i < 8-numBytesAtEachLevel; i++ {
		indexBytes = append(indexBytes, byte(0))
	}
	indexBytes = append(indexBytes, []byte(key[len(key)-numBytesAtEachLevel:])...)
	return int(binary.BigEndian.Uint64(indexBytes))
}

func (key byteTrieKey) getEncodedBytes() []byte {
	return []byte(key)
}

func (key byteTrieKey) isRootKey() bool {
	return len(key) == 0
}
