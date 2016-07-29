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
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/statemgmt"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos"
	"github.com/tecbot/gorocksdb"
	"golang.org/x/net/context"
)

var testParams []string

func TestMain(m *testing.M) {
	testParams = testutil.ParseTestParams()
	testutil.SetupTestConfig()
	os.Exit(m.Run())
}

type blockchainTestWrapper struct {
	t          *testing.T
	blockchain *blockchain
}

func newTestBlockchainWrapper(t *testing.T) *blockchainTestWrapper {
	blockchain, err := newBlockchain()
	testutil.AssertNoError(t, err, "Error while getting handle to chain")
	return &blockchainTestWrapper{t, blockchain}
}

func (testWrapper *blockchainTestWrapper) addNewBlock(block *protos.Block, stateHash []byte) uint64 {
	writeBatch := gorocksdb.NewWriteBatch()
	defer writeBatch.Destroy()
	newBlockNumber, err := testWrapper.blockchain.addPersistenceChangesForNewBlock(context.TODO(), block, stateHash, writeBatch)
	testutil.AssertNoError(testWrapper.t, err, "Error while adding a new block")
	testDBWrapper.WriteToDB(testWrapper.t, writeBatch)
	testWrapper.blockchain.blockPersistenceStatus(true)
	return newBlockNumber
}

func (testWrapper *blockchainTestWrapper) fetchBlockchainSizeFromDB() uint64 {
	size, err := fetchBlockchainSizeFromDB()
	testutil.AssertNoError(testWrapper.t, err, "Error while fetching blockchain size from db")
	return size
}

func (testWrapper *blockchainTestWrapper) getBlock(blockNumber uint64) *protos.Block {
	block, err := testWrapper.blockchain.getBlock(blockNumber)
	testutil.AssertNoError(testWrapper.t, err, "Error while getting block from blockchain")
	return block
}

func (testWrapper *blockchainTestWrapper) getLastBlock() *protos.Block {
	block, err := testWrapper.blockchain.getLastBlock()
	testutil.AssertNoError(testWrapper.t, err, "Error while getting block from blockchain")
	return block
}

func (testWrapper *blockchainTestWrapper) getBlockByHash(blockHash []byte) *protos.Block {
	block, err := testWrapper.blockchain.getBlockByHash(blockHash)
	testutil.AssertNoError(testWrapper.t, err, "Error while getting block by blockhash from blockchain")
	return block
}

func (testWrapper *blockchainTestWrapper) getTransaction(blockNumber uint64, txIndex uint64) *protos.Transaction {
	tx, err := testWrapper.blockchain.getTransaction(blockNumber, txIndex)
	testutil.AssertNoError(testWrapper.t, err, "Error while getting tx from blockchain")
	return tx
}

func (testWrapper *blockchainTestWrapper) getTransactionByBlockHash(blockHash []byte, txIndex uint64) *protos.Transaction {
	tx, err := testWrapper.blockchain.getTransactionByBlockHash(blockHash, txIndex)
	testutil.AssertNoError(testWrapper.t, err, "Error while getting tx from blockchain")
	return tx
}

func (testWrapper *blockchainTestWrapper) getTransactionByID(txID string) *protos.Transaction {
	tx, err := testWrapper.blockchain.getTransactionByID(txID)
	testutil.AssertNoError(testWrapper.t, err, "Error while getting tx from blockchain")
	return tx
}
func (testWrapper *blockchainTestWrapper) populateBlockChainWithSampleData() (blocks []*protos.Block, hashes [][]byte, err error) {
	var allBlocks []*protos.Block
	var allHashes [][]byte

	// -----------------------------<Genesis block>-------------------------------
	// Add the first (genesis block)
	block1 := protos.NewBlock(nil, []byte(testutil.GenerateID(testWrapper.t)))
	allBlocks = append(allBlocks, block1)
	allHashes = append(allHashes, []byte("stateHash1"))
	testWrapper.addNewBlock(block1, []byte("stateHash1"))

	// -----------------------------</Genesis block>------------------------------

	// -----------------------------<Block 2>-------------------------------------
	// Deploy a chaincode
	transaction2a, err := protos.NewTransaction(protos.ChaincodeID{Path: "Contracts"}, testutil.GenerateID(testWrapper.t), "NewContract", []string{"name: MyContract1, code: var x; function setX(json) {x = json.x}}"})
	if err != nil {
		return nil, nil, err
	}
	// Now we add the transaction to the block 2 and add the block to the chain
	transactions2a := []*protos.Transaction{transaction2a}
	block2 := protos.NewBlock(transactions2a, nil)

	allBlocks = append(allBlocks, block2)
	allHashes = append(allHashes, []byte("stateHash2"))
	testWrapper.addNewBlock(block2, []byte("stateHash2"))
	// -----------------------------</Block 2>------------------------------------

	// -----------------------------<Block 3>-------------------------------------
	// Create a transaction
	transaction3a, err := protos.NewTransaction(protos.ChaincodeID{Path: "MyContract"}, testutil.GenerateID(testWrapper.t), "setX", []string{"{x: \"hello\"}"})
	if err != nil {
		return nil, nil, err
	}
	// Create the third block and add it to the chain
	transactions3a := []*protos.Transaction{transaction3a}
	block3 := protos.NewBlock(transactions3a, nil)
	allBlocks = append(allBlocks, block3)
	allHashes = append(allHashes, []byte("stateHash3"))
	testWrapper.addNewBlock(block3, []byte("stateHash3"))

	// -----------------------------</Block 3>------------------------------------
	return allBlocks, allHashes, nil
}

