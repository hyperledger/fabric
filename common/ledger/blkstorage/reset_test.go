/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestResetToGenesisBlkSingleBlkFile(t *testing.T) {
	blockStoreRootDir := "/tmp/testBlockStoreReset"
	require.NoError(t, os.RemoveAll(blockStoreRootDir))
	env := newTestEnv(t, NewConf(blockStoreRootDir, 0))
	defer env.Cleanup()
	provider := env.provider
	store, err := provider.Open("ledger1")
	require.NoError(t, err)

	// Add 50 blocks and shutdown blockstore store
	blocks := testutil.ConstructTestBlocks(t, 50)
	for _, b := range blocks {
		require.NoError(t, store.AddBlock(b))
	}
	store.Shutdown()
	provider.Close()

	ledgerDir := (&Conf{blockStorageDir: blockStoreRootDir}).getLedgerBlockDir("ledger1")

	_, lastOffsetOriginal, numBlocksOriginal, err := scanForLastCompleteBlock(ledgerDir, 0, 0)
	t.Logf("lastOffsetOriginal=%d", lastOffsetOriginal)
	require.NoError(t, err)
	require.Equal(t, 50, numBlocksOriginal)
	fileInfo, err := os.Stat(deriveBlockfilePath(ledgerDir, 0))
	require.NoError(t, err)
	require.Equal(t, fileInfo.Size(), lastOffsetOriginal)

	resetToGenesisBlk(ledgerDir)
	assertBlocksDirOnlyFileWithGenesisBlock(t, ledgerDir, blocks[0])
}

func TestResetToGenesisBlkMultipleBlkFiles(t *testing.T) {
	blockStoreRootDir := "/tmp/testBlockStoreReset"
	require.NoError(t, os.RemoveAll(blockStoreRootDir))
	blocks := testutil.ConstructTestBlocks(t, 20) // 20 blocks persisted in ~5 block files
	blocksPerFile := 20 / 5
	env := newTestEnv(t, NewConf(blockStoreRootDir, 0))
	defer env.Cleanup()
	provider := env.provider
	store, err := provider.Open("ledger1")
	require.NoError(t, err)
	for i, b := range blocks {
		require.NoError(t, store.AddBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 4):file0, (5,8):file1, (9,12):file2, (13, 16):file3, (17,19):file4]
			store.fileMgr.moveToNextFile()
		}
	}
	store.Shutdown()
	provider.Close()

	ledgerDir := (&Conf{blockStorageDir: blockStoreRootDir}).getLedgerBlockDir("ledger1")
	files, err := ioutil.ReadDir(ledgerDir)
	require.NoError(t, err)
	require.Len(t, files, 5)
	resetToGenesisBlk(ledgerDir)
	assertBlocksDirOnlyFileWithGenesisBlock(t, ledgerDir, blocks[0])
}

func TestResetBlockStore(t *testing.T) {
	blockStoreRootDir := "/tmp/testBlockStoreReset"
	os.RemoveAll(blockStoreRootDir)
	blocks1 := testutil.ConstructTestBlocks(t, 20) // 20 blocks persisted in ~5 block files
	blocks2 := testutil.ConstructTestBlocks(t, 40) // 40 blocks persisted in ~5 block files
	maxFileSie := int(0.2 * float64(testutilEstimateTotalSizeOnDisk(t, blocks1)))

	env := newTestEnv(t, NewConf(blockStoreRootDir, maxFileSie))
	defer env.Cleanup()
	provider := env.provider
	store1, err := provider.Open("ledger1")
	require.NoError(t, err)
	store2, _ := provider.Open("ledger2")

	for _, b := range blocks1 {
		store1.AddBlock(b)
	}

	for _, b := range blocks2 {
		store2.AddBlock(b)
	}

	store1.Shutdown()
	store2.Shutdown()
	provider.Close()

	require.NoError(t, ResetBlockStore(blockStoreRootDir))
	// test load and clear preResetHeight for ledger1 and ledger2
	ledgerIDs := []string{"ledger1", "ledger2"}
	h, err := LoadPreResetHeight(blockStoreRootDir, ledgerIDs)
	require.NoError(t, err)
	require.Equal(t,
		map[string]uint64{
			"ledger1": 20,
			"ledger2": 40,
		},
		h,
	)

	env = newTestEnv(t, NewConf(blockStoreRootDir, maxFileSie))
	provider = env.provider
	store1, _ = provider.Open("ledger1")
	store2, _ = provider.Open("ledger2")
	assertBlockStorePostReset(t, store1, blocks1)
	assertBlockStorePostReset(t, store2, blocks2)

	require.NoError(t, ClearPreResetHeight(blockStoreRootDir, ledgerIDs))
	h, err = LoadPreResetHeight(blockStoreRootDir, ledgerIDs)
	require.NoError(t, err)
	require.Equal(t,
		map[string]uint64{},
		h,
	)

	// reset again to test load and clear preResetHeight for ledger2
	require.NoError(t, ResetBlockStore(blockStoreRootDir))
	ledgerIDs = []string{"ledger2"}
	h, err = LoadPreResetHeight(blockStoreRootDir, ledgerIDs)
	require.NoError(t, err)
	require.Equal(t,
		map[string]uint64{
			"ledger2": 40,
		},
		h,
	)
	require.NoError(t, ClearPreResetHeight(blockStoreRootDir, ledgerIDs))
	// verify that ledger1 has preResetHeight file is not deleted
	h, err = LoadPreResetHeight(blockStoreRootDir, []string{"ledger1", "ledger2"})
	require.NoError(t, err)
	require.Equal(t,
		map[string]uint64{
			"ledger1": 20,
		},
		h,
	)
}

