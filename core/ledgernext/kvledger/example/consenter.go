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

package example

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos"
)

// Consenter - a toy Consenter
type Consenter struct {
}

// ConstructConsenter constructs a consenter for example
func ConstructConsenter() *Consenter {
	return &Consenter{}
}

// ConstructBlock constructs a block from a list of transactions
func (c *Consenter) ConstructBlock(transactions ...*protos.Transaction2) *protos.Block2 {
	block := &protos.Block2{}
	for _, tx := range transactions {
		txBytes, _ := proto.Marshal(tx)
		block.Transactions = append(block.Transactions, txBytes)
	}
	return block
}
