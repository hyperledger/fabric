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
)

type dataNode struct {
	dataKey *dataKey
	value   []byte
}

func newDataNode(dataKey *dataKey, value []byte) *dataNode {
	return &dataNode{dataKey, value}
}

func unmarshalDataNodeFromBytes(keyBytes []byte, valueBytes []byte) *dataNode {
	return unmarshalDataNode(newDataKeyFromEncodedBytes(keyBytes), valueBytes)
}

func unmarshalDataNode(dataKey *dataKey, serializedBytes []byte) *dataNode {
	return &dataNode{dataKey, serializedBytes}
}

func (dataNode *dataNode) getCompositeKey() []byte {
	return dataNode.dataKey.compositeKey
}

func (dataNode *dataNode) isDelete() bool {
	return dataNode.value == nil
}

func (dataNode *dataNode) getKeyElements() (string, string) {
	return statemgmt.DecodeCompositeKey(dataNode.getCompositeKey())
}

func (dataNode *dataNode) getValue() []byte {
	return dataNode.value
}

func (dataNode *dataNode) String() string {
	return fmt.Sprintf("dataKey=[%s], value=[%s]", dataNode.dataKey, string(dataNode.value))
}
