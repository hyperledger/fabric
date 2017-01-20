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

package histmgr

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/testutil"
)

var strKeySep = string(compositeKeySep)

func TestConstructCompositeKey(t *testing.T) {
	compositeKey := ConstructCompositeKey("ns1", "key1", 1, 1)

	testutil.AssertEquals(t, compositeKey, "ns1"+strKeySep+"key1"+strKeySep+"1"+strKeySep+"1")
}

func TestConstructPartialCompositeKey(t *testing.T) {
	compositeStartKey := ConstructPartialCompositeKey("ns1", "key1", false)
	compositeEndKey := ConstructPartialCompositeKey("ns1", "key1", true)

	testutil.AssertEquals(t, compositeStartKey, []byte("ns1"+strKeySep+"key1"))
	testutil.AssertEquals(t, compositeEndKey, []byte("ns1"+strKeySep+"key1"+string([]byte("1"))))
}

//TODO For the query results should the strKeySep be included twice - before the blocknum and tran num or just once?
//Should the funciton split remove the first one?
func TestSplitCompositeKey(t *testing.T) {
	compositePartialKey := ConstructPartialCompositeKey("ns1", "key1", false)
	builtReturnValue := []byte("ns1" + strKeySep + "key1" + strKeySep + "1" + strKeySep + "1")

	_, blockNumTranNum := SplitCompositeKey(compositePartialKey, builtReturnValue)

	testutil.AssertEquals(t, blockNumTranNum, string(strKeySep+"1"+strKeySep+"1"))
}
