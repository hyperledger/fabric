/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testBlockDetails struct {
	txIDs           []string
	validationCodes []peer.TxValidationCode
}

func TestBootstrapFromSnapshot(t *testing.T) {
	testPath := testPath()
	env := newTestEnv(t, NewConf(testPath, 0))
	defer func() {
		env.Cleanup()
	}()
	snapshotDir := filepath.Join(testPath, "snapshot")
	require.NoError(t, os.Mkdir(snapshotDir, 0755))

	blocksGenerator, genesisBlock := testutil.NewBlockGenerator(t, "testLedger", false)
	originalBlockStore, err := env.provider.Open("originalLedger")
	require.NoError(t, err)
	txIDGenesisTx, err := protoutil.GetOrComputeTxIDFromEnvelope(genesisBlock.Data.Data[0])
	require.NoError(t, err)

	blocksDetailsBeforeSnapshot := []*testBlockDetails{
		{
			txIDs: []string{txIDGenesisTx},
		},
		{
			txIDs: []string{"txid1", "txid2"},
		},
		{
			txIDs: []string{"txid3", "txid4", "txid5"},
		},
	}

	blocksDetailsAfterSnapshot := []*testBlockDetails{
		{
			txIDs: []string{"txid7", "txid8"},
		},
		{
			txIDs: []string{"txid9", "txid10", "txid11"},
			validationCodes: []peer.TxValidationCode{
				peer.TxValidationCode_BAD_CHANNEL_HEADER,
				peer.TxValidationCode_BAD_CREATOR_SIGNATURE,
			},
		},
	}

	blocksBeforeSnapshot := []*common.Block{
		genesisBlock,
		generateNextTestBlock(blocksGenerator, blocksDetailsBeforeSnapshot[1]),
		generateNextTestBlock(blocksGenerator, blocksDetailsBeforeSnapshot[2]),
	}

	blocksAfterSnapshot := []*common.Block{
		generateNextTestBlock(blocksGenerator, blocksDetailsAfterSnapshot[0]),
		generateNextTestBlock(blocksGenerator, blocksDetailsAfterSnapshot[1]),
	}

	for _, b := range blocksBeforeSnapshot {
		err := originalBlockStore.AddBlock(b)
		require.NoError(t, err)
	}
	_, err = originalBlockStore.ExportTxIds(snapshotDir, testNewHashFunc)
	require.NoError(t, err)
	lastBlockInSnapshot := blocksBeforeSnapshot[len(blocksBeforeSnapshot)-1]

	snapshotInfo := &SnapshotInfo{
		LedgerID:          "bootstrappedLedger",
		LastBlockHash:     protoutil.BlockHeaderHash(lastBlockInSnapshot.Header),
		LastBlockNum:      lastBlockInSnapshot.Header.Number,
		PreviousBlockHash: lastBlockInSnapshot.Header.PreviousHash,
	}

	// bootstrap another blockstore from the snapshot and verify its APIs
	importTxIDsBatchSize = uint64(2) // smaller batch size for testing
	bootstrappedBlockStore, err := env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
	require.NoError(t, err)

	// verify queries just after bootstrapping
	verifyQueriesOnBlocksPriorToSnapshot(
		t,
		bootstrappedBlockStore,
		&common.BlockchainInfo{
			Height:            snapshotInfo.LastBlockNum + 1,
			CurrentBlockHash:  snapshotInfo.LastBlockHash,
			PreviousBlockHash: snapshotInfo.PreviousBlockHash,
		},
		blocksDetailsBeforeSnapshot,
		blocksBeforeSnapshot,
	)

	require.EqualError(t,
		bootstrappedBlockStore.AddBlock(blocksAfterSnapshot[1]),
		"block number should have been 3 but was 4",
	)

	// add more blocks and verify query results again
	for _, b := range blocksAfterSnapshot {
		require.NoError(t, bootstrappedBlockStore.AddBlock(b))
	}
	finalBlock := blocksAfterSnapshot[len(blocksAfterSnapshot)-1]

	expectedBCInfo := &common.BlockchainInfo{
		Height:            finalBlock.Header.Number + 1,
		CurrentBlockHash:  protoutil.BlockHeaderHash(finalBlock.Header),
		PreviousBlockHash: finalBlock.Header.PreviousHash,
	}
	verifyQueriesOnBlocksPriorToSnapshot(t,
		bootstrappedBlockStore,
		expectedBCInfo,
		blocksDetailsBeforeSnapshot,
		blocksBeforeSnapshot,
	)
	verifyQueriesOnBlocksAddedAfterBootstrapping(t,
		bootstrappedBlockStore,
		expectedBCInfo,
		blocksDetailsAfterSnapshot,
		blocksAfterSnapshot,
	)

	// close and re-open the blockstore and again verify the query results
	env.provider.Close()
	env = newTestEnv(t, NewConf(testPath, 0))
	bootstrappedBlockStore, err = env.provider.Open("bootstrappedLedger")
	require.NoError(t, err)
	verifyQueriesOnBlocksPriorToSnapshot(t,
		bootstrappedBlockStore,
		expectedBCInfo,
		blocksDetailsBeforeSnapshot,
		blocksBeforeSnapshot,
	)
	verifyQueriesOnBlocksAddedAfterBootstrapping(t,
		bootstrappedBlockStore,
		expectedBCInfo,
		blocksDetailsAfterSnapshot,
		blocksAfterSnapshot,
	)

	// verify that bootstrapped blockstore itself can export TXIDs
	anotherSnapshotDir := filepath.Join(testPath, "anotherSnapshot")
	require.NoError(t, os.Mkdir(anotherSnapshotDir, 0755))
	fileHashes, err := bootstrappedBlockStore.ExportTxIds(anotherSnapshotDir, testNewHashFunc)
	require.NoError(t, err)
	expectedTxIDs := []string{}
	for _, b := range append(blocksDetailsBeforeSnapshot, blocksDetailsAfterSnapshot...) {
		expectedTxIDs = append(expectedTxIDs, b.txIDs...)
	}
	sort.Slice(expectedTxIDs, func(i, j int) bool {
		ith := expectedTxIDs[i]
		jth := expectedTxIDs[j]
		if len(ith) == len(jth) {
			return ith < jth
		}
		return len(ith) < len(jth)
	})
	verifyExportedTxIDs(t, anotherSnapshotDir, fileHashes, expectedTxIDs...)

	// verify that the bootstrapped blockstore can sync up indexes
	toAddWithoutIndexupdates := []*testBlockDetails{
		{
			txIDs: []string{"txid_syncup_index_1", "txid_syncup_index_2"},
		},
		{
			txIDs: []string{"txid_syncup_index_3", "txid_syncup_index_4"},
		},
	}

	blocks := []*common.Block{}
	blkfileMgr := bootstrappedBlockStore.fileMgr
	originalIndexDB := blkfileMgr.index.db

	// redirect index writes to some random place and add two blocks and then set the original index back
	bootstrappedBlockStore.fileMgr.index.db = env.provider.leveldbProvider.GetDBHandle(filepath.Join(testPath, "someRandomPlace"))
	for _, blockDetails := range toAddWithoutIndexupdates {
		block := generateNextTestBlock(blocksGenerator, blockDetails)
		blocks = append(blocks, block)
		require.NoError(t, blkfileMgr.addBlock(block))
	}
	blkfileMgr.index.db = originalIndexDB

	// before, we test for index sync-up, verify that the last set of blocks not indexed in the original index
	for _, block := range blocks {
		_, err := blkfileMgr.retrieveBlockByNumber(block.Header.Number)
		assert.Exactly(t, ErrNotFoundInIndex, err)
	}

	// close and open should sync-up the index
	env.provider.Close()
	env = newTestEnv(t, NewConf(testPath, 0))
	bootstrappedBlockStore, err = env.provider.Open("bootstrappedLedger")
	require.NoError(t, err)

	expectedBCInfo = &common.BlockchainInfo{
		Height:            blocks[1].Header.Number + 1,
		CurrentBlockHash:  protoutil.BlockHeaderHash(blocks[1].Header),
		PreviousBlockHash: blocks[1].Header.PreviousHash,
	}

	verifyQueriesOnBlocksAddedAfterBootstrapping(
		t,
		bootstrappedBlockStore,
		expectedBCInfo,
		toAddWithoutIndexupdates,
		blocks,
	)

	// verify that the bootstrapped blockstore returns error if indexes are deleted
	env.provider.Close()
	conf := NewConf(testPath, 0)
	require.NoError(t, os.RemoveAll(conf.getIndexDir()))
	env = newTestEnv(t, conf)
	bootstrappedBlockStore, err = env.provider.Open("bootstrappedLedger")
	require.EqualError(t, err,
		fmt.Sprintf(
			"cannot sync index with block files. blockstore is bootstrapped from a snapshot and first available block=[%d]",
			len(blocksBeforeSnapshot),
		),
	)
}

