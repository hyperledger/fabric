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

package protos

import (
	"bytes"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
)

func Test_Block_CreateNew(t *testing.T) {

	chaincodePath := "contract_001"
	/*
		input := &pb.ChaincodeInput{Function: "invoke", Args: {"arg1","arg2"}}
		spec := &pb.ChaincodeSpec{Type: pb.ChaincodeSpec_GOLANG,
			ChaincodeID: &pb.ChaincodeID{Path: chaincodePath}, CtorMsg: input}

		// Build the ChaincodeInvocationSpec message
		chaincodeInvocationSpec := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

		data, err := proto.Marshal(chaincodeInvocationSpec)
	*/
	var data []byte
	cidBytes, err := proto.Marshal(&ChaincodeID{Path: chaincodePath})
	if err != nil {
		t.Fatalf("Could not marshal chaincode: %s", err)
	}
	transaction := &Transaction{Type: 2, ChaincodeID: cidBytes, Payload: data, Txid: "001"}
	t.Logf("Transaction: %v", transaction)

	block := NewBlock([]*Transaction{transaction}, nil)
	t.Logf("Block: %v", block)

	data, err = proto.Marshal(block)
	if err != nil {
		t.Errorf("Error marshalling block: %s", err)
	}
	t.Logf("Marshalled data: %v", data)

	// TODO: This doesn't seem like a proper test. Needs to be edited.
	blockUnmarshalled := &Block{}
	proto.Unmarshal(data, blockUnmarshalled)
	t.Logf("Unmarshalled block := %v", blockUnmarshalled)

}

func TestBlockNonHashData(t *testing.T) {
	block1 := NewBlock(nil, nil)
	block2 := NewBlock(nil, nil)
	time1 := util.CreateUtcTimestamp()
	time.Sleep(100 * time.Millisecond)
	time2 := util.CreateUtcTimestamp()
	block1.NonHashData = &NonHashData{LocalLedgerCommitTimestamp: time1}
	block2.NonHashData = &NonHashData{LocalLedgerCommitTimestamp: time2}
	hash1, err := block1.GetHash()
	if err != nil {
		t.Fatalf("Error generating block1 hash: %s", err)
	}
	hash2, err := block2.GetHash()
	if err != nil {
		t.Fatalf("Error generating block2 hash: %s", err)
	}
	if bytes.Compare(hash1, hash2) != 0 {
		t.Fatalf("Expected block hashes to be equal, but there were not")
	}
	if time1 != block1.NonHashData.LocalLedgerCommitTimestamp {
		t.Fatalf("Expected time1 and block1 times to be equal, but there were not")
	}
	if time2 != block2.NonHashData.LocalLedgerCommitTimestamp {
		t.Fatalf("Expected time2 and block2 times to be equal, but there were not")
	}
}
