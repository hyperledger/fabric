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
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"

	"github.com/hyperledger/fabric/protos/common"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("fsblkstorage", "debug")
	os.Exit(m.Run())
}

func testPath() string {
	if path, err := ioutil.TempDir("", "fsblkstorage-"); err != nil {
		panic(err)
	} else {
		return path
	}
}

type testEnv struct {
	t        testing.TB
	provider *FsBlockstoreProvider
}

func newTestEnv(t testing.TB, conf *Conf) *testEnv {
	attrsToIndex := []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockHash,
		blkstorage.IndexableAttrBlockNum,
		blkstorage.IndexableAttrTxID,
		blkstorage.IndexableAttrBlockNumTranNum,
		blkstorage.IndexableAttrBlockTxID,
		blkstorage.IndexableAttrTxValidationCode,
	}
	return newTestEnvSelectiveIndexing(t, conf, attrsToIndex)
}

func newTestEnvSelectiveIndexing(t testing.TB, conf *Conf, attrsToIndex []blkstorage.IndexableAttr) *testEnv {
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	return &testEnv{t, NewProvider(conf, indexConfig).(*FsBlockstoreProvider)}
}

func (env *testEnv) Cleanup() {
	env.provider.Close()
	env.removeFSPath()
}

func (env *testEnv) removeFSPath() {
	fsPath := env.provider.conf.blockStorageDir
	os.RemoveAll(fsPath)
}

type testBlockfileMgrWrapper struct {
	t            testing.TB
	blockfileMgr *blockfileMgr
}

func newTestBlockfileWrapper(env *testEnv, ledgerid string) *testBlockfileMgrWrapper {
	blkStore, err := env.provider.OpenBlockStore(ledgerid)
	testutil.AssertNoError(env.t, err, "")
	return &testBlockfileMgrWrapper{env.t, blkStore.(*fsBlockStore).fileMgr}
}

func (w *testBlockfileMgrWrapper) addBlocks(blocks []*common.Block) {
	for _, blk := range blocks {
		err := w.blockfileMgr.addBlock(blk)
		testutil.AssertNoError(w.t, err, "Error while adding block to blockfileMgr")
	}
}

func (w *testBlockfileMgrWrapper) testGetBlockByHash(blocks []*common.Block) {
	for i, block := range blocks {
		hash := block.Header.Hash()
		b, err := w.blockfileMgr.retrieveBlockByHash(hash)
		testutil.AssertNoError(w.t, err, fmt.Sprintf("Error while retrieving [%d]th block from blockfileMgr", i))
		testutil.AssertEquals(w.t, b, block)
	}
}

func (w *testBlockfileMgrWrapper) testGetBlockByNumber(blocks []*common.Block, startingNum uint64) {
	for i := 0; i < len(blocks); i++ {
		b, err := w.blockfileMgr.retrieveBlockByNumber(startingNum + uint64(i))
		testutil.AssertNoError(w.t, err, fmt.Sprintf("Error while retrieving [%d]th block from blockfileMgr", i))
		testutil.AssertEquals(w.t, b.Header, blocks[i].Header)
	}
	// test getting the last block
	b, err := w.blockfileMgr.retrieveBlockByNumber(math.MaxUint64)
	iLastBlock := len(blocks) - 1
	testutil.AssertNoError(w.t, err, "Error while retrieving last block from blockfileMgr")
	testutil.AssertEquals(w.t, b, blocks[iLastBlock])
}

func (w *testBlockfileMgrWrapper) close() {
	w.blockfileMgr.close()
}