func TestBootstrapFromSnapshotErrorPaths(t *testing.T) {
	testPath := testPath()
	env := newTestEnv(t, NewConf(testPath, 0))
	defer func() {
		env.Cleanup()
	}()
	snapshotDir := filepath.Join(testPath, "snapshot")
	metadataFile := filepath.Join(snapshotDir, snapshotMetadataFileName)
	dataFile := filepath.Join(snapshotDir, snapshotDataFileName)
	ledgerDir := filepath.Join(testPath, "chains", "bootstrappedLedger")
	bootstrappingSnapshotInfoFile := filepath.Join(ledgerDir, bootstrappingSnapshotInfoFile)

	snapshotInfo := &SnapshotInfo{
		LedgerID:          "bootstrappedLedger",
		LastBlockHash:     []byte("LastBlockHash"),
		LastBlockNum:      5,
		PreviousBlockHash: []byte("PreviousBlockHash"),
	}

	cleanupDirs := func() {
		require.NoError(t, os.RemoveAll(ledgerDir))
		require.NoError(t, os.RemoveAll(snapshotDir))
		require.NoError(t, os.Mkdir(snapshotDir, 0755))
	}

	createSnapshotMetadataFile := func(content uint64) {
		mf, err := snapshot.CreateFile(metadataFile, snapshotFileFormat, testNewHashFunc)
		require.NoError(t, err)
		require.NoError(t, mf.EncodeUVarint(content))
		_, err = mf.Done()
		require.NoError(t, err)
	}
	createSnapshotDataFile := func(content ...string) {
		df, err := snapshot.CreateFile(dataFile, snapshotFileFormat, testNewHashFunc)
		require.NoError(t, err)
		for _, c := range content {
			require.NoError(t, df.EncodeString(c))
		}
		_, err = df.Done()
		require.NoError(t, err)
	}

	t.Run("metadata-file-missing", func(t *testing.T) {
		cleanupDirs()
		_, err := env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		require.Contains(t, err.Error(), "error while opening the snapshot file: "+metadataFile)
	})

	t.Run("bootstapping-more-than-once", func(t *testing.T) {
		cleanupDirs()
		env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		_, err := env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		require.EqualError(t, err, "dir "+ledgerDir+" not empty")
	})

	t.Run("metadata-file-corrupted", func(t *testing.T) {
		cleanupDirs()
		mf, err := snapshot.CreateFile(metadataFile, snapshotFileFormat, testNewHashFunc)
		require.NoError(t, err)
		require.NoError(t, mf.Close())
		_, err = env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		require.Contains(t, err.Error(), "error while reading from the snapshot file: "+metadataFile)
	})

	t.Run("data-file-missing", func(t *testing.T) {
		cleanupDirs()
		createSnapshotMetadataFile(1)
		_, err := env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		require.Contains(t, err.Error(), "error while opening the snapshot file: "+dataFile)
	})

	t.Run("data-file-corrupt", func(t *testing.T) {
		cleanupDirs()
		createSnapshotMetadataFile(2)
		createSnapshotDataFile("single-tx-id")
		_, err := env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		require.Contains(t, err.Error(), "error while reading from snapshot file: "+dataFile)
	})

	t.Run("db-error", func(t *testing.T) {
		cleanupDirs()
		createSnapshotMetadataFile(1)
		createSnapshotDataFile("single-tx-id")
		env.provider.Close()
		defer func() {
			env = newTestEnv(t, NewConf(testPath, 0))
		}()
		_, err := env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		require.Contains(t, err.Error(), "error writing batch to leveldb")
	})

	t.Run("bootstrappedsnapshotInfo-file-corrupt", func(t *testing.T) {
		cleanupDirs()
		createSnapshotMetadataFile(1)
		createSnapshotDataFile("single-tx-id")
		_, err := env.provider.BootstrapFromSnapshottedTxIDs(snapshotDir, snapshotInfo)
		require.NoError(t, err)
		env.provider.Close()
		env = newTestEnv(t, NewConf(testPath, 0))
		require.NoError(t, ioutil.WriteFile(bootstrappingSnapshotInfoFile, []byte("junk-data"), 0644))
		_, err = env.provider.Open(snapshotInfo.LedgerID)
		require.Contains(t, err.Error(), "error while unmarshalling bootstrappingSnapshotInfo")
	})

}

