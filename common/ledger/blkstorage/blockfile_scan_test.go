/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/stretchr/testify/require"
)

func TestBlockFileScanSmallTxOnly(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	blocks := []*common.Block{gb}
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.close()

	filePath := deriveBlockfilePath(env.provider.conf.getLedgerBlockDir(ledgerid), 0)
	_, fileSize, err := fileutil.FileExists(filePath)
	require.NoError(t, err)

	lastBlockBytes, endOffsetLastBlock, numBlocks, err := scanForLastCompleteBlock(env.provider.conf.getLedgerBlockDir(ledgerid), 0, 0)
	require.NoError(t, err)
	require.Equal(t, len(blocks), numBlocks)
	require.Equal(t, fileSize, endOffsetLastBlock)

	expectedLastBlockBytes, _, err := serializeBlock(blocks[len(blocks)-1])
	require.NoError(t, err)
	require.Equal(t, expectedLastBlockBytes, lastBlockBytes)
}

func TestBlockFileScanSmallTxLastTxIncomplete(t *testing.T) {
	env := newTestEnv(t, NewConf(t.TempDir(), 0))
	defer env.Cleanup()
	ledgerid := "testLedger"
	blkfileMgrWrapper := newTestBlockfileWrapper(env, ledgerid)
	bg, gb := testutil.NewBlockGenerator(t, ledgerid, false)
	blocks := []*common.Block{gb}
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blocks = append(blocks, bg.NextTestBlock(0, 0))
	blkfileMgrWrapper.addBlocks(blocks)
	blkfileMgrWrapper.close()

	filePath := deriveBlockfilePath(env.provider.conf.getLedgerBlockDir(ledgerid), 0)
	_, fileSize, err := fileutil.FileExists(filePath)
	require.NoError(t, err)

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o660)
	require.NoError(t, err)
	defer file.Close()
	err = file.Truncate(fileSize - 1)
	require.NoError(t, err)

	lastBlockBytes, _, numBlocks, err := scanForLastCompleteBlock(env.provider.conf.getLedgerBlockDir(ledgerid), 0, 0)
	require.NoError(t, err)
	require.Equal(t, len(blocks)-1, numBlocks)

	expectedLastBlockBytes, _, err := serializeBlock(blocks[len(blocks)-2])
	require.NoError(t, err)
	require.Equal(t, expectedLastBlockBytes, lastBlockBytes)
}
