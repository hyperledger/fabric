/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/stretchr/testify/require"
)

func TestConstructBlockfilesInfo(t *testing.T) {
	ledgerid := "testLedger"
	conf := NewConf(testPath(), 0)
	blkStoreDir := conf.getLedgerBlockDir(ledgerid)
	env := newTestEnv(t, conf)
	util.CreateDirIfMissing(blkStoreDir)
	defer env.Cleanup()

	// constructBlockfilesInfo on an empty block folder should return blockfileInfo with noBlockFiles: true
	blkfilesInfo, err := constructBlockfilesInfo(blkStoreDir)
	require.NoError(t, err)
	require.Equal(t,
		&blockfilesInfo{
			noBlockFiles:       true,
			lastPersistedBlock: 0,
			latestFileSize:     0,
			latestFileNumber:   0,
		},
		blkfilesInfo,
	)

	w := newTestBlockfileWrapper(env, ledgerid)
	defer w.close()
	blockfileMgr := w.blockfileMgr
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)

	// Add a few blocks and verify that blockfilesInfo derived from filesystem should be same as from the blockfile manager
	blockfileMgr.addBlock(gb)
	for _, blk := range bg.NextTestBlocks(3) {
		blockfileMgr.addBlock(blk)
	}
	checkBlockfilesInfoFromFS(t, blkStoreDir, blockfileMgr.blockfilesInfo)

	// Move the chain to new file and check blockfilesInfo derived from file system
	blockfileMgr.moveToNextFile()
	checkBlockfilesInfoFromFS(t, blkStoreDir, blockfileMgr.blockfilesInfo)

	// Add a few blocks that would go to new file and verify that blockfilesInfo derived from filesystem should be same as from the blockfile manager
	for _, blk := range bg.NextTestBlocks(3) {
		blockfileMgr.addBlock(blk)
	}
	checkBlockfilesInfoFromFS(t, blkStoreDir, blockfileMgr.blockfilesInfo)

	// Write a partial block (to simulate a crash) and verify that blockfilesInfo derived from filesystem should be same as from the blockfile manager
	lastTestBlk := bg.NextTestBlocks(1)[0]
	blockBytes, _, err := serializeBlock(lastTestBlk)
	require.NoError(t, err)
	partialByte := append(proto.EncodeVarint(uint64(len(blockBytes))), blockBytes[len(blockBytes)/2:]...)
	blockfileMgr.currentFileWriter.append(partialByte, true)
	checkBlockfilesInfoFromFS(t, blkStoreDir, blockfileMgr.blockfilesInfo)

	// Close the block storage, drop the index and restart and verify
	blkfilesInfoBeforeClose := blockfileMgr.blockfilesInfo
	w.close()
	env.provider.Close()
	indexFolder := conf.getIndexDir()
	require.NoError(t, os.RemoveAll(indexFolder))

	env = newTestEnv(t, conf)
	w = newTestBlockfileWrapper(env, ledgerid)
	blockfileMgr = w.blockfileMgr
	require.Equal(t, blkfilesInfoBeforeClose, blockfileMgr.blockfilesInfo)

	lastBlkIndexed, err := blockfileMgr.index.getLastBlockIndexed()
	require.NoError(t, err)
	require.Equal(t, uint64(6), lastBlkIndexed)

	// Add the last block again after start and check blockfilesInfo again
	require.NoError(t, blockfileMgr.addBlock(lastTestBlk))
	checkBlockfilesInfoFromFS(t, blkStoreDir, blockfileMgr.blockfilesInfo)
}

func TestBinarySearchBlockFileNum(t *testing.T) {
	blockStoreRootDir := testPath()
	blocks := testutil.ConstructTestBlocks(t, 100)
	maxFileSie := int(0.1 * float64(testutilEstimateTotalSizeOnDisk(t, blocks)))
	env := newTestEnv(t, NewConf(blockStoreRootDir, maxFileSie))
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(env, "testLedger")
	blkfileMgr := blkfileMgrWrapper.blockfileMgr

	blkfileMgrWrapper.addBlocks(blocks)

	ledgerDir := (&Conf{blockStorageDir: blockStoreRootDir}).getLedgerBlockDir("testLedger")
	files, err := ioutil.ReadDir(ledgerDir)
	require.NoError(t, err)
	require.Len(t, files, 11)

	for i := uint64(0); i < 100; i++ {
		fileNum, err := binarySearchFileNumForBlock(ledgerDir, i)
		require.NoError(t, err)
		locFromIndex, err := blkfileMgr.index.getBlockLocByBlockNum(i)
		require.NoError(t, err)
		expectedFileNum := locFromIndex.fileSuffixNum
		require.Equal(t, expectedFileNum, fileNum)
	}
}

func checkBlockfilesInfoFromFS(t *testing.T, blkStoreDir string, expected *blockfilesInfo) {
	blkfilesInfo, err := constructBlockfilesInfo(blkStoreDir)
	require.NoError(t, err)
	require.Equal(t, expected, blkfilesInfo)
}
