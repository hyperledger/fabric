/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtstatepurgemgmt

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

const (
	levelDBtestEnvName = "levelDB_TestEnv"
	couchDBtestEnvName = "couchDB_TestEnv"
)

// Tests will be run against each environment in this array
// For example, to skip CouchDB tests, remove &couchDBLockBasedEnv{}
var testEnvs = map[string]privacyenabledstate.TestEnv{
	levelDBtestEnvName: &privacyenabledstate.LevelDBTestEnv{},
	couchDBtestEnvName: &privacyenabledstate.CouchDBTestEnv{},
}

func TestMain(m *testing.M) {
	flogging.ActivateSpec("pvtstatepurgemgmt,privacyenabledstate=debug")
	exitCode := m.Run()
	for _, testEnv := range testEnvs {
		testEnv.StopExternalResource()
	}
	os.Exit(exitCode)
}

func TestPurgeMgr(t *testing.T) {
	for _, dbEnv := range testEnvs {
		t.Run(dbEnv.GetName(), func(t *testing.T) { testPurgeMgr(t, dbEnv) })
	}
}

func testPurgeMgr(t *testing.T, dbEnv privacyenabledstate.TestEnv) {
	ledgerid := "testledger-perge-mgr"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns1", "coll1"}: 1,
			{"ns1", "coll2"}: 2,
			{"ns2", "coll3"}: 4,
			{"ns2", "coll4"}: 4,
		},
	)

	testHelper := &testHelper{}
	testHelper.init(t, ledgerid, btlPolicy, dbEnv)
	defer testHelper.cleanup()

	block1Updates := privacyenabledstate.NewUpdateBatch()
	block1Updates.PubUpdates.Put("ns1", "pubkey1", []byte("pubvalue1-1"), version.NewHeight(1, 1))
	putPvtAndHashUpdates(t, block1Updates, "ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"), version.NewHeight(1, 1))
	putPvtAndHashUpdates(t, block1Updates, "ns1", "coll2", "pvtkey2", []byte("pvtvalue2-1"), version.NewHeight(1, 1))
	putPvtAndHashUpdates(t, block1Updates, "ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"), version.NewHeight(1, 1))
	putPvtAndHashUpdates(t, block1Updates, "ns2", "coll4", "pvtkey4", []byte("pvtvalue4-1"), version.NewHeight(1, 1))
	testHelper.commitUpdatesForTesting(1, block1Updates)
	testHelper.checkPvtdataExists("ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"))
	testHelper.checkPvtdataExists("ns1", "coll2", "pvtkey2", []byte("pvtvalue2-1"))
	testHelper.checkPvtdataExists("ns2", "coll3", "pvtkey3", []byte("pvtvalue3-1"))
	testHelper.checkPvtdataExists("ns2", "coll4", "pvtkey4", []byte("pvtvalue4-1"))

	block2Updates := privacyenabledstate.NewUpdateBatch()
	putPvtAndHashUpdates(t, block2Updates, "ns1", "coll2", "pvtkey2", []byte("pvtvalue2-2"), version.NewHeight(2, 1))
	deletePvtAndHashUpdates(t, block2Updates, "ns2", "coll4", "pvtkey4", version.NewHeight(2, 1))
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

func TestPurgeMgrForCommittingPvtDataOfOldBlocks(t *testing.T) {
	for _, dbEnv := range testEnvs {
		t.Run(dbEnv.GetName(), func(t *testing.T) { testPurgeMgrForCommittingPvtDataOfOldBlocks(t, dbEnv) })
	}
}

