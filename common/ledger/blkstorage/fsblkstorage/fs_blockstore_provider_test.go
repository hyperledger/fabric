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
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	putil "github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, uint64(len(expectedBlocks)), bcInfo.Height)
	assert.Equal(t, expectedBlocks[len(expectedBlocks)-1].GetHeader().Hash(), bcInfo.CurrentBlockHash)

	itr, _ := store.RetrieveBlocks(0)
	for i := 0; i < len(expectedBlocks); i++ {
		block, _ := itr.Next()
		assert.Equal(t, expectedBlocks[i], block)
	}

	for blockNum := 0; blockNum < len(expectedBlocks); blockNum++ {
		block := expectedBlocks[blockNum]
		flags := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
		retrievedBlock, _ := store.RetrieveBlockByNumber(uint64(blockNum))
		assert.Equal(t, block, retrievedBlock)

		retrievedBlock, _ = store.RetrieveBlockByHash(block.Header.Hash())
		assert.Equal(t, block, retrievedBlock)

		for txNum := 0; txNum < len(block.Data.Data); txNum++ {
			txEnvBytes := block.Data.Data[txNum]
			txEnv, _ := putil.GetEnvelopeFromBlock(txEnvBytes)
			txid, err := putil.GetOrComputeTxIDFromEnvelope(txEnvBytes)
			assert.NoError(t, err)

			retrievedBlock, _ := store.RetrieveBlockByTxID(txid)
			assert.Equal(t, block, retrievedBlock)

			retrievedTxEnv, _ := store.RetrieveTxByID(txid)
			assert.Equal(t, txEnv, retrievedTxEnv)

			retrievedTxEnv, _ = store.RetrieveTxByBlockNumTranNum(uint64(blockNum), uint64(txNum))
			assert.Equal(t, txEnv, retrievedTxEnv)

			retrievedTxValCode, err := store.RetrieveTxValidationCodeByTxID(txid)
			assert.NoError(t, err)
			assert.Equal(t, flags.Flag(txNum), retrievedTxValCode)
		}
	}
}

func checkWithWrongInputs(t *testing.T, store blkstorage.BlockStore, numBlocks int) {
	block, err := store.RetrieveBlockByHash([]byte("non-existent-hash"))
	assert.Nil(t, block)
	assert.Equal(t, blkstorage.ErrNotFoundInIndex, err)

	block, err = store.RetrieveBlockByTxID("non-existent-txid")
	assert.Nil(t, block)
	assert.Equal(t, blkstorage.ErrNotFoundInIndex, err)

	tx, err := store.RetrieveTxByID("non-existent-txid")
	assert.Nil(t, tx)
	assert.Equal(t, blkstorage.ErrNotFoundInIndex, err)

	tx, err = store.RetrieveTxByBlockNumTranNum(uint64(numBlocks+1), uint64(0))
	assert.Nil(t, tx)
	assert.Equal(t, blkstorage.ErrNotFoundInIndex, err)

	txCode, err := store.RetrieveTxValidationCodeByTxID("non-existent-txid")
	assert.Equal(t, peer.TxValidationCode(-1), txCode)
	assert.Equal(t, blkstorage.ErrNotFoundInIndex, err)
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
	assert.Equal(t, numStores, len(storeNames))

	for i := 0; i < numStores; i++ {
		exists, err := provider.Exists(constructLedgerid(i))
		assert.NoError(t, err)
		assert.Equal(t, true, exists)
	}

	exists, err := provider.Exists(constructLedgerid(numStores + 1))
	assert.NoError(t, err)
	assert.Equal(t, false, exists)

}

func constructLedgerid(id int) string {
	return fmt.Sprintf("ledger_%d", id)
}
