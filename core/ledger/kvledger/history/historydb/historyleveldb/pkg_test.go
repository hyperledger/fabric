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

package historyleveldb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/spf13/viper"
)

/////// levelDBLockBasedHistoryEnv //////

type levelDBLockBasedHistoryEnv struct {
	t                     testing.TB
	testBlockStorageEnv   *testBlockStoreEnv
	testDBEnv             *stateleveldb.TestVDBEnv
	txmgr                 txmgr.TxMgr
	testHistoryDBProvider historydb.HistoryDBProvider
	testHistoryDB         historydb.HistoryDB
}

func NewTestHistoryEnv(t *testing.T) *levelDBLockBasedHistoryEnv {

	viper.Set("ledger.history.enableHistoryDatabase", "true")

	blockStorageTestEnv := newBlockStorageTestEnv(t)

	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	testDB, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	txMgr := lockbasedtxmgr.NewLockBasedTxMgr(testDB)

	testHistoryDBProvider := NewHistoryDBProvider()
	testHistoryDB, err := testHistoryDBProvider.GetDBHandle("TestHistoryDB")
	testutil.AssertNoError(t, err, "")

	return &levelDBLockBasedHistoryEnv{t, blockStorageTestEnv, testDBEnv, txMgr, testHistoryDBProvider, testHistoryDB}
}

func (env *levelDBLockBasedHistoryEnv) cleanup() {
	defer env.txmgr.Shutdown()
	defer env.testDBEnv.Cleanup()
	defer env.testBlockStorageEnv.cleanup()

	// clean up history
	env.testHistoryDBProvider.Close()
	removeDBPath(env.t, "Cleanup")
}

func removeDBPath(t testing.TB, caller string) {
	dbPath := ledgerconfig.GetHistoryLevelDBPath()
	if err := os.RemoveAll(dbPath); err != nil {
		t.Fatalf("Err: %s", err)
		t.FailNow()
	}
	logger.Debugf("Removed folder [%s] for history test environment for %s", dbPath, caller)
}

/////// testBlockStoreEnv//////

type testBlockStoreEnv struct {
	t               testing.TB
	provider        *fsblkstorage.FsBlockstoreProvider
	blockStorageDir string
}

func newBlockStorageTestEnv(t testing.TB) *testBlockStoreEnv {

	testPath, err := ioutil.TempDir("", "historyleveldb-")
	if err != nil {
		panic(err)
	}
	conf := fsblkstorage.NewConf(testPath, 0)

	attrsToIndex := []blkstorage.IndexableAttr{
		blkstorage.IndexableAttrBlockHash,
		blkstorage.IndexableAttrBlockNum,
		blkstorage.IndexableAttrTxID,
		blkstorage.IndexableAttrBlockNumTranNum,
	}
	indexConfig := &blkstorage.IndexConfig{AttrsToIndex: attrsToIndex}

	blockStorageProvider := fsblkstorage.NewProvider(conf, indexConfig).(*fsblkstorage.FsBlockstoreProvider)

	return &testBlockStoreEnv{t, blockStorageProvider, testPath}
}

func (env *testBlockStoreEnv) cleanup() {
	env.provider.Close()
	env.removeFSPath()
}

func (env *testBlockStoreEnv) removeFSPath() {
	fsPath := env.blockStorageDir
	os.RemoveAll(fsPath)
}
