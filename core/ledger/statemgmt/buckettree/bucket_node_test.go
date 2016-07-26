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

func TestBucketNodeComputeHash(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	bucketNode := newBucketNode(newBucketKey(2, 7))
	testutil.AssertEquals(t, bucketNode.computeCryptoHash(), nil)

	childKey1 := newBucketKey(3, 19)
	bucketNode.setChildCryptoHash(childKey1, []byte("cryptoHashChild1"))
	testutil.AssertEquals(t, bucketNode.computeCryptoHash(), []byte("cryptoHashChild1"))

	childKey3 := newBucketKey(3, 21)
	bucketNode.setChildCryptoHash(childKey3, []byte("cryptoHashChild3"))
	testutil.AssertEquals(t, bucketNode.computeCryptoHash(), testutil.ComputeCryptoHash([]byte("cryptoHashChild1cryptoHashChild3")))

	childKey2 := newBucketKey(3, 20)
	bucketNode.setChildCryptoHash(childKey2, []byte("cryptoHashChild2"))
	testutil.AssertEquals(t, bucketNode.computeCryptoHash(), testutil.ComputeCryptoHash([]byte("cryptoHashChild1cryptoHashChild2cryptoHashChild3")))
}

func TestBucketNodeMerge(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	bucketNode := newBucketNode(newBucketKey(2, 7))
	bucketNode.childrenCryptoHash[0] = []byte("cryptoHashChild1")
	bucketNode.childrenUpdated[0] = true
	bucketNode.childrenCryptoHash[2] = []byte("cryptoHashChild3")
	bucketNode.childrenUpdated[2] = true

	dbBucketNode := newBucketNode(newBucketKey(2, 7))
	dbBucketNode.childrenCryptoHash[0] = []byte("DBcryptoHashChild1")
	dbBucketNode.childrenCryptoHash[1] = []byte("DBcryptoHashChild2")

	bucketNode.mergeBucketNode(dbBucketNode)
	testutil.AssertEquals(t, bucketNode.childrenCryptoHash[0], []byte("cryptoHashChild1"))
	testutil.AssertEquals(t, bucketNode.childrenCryptoHash[1], []byte("DBcryptoHashChild2"))
	testutil.AssertEquals(t, bucketNode.childrenCryptoHash[2], []byte("cryptoHashChild3"))
}

func TestBucketNodeMarshalUnmarshal(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	bucketNode := newBucketNode(newBucketKey(2, 7))
	childKey1 := newBucketKey(3, 19)
	bucketNode.setChildCryptoHash(childKey1, []byte("cryptoHashChild1"))

	childKey3 := newBucketKey(3, 21)
	bucketNode.setChildCryptoHash(childKey3, []byte("cryptoHashChild3"))

	serializedBytes := bucketNode.marshal()
	deserializedBucketNode := unmarshalBucketNode(newBucketKey(2, 7), serializedBytes)
	testutil.AssertEquals(t, bucketNode.bucketKey, deserializedBucketNode.bucketKey)
	testutil.AssertEquals(t, bucketNode.childrenCryptoHash, deserializedBucketNode.childrenCryptoHash)
}
