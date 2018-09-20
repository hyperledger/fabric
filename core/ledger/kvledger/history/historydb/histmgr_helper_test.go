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

package historydb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var strKeySep = string(CompositeKeySep)

func TestConstructCompositeKey(t *testing.T) {
	compositeKey := ConstructCompositeHistoryKey("ns1", "key1", 1, 1)
	assert.NotNil(t, compositeKey)
	//historyleveldb_test.go tests the actual output
}

func TestConstructPartialCompositeKey(t *testing.T) {
	compositeStartKey := ConstructPartialCompositeHistoryKey("ns1", "key1", false)
	compositeEndKey := ConstructPartialCompositeHistoryKey("ns1", "key1", true)

	assert.Equal(t, []byte("ns1"+strKeySep+"key1"+strKeySep), compositeStartKey)
	assert.Equal(t, []byte("ns1"+strKeySep+"key1"+strKeySep+string([]byte{0xff})), compositeEndKey)
}

func TestSplitCompositeKey(t *testing.T) {
	compositeFullKey := []byte("ns1" + strKeySep + "key1" + strKeySep + "extra bytes to split")
	compositePartialKey := ConstructPartialCompositeHistoryKey("ns1", "key1", false)

	_, extraBytes := SplitCompositeHistoryKey(compositeFullKey, compositePartialKey)
	// second position should hold the extra bytes that were split off
	assert.Equal(t, []byte("extra bytes to split"), extraBytes)
}
