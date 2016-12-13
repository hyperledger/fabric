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

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
)

type testEnv interface {
	init(t *testing.T)
	getName() string
	getTxMgr() txmgr.TxMgr
	getVDB() statedb.VersionedDB
	cleanup()
}

var testEnvs = []testEnv{&levelDBLockBasedEnv{}}

type levelDBLockBasedEnv struct {
	testDBEnv *stateleveldb.TestVDBEnv
	txmgr     txmgr.TxMgr
}

func (env *levelDBLockBasedEnv) getName() string {
	return "levelDB_LockBasedTxMgr"
}

func (env *levelDBLockBasedEnv) init(t *testing.T) {
	testDBPath := "/tmp/fabric/core/ledger/kvledger/txmgmt/lockbasedtxmgmt"
	testDBEnv := stateleveldb.NewTestVDBEnv(t, testDBPath)
	txMgr := lockbasedtxmgr.NewLockBasedTxMgr(testDBEnv.DB)
	env.testDBEnv = testDBEnv
	env.txmgr = txMgr
}

func (env *levelDBLockBasedEnv) getTxMgr() txmgr.TxMgr {
	return env.txmgr
}

func (env *levelDBLockBasedEnv) getVDB() statedb.VersionedDB {
	return env.testDBEnv.DB
}

func (env *levelDBLockBasedEnv) cleanup() {
	defer env.txmgr.Shutdown()
	defer env.testDBEnv.Cleanup()
}

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
