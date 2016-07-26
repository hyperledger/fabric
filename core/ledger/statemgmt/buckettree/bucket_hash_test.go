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
	"testing"

	"github.com/hyperledger/fabric/core/ledger/testutil"
)

func TestBucketHashCalculator(t *testing.T) {
	initConfig(nil)
	c := newBucketHashCalculator(newBucketKey(1, 1))

	testutil.AssertEquals(t, c.computeCryptoHash(), nil)

	c.addNextNode(newDataNode(newDataKey("chaincodeID1", "key1"), []byte("value1")))

	c.addNextNode(newDataNode(newDataKey("chaincodeID_2", "key_1"), []byte("value_1")))
	c.addNextNode(newDataNode(newDataKey("chaincodeID_2", "key_2"), []byte("value_2")))

	c.addNextNode(newDataNode(newDataKey("chaincodeID3", "key1"), []byte("value1")))
	c.addNextNode(newDataNode(newDataKey("chaincodeID3", "key2"), []byte("value2")))
	c.addNextNode(newDataNode(newDataKey("chaincodeID3", "key3"), []byte("value3")))

	hash := c.computeCryptoHash()
	expectedHashContent := expectedBucketHashContentForTest(
		[]string{"chaincodeID1", "key1", "value1"},
		[]string{"chaincodeID_2", "key_1", "value_1", "key_2", "value_2"},
		[]string{"chaincodeID3", "key1", "value1", "key2", "value2", "key3", "value3"},
	)
	t.Logf("Actual HashContent = %#v\n Expected HashContent = %#v", c.hashingData, expectedHashContent)
	testutil.AssertEquals(t, hash, testutil.ComputeCryptoHash(expectedHashContent))
}
