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

package util

import (
	"encoding/binary"
	"fmt"

	"github.com/golang/protobuf/proto"
)

// EncodeOrderPreservingVarUint64 returns a byte-representation for a uint64 number such that
// all zero-bits starting bytes are trimmed in order to reduce the length of the array
// For preserving the order in a default bytes-comparison, first byte contains the number of remaining bytes.
// The presence of first byte also allows to use the returned bytes as part of other larger byte array such as a
// composite-key representation in db
func EncodeOrderPreservingVarUint64(number uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, number)
	startingIndex := 0
	size := 0
	for i, b := range bytes {
		if b != 0x00 {
			startingIndex = i
			size = 8 - i
			break
		}
	}
	sizeBytes := proto.EncodeVarint(uint64(size))
	if len(sizeBytes) > 1 {
		panic(fmt.Errorf("[]sizeBytes should not be more than one byte because the max number it needs to hold is 8. size=%d", size))
	}
	encodedBytes := make([]byte, size+1)
	encodedBytes[0] = sizeBytes[0]
	copy(encodedBytes[1:], bytes[startingIndex:])
	return encodedBytes
}

// DecodeOrderPreservingVarUint64 decodes the number from the bytes obtained from method 'EncodeOrderPreservingVarUint64'.
// Also, returns the number of bytes that are consumed in the process
func DecodeOrderPreservingVarUint64(bytes []byte) (uint64, int) {
	s, _ := proto.DecodeVarint(bytes)
	size := int(s)
	decodedBytes := make([]byte, 8)
	copy(decodedBytes[8-size:], bytes[1:size+1])
	numBytesConsumed := size + 1
	return binary.BigEndian.Uint64(decodedBytes), numBytesConsumed
}