func TestRecordHeight(t *testing.T) {
	blockStoreRootDir := "/tmp/testBlockStoreReset"
	require.NoError(t, os.RemoveAll(blockStoreRootDir))
	env := newTestEnv(t, NewConf(blockStoreRootDir, 0))
	defer env.Cleanup()
	provider := env.provider
	store, err := provider.Open("ledger1")
	require.NoError(t, err)

	blocks := testutil.ConstructTestBlocks(t, 60)

	// Add 50 blocks, record, and require the recording of the current height
	for _, b := range blocks[:50] {
		require.NoError(t, store.AddBlock(b))
	}
	ledgerDir := (&Conf{blockStorageDir: blockStoreRootDir}).getLedgerBlockDir("ledger1")
	require.NoError(t, recordHeightIfGreaterThanPreviousRecording(ledgerDir))
	assertRecordedHeight(t, ledgerDir, "50")

	// Add 10 more blocks, record again and require that the previous recorded info is overwritten with new current height
	for _, b := range blocks[50:] {
		require.NoError(t, store.AddBlock(b))
	}
	require.NoError(t, recordHeightIfGreaterThanPreviousRecording(ledgerDir))
	assertRecordedHeight(t, ledgerDir, "60")

	// truncate the most recent block file to half
	// record again and require that the previous recorded info is NOT overwritten with new current height
	// because the current height is less than the previously recorded height
	lastFileNum, err := retrieveLastFileSuffix(ledgerDir)
	require.NoError(t, err)
	lastFile := deriveBlockfilePath(ledgerDir, lastFileNum)
	fileInfo, err := os.Stat(lastFile)
	require.NoError(t, err)
	require.NoError(t, os.Truncate(lastFile, fileInfo.Size()/2))
	blkfilesInfo, err := constructBlockfilesInfo(ledgerDir)
	require.NoError(t, err)
	require.True(t, blkfilesInfo.lastPersistedBlock < 59)
	require.NoError(t, recordHeightIfGreaterThanPreviousRecording(ledgerDir))
	assertRecordedHeight(t, ledgerDir, "60")
}

func assertBlocksDirOnlyFileWithGenesisBlock(t *testing.T, ledgerDir string, genesisBlock *common.Block) {
	files, err := ioutil.ReadDir(ledgerDir)
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "__backupGenesisBlockBytes", files[0].Name())
	require.Equal(t, "blockfile_000000", files[1].Name())
	blockBytes, lastOffset, numBlocks, err := scanForLastCompleteBlock(ledgerDir, 0, 0)
	require.NoError(t, err)
	t.Logf("lastOffset=%d", lastOffset)
	require.Equal(t, 1, numBlocks)
	block0, err := deserializeBlock(blockBytes)
	require.NoError(t, err)
	require.Equal(t, genesisBlock, block0)
	fileInfo, err := os.Stat(deriveBlockfilePath(ledgerDir, 0))
	require.NoError(t, err)
	require.Equal(t, fileInfo.Size(), lastOffset)
}

func assertBlockStorePostReset(t *testing.T, store *BlockStore, originallyCommittedBlocks []*common.Block) {
	bcInfo, _ := store.GetBlockchainInfo()
	t.Logf("bcInfo = %s", spew.Sdump(bcInfo))
	require.Equal(t,
		&common.BlockchainInfo{
			Height:            1,
			CurrentBlockHash:  protoutil.BlockHeaderHash(originallyCommittedBlocks[0].Header),
			PreviousBlockHash: nil,
		},
		bcInfo)

	blk, err := store.RetrieveBlockByNumber(0)
	require.NoError(t, err)
	require.Equal(t, originallyCommittedBlocks[0], blk)

	_, err = store.RetrieveBlockByNumber(1)
	require.EqualError(t, err, "no such block number [1] in index")

	err = store.AddBlock(originallyCommittedBlocks[0])
	require.EqualError(t, err, "block number should have been 1 but was 0")

	for _, b := range originallyCommittedBlocks[1:] {
		require.NoError(t, store.AddBlock(b))
	}

	for i := 0; i < len(originallyCommittedBlocks); i++ {
		blk, err := store.RetrieveBlockByNumber(uint64(i))
		require.NoError(t, err)
		require.Equal(t, originallyCommittedBlocks[i], blk)
	}
}

func assertRecordedHeight(t *testing.T, ledgerDir, expectedRecordedHt string) {
	bytes, err := ioutil.ReadFile(path.Join(ledgerDir, fileNamePreRestHt))
	require.NoError(t, err)
	require.Equal(t, expectedRecordedHt, string(bytes))
}

func testutilEstimateTotalSizeOnDisk(t *testing.T, blocks []*common.Block) int {
	size := 0
	for _, block := range blocks {
		by, _, err := serializeBlock(block)
		require.NoError(t, err)
		blockBytesSize := len(by)
		encodedLen := proto.EncodeVarint(uint64(blockBytesSize))
		size += blockBytesSize + len(encodedLen)
	}
	return size
}
