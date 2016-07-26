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
	"github.com/golang/protobuf/proto"
	openchainUtil "github.com/hyperledger/fabric/core/util"
)

type bucketHashCalculator struct {
	bucketKey          *bucketKey
	currentChaincodeID string
	dataNodes          []*dataNode
	hashingData        []byte
}

func newBucketHashCalculator(bucketKey *bucketKey) *bucketHashCalculator {
	return &bucketHashCalculator{bucketKey, "", nil, nil}
}

// addNextNode - this method assumes that the datanodes are added in the increasing order of the keys
func (c *bucketHashCalculator) addNextNode(dataNode *dataNode) {
	chaincodeID, _ := dataNode.getKeyElements()
	if chaincodeID != c.currentChaincodeID {
		c.appendCurrentChaincodeData()
		c.currentChaincodeID = chaincodeID
		c.dataNodes = nil
	}
	c.dataNodes = append(c.dataNodes, dataNode)
}

func (c *bucketHashCalculator) computeCryptoHash() []byte {
	if c.currentChaincodeID != "" {
		c.appendCurrentChaincodeData()
		c.currentChaincodeID = ""
		c.dataNodes = nil
	}
	logger.Debugf("Hashable content for bucket [%s]: length=%d, contentInStringForm=[%s]", c.bucketKey, len(c.hashingData), string(c.hashingData))
	if c.hashingData == nil {
		return nil
	}
	return openchainUtil.ComputeCryptoHash(c.hashingData)
}

func (c *bucketHashCalculator) appendCurrentChaincodeData() {
	if c.currentChaincodeID == "" {
		return
	}
	c.appendSizeAndData([]byte(c.currentChaincodeID))
	c.appendSize(len(c.dataNodes))
	for _, dataNode := range c.dataNodes {
		_, key := dataNode.getKeyElements()
		value := dataNode.getValue()
		c.appendSizeAndData([]byte(key))
		c.appendSizeAndData(value)
	}
}

func (c *bucketHashCalculator) appendSizeAndData(b []byte) {
	c.appendSize(len(b))
	c.hashingData = append(c.hashingData, b...)
}

func (c *bucketHashCalculator) appendSize(size int) {
	c.hashingData = append(c.hashingData, proto.EncodeVarint(uint64(size))...)
}
