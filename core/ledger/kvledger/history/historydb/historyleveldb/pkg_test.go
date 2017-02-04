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
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/history/historydb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/spf13/viper"
)

type levelDBLockBasedHistoryEnv struct {
	t                     testing.TB
	TestDBEnv             *stateleveldb.TestVDBEnv
	TestDB                statedb.VersionedDB
	Txmgr                 txmgr.TxMgr
	TestHistoryDBProvider historydb.HistoryDBProvider
	TestHistoryDB         historydb.HistoryDB
}

func NewTestHistoryEnv(t *testing.T) *levelDBLockBasedHistoryEnv {

	//testutil.SetupCoreYAMLConfig("./../../../../../../peer")
	viper.Set("ledger.state.historyDatabase", "true")

	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgerhistorytests")
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	testDB, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	txMgr := lockbasedtxmgr.NewLockBasedTxMgr(testDB)

	testHistoryDBProvider := NewHistoryDBProvider()
	testHistoryDB, err := testHistoryDBProvider.GetDBHandle("TestHistoryDB")
	testutil.AssertNoError(t, err, "")

	return &levelDBLockBasedHistoryEnv{t, testDBEnv, testDB, txMgr, testHistoryDBProvider, testHistoryDB}

}

func (env *levelDBLockBasedHistoryEnv) cleanup() {
	defer env.Txmgr.Shutdown()
	defer env.TestDBEnv.Cleanup()

	// clean up history
	env.TestHistoryDBProvider.Close()
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
