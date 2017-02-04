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

package fsledger

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
)

const (
	testFolder = "/tmp/fabric/orderer/ledger/fsledger"
)

func TestOrdererLedger(t *testing.T) {
	conf := fsblkstorage.NewConf(testFolder, 0)
	cleanup(t)
	defer cleanup(t)

	ordererLedgerProvider := NewProvider(conf)
	defer ordererLedgerProvider.Close()

	ordererLedger, _ := ordererLedgerProvider.Create("testLedger")
	defer ordererLedger.Close()

	// Construct test blocks and add to orderer ledger
	blocks := testutil.ConstructTestBlocks(t, 10)
	for _, block := range blocks {
		ordererLedger.Commit(block)
	}

	// test GetBlockchainInfo()
	bcInfo, err := ordererLedger.GetBlockchainInfo()
	testutil.AssertNoError(t, err, "Error in getting BlockchainInfo")
	testutil.AssertEquals(t, bcInfo.Height, uint64(10))

	// test GetBlockByNumber()
	block, err := ordererLedger.GetBlockByNumber(2)
	testutil.AssertNoError(t, err, "Error in getting block by number")
	testutil.AssertEquals(t, block, blocks[1])

	// get blocks iterator for block number starting from 3
	itr, err := ordererLedger.GetBlocksIterator(3)
	testutil.AssertNoError(t, err, "Error in getting iterator")
	blockHolder, err := itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, blockHolder.(ledger.BlockHolder).GetBlock(), blocks[2])
	// get next block from iterator. The block should be 4th block
	blockHolder, err = itr.Next()
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, blockHolder.(ledger.BlockHolder).GetBlock(), blocks[3])
}

func cleanup(t *testing.T) {
	err := os.RemoveAll(testFolder)
	if err != nil {
		t.Fatalf("Error in cleanup:%s", err)
	}
}
