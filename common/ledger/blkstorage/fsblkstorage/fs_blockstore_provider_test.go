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

package fsblkstorage

import (
	"testing"

	"fmt"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
)

func TestMultipleBlockStores(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()

	provider := env.provider
	store1, _ := provider.OpenBlockStore("ledger1")
	defer store1.Shutdown()

	store2, _ := provider.CreateBlockStore("ledger2")
	defer store2.Shutdown()

	blocks1 := testutil.ConstructTestBlocks(t, 5)
	for _, b := range blocks1 {
		store1.AddBlock(b)
	}

	blocks2 := testutil.ConstructTestBlocks(t, 10)
	for _, b := range blocks2 {
		store2.AddBlock(b)
	}
	checkBlocks(t, blocks1, store1)
	checkBlocks(t, blocks2, store2)
	checkWithWrongInputs(t, store1, 5)
	checkWithWrongInputs(t, store2, 10)
}

func checkBlocks(t *testing.T, expectedBlocks []*common.Block, store blkstorage.BlockStore) {
	bcInfo, _ := store.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo.Height, uint64(len(expectedBlocks)))
	testutil.AssertEquals(t, bcInfo.CurrentBlockHash, expectedBlocks[len(expectedBlocks)-1].GetHeader().Hash())

	itr, _ := store.RetrieveBlocks(0)
	for i := 0; i < len(expectedBlocks); i++ {
		block, _ := itr.Next()
		testutil.AssertEquals(t, block, expectedBlocks[i])
	}

	for blockNum := 0; blockNum < len(expectedBlocks); blockNum++ {
		block := expectedBlocks[blockNum]
		flags := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
		retrievedBlock, _ := store.RetrieveBlockByNumber(uint64(blockNum))
		testutil.AssertEquals(t, retrievedBlock, block)

		retrievedBlock, _ = store.RetrieveBlockByHash(block.Header.Hash())
		testutil.AssertEquals(t, retrievedBlock, block)

		for txNum := 0; txNum < len(block.Data.Data); txNum++ {
			txEnvBytes := block.Data.Data[txNum]
			txEnv, _ := utils.GetEnvelopeFromBlock(txEnvBytes)
			txid, err := extractTxID(txEnvBytes)
			testutil.AssertNoError(t, err, "")

			retrievedBlock, _ := store.RetrieveBlockByTxID(txid)
			testutil.AssertEquals(t, retrievedBlock, block)

			retrievedTxEnv, _ := store.RetrieveTxByID(txid)
			testutil.AssertEquals(t, retrievedTxEnv, txEnv)

			retrievedTxEnv, _ = store.RetrieveTxByBlockNumTranNum(uint64(blockNum), uint64(txNum))
			testutil.AssertEquals(t, retrievedTxEnv, txEnv)

			retrievedTxValCode, err := store.RetrieveTxValidationCodeByTxID(txid)
			testutil.AssertNoError(t, err, "")
			testutil.AssertEquals(t, retrievedTxValCode, flags.Flag(txNum))
		}
	}
}

func checkWithWrongInputs(t *testing.T, store blkstorage.BlockStore, numBlocks int) {
	block, err := store.RetrieveBlockByHash([]byte("non-existent-hash"))
	testutil.AssertNil(t, block)
	testutil.AssertEquals(t, err, blkstorage.ErrNotFoundInIndex)

	block, err = store.RetrieveBlockByTxID("non-existent-txid")
	testutil.AssertNil(t, block)
	testutil.AssertEquals(t, err, blkstorage.ErrNotFoundInIndex)

	tx, err := store.RetrieveTxByID("non-existent-txid")
	testutil.AssertNil(t, tx)
	testutil.AssertEquals(t, err, blkstorage.ErrNotFoundInIndex)

	tx, err = store.RetrieveTxByBlockNumTranNum(uint64(numBlocks+1), uint64(0))
	testutil.AssertNil(t, tx)
	testutil.AssertEquals(t, err, blkstorage.ErrNotFoundInIndex)

	txCode, err := store.RetrieveTxValidationCodeByTxID("non-existent-txid")
	testutil.AssertEquals(t, txCode, peer.TxValidationCode(-1))
	testutil.AssertEquals(t, err, blkstorage.ErrNotFoundInIndex)
}

func TestBlockStoreProvider(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()

	provider := env.provider
	stores := []blkstorage.BlockStore{}
	numStores := 10
	for i := 0; i < numStores; i++ {
		store, _ := provider.OpenBlockStore(constructLedgerid(i))
		defer store.Shutdown()
		stores = append(stores, store)
	}

	storeNames, _ := provider.List()
	testutil.AssertEquals(t, len(storeNames), numStores)

	for i := 0; i < numStores; i++ {
		exists, err := provider.Exists(constructLedgerid(i))
		testutil.AssertNoError(t, err, "")
		testutil.AssertEquals(t, exists, true)
	}

	exists, err := provider.Exists(constructLedgerid(numStores + 1))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, exists, false)

}

func constructLedgerid(id int) string {
	return fmt.Sprintf("ledger_%d", id)
}
