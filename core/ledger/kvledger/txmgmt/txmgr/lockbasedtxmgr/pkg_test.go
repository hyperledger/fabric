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

package lockbasedtxmgr

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/spf13/viper"
)

const (
	testFilesystemPath = "/tmp/fabric/ledgertests/kvledger/txmgmt/txmgr/lockbasedtxmgr"
)

type testEnv interface {
	init(t *testing.T, testLedgerID string)
	getName() string
	getTxMgr() txmgr.TxMgr
	getVDB() statedb.VersionedDB
	cleanup()
}

// Tests will be run against each environment in this array
// For example, to skip CouchDB tests, remove &couchDBLockBasedEnv{}
var testEnvs = []testEnv{&levelDBLockBasedEnv{}, &couchDBLockBasedEnv{}}

///////////// LevelDB Environment //////////////

const levelDBtestEnvName = "levelDB_LockBasedTxMgr"

type levelDBLockBasedEnv struct {
	testLedgerID string
	testDBEnv    *stateleveldb.TestVDBEnv
	testDB       statedb.VersionedDB
	txmgr        txmgr.TxMgr
}

func (env *levelDBLockBasedEnv) getName() string {
	return levelDBtestEnvName
}

func (env *levelDBLockBasedEnv) init(t *testing.T, testLedgerID string) {
	viper.Set("peer.fileSystemPath", testFilesystemPath)
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	testDB, err := testDBEnv.DBProvider.GetDBHandle(testLedgerID)
	testutil.AssertNoError(t, err, "")

	txMgr := NewLockBasedTxMgr(testDB)
	env.testLedgerID = testLedgerID
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

const couchDBtestEnvName = "couchDB_LockBasedTxMgr"

type couchDBLockBasedEnv struct {
	testLedgerID string
	testDBEnv    *statecouchdb.TestVDBEnv
	testDB       statedb.VersionedDB
	txmgr        txmgr.TxMgr
}

func (env *couchDBLockBasedEnv) getName() string {
	return couchDBtestEnvName
}

func (env *couchDBLockBasedEnv) init(t *testing.T, testLedgerID string) {
	viper.Set("peer.fileSystemPath", testFilesystemPath)
	// both vagrant and CI have couchdb configured at host "couchdb"
	viper.Set("ledger.state.couchDBConfig.couchDBAddress", "couchdb:5984")
	// Replace with correct username/password such as
	// admin/admin if user security is enabled on couchdb.
	viper.Set("ledger.state.couchDBConfig.username", "")
	viper.Set("ledger.state.couchDBConfig.password", "")
	viper.Set("ledger.state.couchDBConfig.maxRetries", 3)
	viper.Set("ledger.state.couchDBConfig.maxRetriesOnStartup", 10)
	viper.Set("ledger.state.couchDBConfig.requestTimeout", time.Second*35)
	testDBEnv := statecouchdb.NewTestVDBEnv(t)
	testDB, err := testDBEnv.DBProvider.GetDBHandle(testLedgerID)
	testutil.AssertNoError(t, err, "")

	txMgr := NewLockBasedTxMgr(testDB)
	env.testLedgerID = testLedgerID
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
	defer env.testDBEnv.Cleanup(env.testLedgerID)
}

//////////// txMgrTestHelper /////////////

type txMgrTestHelper struct {
	t     *testing.T
	txMgr txmgr.TxMgr
	bg    *testutil.BlockGenerator
}

func newTxMgrTestHelper(t *testing.T, txMgr txmgr.TxMgr) *txMgrTestHelper {
	bg, _ := testutil.NewBlockGenerator(t, "testLedger", false)
	return &txMgrTestHelper{t, txMgr, bg}
}

func (h *txMgrTestHelper) validateAndCommitRWSet(txRWSet []byte) {
	block := h.bg.NextBlock([][]byte{txRWSet})
	err := h.txMgr.ValidateAndPrepare(block, true)
	testutil.AssertNoError(h.t, err, "")
	txsFltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxNum := 0
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsInvalid(i) {
			invalidTxNum++
		}
	}
	testutil.AssertEquals(h.t, invalidTxNum, 0)
	err = h.txMgr.Commit()
	testutil.AssertNoError(h.t, err, "")
}

func (h *txMgrTestHelper) checkRWsetInvalid(txRWSet []byte) {
	block := h.bg.NextBlock([][]byte{txRWSet})
	err := h.txMgr.ValidateAndPrepare(block, true)
	testutil.AssertNoError(h.t, err, "")
	txsFltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxNum := 0
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsInvalid(i) {
			invalidTxNum++
		}
	}
	testutil.AssertEquals(h.t, invalidTxNum, 1)
}
