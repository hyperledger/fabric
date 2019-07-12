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
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("fsblkstorage=debug")
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

var attrsToIndex = []blkstorage.IndexableAttr{
	blkstorage.IndexableAttrBlockHash,
	blkstorage.IndexableAttrBlockNum,
	blkstorage.IndexableAttrTxID,
	blkstorage.IndexableAttrBlockNumTranNum,
	blkstorage.IndexableAttrBlockTxID,
	blkstorage.IndexableAttrTxValidationCode,
}

func newTestEnv(t testing.TB, conf *Conf) *testEnv {
	return newTestEnvWithMetricsProvider(t, conf, &disabled.Provider{})
}

func newTestEnvWithMetricsProvider(t testing.TB, conf *Conf, metricsProvider metrics.Provider) *testEnv {
	return newTestEnvSelectiveIndexing(t, conf, attrsToIndex, metricsProvider)
}

func newTestEnvSelectiveIndexing(t testing.TB, conf *Conf, attrsToIndex []blkstorage.IndexableAttr, metricsProvider metrics.Provider) *testEnv {
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}
	return &testEnv{t, NewProvider(conf, indexConfig, metricsProvider).(*FsBlockstoreProvider)}
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
	assert.NoError(env.t, err)
	return &testBlockfileMgrWrapper{env.t, blkStore.(*fsBlockStore).fileMgr}
}

func (w *testBlockfileMgrWrapper) addBlocks(blocks []*common.Block) {
	for _, blk := range blocks {
		err := w.blockfileMgr.addBlock(blk)
		assert.NoError(w.t, err, "Error while adding block to blockfileMgr")
	}
}

func (w *testBlockfileMgrWrapper) testGetBlockByHash(blocks []*common.Block, expectedErr error) {
	for i, block := range blocks {
		hash := block.Header.Hash()
		b, err := w.blockfileMgr.retrieveBlockByHash(hash)
		if expectedErr != nil {
			assert.Error(w.t, err, expectedErr.Error())
			continue
		}
		assert.NoError(w.t, err, "Error while retrieving [%d]th block from blockfileMgr", i)
		assert.Equal(w.t, block, b)
	}
}

func (w *testBlockfileMgrWrapper) testGetBlockByNumber(blocks []*common.Block, startingNum uint64, expectedErr error) {
	for i := 0; i < len(blocks); i++ {
		b, err := w.blockfileMgr.retrieveBlockByNumber(startingNum + uint64(i))
		if expectedErr != nil {
			assert.Equal(w.t, err.Error(), expectedErr.Error())
			continue
		}
		assert.NoError(w.t, err, "Error while retrieving [%d]th block from blockfileMgr", i)
		assert.Equal(w.t, blocks[i], b)
	}
	// test getting the last block
	b, err := w.blockfileMgr.retrieveBlockByNumber(math.MaxUint64)
	iLastBlock := len(blocks) - 1
	assert.NoError(w.t, err, "Error while retrieving last block from blockfileMgr")
	assert.Equal(w.t, blocks[iLastBlock], b)
}

func (w *testBlockfileMgrWrapper) testGetBlockByTxID(blocks []*common.Block, expectedErr error) {
	for i, block := range blocks {
		for _, txEnv := range block.Data.Data {
			txID, err := utils.GetOrComputeTxIDFromEnvelope(txEnv)
			assert.NoError(w.t, err)
			b, err := w.blockfileMgr.retrieveBlockByTxID(txID)
			if expectedErr != nil {
				assert.Equal(w.t, err.Error(), expectedErr.Error())
				continue
			}
			assert.NoError(w.t, err, "Error while retrieving [%d]th block from blockfileMgr", i)
			assert.Equal(w.t, block, b)
		}
	}
}

func (w *testBlockfileMgrWrapper) testGetTransactionByTxID(txID string, expectedEnvelope []byte, expectedErr error) {
	envelope, err := w.blockfileMgr.retrieveTransactionByID(txID)
	if expectedErr != nil {
		assert.Equal(w.t, err.Error(), expectedErr.Error())
		return
	}
	actualEnvelope, err := proto.Marshal(envelope)
	assert.NoError(w.t, err)
	assert.Equal(w.t, expectedEnvelope, actualEnvelope)
}

func (w *testBlockfileMgrWrapper) close() {
	w.blockfileMgr.close()
}
