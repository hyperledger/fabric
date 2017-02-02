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

package commontests

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/spf13/viper"
)

const (
	testFilesystemPath = "/tmp/fabric/ledgertests/kvledger/txmgmt/txmgr/commontests"
)

type testEnv interface {
	init(t *testing.T)
	getName() string
	getTxMgr() txmgr.TxMgr
	getVDB() statedb.VersionedDB
	cleanup()
}

var testEnvs = []testEnv{}

func init() {
	//call a helper method to load the core.yaml so that we can detect whether couch is configured
	ledgertestutil.SetupCoreYAMLConfig("./../../../../../../peer")

	//Only run the tests if CouchDB is explitily enabled in the code,
	//otherwise CouchDB may not be installed and all the tests would fail
	if ledgerconfig.IsCouchDBEnabled() == true {
		testEnvs = []testEnv{&levelDBLockBasedEnv{}, &couchDBLockBasedEnv{}}
	} else {
		testEnvs = []testEnv{&levelDBLockBasedEnv{}}
	}

}

///////////// LevelDB Environment //////////////

type levelDBLockBasedEnv struct {
	testDBEnv *stateleveldb.TestVDBEnv
	testDB    statedb.VersionedDB
	txmgr     txmgr.TxMgr
}

func (env *levelDBLockBasedEnv) getName() string {
	return "levelDB_LockBasedTxMgr"
}

func (env *levelDBLockBasedEnv) init(t *testing.T) {
	viper.Set("peer.fileSystemPath", testFilesystemPath)
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	testDB, err := testDBEnv.DBProvider.GetDBHandle("TestDB")
	testutil.AssertNoError(t, err, "")

	txMgr := lockbasedtxmgr.NewLockBasedTxMgr(testDB)
	env.testDBEnv = testDBEnv
	env.testDB = testDB
	env.txmgr = txMgr
}

func (env *levelDBLockBasedEnv) getTxMgr() txmgr.TxMgr {
	return env.txmgr
}

func (env *levelDBLockBasedEnv) getVDB() statedb.VersionedDB {
	return env.testDB
}

func (env *levelDBLockBasedEnv) cleanup() {
	defer env.txmgr.Shutdown()
	defer env.testDBEnv.Cleanup()
}

///////////// CouchDB Environment //////////////

var couchTestChainID = "TxmgrTestDB"

type couchDBLockBasedEnv struct {
	testDBEnv *statecouchdb.TestVDBEnv
	testDB    statedb.VersionedDB
	txmgr     txmgr.TxMgr
}

func (env *couchDBLockBasedEnv) getName() string {
	return "couchDB_LockBasedTxMgr"
}

func (env *couchDBLockBasedEnv) init(t *testing.T) {
	viper.Set("peer.fileSystemPath", testFilesystemPath)
	viper.Set("ledger.state.couchDBConfig.couchDBAddress", "127.0.0.1:5984")
	testDBEnv := statecouchdb.NewTestVDBEnv(t)
	testDB, err := testDBEnv.DBProvider.GetDBHandle(couchTestChainID)
	testutil.AssertNoError(t, err, "")

	txMgr := lockbasedtxmgr.NewLockBasedTxMgr(testDB)
	env.testDBEnv = testDBEnv
	env.testDB = testDB
	env.txmgr = txMgr
}

func (env *couchDBLockBasedEnv) getTxMgr() txmgr.TxMgr {
	return env.txmgr
}

func (env *couchDBLockBasedEnv) getVDB() statedb.VersionedDB {
	return env.testDB
}

func (env *couchDBLockBasedEnv) cleanup() {
	defer env.txmgr.Shutdown()
	defer env.testDBEnv.Cleanup(couchTestChainID)
}

//////////// txMgrTestHelper /////////////

type txMgrTestHelper struct {
	t     *testing.T
	txMgr txmgr.TxMgr
	bg    *testutil.BlockGenerator
}

func newTxMgrTestHelper(t *testing.T, txMgr txmgr.TxMgr) *txMgrTestHelper {
	return &txMgrTestHelper{t, txMgr, testutil.NewBlockGenerator(t)}
}

func (h *txMgrTestHelper) validateAndCommitRWSet(txRWSet []byte) {
	block := h.bg.NextBlock([][]byte{txRWSet}, false)
	err := h.txMgr.ValidateAndPrepare(block, true)
	testutil.AssertNoError(h.t, err, "")
	txsFltr := util.NewFilterBitArrayFromBytes(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxNum := 0
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsSet(uint(i)) {
			invalidTxNum++
		}
	}
	testutil.AssertEquals(h.t, invalidTxNum, 0)
	err = h.txMgr.Commit()
	testutil.AssertNoError(h.t, err, "")
}

func (h *txMgrTestHelper) checkRWsetInvalid(txRWSet []byte) {
	block := h.bg.NextBlock([][]byte{txRWSet}, false)
	err := h.txMgr.ValidateAndPrepare(block, true)
	testutil.AssertNoError(h.t, err, "")
	txsFltr := util.NewFilterBitArrayFromBytes(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxNum := 0
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsSet(uint(i)) {
			invalidTxNum++
		}
	}
	testutil.AssertEquals(h.t, invalidTxNum, 1)
}
