/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("pvtstatepurgemgmt", "debug")
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/pvtstatepurgemgmt")
	os.Exit(m.Run())
}

func TestPergeMgr(t *testing.T) {
	dbEnvs := []privacyenabledstate.TestEnv{
		&privacyenabledstate.LevelDBCommonStorageTestEnv{},
		&privacyenabledstate.CouchDBCommonStorageTestEnv{},
	}
	for _, dbEnv := range dbEnvs {
		t.Run(dbEnv.GetName(), func(t *testing.T) { testPergeMgr(t, dbEnv) })
	}
}

func testPergeMgr(t *testing.T, dbEnv privacyenabledstate.TestEnv) {
	ledgerid := "testledger-perge-mgr"
	cs := btltestutil.NewMockCollectionStore()
	cs.SetBTL("ns1", "coll1", 1)
	cs.SetBTL("ns1", "coll2", 2)
	cs.SetBTL("ns2", "coll3", 4)
	cs.SetBTL("ns2", "coll4", 4)
	btlPolicy := pvtdatapolicy.ConstructBTLPolicy(cs)

	testHelper := &testHelper{}
	testHelper.init(t, ledgerid, btlPolicy, dbEnv)
	defer testHelper.cleanup()

	block1Updates := privacyenabledstate.NewUpdateBatch()
	block1Updates.PubUpdates.Put("ns1", "pubkey1", []byte("pubvalue1-1"), version.NewHeight(1, 1))
	putPvtUpdates(t, block1Updates, "ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"), version.NewHeight(1, 1))
	putPvtUpdates(t, block1Updates, "ns1", "coll2", "pvtkey2", []byte("pvtvalue2-1"), version.NewHeight(1, 1))
	putPvtUpdates(t, block1Updates, "ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"), version.NewHeight(1, 1))
	putPvtUpdates(t, block1Updates, "ns2", "coll4", "pvtkey4", []byte("pvtvalue4-1"), version.NewHeight(1, 1))
	testHelper.commitUpdatesForTesting(1, block1Updates)
	testHelper.checkPvtdataExists("ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"))
	testHelper.checkPvtdataExists("ns1", "coll2", "pvtkey2", []byte("pvtvalue2-1"))
	testHelper.checkPvtdataExists("ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"))
	testHelper.checkPvtdataExists("ns2", "coll4", "pvtkey4", []byte("pvtvalue4-1"))

	block2Updates := privacyenabledstate.NewUpdateBatch()
	putPvtUpdates(t, block2Updates, "ns1", "coll2", "pvtkey2", []byte("pvtvalue2-2"), version.NewHeight(2, 1))
	deletePvtUpdates(t, block2Updates, "ns2", "coll4", "pvtkey4", version.NewHeight(2, 1))
	testHelper.commitUpdatesForTesting(2, block2Updates)
	testHelper.checkPvtdataExists("ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"))
	testHelper.checkPvtdataExists("ns1", "coll2", "pvtkey2", []byte("pvtvalue2-2"))
	testHelper.checkPvtdataExists("ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"))
	testHelper.checkPvtdataDoesNotExist("ns1", "coll4", "pvtkey4")

	noPvtdataUpdates := privacyenabledstate.NewUpdateBatch()
	testHelper.commitUpdatesForTesting(3, noPvtdataUpdates)
	testHelper.checkPvtdataDoesNotExist("ns1", "coll1", "pvtkey1")
	testHelper.checkPvtdataExists("ns1", "coll2", "pvtkey2", []byte("pvtvalue2-2"))
	testHelper.checkPvtdataExists("ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"))
	testHelper.checkPvtdataDoesNotExist("ns1", "coll4", "pvtkey4")

	testHelper.commitUpdatesForTesting(4, noPvtdataUpdates)
	testHelper.checkPvtdataDoesNotExist("ns1", "coll1", "pvtkey1")
	testHelper.checkPvtdataExists("ns1", "coll2", "pvtkey2", []byte("pvtvalue2-2"))
	testHelper.checkPvtdataExists("ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"))
	testHelper.checkPvtdataDoesNotExist("ns1", "coll4", "pvtkey4")

	testHelper.commitUpdatesForTesting(5, noPvtdataUpdates)
	testHelper.checkPvtdataDoesNotExist("ns1", "coll1", "pvtkey1")
	testHelper.checkPvtdataDoesNotExist("ns1", "coll2", "pvtkey2")
	testHelper.checkPvtdataExists("ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"))
	testHelper.checkPvtdataDoesNotExist("ns1", "coll4", "pvtkey4")

	testHelper.commitUpdatesForTesting(6, noPvtdataUpdates)
	testHelper.checkPvtdataDoesNotExist("ns1", "coll1", "pvtkey1")
	testHelper.checkPvtdataDoesNotExist("ns1", "coll2", "pvtkey2")
	testHelper.checkPvtdataDoesNotExist("ns2", "coll3", "pvtkey3")
	testHelper.checkPvtdataDoesNotExist("ns1", "coll4", "pvtkey4")
}

type testHelper struct {
	t              *testing.T
	bookkeepingEnv *bookkeeping.TestEnv
	dbEnv          privacyenabledstate.TestEnv

	db       privacyenabledstate.DB
	purgeMgr PurgeMgr
}

func (h *testHelper) init(t *testing.T, ledgerid string, btlPolicy pvtdatapolicy.BTLPolicy, dbEnv privacyenabledstate.TestEnv) {
	h.t = t
	h.bookkeepingEnv = bookkeeping.NewTestEnv(t)
	dbEnv.Init(t)
	h.dbEnv = dbEnv
	h.db = h.dbEnv.GetDBHandle(ledgerid)
	var err error
	if h.purgeMgr, err = InstantiatePurgeMgr(ledgerid, h.db, btlPolicy, h.bookkeepingEnv.TestProvider); err != nil {
		t.Fatalf("err:%s", err)
	}
}

func (h *testHelper) cleanup() {
	h.bookkeepingEnv.Cleanup()
	h.dbEnv.Cleanup()
}

func (h *testHelper) commitUpdatesForTesting(blkNum uint64, updates *privacyenabledstate.UpdateBatch) {
	h.purgeMgr.PrepareForExpiringKeys(blkNum)
	assert.NoError(h.t, h.purgeMgr.DeleteExpiredAndUpdateBookkeeping(updates.PvtUpdates, updates.HashUpdates))
	assert.NoError(h.t, h.db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(blkNum, 1)))
	h.db.ClearCachedVersions()
	h.purgeMgr.BlockCommitDone()
}

func (h *testHelper) checkPvtdataExists(ns, coll, key string, value []byte) {
	vv, _ := h.fetchPvtdataFronDB(ns, coll, key)
	vv, hashVersion := h.fetchPvtdataFronDB(ns, coll, key)
	assert.NotNil(h.t, vv)
	assert.Equal(h.t, value, vv.Value)
	assert.Equal(h.t, vv.Version, hashVersion)
}

func (h *testHelper) checkPvtdataDoesNotExist(ns, coll, key string) {
	vv, hashVersion := h.fetchPvtdataFronDB(ns, coll, key)
	assert.Nil(h.t, vv)
	assert.Nil(h.t, hashVersion)
}

func (h *testHelper) fetchPvtdataFronDB(ns, coll, key string) (kv *statedb.VersionedValue, hashVersion *version.Height) {
	var err error
	kv, err = h.db.GetPrivateData(ns, coll, key)
	assert.NoError(h.t, err)
	hashVersion, err = h.db.GetKeyHashVersion(ns, coll, util.ComputeStringHash(key))
	assert.NoError(h.t, err)
	return
}
