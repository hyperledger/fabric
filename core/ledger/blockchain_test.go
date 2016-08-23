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
	"time"

	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/util"
	"github.com/hyperledger/fabric/protos"
)

func TestBlockchain_InfoNoBlock(t *testing.T) {
	testDBWrapper.CleanDB(t)
	blockchainTestWrapper := newTestBlockchainWrapper(t)
	blockchain := blockchainTestWrapper.blockchain
	blockchainInfo, err := blockchain.getBlockchainInfo()
	testutil.AssertNoError(t, err, "Error while invoking getBlockchainInfo() on an emply blockchain")
	testutil.AssertEquals(t, blockchainInfo.Height, uint64(0))
	testutil.AssertEquals(t, blockchainInfo.CurrentBlockHash, nil)
	testutil.AssertEquals(t, blockchainInfo.PreviousBlockHash, nil)
}

func TestBlockchain_Info(t *testing.T) {
	testDBWrapper.CleanDB(t)
	blockchainTestWrapper := newTestBlockchainWrapper(t)
	blocks, _, _ := blockchainTestWrapper.populateBlockChainWithSampleData()

	blockchain := blockchainTestWrapper.blockchain
	blockchainInfo, _ := blockchain.getBlockchainInfo()
	testutil.AssertEquals(t, blockchainInfo.Height, uint64(3))
	currentBlockHash, _ := blocks[len(blocks)-1].GetHash()
	previousBlockHash, _ := blocks[len(blocks)-2].GetHash()
	testutil.AssertEquals(t, blockchainInfo.CurrentBlockHash, currentBlockHash)
	testutil.AssertEquals(t, blockchainInfo.PreviousBlockHash, previousBlockHash)
}

func TestBlockChain_SingleBlock(t *testing.T) {
	testDBWrapper.CleanDB(t)
	blockchainTestWrapper := newTestBlockchainWrapper(t)
	blockchain := blockchainTestWrapper.blockchain

	// Create the Chaincode specification
	chaincodeSpec := &protos.ChaincodeSpec{Type: protos.ChaincodeSpec_GOLANG,
		ChaincodeID: &protos.ChaincodeID{Path: "Contracts"},
		CtorMsg:     &protos.ChaincodeInput{Args: util.ToChaincodeArgs("Initialize", "param1")}}
	chaincodeDeploymentSepc := &protos.ChaincodeDeploymentSpec{ChaincodeSpec: chaincodeSpec}
	uuid := testutil.GenerateID(t)
	newChaincodeTx, err := protos.NewChaincodeDeployTransaction(chaincodeDeploymentSepc, uuid)
	testutil.AssertNoError(t, err, "Failed to create new chaincode Deployment Transaction")
	t.Logf("New chaincode tx: %v", newChaincodeTx)

	block1 := protos.NewBlock([]*protos.Transaction{newChaincodeTx}, nil)
	blockNumber := blockchainTestWrapper.addNewBlock(block1, []byte("stateHash1"))
	t.Logf("New chain: %v", blockchain)
	testutil.AssertEquals(t, blockNumber, uint64(0))
	testutil.AssertEquals(t, blockchain.getSize(), uint64(1))
	testutil.AssertEquals(t, blockchainTestWrapper.fetchBlockchainSizeFromDB(), uint64(1))
}

func TestBlockChain_SimpleChain(t *testing.T) {
	testDBWrapper.CleanDB(t)
	blockchainTestWrapper := newTestBlockchainWrapper(t)
	blockchain := blockchainTestWrapper.blockchain
	allBlocks, allStateHashes, err := blockchainTestWrapper.populateBlockChainWithSampleData()
	if err != nil {
		t.Logf("Error populating block chain with sample data: %s", err)
		t.Fail()
	}
	testutil.AssertEquals(t, blockchain.getSize(), uint64(len(allBlocks)))
	testutil.AssertEquals(t, blockchainTestWrapper.fetchBlockchainSizeFromDB(), uint64(len(allBlocks)))

	for i := range allStateHashes {
		t.Logf("Checking state hash for block number = [%d]", i)
		testutil.AssertEquals(t, blockchainTestWrapper.getBlock(uint64(i)).GetStateHash(), allStateHashes[i])
	}

	for i := range allBlocks {
		t.Logf("Checking block hash for block number = [%d]", i)
		blockhash, _ := blockchainTestWrapper.getBlock(uint64(i)).GetHash()
		expectedBlockHash, _ := allBlocks[i].GetHash()
		testutil.AssertEquals(t, blockhash, expectedBlockHash)
	}

	testutil.AssertNil(t, blockchainTestWrapper.getBlock(uint64(0)).PreviousBlockHash)

	i := 1
	for i < len(allBlocks) {
		t.Logf("Checking previous block hash for block number = [%d]", i)
		expectedPreviousBlockHash, _ := allBlocks[i-1].GetHash()
		testutil.AssertEquals(t, blockchainTestWrapper.getBlock(uint64(i)).PreviousBlockHash, expectedPreviousBlockHash)
		i++
	}
}

func TestBlockChainEmptyChain(t *testing.T) {
	testDBWrapper.CleanDB(t)
	blockchainTestWrapper := newTestBlockchainWrapper(t)
	testutil.AssertEquals(t, blockchainTestWrapper.blockchain.getSize(), uint64(0))
	block := blockchainTestWrapper.getLastBlock()
	if block != nil {
		t.Fatalf("Get last block on an empty chain should return nil.")
	}
	t.Logf("last block = [%s]", block)
}

func TestBlockchainBlockLedgerCommitTimestamp(t *testing.T) {
	testDBWrapper.CleanDB(t)
	blockchainTestWrapper := newTestBlockchainWrapper(t)
	block1 := protos.NewBlock(nil, nil)
	startTime := util.CreateUtcTimestamp()
	time.Sleep(2 * time.Second)
	blockchainTestWrapper.addNewBlock(block1, []byte("stateHash1"))
	lastBlock := blockchainTestWrapper.getLastBlock()
	if lastBlock.NonHashData == nil {
		t.Fatal("Expected block to have non-hash-data, but it was nil")
	}
	if lastBlock.NonHashData.LocalLedgerCommitTimestamp == nil {
		t.Fatal("Expected block to have non-hash-data timestamp, but it was nil")
	}
	if startTime.Seconds >= lastBlock.NonHashData.LocalLedgerCommitTimestamp.Seconds {
		t.Fatal("Expected block time to be after start time")
	}
}
