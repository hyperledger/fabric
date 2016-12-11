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

// This package provides unit tests for blocks
package utils

import (
	"testing"

	"github.com/hyperledger/fabric/protos/common"
)

func TestGetChainIDFromBlock(t *testing.T) {
	var err error
	var gb *common.Block
	var cid string

	testChainID := "myuniquetestchainid"

	gb, err = MakeConfigurationBlock(testChainID)
	if err != nil {
		t.Fatalf("failed to create test configuration block: %s", err)
	}
	cid, err = GetChainIDFromBlock(gb)
	if err != nil {
		t.Fatalf("failed to get chain ID from block: %s", err)
	}
	if testChainID != cid {
		t.Fatalf("failed with wrong chain ID: Actual=%s; Got=%s", testChainID, cid)
	}

	badBlock := gb
	badBlock.Data = nil
	_, err = GetChainIDFromBlock(badBlock)
	// We should get error
	if err == nil {
		t.Fatalf("error is expected -- the block must not be marshallable")
	}
}

func TestGetBlockFromBlockBytes(t *testing.T) {
	testChainID := "myuniquetestchainid"
	gb, err := MakeConfigurationBlock(testChainID)
	if err != nil {
		t.Fatalf("failed to create test configuration block: %s", err)
	}
	blockBytes, err := Marshal(gb)
	if err != nil {
		t.Fatalf("failed to marshal block: %s", err)
	}
	_, err = GetBlockFromBlockBytes(blockBytes)
	if err != nil {
		t.Fatalf("failed to get block from block bytes: %s", err)
	}
}
