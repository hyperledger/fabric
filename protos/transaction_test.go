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
	"testing"

	"github.com/golang/protobuf/proto"
)

func Test_Transaction_CreateNew(t *testing.T) {

	cidBytes, err := proto.Marshal(&ChaincodeID{Path: "Contract001"})
	if err != nil {
		t.Fatalf("Could not marshal chaincode: %s", err)
	}
	tx := &Transaction{ChaincodeID: cidBytes}
	t.Logf("Transaction: %v", tx)

	data, err := proto.Marshal(tx)
	if err != nil {
		t.Errorf("Error marshalling transaction: %s", err)
	}
	t.Logf("Marshalled data: %v", data)

	// TODO: This doesn't seem like a proper test. Needs to be edited.
	txUnmarshalled := &Transaction{}
	err = proto.Unmarshal(data, txUnmarshalled)
	t.Logf("Unmarshalled transaction: %v", txUnmarshalled)
	if err != nil {
		t.Errorf("Error unmarshalling block: %s", err)
	}

}
