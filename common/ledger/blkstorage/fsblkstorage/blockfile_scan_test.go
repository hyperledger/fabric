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
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/ledger/util"
	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/protos/common"
)

func TestBlockFileScanSmallTxOnly(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
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
	_, fileSize, err := util.FileExists(filePath)
	assert.NoError(t, err)

	lastBlockBytes, endOffsetLastBlock, numBlocks, err := scanForLastCompleteBlock(env.provider.conf.getLedgerBlockDir(ledgerid), 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, len(blocks), numBlocks)
	assert.Equal(t, fileSize, endOffsetLastBlock)

	expectedLastBlockBytes, _, err := serializeBlock(blocks[len(blocks)-1])
	assert.Equal(t, expectedLastBlockBytes, lastBlockBytes)
}

func TestBlockFileScanSmallTxLastTxIncomplete(t *testing.T) {
	env := newTestEnv(t, NewConf(testPath(), 0))
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
	_, fileSize, err := util.FileExists(filePath)
	assert.NoError(t, err)

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	defer file.Close()
	assert.NoError(t, err)
	err = file.Truncate(fileSize - 1)
	assert.NoError(t, err)

	lastBlockBytes, _, numBlocks, err := scanForLastCompleteBlock(env.provider.conf.getLedgerBlockDir(ledgerid), 0, 0)
	assert.NoError(t, err)
	assert.Equal(t, len(blocks)-1, numBlocks)

	expectedLastBlockBytes, _, err := serializeBlock(blocks[len(blocks)-2])
	assert.Equal(t, expectedLastBlockBytes, lastBlockBytes)
}