func buildTestTx(tb testing.TB) (*protos.Transaction, string) {
	uuid := util.GenerateUUID()
	tx, err := protos.NewTransaction(protos.ChaincodeID{Path: "testUrl"}, uuid, "anyfunction", []string{"param1, param2"})
	testutil.AssertNil(tb, err)
	return tx, uuid
}

func buildTestBlock(t *testing.T) (*protos.Block, error) {
	transactions := []*protos.Transaction{}
	tx, _ := buildTestTx(t)
	transactions = append(transactions, tx)
	block := protos.NewBlock(transactions, nil)
	return block, nil
}

type ledgerTestWrapper struct {
	ledger *Ledger
	tb     testing.TB
}

func createFreshDBAndTestLedgerWrapper(tb testing.TB) *ledgerTestWrapper {
	testDBWrapper.CleanDB(tb)
	ledger, err := GetNewLedger()
	testutil.AssertNoError(tb, err, "Error while constructing ledger")
	return &ledgerTestWrapper{ledger, tb}
}

func (ledgerTestWrapper *ledgerTestWrapper) GetState(chaincodeID string, key string, committed bool) []byte {
	value, err := ledgerTestWrapper.ledger.GetState(chaincodeID, key, committed)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error while getting state from ledger")
	return value
}

func (ledgerTestWrapper *ledgerTestWrapper) GetBlockByNumber(blockNumber uint64) *protos.Block {
	block, err := ledgerTestWrapper.ledger.GetBlockByNumber(blockNumber)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error while getting block from ledger")
	return block
}

func (ledgerTestWrapper *ledgerTestWrapper) VerifyChain(highBlock, lowBlock uint64) uint64 {
	result, err := ledgerTestWrapper.ledger.VerifyChain(highBlock, lowBlock)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error while verifying chain")
	return result
}

func (ledgerTestWrapper *ledgerTestWrapper) PutRawBlock(block *protos.Block, blockNumber uint64) {
	err := ledgerTestWrapper.ledger.PutRawBlock(block, blockNumber)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error while verifying chain")
}

func (ledgerTestWrapper *ledgerTestWrapper) GetStateDelta(blockNumber uint64) *statemgmt.StateDelta {
	delta, err := ledgerTestWrapper.ledger.GetStateDelta(blockNumber)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error while getting state delta from ledger")
	return delta
}

func (ledgerTestWrapper *ledgerTestWrapper) GetTempStateHash() []byte {
	hash, err := ledgerTestWrapper.ledger.GetTempStateHash()
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error while getting state hash from ledger")
	return hash
}

func (ledgerTestWrapper *ledgerTestWrapper) ApplyStateDelta(id interface{}, delta *statemgmt.StateDelta) {
	err := ledgerTestWrapper.ledger.ApplyStateDelta(id, delta)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error applying state delta")
}

func (ledgerTestWrapper *ledgerTestWrapper) CommitStateDelta(id interface{}) {
	err := ledgerTestWrapper.ledger.CommitStateDelta(id)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error committing state delta")
}

func (ledgerTestWrapper *ledgerTestWrapper) RollbackStateDelta(id interface{}) {
	err := ledgerTestWrapper.ledger.RollbackStateDelta(id)
	testutil.AssertNoError(ledgerTestWrapper.tb, err, "error rolling back state delta")
}