func generateNextTestBlock(bg *testutil.BlockGenerator, d *testBlockDetails) *common.Block {
	txContents := [][]byte{}
	for _, txID := range d.txIDs {
		txContents = append(txContents, []byte("dummy content for txid = "+txID))
	}
	block := bg.NextBlockWithTxid(txContents, d.txIDs)
	for i, validationCode := range d.validationCodes {
		txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER]).SetFlag(i, validationCode)
	}
	return block
}

func verifyQueriesOnBlocksPriorToSnapshot(
	t *testing.T,
	bootstrappedBlockStore *BlockStore,
	expectedBCInfo *common.BlockchainInfo,
	blocksDetailsBeforeSnapshot []*testBlockDetails,
	blocksBeforeSnapshot []*common.Block,
) {
	bci, err := bootstrappedBlockStore.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, expectedBCInfo, bci)

	for _, b := range blocksBeforeSnapshot {
		blockNum := b.Header.Number
		blockHash := protoutil.BlockHeaderHash(b.Header)
		expectedErrStr := fmt.Sprintf(
			"cannot serve block [%d]. The ledger is bootstrapped from a snapshot. First available block = [%d]",
			blockNum, len(blocksBeforeSnapshot),
		)

		_, err := bootstrappedBlockStore.RetrieveBlockByNumber(blockNum)
		require.EqualError(t, err, expectedErrStr)

		_, err = bootstrappedBlockStore.RetrieveBlocks(blockNum)
		require.EqualError(t, err, expectedErrStr)

		_, err = bootstrappedBlockStore.RetrieveTxByBlockNumTranNum(blockNum, 0)
		require.EqualError(t, err, expectedErrStr)

		_, err = bootstrappedBlockStore.RetrieveBlockByHash(blockHash)
		require.Equal(t, ErrNotFoundInIndex, err)
	}

	bootstrappingSnapshotHeight := uint64(len(blocksDetailsBeforeSnapshot))
	for _, d := range blocksDetailsBeforeSnapshot {
		for _, txID := range d.txIDs {
			expectedErrorStr := fmt.Sprintf(
				"details for the TXID [%s] not available. Ledger bootstrapped from a snapshot. First available block = [%d]",
				txID, bootstrappingSnapshotHeight,
			)
			_, err := bootstrappedBlockStore.RetrieveBlockByTxID(txID)
			require.EqualError(t, err, expectedErrorStr)

			_, err = bootstrappedBlockStore.RetrieveTxByID(txID)
			require.EqualError(t, err, expectedErrorStr)

			_, err = bootstrappedBlockStore.RetrieveTxValidationCodeByTxID(txID)
			require.EqualError(t, err, expectedErrorStr)
		}
	}
}

