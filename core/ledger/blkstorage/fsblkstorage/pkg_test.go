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
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/protos"
)

type testEnv struct {
	conf *Conf
}

func newTestEnv(t testing.TB) *testEnv {
	conf := NewConf("/tmp/tests/ledger/blkstorage/fsblkstorage", 0)
	os.RemoveAll(conf.dbPath)
	os.RemoveAll(conf.blockfilesDir)
	return &testEnv{conf}
}

func (env *testEnv) Cleanup() {
	os.RemoveAll(env.conf.dbPath)
	os.RemoveAll(env.conf.blockfilesDir)
}

type testBlockfileMgrWrapper struct {
	t            testing.TB
	blockfileMgr *blockfileMgr
}

func newTestBlockfileWrapper(t testing.TB, env *testEnv) *testBlockfileMgrWrapper {
	return &testBlockfileMgrWrapper{t, newBlockfileMgr(env.conf)}
}

func (w *testBlockfileMgrWrapper) addBlocks(blocks []*protos.Block2) {
	for _, blk := range blocks {
		err := w.blockfileMgr.addBlock(blk)
		testutil.AssertNoError(w.t, err, "Error while adding block to blockfileMgr")
	}
}

func (w *testBlockfileMgrWrapper) testGetBlockByHash(blocks []*protos.Block2) {
	for i, block := range blocks {
		serBlock, err := protos.ConstructSerBlock2(block)
		testutil.AssertNoError(w.t, err, "Error while getting hash from block")
		b, err := w.blockfileMgr.retrieveBlockByHash(serBlock.ComputeHash())
		testutil.AssertNoError(w.t, err, fmt.Sprintf("Error while retrieving [%d]th block from blockfileMgr", i))
		testutil.AssertEquals(w.t, b, block)
	}
}

func (w *testBlockfileMgrWrapper) testGetBlockByNumber(blocks []*protos.Block2, startingNum uint64) {
	for i := 0; i < len(blocks); i++ {
		b, err := w.blockfileMgr.retrieveBlockByNumber(startingNum + uint64(i))
		testutil.AssertNoError(w.t, err, fmt.Sprintf("Error while retrieving [%d]th block from blockfileMgr", i))
		testutil.AssertEquals(w.t, b, blocks[i])
	}
}

func (w *testBlockfileMgrWrapper) close() {
	w.blockfileMgr.close()
	w.blockfileMgr.db.Close()
}
