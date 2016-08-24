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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/protos"
	"github.com/tecbot/gorocksdb"
)

func TestIndexesAsync_GetBlockByBlockNumber(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetBlockByBlockNumber(t)
}

func TestIndexesAsync_GetBlockByBlockHash(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetBlockByBlockHash(t)
}

func TestIndexesAsync_GetBlockByBlockHashWrongHash(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetBlockByBlockHashWrongHash(t)
}

func TestIndexesAsync_GetTransactionByBlockNumberAndTxIndex(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetTransactionByBlockNumberAndTxIndex(t)
}

func TestIndexesAsync_GetTransactionByBlockHashAndTxIndex(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetTransactionByBlockHashAndTxIndex(t)
}

func TestIndexesAsync_GetTransactionByID(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()
	testIndexesGetTransactionByID(t)
}

func TestIndexesAsync_IndexingErrorScenario(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()

	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	chain := testBlockchainWrapper.blockchain
	asyncIndexer, _ := chain.indexer.(*blockchainIndexerAsync)

	defer func() {
		// first stop and then set the error to nil.
		// Otherwise stop may hang (waiting for catching up the index with the committing block)
		testBlockchainWrapper.blockchain.indexer.stop()
		asyncIndexer.indexerState.setError(nil)
	}()

	blocks, _, err := testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}

	t.Log("Setting an error artificially so as to client query gets an error")
	asyncIndexer.indexerState.setError(errors.New("Error created for testing"))

	// populate more data after error
	_, _, err = testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}
	fmt.Println("Going to execute QUERY")
	blockHash, _ := blocks[0].GetHash()
	// index query should throw error

	_, err = chain.getBlockByHash(blockHash)
	fmt.Println("executed QUERY")
	if err == nil {
		t.Fatal("Error expected during execution of client query")
	}
}

func TestIndexesAsync_ClientWaitScenario(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()

	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)
	defer func() { testBlockchainWrapper.blockchain.indexer.stop() }()

	chain := testBlockchainWrapper.blockchain
	blocks, _, err := testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}
	t.Log("Increasing size of blockchain by one artificially so as to make client wait")
	chain.size = chain.size + 1
	t.Log("Resetting size of blockchain to original and adding one block in a separate go routine so as to wake up the client")
	go func() {
		time.Sleep(2 * time.Second)
		chain.size = chain.size - 1
		blk, err := buildTestBlock(t)
		if err != nil {
			t.Logf("Error building test block: %s", err)
			t.Fail()
		}
		testBlockchainWrapper.addNewBlock(blk, []byte("stateHash"))
	}()
	t.Log("Executing client query. The client would wait and will be woken up")
	blockHash, _ := blocks[0].GetHash()
	block := testBlockchainWrapper.getBlockByHash(blockHash)
	testutil.AssertEquals(t, block, blocks[0])
}

type NoopIndexer struct {
}

func (noop *NoopIndexer) isSynchronous() bool {
	return true
}
func (noop *NoopIndexer) start(blockchain *blockchain) error {
	return nil
}
func (noop *NoopIndexer) createIndexes(block *protos.Block, blockNumber uint64, blockHash []byte, writeBatch *gorocksdb.WriteBatch) error {
	return nil
}
func (noop *NoopIndexer) fetchBlockNumberByBlockHash(blockHash []byte) (uint64, error) {
	return 0, nil
}
func (noop *NoopIndexer) fetchTransactionIndexByID(txID string) (uint64, uint64, error) {
	return 0, 0, nil
}
func (noop *NoopIndexer) stop() {
}

func TestIndexesAsync_IndexPendingBlocks(t *testing.T) {
	defaultSetting := indexBlockDataSynchronously
	indexBlockDataSynchronously = false
	defer func() { indexBlockDataSynchronously = defaultSetting }()

	testDBWrapper.CleanDB(t)
	testBlockchainWrapper := newTestBlockchainWrapper(t)

	// stop the original indexer and change the indexer to Noop - so, no block is indexed
	chain := testBlockchainWrapper.blockchain
	chain.indexer.stop()
	chain.indexer = &NoopIndexer{}
	blocks, _, err := testBlockchainWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Fatalf("Error populating block chain with sample data: %s", err)
	}

	// close the db
	testDBWrapper.CloseDB(t)
	// open the db again and create new instance of blockchain (and the associated async indexer)
	// the indexer should index the pending blocks
	testDBWrapper.OpenDB(t)
	testBlockchainWrapper = newTestBlockchainWrapper(t)
	defer chain.indexer.stop()

	blockHash, _ := blocks[0].GetHash()
	block := testBlockchainWrapper.getBlockByHash(blockHash)
	testutil.AssertEquals(t, block, blocks[0])

	blockHash, _ = blocks[len(blocks)-1].GetHash()
	block = testBlockchainWrapper.getBlockByHash(blockHash)
	testutil.AssertEquals(t, block, blocks[len(blocks)-1])
}
