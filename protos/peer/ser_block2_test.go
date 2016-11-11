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

package peer

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
)

func TestSerBlock2(t *testing.T) {
	tx1 := &Transaction2{}
	tx1.Actions = []*TransactionAction{
		&TransactionAction{Header: []byte("action1"), Payload: []byte("payload1")},
		&TransactionAction{Header: []byte("action2"), Payload: []byte("payload2")}}

	tx1Bytes, err := proto.Marshal(tx1)
	if err != nil {
		t.Fatalf("Error:%s", err)
	}

	tx2 := &Transaction2{}
	tx2.Actions = []*TransactionAction{
		&TransactionAction{Header: []byte("action1"), Payload: []byte("payload1")},
		&TransactionAction{Header: []byte("action2"), Payload: []byte("payload2")}}

	tx2Bytes, err := proto.Marshal(tx2)
	if err != nil {
		t.Fatalf("Error:%s", err)
	}

	block := &Block2{}
	block.PreviousBlockHash = []byte("PreviousBlockHash")
	block.Transactions = [][]byte{tx1Bytes, tx2Bytes}
	testSerBlock2(t, block)
}

func testSerBlock2(t *testing.T, block *Block2) {
	serBlock, err := ConstructSerBlock2(block)
	if err != nil {
		t.Fatalf("Error:%s", err)
	}
	serDeBlock, err := serBlock.ToBlock2()
	if err != nil {
		t.Fatalf("Error:%s", err)
	}
	if !reflect.DeepEqual(block, serDeBlock) {
		t.Fatalf("Block is not same after serialization-deserialization. \n\t Expected=%#v, \n\t Actual=%#v", block, serDeBlock)
	}
}
