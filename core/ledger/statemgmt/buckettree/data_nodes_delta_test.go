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

package buckettree

import (
	"sort"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestDataNodesSort(t *testing.T) {
	dataNodes := dataNodes{}
	dataNode1 := newDataNode(newDataKey("chaincodeID1", "key1"), []byte("value1_1"))
	dataNode2 := newDataNode(newDataKey("chaincodeID1", "key2"), []byte("value1_2"))
	dataNode3 := newDataNode(newDataKey("chaincodeID2", "key1"), []byte("value2_1"))
	dataNode4 := newDataNode(newDataKey("chaincodeID2", "key2"), []byte("value2_2"))
	dataNodes = append(dataNodes, []*dataNode{dataNode2, dataNode4, dataNode3, dataNode1}...)
	sort.Sort(dataNodes)
	testutil.AssertSame(t, dataNodes[0], dataNode1)
	testutil.AssertSame(t, dataNodes[1], dataNode2)
	testutil.AssertSame(t, dataNodes[2], dataNode3)
	testutil.AssertSame(t, dataNodes[3], dataNode4)
}

func TestDataNodesDelta(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	stateDelta := statemgmt.NewStateDelta()
	stateDelta.Set("chaincodeID1", "key1", []byte("value1_1"), nil)
	stateDelta.Set("chaincodeID1", "key2", []byte("value1_2"), nil)
	stateDelta.Set("chaincodeID2", "key1", []byte("value2_1"), nil)
	stateDelta.Set("chaincodeID2", "key2", []byte("value2_2"), nil)

	dataNodesDelta := newDataNodesDelta(stateDelta)
	affectedBuckets := dataNodesDelta.getAffectedBuckets()
	testutil.AssertContains(t, affectedBuckets, newDataKey("chaincodeID1", "key1").getBucketKey())
	testutil.AssertContains(t, affectedBuckets, newDataKey("chaincodeID1", "key2").getBucketKey())
	testutil.AssertContains(t, affectedBuckets, newDataKey("chaincodeID2", "key1").getBucketKey())
	testutil.AssertContains(t, affectedBuckets, newDataKey("chaincodeID2", "key2").getBucketKey())

}