func testPurgeMgrForCommittingPvtDataOfOldBlocks(t *testing.T, dbEnv privacyenabledstate.TestEnv) {
	ledgerid := "testledger-purge-mgr-pvtdata-oldblocks"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns1", "coll1"}: 1,
		},
	)

	testHelper := &testHelper{}
	testHelper.init(t, ledgerid, btlPolicy, dbEnv)
	defer testHelper.cleanup()

	// committing block 1
	block1Updates := privacyenabledstate.NewUpdateBatch()
	// pvt data pvtkey1 is missing but the pvtkey2 is present.
	// pvtkey1 and pvtkey2 both would get expired and purged while committing block 3
	putHashUpdates(block1Updates, "ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"), version.NewHeight(1, 1))
	putPvtAndHashUpdates(t, block1Updates, "ns1", "coll1", "pvtkey2", []byte("pvtvalue1-2"), version.NewHeight(1, 1))
	testHelper.commitUpdatesForTesting(1, block1Updates)

	// pvtkey1 should not exist but pvtkey2 should exist
	testHelper.checkOnlyPvtKeyDoesNotExist("ns1", "coll1", "pvtkey1")
	testHelper.checkPvtdataExists("ns1", "coll1", "pvtkey2", []byte("pvtvalue1-2"))

	// committing block 2
	block2Updates := privacyenabledstate.NewUpdateBatch()
	testHelper.commitUpdatesForTesting(2, block2Updates)

	// Commit pvtkey1 via commit of missing data and this should be added to toPurge list as it
	// should be removed while committing block 3
	block1PvtData := privacyenabledstate.NewUpdateBatch()
	putPvtUpdates(block1PvtData, "ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"), version.NewHeight(1, 1))
	testHelper.commitPvtDataOfOldBlocksForTesting(block1PvtData)

	// both pvtkey1 and pvtkey1 should exist
	testHelper.checkPvtdataExists("ns1", "coll1", "pvtkey1", []byte("pvtvalue1-1"))
	testHelper.checkPvtdataExists("ns1", "coll1", "pvtkey2", []byte("pvtvalue1-2"))

	// committing block 3
	block3Updates := privacyenabledstate.NewUpdateBatch()
	testHelper.commitUpdatesForTesting(3, block3Updates)

	// both pvtkey1 and pvtkey1 should not exist
	testHelper.checkPvtdataDoesNotExist("ns1", "coll1", "pvtkey1")
	testHelper.checkPvtdataDoesNotExist("ns1", "coll1", "pvtkey2")
}

func TestKeyUpdateBeforeExpiryBlock(t *testing.T) {
	dbEnv := testEnvs[levelDBtestEnvName]
	ledgerid := "testledger-perge-mgr"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 1, // expiry block = committing block + 2
		},
	)
	helper := &testHelper{}
	helper.init(t, ledgerid, btlPolicy, dbEnv)
	defer helper.cleanup()

	// block-1 updates: Update only hash of the pvt key
	block1Updates := privacyenabledstate.NewUpdateBatch()
	putHashUpdates(block1Updates, "ns", "coll", "pvtkey", []byte("pvtvalue-1"), version.NewHeight(1, 1))
	helper.commitUpdatesForTesting(1, block1Updates)
	expInfo, _ := helper.purgeMgr.expKeeper.retrieve(3)
	require.Len(t, expInfo, 1)

	// block-2 update: Update both hash and pvt data
	block2Updates := privacyenabledstate.NewUpdateBatch()
	putPvtAndHashUpdates(t, block2Updates, "ns", "coll", "pvtkey", []byte("pvtvalue-2"), version.NewHeight(2, 1))
	helper.commitUpdatesForTesting(2, block2Updates)
	helper.checkExpiryEntryExistsForBlockNum(3, 1)
	helper.checkExpiryEntryExistsForBlockNum(4, 1)

	// block-3 update: no Updates
	noPvtdataUpdates := privacyenabledstate.NewUpdateBatch()
	helper.commitUpdatesForTesting(3, noPvtdataUpdates)
	helper.checkPvtdataExists("ns", "coll", "pvtkey", []byte("pvtvalue-2"))
	helper.checkNoExpiryEntryExistsForBlockNum(3)
	helper.checkExpiryEntryExistsForBlockNum(4, 1)

	// block-4 update: no Updates
	noPvtdataUpdates = privacyenabledstate.NewUpdateBatch()
	helper.commitUpdatesForTesting(4, noPvtdataUpdates)
	helper.checkPvtdataDoesNotExist("ns", "coll", "pvtkey")
	helper.checkNoExpiryEntryExistsForBlockNum(4)
}

