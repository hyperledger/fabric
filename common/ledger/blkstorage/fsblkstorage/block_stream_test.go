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
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestBlockfileStream(t *testing.T) {
	testBlockfileStream(t, 0)
	testBlockfileStream(t, 1)
	testBlockfileStream(t, 10)
}

func testBlockfileStream(t *testing.T, numBlocks int) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	ledgerid := "testledger"
	w := newTestBlockfileWrapper(env, ledgerid)
	blocks := testutil.ConstructTestBlocks(t, numBlocks)
	w.addBlocks(blocks)
	w.close()

	s, err := newBlockfileStream(w.blockfileMgr.rootDir, 0, 0)
	defer s.close()
	assert.NoError(t, err, "Error in constructing blockfile stream")

	blockCount := 0
	for {
		blockBytes, err := s.nextBlockBytes()
		assert.NoError(t, err, "Error in getting next block")
		if blockBytes == nil {
			break
		}
		blockCount++
	}
	// After the stream has been exhausted, both blockBytes and err should be nil
	blockBytes, err := s.nextBlockBytes()
	assert.Nil(t, blockBytes)
	assert.NoError(t, err, "Error in getting next block after exhausting the file")
	assert.Equal(t, numBlocks, blockCount)
}

func TestBlockFileStreamUnexpectedEOF(t *testing.T) {
	partialBlockBytes := []byte{}
	dummyBlockBytes := testutil.ConstructRandomBytes(t, 100)
	lenBytes := proto.EncodeVarint(uint64(len(dummyBlockBytes)))
	partialBlockBytes = append(partialBlockBytes, lenBytes...)
	partialBlockBytes = append(partialBlockBytes, dummyBlockBytes...)
	testBlockFileStreamUnexpectedEOF(t, 10, partialBlockBytes[:1])
	testBlockFileStreamUnexpectedEOF(t, 10, partialBlockBytes[:2])
	testBlockFileStreamUnexpectedEOF(t, 10, partialBlockBytes[:5])
	testBlockFileStreamUnexpectedEOF(t, 10, partialBlockBytes[:20])
}

func testBlockFileStreamUnexpectedEOF(t *testing.T, numBlocks int, partialBlockBytes []byte) {
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	w := newTestBlockfileWrapper(env, "testLedger")
	blockfileMgr := w.blockfileMgr
	blocks := testutil.ConstructTestBlocks(t, numBlocks)
	w.addBlocks(blocks)
	blockfileMgr.currentFileWriter.append(partialBlockBytes, true)
	w.close()
	s, err := newBlockfileStream(blockfileMgr.rootDir, 0, 0)
	defer s.close()
	assert.NoError(t, err, "Error in constructing blockfile stream")

	for i := 0; i < numBlocks; i++ {
		blockBytes, err := s.nextBlockBytes()
		assert.NotNil(t, blockBytes)
		assert.NoError(t, err, "Error in getting next block")
	}
	blockBytes, err := s.nextBlockBytes()
	assert.Nil(t, blockBytes)
	assert.Exactly(t, ErrUnexpectedEndOfBlockfile, err)
}

func TestBlockStream(t *testing.T) {
	testBlockStream(t, 1)
	testBlockStream(t, 2)
	testBlockStream(t, 10)
}

func testBlockStream(t *testing.T, numFiles int) {
	ledgerID := "testLedger"
	env := newTestEnv(t, NewConf(testPath(), 0))
	defer env.Cleanup()
	w := newTestBlockfileWrapper(env, ledgerID)
	defer w.close()
	blockfileMgr := w.blockfileMgr

	numBlocksInEachFile := 10
	bg, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	w.addBlocks([]*common.Block{gb})
	for i := 0; i < numFiles; i++ {
		numBlocks := numBlocksInEachFile
		if i == 0 {
			// genesis block already added so adding one less block
			numBlocks = numBlocksInEachFile - 1
		}
		blocks := bg.NextTestBlocks(numBlocks)
		w.addBlocks(blocks)
		blockfileMgr.moveToNextFile()
	}
	s, err := newBlockStream(blockfileMgr.rootDir, 0, 0, numFiles-1)
	defer s.close()
	assert.NoError(t, err, "Error in constructing new block stream")
	blockCount := 0
	for {
		blockBytes, err := s.nextBlockBytes()
		assert.NoError(t, err, "Error in getting next block")
		if blockBytes == nil {
			break
		}
		blockCount++
	}
	// After the stream has been exhausted, both blockBytes and err should be nil
	blockBytes, err := s.nextBlockBytes()
	assert.Nil(t, blockBytes)
	assert.NoError(t, err, "Error in getting next block after exhausting the file")
	assert.Equal(t, numFiles*numBlocksInEachFile, blockCount)
}
