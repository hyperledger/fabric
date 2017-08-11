/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatatxmgr

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/transientstore"
)

var (
	// TestEnvs lists all the txmgr environments (e.g. with leveldb and with couchdb) for testing
	TestEnvs = []*TestEnv{
		createEnv(&privacyenabledstate.LevelDBCommonStorageTestEnv{}),
		//createEnv(&privacyenabledstate.CouchDBCommonStorageTestEnv{}),
	}
)

// TestEnv reprsents a test txmgr test environment for testing
type TestEnv struct {
	t         *testing.T
	Name      string
	LedgerID  string
	DBEnv     privacyenabledstate.TestEnv
	TStoreEnv *transientstore.StoreEnv

	DB     privacyenabledstate.DB
	TStore transientstore.Store
	Txmgr  txmgr.TxMgr
}

func createEnv(statedbEnv privacyenabledstate.TestEnv) *TestEnv {
	return &TestEnv{Name: statedbEnv.GetName(), DBEnv: statedbEnv}
}

// Init initializes the test environment
func (env *TestEnv) Init(t *testing.T, testLedgerID string) {
	env.t = t
	env.DBEnv.Init(t)
	env.DB = env.DBEnv.GetDBHandle(testLedgerID)
	env.TStoreEnv = transientstore.NewTestStoreEnv(t)
	var err error
	env.TStore, err = env.TStoreEnv.TestStoreProvider.OpenStore(testLedgerID)
	testutil.AssertNoError(t, err, "")
	env.Txmgr = NewLockbasedTxMgr(env.DB, env.TStore)
}

// Cleanup cleansup the test environment
func (env *TestEnv) Cleanup() {
	env.Txmgr.Shutdown()
	env.DBEnv.Cleanup()
	env.TStoreEnv.Cleanup()
}

type TestTx struct {
	ID                string
	SimulationResults *ledger.TxSimulationResults
}