func TestOnlyHashUpdateInExpiryBlock(t *testing.T) {
	dbEnv := testEnvs[levelDBtestEnvName]
	ledgerid := "testledger-perge-mgr"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 1, // expiry block = committing block + 2
		},
	)
	helper := &testHelper{}
	helper.init(t, ledgerid, btlPolicy, dbEnv)
	defer helper.cleanup()

	// block-1 updates: Add pvt data
	block1Updates := privacyenabledstate.NewUpdateBatch()
	putPvtAndHashUpdates(t, block1Updates,
		"ns", "coll", "pvtkey", []byte("pvtvalue-1"), version.NewHeight(1, 1))
	helper.commitUpdatesForTesting(1, block1Updates)
	helper.checkExpiryEntryExistsForBlockNum(3, 1)

	// block-2 update: No Updates
	noPvtdataUpdates := privacyenabledstate.NewUpdateBatch()
	helper.commitUpdatesForTesting(2, noPvtdataUpdates)
	helper.checkPvtdataExists(
		"ns", "coll", "pvtkey", []byte("pvtvalue-1"))
	helper.checkExpiryEntryExistsForBlockNum(3, 1)

	// block-3 update: Update hash only
	block3Updates := privacyenabledstate.NewUpdateBatch()
	putHashUpdates(block3Updates,
		"ns", "coll", "pvtkey", []byte("pvtvalue-3"), version.NewHeight(3, 1))
	helper.commitUpdatesForTesting(3, block3Updates)
	helper.checkOnlyKeyHashExists("ns", "coll", "pvtkey")
	helper.checkNoExpiryEntryExistsForBlockNum(3)
	helper.checkExpiryEntryExistsForBlockNum(5, 1)

	// block-4 update: no Updates
	noPvtdataUpdates = privacyenabledstate.NewUpdateBatch()
	helper.commitUpdatesForTesting(4, noPvtdataUpdates)
	helper.checkExpiryEntryExistsForBlockNum(5, 1)

	// block-5 update: no Updates
	noPvtdataUpdates = privacyenabledstate.NewUpdateBatch()
	helper.commitUpdatesForTesting(5, noPvtdataUpdates)
	helper.checkPvtdataDoesNotExist("ns", "coll", "pvtkey")
	helper.checkNoExpiryEntryExistsForBlockNum(5)
}

func TestOnlyHashDeleteBeforeExpiryBlock(t *testing.T) {
	dbEnv := testEnvs[levelDBtestEnvName]
	ledgerid := "testledger-perge-mgr"
	btlPolicy := btltestutil.SampleBTLPolicy(
		map[[2]string]uint64{
			{"ns", "coll"}: 1, // expiry block = committing block + 2
		},
	)
	testHelper := &testHelper{}
	testHelper.init(t, ledgerid, btlPolicy, dbEnv)
	defer testHelper.cleanup()

	// block-1 updates: add pvt key
	block1Updates := privacyenabledstate.NewUpdateBatch()
	putPvtAndHashUpdates(t, block1Updates,
		"ns", "coll", "pvtkey", []byte("pvtvalue-1"), version.NewHeight(1, 1))
	testHelper.commitUpdatesForTesting(1, block1Updates)

	// block-2 update: delete Hash only
	block2Updates := privacyenabledstate.NewUpdateBatch()
	deleteHashUpdates(block2Updates, "ns", "coll", "pvtkey", version.NewHeight(2, 1))
	testHelper.commitUpdatesForTesting(2, block2Updates)
	testHelper.checkOnlyPvtKeyExists("ns", "coll", "pvtkey", []byte("pvtvalue-1"))

	// block-3 update: no updates
	noPvtdataUpdates := privacyenabledstate.NewUpdateBatch()
	testHelper.commitUpdatesForTesting(3, noPvtdataUpdates)
	testHelper.checkPvtdataDoesNotExist("ns", "coll", "pvtkey")
}

