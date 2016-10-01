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

	"github.com/hyperledger/fabric/core/ledgernext/testutil"
)

type noopIndex struct {
}

func (i *noopIndex) getLastBlockIndexed() (uint64, error) {
	return 0, nil
}
func (i *noopIndex) indexBlock(blockIdxInfo *blockIdxInfo) error {
	return nil
}
func (i *noopIndex) getBlockLocByHash(blockHash []byte) (*fileLocPointer, error) {
	return nil, nil
}
func (i *noopIndex) getBlockLocByBlockNum(blockNum uint64) (*fileLocPointer, error) {
	return nil, nil
}
func (i *noopIndex) getTxLoc(txID string) (*fileLocPointer, error) {
	return nil, nil
}

func TestBlockIndexSync(t *testing.T) {
	testBlockIndexSync(t, 10, 5, false)
	testBlockIndexSync(t, 10, 5, true)
	testBlockIndexSync(t, 10, 0, true)
	testBlockIndexSync(t, 10, 10, true)
}

func testBlockIndexSync(t *testing.T, numBlocks int, numBlocksToIndex int, syncByRestart bool) {
	env := newTestEnv(t)
	defer env.Cleanup()
	blkfileMgrWrapper := newTestBlockfileWrapper(t, env)
	defer blkfileMgrWrapper.close()
	blkfileMgr := blkfileMgrWrapper.blockfileMgr
	origIndex := blkfileMgr.index
	// construct blocks for testing
	blocks := testutil.ConstructTestBlocks(t, numBlocks)
	// add a few blocks
	blkfileMgrWrapper.addBlocks(blocks[:numBlocksToIndex])

	// Plug-in a noop index and add remaining blocks
	blkfileMgr.index = &noopIndex{}
	blkfileMgrWrapper.addBlocks(blocks[numBlocksToIndex:])

	// Plug-in back the original index
	blkfileMgr.index = origIndex
	// The first set of blocks should be present in the orginal index
	for i := 1; i <= numBlocksToIndex; i++ {
		block, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
		testutil.AssertNoError(t, err, fmt.Sprintf("block [%d] should have been present in the index", i))
		testutil.AssertEquals(t, block, blocks[i-1])
	}

	// The last set of blocks should not be present in the original index
	for i := numBlocksToIndex + 1; i <= numBlocks; i++ {
		_, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
		testutil.AssertSame(t, err, ErrNotFoundInIndex)
	}

	// perform index sync
	if syncByRestart {
		blkfileMgrWrapper.close()
		blkfileMgrWrapper = newTestBlockfileWrapper(t, env)
		defer blkfileMgrWrapper.close()
		blkfileMgr = blkfileMgrWrapper.blockfileMgr
	} else {
		blkfileMgr.syncIndex()
	}

	// Now, last set of blocks should also be present in original index
	for i := numBlocksToIndex + 1; i <= numBlocks; i++ {
		block, err := blkfileMgr.retrieveBlockByNumber(uint64(i))
		testutil.AssertNoError(t, err, fmt.Sprintf("block [%d] should have been present in the index", i))
		testutil.AssertEquals(t, block, blocks[i-1])
	}
}