func verifyQueriesOnBlocksAddedAfterBootstrapping(t *testing.T,
	bootstrappedBlockStore *BlockStore,
	expectedBCInfo *common.BlockchainInfo,
	blocksDetailsAfterSnapshot []*testBlockDetails,
	blocksAfterSnapshot []*common.Block,
) {
	bci, err := bootstrappedBlockStore.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, expectedBCInfo, bci)

	for _, b := range blocksAfterSnapshot {
		retrievedBlock, err := bootstrappedBlockStore.RetrieveBlockByNumber(b.Header.Number)
		require.NoError(t, err)
		require.Equal(t, b, retrievedBlock)

		retrievedBlock, err = bootstrappedBlockStore.RetrieveBlockByHash(protoutil.BlockHeaderHash(b.Header))
		require.NoError(t, err)
		require.Equal(t, b, retrievedBlock)

		itr, err := bootstrappedBlockStore.RetrieveBlocks(b.Header.Number)
		require.NoError(t, err)
		blk, err := itr.Next()
		require.NoError(t, err)
		require.Equal(t, b, blk)
		itr.Close()

		retrievedTxEnv, err := bootstrappedBlockStore.RetrieveTxByBlockNumTranNum(b.Header.Number, 0)
		require.NoError(t, err)
		expectedTxEnv, err := protoutil.GetEnvelopeFromBlock(b.Data.Data[0])
		require.NoError(t, err)
		require.Equal(t, expectedTxEnv, retrievedTxEnv)
	}

	for i, d := range blocksDetailsAfterSnapshot {
		block := blocksAfterSnapshot[i]
		for j, txID := range d.txIDs {
			retrievedBlock, err := bootstrappedBlockStore.RetrieveBlockByTxID(txID)
			require.Equal(t, block, retrievedBlock)

			retrievedTxEnv, err := bootstrappedBlockStore.RetrieveTxByID(txID)
			expectedTxEnv, err := protoutil.GetEnvelopeFromBlock(block.Data.Data[j])
			require.NoError(t, err)
			require.Equal(t, expectedTxEnv, retrievedTxEnv)
		}

		for j, validationCode := range d.validationCodes {
			retrievedValidationCode, err := bootstrappedBlockStore.RetrieveTxValidationCodeByTxID(d.txIDs[j])
			require.NoError(t, err)
			require.Equal(t, validationCode, retrievedValidationCode)
		}
	}
}
