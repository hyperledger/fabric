/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultipleBlockStores(t *testing.T) {
	tempdir := testPath()
	defer os.RemoveAll(tempdir)

	env := newTestEnv(t, NewConf(tempdir, 0))
	provider := env.provider
	defer provider.Close()

	subdirs, err := provider.List()
	require.NoError(t, err)
	require.Empty(t, subdirs)

	store1, err := provider.OpenBlockStore("ledger1")
	require.NoError(t, err)
	defer store1.Shutdown()
	store2, err := provider.CreateBlockStore("ledger2")
	require.NoError(t, err)
	defer store2.Shutdown()

	blocks1 := testutil.ConstructTestBlocks(t, 5)
	for _, b := range blocks1 {
		err := store1.AddBlock(b)
		require.NoError(t, err)
	}

	blocks2 := testutil.ConstructTestBlocks(t, 10)
	for _, b := range blocks2 {
		err := store2.AddBlock(b)
		require.NoError(t, err)
	}
	checkBlocks(t, blocks1, store1)
	checkBlocks(t, blocks2, store2)
	checkWithWrongInputs(t, store1, 5)
	checkWithWrongInputs(t, store2, 10)

	store1.Shutdown()
	store2.Shutdown()
	provider.Close()

	// Reopen provider
	newenv := newTestEnv(t, NewConf(tempdir, 0))
	newprovider := newenv.provider
	defer newprovider.Close()

	subdirs, err = newprovider.List()
	require.NoError(t, err)
	require.Len(t, subdirs, 2)

	newstore1, err := newprovider.OpenBlockStore("ledger1")
	require.NoError(t, err)
	defer newstore1.Shutdown()
	newstore2, err := newprovider.CreateBlockStore("ledger2")
	require.NoError(t, err)
	defer newstore2.Shutdown()

	checkBlocks(t, blocks1, newstore1)
	checkBlocks(t, blocks2, newstore2)
	checkWithWrongInputs(t, newstore1, 5)
	checkWithWrongInputs(t, newstore2, 10)
}

func checkBlocks(t *testing.T, expectedBlocks []*common.Block, store blkstorage.BlockStore) {
	bcInfo, _ := store.GetBlockchainInfo()
	assert.Equal(t, uint64(len(expectedBlocks)), bcInfo.Height)
	assert.Equal(t, protoutil.BlockHeaderHash(expectedBlocks[len(expectedBlocks)-1].GetHeader()), bcInfo.CurrentBlockHash)

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

		retrievedBlock, _ = store.RetrieveBlockByHash(protoutil.BlockHeaderHash(block.Header))
		assert.Equal(t, block, retrievedBlock)

		for txNum := 0; txNum < len(block.Data.Data); txNum++ {
			txEnvBytes := block.Data.Data[txNum]
			txEnv, _ := protoutil.GetEnvelopeFromBlock(txEnvBytes)
			txid, err := protoutil.GetOrComputeTxIDFromEnvelope(txEnvBytes)
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
	storeNames, err := provider.List()
	assert.NoError(t, err)
	assert.Empty(t, storeNames)

	var stores []blkstorage.BlockStore
	numStores := 10
	for i := 0; i < numStores; i++ {
		store, _ := provider.OpenBlockStore(constructLedgerid(i))
		defer store.Shutdown()
		stores = append(stores, store)
	}
	assert.Equal(t, numStores, len(stores))

	storeNames, err = provider.List()
	assert.NoError(t, err)
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
