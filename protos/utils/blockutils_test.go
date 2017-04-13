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
package utils_test

import (
	"testing"

	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/protos/common"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

func TestGetChainIDFromBlock(t *testing.T) {
	var err error
	var gb *common.Block
	var cid string

	testChainID := "myuniquetestchainid"

	gb, err = configtxtest.MakeGenesisBlock(testChainID)
	if err != nil {
		t.Fatalf("failed to create test configuration block: %s", err)
	}
	cid, err = utils.GetChainIDFromBlock(gb)
	if err != nil {
		t.Fatalf("failed to get chain ID from block: %s", err)
	}
	if testChainID != cid {
		t.Fatalf("failed with wrong chain ID: Actual=%s; Got=%s", testChainID, cid)
	}

	badBlock := gb
	badBlock.Data = nil
	_, err = utils.GetChainIDFromBlock(badBlock)
	// We should get error
	if err == nil {
		t.Fatalf("error is expected -- the block must not be marshallable")
	}
}

func TestGetBlockFromBlockBytes(t *testing.T) {
	testChainID := "myuniquetestchainid"
	gb, err := configtxtest.MakeGenesisBlock(testChainID)
	if err != nil {
		t.Fatalf("failed to create test configuration block: %s", err)
	}
	blockBytes, err := utils.Marshal(gb)
	if err != nil {
		t.Fatalf("failed to marshal block: %s", err)
	}
	_, err = utils.GetBlockFromBlockBytes(blockBytes)
	if err != nil {
		t.Fatalf("failed to get block from block bytes: %s", err)
	}
}

func TestGetMetadataFromNewBlock(t *testing.T) {
	block := common.NewBlock(0, nil)
	md, err := utils.GetMetadataFromBlock(block, cb.BlockMetadataIndex_ORDERER)
	if err != nil {
		t.Fatal("Expected no error when extracting metadata from new block")
	}
	if md.Value != nil {
		t.Fatal("Expected metadata field value to be nil, got", md.Value)
	}
	if len(md.Value) > 0 {
		t.Fatal("Expected length of metadata field value to be 0, got", len(md.Value))
	}

}
