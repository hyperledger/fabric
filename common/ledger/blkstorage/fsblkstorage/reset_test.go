/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package fsblkstorage

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestResetToGenesisBlkSingleBlkFile(t *testing.T) {
	blockStoreRootDir := "/tmp/testBlockStoreReset"
	assert.NoError(t, os.RemoveAll(blockStoreRootDir))
	env := newTestEnv(t, NewConf(blockStoreRootDir, 0))
	defer env.Cleanup()
	provider := env.provider
	store, err := provider.CreateBlockStore("ledger1")
	assert.NoError(t, err)

	// Add 50 blocks and shutdown blockstore store
	blocks := testutil.ConstructTestBlocks(t, 50)
	for _, b := range blocks {
		assert.NoError(t, store.AddBlock(b))
	}
	store.Shutdown()
	provider.Close()

	ledgerDir := (&Conf{blockStorageDir: blockStoreRootDir}).getLedgerBlockDir("ledger1")

	_, lastOffsetOriginal, numBlocksOriginal, err := scanForLastCompleteBlock(ledgerDir, 0, 0)
	t.Logf("lastOffsetOriginal=%d", lastOffsetOriginal)
	assert.NoError(t, err)
	assert.Equal(t, 50, numBlocksOriginal)
	fileInfo, err := os.Stat(deriveBlockfilePath(ledgerDir, 0))
	assert.NoError(t, err)
	assert.Equal(t, fileInfo.Size(), lastOffsetOriginal)

	resetToGenesisBlk(ledgerDir)
	assertBlocksDirOnlyFileWithGenesisBlock(t, ledgerDir, blocks[0])
}

func TestResetToGenesisBlkMultipleBlkFiles(t *testing.T) {
	blockStoreRootDir := "/tmp/testBlockStoreReset"
	assert.NoError(t, os.RemoveAll(blockStoreRootDir))
	blocks := testutil.ConstructTestBlocks(t, 20) // 20 blocks persisted in ~5 block files
	blocksPerFile := 20 / 5
	env := newTestEnv(t, NewConf(blockStoreRootDir, 0))
	defer env.Cleanup()
	provider := env.provider
	store, err := provider.CreateBlockStore("ledger1")
	assert.NoError(t, err)
	for i, b := range blocks {
		assert.NoError(t, store.AddBlock(b))
		if i != 0 && i%blocksPerFile == 0 {
			// block ranges in files [(0, 4):file0, (5,8):file1, (9,12):file2, (13, 16):file3, (17,19):file4]
			store.(*fsBlockStore).fileMgr.moveToNextFile()
		}
	}
	store.Shutdown()
	provider.Close()

	ledgerDir := (&Conf{blockStorageDir: blockStoreRootDir}).getLedgerBlockDir("ledger1")
	files, err := ioutil.ReadDir(ledgerDir)
	assert.Len(t, files, 5)
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
	store1, err := provider.OpenBlockStore("ledger1")
	assert.NoError(t, err)
	store2, _ := provider.CreateBlockStore("ledger2")

	for _, b := range blocks1 {
		store1.AddBlock(b)
	}

	for _, b := range blocks2 {
		store2.AddBlock(b)
	}

	store1.Shutdown()
	store2.Shutdown()
	provider.Close()

	assert.NoError(t, ResetBlockStore(blockStoreRootDir))
	h, err := LoadPreResetHeight(blockStoreRootDir)
	assert.Equal(t,
		map[string]uint64{
			"ledger1": 20,
			"ledger2": 40,
		},
		h,
	)

	env = newTestEnv(t, NewConf(blockStoreRootDir, maxFileSie))
	provider = env.provider
	store1, _ = provider.OpenBlockStore("ledger1")
	store2, _ = provider.CreateBlockStore("ledger2")
	assertBlockStorePostReset(t, store1, blocks1)
	assertBlockStorePostReset(t, store2, blocks2)

	assert.NoError(t, ClearPreResetHeight(blockStoreRootDir))
	h, err = LoadPreResetHeight(blockStoreRootDir)
	assert.Equal(t,
		map[string]uint64{},
		h,
	)
}