type testHelper struct {
	t              *testing.T
	bookkeepingEnv *bookkeeping.TestEnv
	dbEnv          privacyenabledstate.TestEnv

	db       *privacyenabledstate.DB
	purgeMgr *PurgeMgr
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
	require.NoError(h.t, h.purgeMgr.UpdateExpiryInfo(updates.PvtUpdates, updates.HashUpdates))
	require.NoError(h.t, h.purgeMgr.AddExpiredEntriesToUpdateBatch(updates.PvtUpdates, updates.HashUpdates))
	require.NoError(h.t, h.db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(blkNum, 1)))
	h.db.ClearCachedVersions()
	require.NoError(h.t, h.purgeMgr.BlockCommitDone())
}

func (h *testHelper) commitPvtDataOfOldBlocksForTesting(updates *privacyenabledstate.UpdateBatch) {
	require.NoError(h.t, h.purgeMgr.UpdateExpiryInfoOfPvtDataOfOldBlocks(updates.PvtUpdates))
	require.NoError(h.t, h.db.ApplyPrivacyAwareUpdates(updates, nil))
}

func (h *testHelper) checkPvtdataExists(ns, coll, key string, value []byte) {
	vv, hashVersion := h.fetchPvtdataFronDB(ns, coll, key)
	require.NotNil(h.t, vv)
	require.Equal(h.t, value, vv.Value)
	require.Equal(h.t, vv.Version, hashVersion)
}

func (h *testHelper) checkPvtdataDoesNotExist(ns, coll, key string) {
	vv, hashVersion := h.fetchPvtdataFronDB(ns, coll, key)
	require.Nil(h.t, vv)
	require.Nil(h.t, hashVersion)
}

func (h *testHelper) checkOnlyPvtKeyExists(ns, coll, key string, value []byte) {
	vv, hashVersion := h.fetchPvtdataFronDB(ns, coll, key)
	require.NotNil(h.t, vv)
	require.Nil(h.t, hashVersion)
	require.Equal(h.t, value, vv.Value)
}

func (h *testHelper) checkOnlyPvtKeyDoesNotExist(ns, coll, key string) {
	kv, err := h.db.GetPrivateData(ns, coll, key)
	require.Nil(h.t, err)
	require.Nil(h.t, kv)
}

func (h *testHelper) checkOnlyKeyHashExists(ns, coll, key string) {
	vv, hashVersion := h.fetchPvtdataFronDB(ns, coll, key)
	require.Nil(h.t, vv)
	require.NotNil(h.t, hashVersion)
}

func (h *testHelper) fetchPvtdataFronDB(ns, coll, key string) (kv *statedb.VersionedValue, hashVersion *version.Height) {
	var err error
	kv, err = h.db.GetPrivateData(ns, coll, key)
	require.NoError(h.t, err)
	hashVersion, err = h.db.GetKeyHashVersion(ns, coll, util.ComputeStringHash(key))
	require.NoError(h.t, err)
	return
}

func (h *testHelper) checkExpiryEntryExistsForBlockNum(expiringBlk uint64, expectedNumEntries int) {
	expInfo, err := h.purgeMgr.expKeeper.retrieve(expiringBlk)
	require.NoError(h.t, err)
	require.Len(h.t, expInfo, expectedNumEntries)
}

func (h *testHelper) checkNoExpiryEntryExistsForBlockNum(expiringBlk uint64) {
	expInfo, err := h.purgeMgr.expKeeper.retrieve(expiringBlk)
	require.NoError(h.t, err)
	require.Len(h.t, expInfo, 0)
}
