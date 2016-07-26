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

	"github.com/golang/protobuf/proto"
	openchainUtil "github.com/hyperledger/fabric/core/util"
)

type bucketNode struct {
	bucketKey          *bucketKey
	childrenCryptoHash [][]byte
	childrenUpdated    []bool
	markedForDeletion  bool
}

func newBucketNode(bucketKey *bucketKey) *bucketNode {
	maxChildren := conf.getMaxGroupingAtEachLevel()
	return &bucketNode{bucketKey, make([][]byte, maxChildren), make([]bool, maxChildren), false}
}

func unmarshalBucketNode(bucketKey *bucketKey, serializedBytes []byte) *bucketNode {
	bucketNode := newBucketNode(bucketKey)
	buffer := proto.NewBuffer(serializedBytes)
	for i := 0; i < conf.getMaxGroupingAtEachLevel(); i++ {
		childCryptoHash, err := buffer.DecodeRawBytes(false)
		if err != nil {
			panic(fmt.Errorf("this error should not occur: %s", err))
		}
		//protobuf's buffer.EncodeRawBytes/buffer.DecodeRawBytes convert a nil into a zero length byte-array, so nil check would not work
		if len(childCryptoHash) != 0 {
			bucketNode.childrenCryptoHash[i] = childCryptoHash
		}
	}
	return bucketNode
}

func (bucketNode *bucketNode) marshal() []byte {
	buffer := proto.NewBuffer([]byte{})
	for i := 0; i < conf.getMaxGroupingAtEachLevel(); i++ {
		buffer.EncodeRawBytes(bucketNode.childrenCryptoHash[i])
	}
	return buffer.Bytes()
}

func (bucketNode *bucketNode) setChildCryptoHash(childKey *bucketKey, cryptoHash []byte) {
	i := bucketNode.bucketKey.getChildIndex(childKey)
	bucketNode.childrenCryptoHash[i] = cryptoHash
	bucketNode.childrenUpdated[i] = true
}

func (bucketNode *bucketNode) mergeBucketNode(anotherBucketNode *bucketNode) {
	if !bucketNode.bucketKey.equals(anotherBucketNode.bucketKey) {
		panic(fmt.Errorf("Nodes with different keys can not be merged. BaseKey=[%#v], MergeKey=[%#v]", bucketNode.bucketKey, anotherBucketNode.bucketKey))
	}
	for i, childCryptoHash := range anotherBucketNode.childrenCryptoHash {
		if !bucketNode.childrenUpdated[i] {
			bucketNode.childrenCryptoHash[i] = childCryptoHash
		}
	}
}

func (bucketNode *bucketNode) computeCryptoHash() []byte {
	cryptoHashContent := []byte{}
	numChildren := 0
	for i, childCryptoHash := range bucketNode.childrenCryptoHash {
		if childCryptoHash != nil {
			numChildren++
			logger.Debugf("Appending crypto-hash for child bucket = [%s]", bucketNode.bucketKey.getChildKey(i))
			cryptoHashContent = append(cryptoHashContent, childCryptoHash...)
		}
	}
	if numChildren == 0 {
		logger.Debugf("Returning <nil> crypto-hash of bucket = [%s] - because, it has not children", bucketNode.bucketKey)
		bucketNode.markedForDeletion = true
		return nil
	}
	if numChildren == 1 {
		logger.Debugf("Propagating crypto-hash of single child node for bucket = [%s]", bucketNode.bucketKey)
		return cryptoHashContent
	}
	logger.Debugf("Computing crypto-hash for bucket [%s] by merging [%d] children", bucketNode.bucketKey, numChildren)
	return openchainUtil.ComputeCryptoHash(cryptoHashContent)
}

func (bucketNode *bucketNode) String() string {
	numChildren := 0
	for i := range bucketNode.childrenCryptoHash {
		if bucketNode.childrenCryptoHash[i] != nil {
			numChildren++
		}
	}
	str := fmt.Sprintf("bucketKey={%s}\n NumChildren={%d}\n", bucketNode.bucketKey, numChildren)
	if numChildren == 0 {
		return str
	}

	str = str + "Childern crypto-hashes:\n"
	for i := range bucketNode.childrenCryptoHash {
		childCryptoHash := bucketNode.childrenCryptoHash[i]
		if childCryptoHash != nil {
			str = str + fmt.Sprintf("childNumber={%d}, cryptoHash={%x}\n", i, childCryptoHash)
		}
	}
	return str
}
