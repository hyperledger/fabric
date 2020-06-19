/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"sync"

	"github.com/hyperledger/fabric/common/ledger"
)

// blocksItr - an iterator for iterating over a sequence of blocks
type blocksItr struct {
	mgr                  *blockfileMgr
	maxBlockNumAvailable uint64
	blockNumToRetrieve   uint64
	stream               *blockStream
	closeMarker          bool
	closeMarkerLock      *sync.Mutex
}

func newBlockItr(mgr *blockfileMgr, startBlockNum uint64) *blocksItr {
	mgr.blkfilesInfoCond.L.Lock()
	defer mgr.blkfilesInfoCond.L.Unlock()
	return &blocksItr{mgr, mgr.blockfilesInfo.lastPersistedBlock, startBlockNum, nil, false, &sync.Mutex{}}
}

func (itr *blocksItr) waitForBlock(blockNum uint64) uint64 {
	itr.mgr.blkfilesInfoCond.L.Lock()
	defer itr.mgr.blkfilesInfoCond.L.Unlock()
	for itr.mgr.blockfilesInfo.lastPersistedBlock < blockNum && !itr.shouldClose() {
		logger.Debugf("Going to wait for newer blocks. maxAvailaBlockNumber=[%d], waitForBlockNum=[%d]",
			itr.mgr.blockfilesInfo.lastPersistedBlock, blockNum)
		itr.mgr.blkfilesInfoCond.Wait()
		logger.Debugf("Came out of wait. maxAvailaBlockNumber=[%d]", itr.mgr.blockfilesInfo.lastPersistedBlock)
	}
	return itr.mgr.blockfilesInfo.lastPersistedBlock
}

func (itr *blocksItr) initStream() error {
	var lp *fileLocPointer
	var err error
	if lp, err = itr.mgr.index.getBlockLocByBlockNum(itr.blockNumToRetrieve); err != nil {
		return err
	}
	if itr.stream, err = newBlockStream(itr.mgr.rootDir, lp.fileSuffixNum, int64(lp.offset), -1); err != nil {
		return err
	}
	return nil
}

func (itr *blocksItr) shouldClose() bool {
	itr.closeMarkerLock.Lock()
	defer itr.closeMarkerLock.Unlock()
	return itr.closeMarker
}

// Next moves the cursor to next block and returns true iff the iterator is not exhausted
func (itr *blocksItr) Next() (ledger.QueryResult, error) {
	if itr.maxBlockNumAvailable < itr.blockNumToRetrieve {
		itr.maxBlockNumAvailable = itr.waitForBlock(itr.blockNumToRetrieve)
	}
	itr.closeMarkerLock.Lock()
	defer itr.closeMarkerLock.Unlock()
	if itr.closeMarker {
		return nil, nil
	}
	if itr.stream == nil {
		logger.Debugf("Initializing block stream for iterator. itr.maxBlockNumAvailable=%d", itr.maxBlockNumAvailable)
		if err := itr.initStream(); err != nil {
			return nil, err
		}
	}
	nextBlockBytes, err := itr.stream.nextBlockBytes()
	if err != nil {
		return nil, err
	}
	itr.blockNumToRetrieve++
	return deserializeBlock(nextBlockBytes)
}

// Close releases any resources held by the iterator
func (itr *blocksItr) Close() {
	itr.mgr.blkfilesInfoCond.L.Lock()
	defer itr.mgr.blkfilesInfoCond.L.Unlock()
	itr.closeMarkerLock.Lock()
	defer itr.closeMarkerLock.Unlock()
	itr.closeMarker = true
	itr.mgr.blkfilesInfoCond.Broadcast()
	if itr.stream != nil {
		itr.stream.close()
	}
}
