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

package common

import (
	"math"
	"testing"
)

func TestGoodBlockHeaderBytes(t *testing.T) {
	goodBlockHeader := &BlockHeader{
		Number:       1,
		PreviousHash: []byte("foo"),
		DataHash:     []byte("bar"),
	}

	_ = goodBlockHeader.Bytes() // Should not panic
}

func TestBadBlockHeaderBytes(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("Should have panicked on block number too high to encode as int64")
		}
	}()

	badBlockHeader := &BlockHeader{
		Number:       math.MaxUint64,
		PreviousHash: []byte("foo"),
		DataHash:     []byte("bar"),
	}

	_ = badBlockHeader.Bytes() // Should panic
}
