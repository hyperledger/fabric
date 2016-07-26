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

func TestBucketKeyGetParentKey(t *testing.T) {
	conf = newConfig(26, 2, fnvHash)
	bucketKey := newBucketKey(5, 24)
	parentKey := bucketKey.getParentKey()
	testutil.AssertEquals(t, parentKey.level, 4)
	testutil.AssertEquals(t, parentKey.bucketNumber, 12)

	bucketKey = newBucketKey(5, 25)
	parentKey = bucketKey.getParentKey()
	testutil.AssertEquals(t, parentKey.level, 4)
	testutil.AssertEquals(t, parentKey.bucketNumber, 13)

	conf = newConfig(26, 3, fnvHash)
	bucketKey = newBucketKey(3, 24)
	parentKey = bucketKey.getParentKey()
	testutil.AssertEquals(t, parentKey.level, 2)
	testutil.AssertEquals(t, parentKey.bucketNumber, 8)

	bucketKey = newBucketKey(3, 25)
	parentKey = bucketKey.getParentKey()
	testutil.AssertEquals(t, parentKey.level, 2)
	testutil.AssertEquals(t, parentKey.bucketNumber, 9)
}

func TestBucketKeyEqual(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	bucketKey1 := newBucketKey(1, 2)
	bucketKey2 := newBucketKey(1, 2)
	testutil.AssertEquals(t, bucketKey1.equals(bucketKey2), true)
	bucketKey2 = newBucketKey(2, 2)
	testutil.AssertEquals(t, bucketKey1.equals(bucketKey2), false)
	bucketKey2 = newBucketKey(1, 3)
	testutil.AssertEquals(t, bucketKey1.equals(bucketKey2), false)
	bucketKey2 = newBucketKey(2, 3)
	testutil.AssertEquals(t, bucketKey1.equals(bucketKey2), false)
}

func TestBucketKeyWrongLevelCausePanic(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	defer testutil.AssertPanic(t, "A panic should occur when asking for a level beyond a valid range")
	newBucketKey(4, 1)
}

func TestBucketKeyWrongBucketNumberCausePanic_1(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	defer testutil.AssertPanic(t, "A panic should occur when asking for a level beyond a valid range")
	newBucketKey(1, 4)
}

func TestBucketKeyWrongBucketNumberCausePanic_2(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	defer testutil.AssertPanic(t, "A panic should occur when asking for a level beyond a valid range")
	newBucketKey(3, 27)
}

func TestBucketKeyWrongBucketNumberCausePanic_3(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	defer testutil.AssertPanic(t, "A panic should occur when asking for a level beyond a valid range")
	newBucketKey(0, 2)
}

func TestBucketKeyGetChildIndex(t *testing.T) {
	conf = newConfig(26, 3, fnvHash)
	bucketKey := newBucketKey(3, 22)
	testutil.AssertEquals(t, bucketKey.getParentKey().getChildIndex(bucketKey), 0)

	bucketKey = newBucketKey(3, 23)
	testutil.AssertEquals(t, bucketKey.getParentKey().getChildIndex(bucketKey), 1)

	bucketKey = newBucketKey(3, 24)
	testutil.AssertEquals(t, bucketKey.getParentKey().getChildIndex(bucketKey), 2)
}
