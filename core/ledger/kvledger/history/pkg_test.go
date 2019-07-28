/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package history

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/txmgr/lockbasedtxmgr"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/assert"
)

/////// levelDBLockBasedHistoryEnv //////

type levelDBLockBasedHistoryEnv struct {
	t                     testing.TB
	testBlockStorageEnv   *testBlockStoreEnv
	testDBEnv             privacyenabledstate.TestEnv
	testBookkeepingEnv    *bookkeeping.TestEnv
	txmgr                 txmgr.TxMgr
	testHistoryDBProvider *DBProvider
	testHistoryDB         *DB
	testHistoryDBPath     string
}

func newTestHistoryEnv(t *testing.T) *levelDBLockBasedHistoryEnv {
	testLedgerID := "TestLedger"

	blockStorageTestEnv := newBlockStorageTestEnv(t)

	testDBEnv := &privacyenabledstate.LevelDBCommonStorageTestEnv{}
	testDBEnv.Init(t)
	testDB := testDBEnv.GetDBHandle(testLedgerID)
	testBookkeepingEnv := bookkeeping.NewTestEnv(t)

	testHistoryDBPath, err := ioutil.TempDir("", "historyldb")
	if err != nil {
		t.Fatalf("Failed to create history database directory: %s", err)
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)
	txMgr, err := lockbasedtxmgr.NewLockBasedTxMgr(
		testLedgerID,
		testDB,
		nil,
		nil,
		testBookkeepingEnv.TestProvider,
		&mock.DeployedChaincodeInfoProvider{},
		nil,
		cryptoProvider,
	)

	assert.NoError(t, err)
	testHistoryDBProvider, err := NewDBProvider(testHistoryDBPath)
	assert.NoError(t, err)
	testHistoryDB, err := testHistoryDBProvider.GetDBHandle("TestHistoryDB")
	assert.NoError(t, err)

	return &levelDBLockBasedHistoryEnv{
		t,
		blockStorageTestEnv,
		testDBEnv,
		testBookkeepingEnv,
		txMgr,
		testHistoryDBProvider,
		testHistoryDB,
		testHistoryDBPath,
	}
}

func (env *levelDBLockBasedHistoryEnv) cleanup() {
	env.txmgr.Shutdown()
	env.testDBEnv.Cleanup()
	env.testBlockStorageEnv.cleanup()
	env.testBookkeepingEnv.Cleanup()
	// clean up history
	env.testHistoryDBProvider.Close()
	os.RemoveAll(env.testHistoryDBPath)
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

	p, err := fsblkstorage.NewProvider(conf, indexConfig, &disabled.Provider{})
	assert.NoError(t, err)
	return &testBlockStoreEnv{t, p.(*fsblkstorage.FsBlockstoreProvider), testPath}
}

func (env *testBlockStoreEnv) cleanup() {
	env.provider.Close()
	env.removeFSPath()
}

func (env *testBlockStoreEnv) removeFSPath() {
	fsPath := env.blockStorageDir
	os.RemoveAll(fsPath)
}
