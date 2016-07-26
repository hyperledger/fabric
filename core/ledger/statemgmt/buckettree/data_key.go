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
	"fmt"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/util"
)

type dataKey struct {
	bucketKey    *bucketKey
	compositeKey []byte
}

func newDataKey(chaincodeID string, key string) *dataKey {
	logger.Debugf("Enter - newDataKey. chaincodeID=[%s], key=[%s]", chaincodeID, key)
	compositeKey := statemgmt.ConstructCompositeKey(chaincodeID, key)
	bucketHash := conf.computeBucketHash(compositeKey)
	// Adding one because - we start bucket-numbers 1 onwards
	bucketNumber := int(bucketHash)%conf.getNumBucketsAtLowestLevel() + 1
	dataKey := &dataKey{newBucketKeyAtLowestLevel(bucketNumber), compositeKey}
	logger.Debugf("Exit - newDataKey=[%s]", dataKey)
	return dataKey
}

func minimumPossibleDataKeyBytesFor(bucketKey *bucketKey) []byte {
	min := encodeBucketNumber(bucketKey.bucketNumber)
	min = append(min, byte(0))
	return min
}

func minimumPossibleDataKeyBytes(bucketNumber int, chaincodeID string, key string) []byte {
	b := encodeBucketNumber(bucketNumber)
	b = append(b, statemgmt.ConstructCompositeKey(chaincodeID, key)...)
	return b
}

func (key *dataKey) getBucketKey() *bucketKey {
	return key.bucketKey
}

func encodeBucketNumber(bucketNumber int) []byte {
	return util.EncodeOrderPreservingVarUint64(uint64(bucketNumber))
}

func decodeBucketNumber(encodedBytes []byte) (int, int) {
	bucketNum, bytesConsumed := util.DecodeOrderPreservingVarUint64(encodedBytes)
	return int(bucketNum), bytesConsumed
}

func (key *dataKey) getEncodedBytes() []byte {
	encodedBytes := encodeBucketNumber(key.bucketKey.bucketNumber)
	encodedBytes = append(encodedBytes, key.compositeKey...)
	return encodedBytes
}

func newDataKeyFromEncodedBytes(encodedBytes []byte) *dataKey {
	bucketNum, l := decodeBucketNumber(encodedBytes)
	compositeKey := encodedBytes[l:]
	return &dataKey{newBucketKeyAtLowestLevel(bucketNum), compositeKey}
}

func (key *dataKey) String() string {
	return fmt.Sprintf("bucketKey=[%s], compositeKey=[%s]", key.bucketKey, string(key.compositeKey))
}

func (key *dataKey) clone() *dataKey {
	clone := &dataKey{key.bucketKey.clone(), key.compositeKey}
	return clone
}
