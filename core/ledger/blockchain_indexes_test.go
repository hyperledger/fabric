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

package ledger

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/protos"
)

func TestIndexes_GetBlockByBlockNumber(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = true
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetBlockByBlockNumber(t)
}

func TestIndexes_GetBlockByBlockHash(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = true
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetBlockByBlockHash(t)
}

func TestIndexes_GetBlockByBlockHashWrongHash(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = true
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetBlockByBlockHashWrongHash(t)
}

func TestIndexes_GetTransactionByBlockNumberAndTxIndex(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = true
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetTransactionByBlockNumberAndTxIndex(t)
}

func TestIndexes_GetTransactionByBlockHashAndTxIndex(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = true
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetTransactionByBlockHashAndTxIndex(t)
}

func TestIndexes_GetTransactionByID(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = true
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetTransactionByID(t)
}

func testIndexesGetBlockByBlockNumber(t *testing.T) {
	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	defer func() { testBlockchainWrapper.blockchain.indexer.stop() }()
	blocks, _, err := testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}
	for i := range blocks {
		testutil.AssertEquals(t, testBlockchainWrapper.getBlock(uint64(i)), blocks[i])
	}
}

func testIndexesGetBlockByBlockHash(t *testing.T) {
	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	defer func() { testBlockchainWrapper.blockchain.indexer.stop() }()
	blocks, _, err := testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}
	for i := range blocks {
		blockHash, _ := blocks[i].GetHash()
		testutil.AssertEquals(t, testBlockchainWrapper.getBlockByHash(blockHash), blocks[i])
	}
}

func testIndexesGetBlockByBlockHashWrongHash(t *testing.T) {
	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	defer func() { testBlockchainWrapper.blockchain.indexer.stop() }()
	_, err := testBlockchainWrapper.blockchain.getBlockByHash([]byte("NotAnActualHash"))
	ledgerErr, ok := err.(*Error)
	if !(ok && ledgerErr.Type() == ErrorTypeBlockNotFound) {
		t.Fatal("A 'LedgerError' of type 'ErrorTypeBlockNotFound' should have been thrown")
	} else {
		t.Logf("An expected error [%s] is received", err)
	}
}

func testIndexesGetTransactionByBlockNumberAndTxIndex(t *testing.T) {
	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	defer func() { testBlockchainWrapper.blockchain.indexer.stop() }()
	blocks, _, err := testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}
	for i, block := range blocks {
		for j, tx := range block.GetTransactions() {
			testutil.AssertEquals(t, testBlockchainWrapper.getTransaction(uint64(i), uint64(j)), tx)
		}
	}
}

func testIndexesGetTransactionByBlockHashAndTxIndex(t *testing.T) {
	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	defer func() { testBlockchainWrapper.blockchain.indexer.stop() }()
	blocks, _, err := testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}
	for _, block := range blocks {
		blockHash, _ := block.GetHash()
		for j, tx := range block.GetTransactions() {
			testutil.AssertEquals(t, testBlockchainWrapper.getTransactionByBlockHash(blockHash, uint64(j)), tx)
		}
	}
}

func testIndexesGetTransactionByID(t *testing.T) {
	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	defer func() { testBlockchainWrapper.blockchain.indexer.stop() }()
	tx1, uuid1 := buildTestTx(t)
	tx2, uuid2 := buildTestTx(t)
	block1 := protos.NewBlock([]*protos.Transaction{tx1, tx2}, nil)
	testBlockchainWrapper.addNewBlock(block1, []byte("stateHash1"))

	tx3, uuid3 := buildTestTx(t)
	tx4, uuid4 := buildTestTx(t)
	block2 := protos.NewBlock([]*protos.Transaction{tx3, tx4}, nil)
	testBlockchainWrapper.addNewBlock(block2, []byte("stateHash2"))

	testutil.AssertEquals(t, testBlockchainWrapper.getTransactionByID(uuid1), tx1)
	testutil.AssertEquals(t, testBlockchainWrapper.getTransactionByID(uuid2), tx2)
	testutil.AssertEquals(t, testBlockchainWrapper.getTransactionByID(uuid3), tx3)
	testutil.AssertEquals(t, testBlockchainWrapper.getTransactionByID(uuid4), tx4)
}