func TestRecordHeight(t *testing.T) {
	blockStoreRootDir := "/tmp/testBlockStoreReset"
	assert.NoError(t, os.RemoveAll(blockStoreRootDir))
	env := newTestEnv(t, NewConf(blockStoreRootDir, 0))
	defer env.Cleanup()
	provider := env.provider
	store, err := provider.CreateBlockStore("ledger1")
	assert.NoError(t, err)

	blocks := testutil.ConstructTestBlocks(t, 60)

	// Add 50 blocks, record, and assert the recording of the current height
	for _, b := range blocks[:50] {
		assert.NoError(t, store.AddBlock(b))
	}
	ledgerDir := (&Conf{blockStorageDir: blockStoreRootDir}).getLedgerBlockDir("ledger1")
	assert.NoError(t, recordHeightIfGreaterThanPreviousRecording(ledgerDir))
	assertRecordedHeight(t, ledgerDir, "50")

	// Add 10 more blocks, record again and assert that the previous recorded info is overwritten with new current height
	for _, b := range blocks[50:] {
		assert.NoError(t, store.AddBlock(b))
	}
	assert.NoError(t, recordHeightIfGreaterThanPreviousRecording(ledgerDir))
	assertRecordedHeight(t, ledgerDir, "60")

	// truncate the most recent block file to half
	// record again and assert that the previous recorded info is NOT overwritten with new current height
	// because the current height is less than the previously recorded height
	lastFileNum, err := retrieveLastFileSuffix(ledgerDir)
	assert.NoError(t, err)
	lastFile := deriveBlockfilePath(ledgerDir, lastFileNum)
	fileInfo, err := os.Stat(lastFile)
	assert.NoError(t, err)
	assert.NoError(t, os.Truncate(lastFile, fileInfo.Size()/2))
	checkpointInfo, err := constructCheckpointInfoFromBlockFiles(ledgerDir)
	assert.NoError(t, err)
	assert.True(t, checkpointInfo.lastBlockNumber < 59)
	assert.NoError(t, recordHeightIfGreaterThanPreviousRecording(ledgerDir))
	assertRecordedHeight(t, ledgerDir, "60")
}

func assertBlocksDirOnlyFileWithGenesisBlock(t *testing.T, ledgerDir string, genesisBlock *common.Block) {
	files, err := ioutil.ReadDir(ledgerDir)
	assert.Len(t, files, 2)
	assert.Equal(t, "__backupGenesisBlockBytes", files[0].Name())
	assert.Equal(t, "blockfile_000000", files[1].Name())
	blockBytes, lastOffset, numBlocks, err := scanForLastCompleteBlock(ledgerDir, 0, 0)
	assert.NoError(t, err)
	t.Logf("lastOffset=%d", lastOffset)
	assert.Equal(t, 1, numBlocks)
	block0, err := deserializeBlock(blockBytes)
	assert.NoError(t, err)
	assert.Equal(t, genesisBlock, block0)
	fileInfo, err := os.Stat(deriveBlockfilePath(ledgerDir, 0))
	assert.NoError(t, err)
	assert.Equal(t, fileInfo.Size(), lastOffset)
}

func assertBlockStorePostReset(t *testing.T, store blkstorage.BlockStore, originallyCommittedBlocks []*common.Block) {
	bcInfo, _ := store.GetBlockchainInfo()
	t.Logf("bcInfo = %s", spew.Sdump(bcInfo))
	assert.Equal(t,
		&common.BlockchainInfo{
			Height:            1,
			CurrentBlockHash:  originallyCommittedBlocks[0].Header.Hash(),
			PreviousBlockHash: nil,
		},
		bcInfo)

	blk, err := store.RetrieveBlockByNumber(0)
	assert.NoError(t, err)
	assert.Equal(t, originallyCommittedBlocks[0], blk)

	blk, err = store.RetrieveBlockByNumber(1)
	assert.Error(t, err)
	assert.Equal(t, err, blkstorage.ErrNotFoundInIndex)

	err = store.AddBlock(originallyCommittedBlocks[0])
	assert.EqualError(t, err, "block number should have been 1 but was 0")

	for _, b := range originallyCommittedBlocks[1:] {
		assert.NoError(t, store.AddBlock(b))
	}

	for i := 0; i < len(originallyCommittedBlocks); i++ {
		blk, err := store.RetrieveBlockByNumber(uint64(i))
		assert.NoError(t, err)
		assert.Equal(t, originallyCommittedBlocks[i], blk)
	}
}

func assertRecordedHeight(t *testing.T, ledgerDir, expectedRecordedHt string) {
	bytes, err := ioutil.ReadFile(path.Join(ledgerDir, fileNamePreRestHt))
	assert.NoError(t, err)
	assert.Equal(t, expectedRecordedHt, string(bytes))
}

func testutilEstimateTotalSizeOnDisk(t *testing.T, blocks []*common.Block) int {
	size := 0
	for _, block := range blocks {
		by, _, err := serializeBlock(block)
		assert.NoError(t, err)
		blockBytesSize := len(by)
		encodedLen := proto.EncodeVarint(uint64(blockBytesSize))
		size += blockBytesSize + len(encodedLen)
	}
	return size
}
