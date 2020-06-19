/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestIndexConfig(t *testing.T) {
	ic := &IndexConfig{
		AttrsToIndex: []IndexableAttr{
			IndexableAttrBlockNum,
			IndexableAttrTxID,
		},
	}

	require := require.New(t)
	require.True(ic.Contains(IndexableAttrBlockNum))
	require.True(ic.Contains(IndexableAttrTxID))
	require.False(ic.Contains(IndexableAttrBlockNumTranNum))
}

func TestMultipleBlockStores(t *testing.T) {
	tempdir := testPath()
	defer os.RemoveAll(tempdir)

	env := newTestEnv(t, NewConf(tempdir, 0))
	provider := env.provider
	defer provider.Close()

	subdirs, err := provider.List()
	require.NoError(t, err)
	require.Empty(t, subdirs)

	store1, err := provider.Open("ledger1")
	require.NoError(t, err)
	defer store1.Shutdown()
	store2, err := provider.Open("ledger2")
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

	newstore1, err := newprovider.Open("ledger1")
	require.NoError(t, err)
	defer newstore1.Shutdown()
	newstore2, err := newprovider.Open("ledger2")
	require.NoError(t, err)
	defer newstore2.Shutdown()

	checkBlocks(t, blocks1, newstore1)
	checkBlocks(t, blocks2, newstore2)
	checkWithWrongInputs(t, newstore1, 5)
	checkWithWrongInputs(t, newstore2, 10)
}

func checkBlocks(t *testing.T, expectedBlocks []*common.Block, store *BlockStore) {
	bcInfo, _ := store.GetBlockchainInfo()
	require.Equal(t, uint64(len(expectedBlocks)), bcInfo.Height)
	require.Equal(t, protoutil.BlockHeaderHash(expectedBlocks[len(expectedBlocks)-1].GetHeader()), bcInfo.CurrentBlockHash)

	itr, _ := store.RetrieveBlocks(0)
	for i := 0; i < len(expectedBlocks); i++ {
		block, _ := itr.Next()
		require.Equal(t, expectedBlocks[i], block)
	}

	for blockNum := 0; blockNum < len(expectedBlocks); blockNum++ {
		block := expectedBlocks[blockNum]
		flags := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
		retrievedBlock, _ := store.RetrieveBlockByNumber(uint64(blockNum))
		require.Equal(t, block, retrievedBlock)

		retrievedBlock, _ = store.RetrieveBlockByHash(protoutil.BlockHeaderHash(block.Header))
		require.Equal(t, block, retrievedBlock)

		for txNum := 0; txNum < len(block.Data.Data); txNum++ {
			txEnvBytes := block.Data.Data[txNum]
			txEnv, _ := protoutil.GetEnvelopeFromBlock(txEnvBytes)
			txid, err := protoutil.GetOrComputeTxIDFromEnvelope(txEnvBytes)
			require.NoError(t, err)

			retrievedBlock, _ := store.RetrieveBlockByTxID(txid)
			require.Equal(t, block, retrievedBlock)

			retrievedTxEnv, _ := store.RetrieveTxByID(txid)
			require.Equal(t, txEnv, retrievedTxEnv)

			retrievedTxEnv, _ = store.RetrieveTxByBlockNumTranNum(uint64(blockNum), uint64(txNum))
			require.Equal(t, txEnv, retrievedTxEnv)

			retrievedTxValCode, err := store.RetrieveTxValidationCodeByTxID(txid)
			require.NoError(t, err)
			require.Equal(t, flags.Flag(txNum), retrievedTxValCode)
		}
	}
}

func checkWithWrongInputs(t *testing.T, store *BlockStore, numBlocks int) {
	block, err := store.RetrieveBlockByHash([]byte("non-existent-hash"))
	require.Nil(t, block)
	require.Equal(t, ErrNotFoundInIndex, err)

	block, err = store.RetrieveBlockByTxID("non-existent-txid")
	require.Nil(t, block)
	require.Equal(t, ErrNotFoundInIndex, err)

	tx, err := store.RetrieveTxByID("non-existent-txid")
	require.Nil(t, tx)
	require.Equal(t, ErrNotFoundInIndex, err)

	tx, err = store.RetrieveTxByBlockNumTranNum(uint64(numBlocks+1), uint64(0))
	require.Nil(t, tx)
	require.Equal(t, ErrNotFoundInIndex, err)

	txCode, err := store.RetrieveTxValidationCodeByTxID("non-existent-txid")
	require.Equal(t, peer.TxValidationCode(-1), txCode)
	require.Equal(t, ErrNotFoundInIndex, err)
}

func TestBlockStoreProvider(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()

	provider := env.provider
	storeNames, err := provider.List()
	require.NoError(t, err)
	require.Empty(t, storeNames)

	var stores []*BlockStore
	numStores := 10
	for i := 0; i < numStores; i++ {
		store, _ := provider.Open(constructLedgerid(i))
		defer store.Shutdown()
		stores = append(stores, store)
	}
	require.Equal(t, numStores, len(stores))

	storeNames, err = provider.List()
	require.NoError(t, err)
	require.Equal(t, numStores, len(storeNames))

	for i := 0; i < numStores; i++ {
		exists, err := provider.Exists(constructLedgerid(i))
		require.NoError(t, err)
		require.Equal(t, true, exists)
	}

	exists, err := provider.Exists(constructLedgerid(numStores + 1))
	require.NoError(t, err)
	require.Equal(t, false, exists)

}

func constructLedgerid(id int) string {
	return fmt.Sprintf("ledger_%d", id)
}
