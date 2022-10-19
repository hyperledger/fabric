/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txmgr

import (
	"fmt"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/bookkeeping"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	btltestutil "github.com/hyperledger/fabric/core/ledger/pvtdatapolicy/testutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec(
		"lockbasedtxmgr,statevalidator,statebasedval,statecouchdb,valimpl,pvtstatepurgemgmt,valinternal=debug",
	)
	exitCode := m.Run()
	for _, testEnv := range testEnvs {
		testEnv.stopExternalResource()
	}
	os.Exit(exitCode)
}

type testEnv interface {
	cleanup()
	getName() string
	getTxMgr() *LockBasedTxMgr
	getVDB() *privacyenabledstate.DB
	init(t *testing.T, testLedgerID string, btlPolicy pvtdatapolicy.BTLPolicy)
	stopExternalResource()
}

const (
	levelDBtestEnvName = "levelDB_LockBasedTxMgr"
	couchDBtestEnvName = "couchDB_LockBasedTxMgr"
)

// Tests will be run against each environment in this array
// For example, to skip CouchDB tests, remove the entry for couch environment
var testEnvs = []testEnv{
	&lockBasedEnv{name: levelDBtestEnvName, testDBEnv: &privacyenabledstate.LevelDBTestEnv{}},
	&lockBasedEnv{name: couchDBtestEnvName, testDBEnv: &privacyenabledstate.CouchDBTestEnv{}},
}

var testEnvsMap = map[string]testEnv{
	levelDBtestEnvName: testEnvs[0],
	couchDBtestEnvName: testEnvs[1],
}

///////////// LevelDB Environment //////////////

type lockBasedEnv struct {
	dbInitialized      bool
	name               string
	t                  testing.TB
	testBookkeepingEnv *bookkeeping.TestEnv
	testDB             *privacyenabledstate.DB
	testDBEnv          privacyenabledstate.TestEnv
	txmgr              *LockBasedTxMgr
}

func (env *lockBasedEnv) getName() string {
	return env.name
}

func (env *lockBasedEnv) init(t *testing.T, testLedgerID string, btlPolicy pvtdatapolicy.BTLPolicy) {
	var err error
	env.t = t
	if env.dbInitialized == false {
		env.testDBEnv.Init(t)
		env.dbInitialized = true
	}
	env.testDB = env.testDBEnv.GetDBHandle(testLedgerID)
	require.NoError(t, err)
	if btlPolicy == nil {
		btlPolicy = btltestutil.SampleBTLPolicy(
			map[[2]string]uint64{},
		)
	}
	env.testBookkeepingEnv = bookkeeping.NewTestEnv(t)

	txmgrInitializer := &Initializer{
		LedgerID:            testLedgerID,
		DB:                  env.testDB,
		StateListeners:      nil,
		BtlPolicy:           btlPolicy,
		BookkeepingProvider: env.testBookkeepingEnv.TestProvider,
		CCInfoProvider:      &mock.DeployedChaincodeInfoProvider{},
		CustomTxProcessors:  nil,
		HashFunc:            testHashFunc,
	}
	env.txmgr, err = NewLockBasedTxMgr(txmgrInitializer)
	require.NoError(t, err)
}

func (env *lockBasedEnv) getTxMgr() *LockBasedTxMgr {
	return env.txmgr
}

func (env *lockBasedEnv) getVDB() *privacyenabledstate.DB {
	return env.testDB
}

func (env *lockBasedEnv) cleanup() {
	if env.dbInitialized {
		env.txmgr.Shutdown()
		env.testDBEnv.Cleanup()
		env.testBookkeepingEnv.Cleanup()
		env.dbInitialized = false
	}
}

func (env *lockBasedEnv) stopExternalResource() {
	env.testDBEnv.StopExternalResource()
}

//////////// txMgrTestHelper /////////////

type txMgrTestHelper struct {
	t     *testing.T
	txMgr *LockBasedTxMgr
	bg    *testutil.BlockGenerator
}

func newTxMgrTestHelper(t *testing.T, txMgr *LockBasedTxMgr) *txMgrTestHelper {
	bg, _ := testutil.NewBlockGenerator(t, "testLedger", false)
	return &txMgrTestHelper{t, txMgr, bg}
}

func (h *txMgrTestHelper) validateAndCommitRWSet(txRWSet *rwset.TxReadWriteSet) {
	rwSetBytes, _ := proto.Marshal(txRWSet)
	block := h.bg.NextBlock([][]byte{rwSetBytes})
	_, _, _, err := h.txMgr.ValidateAndPrepare(&ledger.BlockAndPvtData{Block: block, PvtData: nil}, true)
	require.NoError(h.t, err)
	txsFltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxNum := 0
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsInvalid(i) {
			invalidTxNum++
		}
	}
	require.Equal(h.t, 0, invalidTxNum)
	err = h.txMgr.Commit()
	require.NoError(h.t, err)
}

func (h *txMgrTestHelper) checkRWsetInvalid(txRWSet *rwset.TxReadWriteSet) {
	rwSetBytes, _ := proto.Marshal(txRWSet)
	block := h.bg.NextBlock([][]byte{rwSetBytes})
	_, _, _, err := h.txMgr.ValidateAndPrepare(&ledger.BlockAndPvtData{Block: block, PvtData: nil}, true)
	require.NoError(h.t, err)
	txsFltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	invalidTxNum := 0
	for i := 0; i < len(block.Data.Data); i++ {
		if txsFltr.IsInvalid(i) {
			invalidTxNum++
		}
	}
	require.Equal(h.t, 1, invalidTxNum)
}

func populateCollConfigForTest(t *testing.T, txMgr *LockBasedTxMgr, nsColls []collConfigkey, ht *version.Height) {
	m := map[string]*peer.CollectionConfigPackage{}
	for _, nsColl := range nsColls {
		ns, coll := nsColl.ns, nsColl.coll
		pkg, ok := m[ns]
		if !ok {
			pkg = &peer.CollectionConfigPackage{}
			m[ns] = pkg
		}
		sCollConfig := &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name: coll,
			},
		}
		pkg.Config = append(pkg.Config, &peer.CollectionConfig{Payload: sCollConfig})
	}
	ccInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	ccInfoProvider.AllCollectionsConfigPkgStub = func(channelName, ccName string, qe ledger.SimpleQueryExecutor) (*peer.CollectionConfigPackage, error) {
		fmt.Printf("retrieveing info for [%s] from [%s]\n", ccName, m)
		return m[ccName], nil
	}
	txMgr.ccInfoProvider = ccInfoProvider
}

func testutilPopulateDB(
	t *testing.T, txMgr *LockBasedTxMgr, ns string,
	data []*queryresult.KV, pvtdataHashes []*testutilPvtdata,
	version *version.Height,
) {
	updates := privacyenabledstate.NewUpdateBatch()
	for _, kv := range data {
		updates.PubUpdates.Put(ns, kv.Key, kv.Value, version)
	}
	for _, p := range pvtdataHashes {
		updates.HashUpdates.Put(ns, p.coll, util.ComputeStringHash(p.key), util.ComputeHash(p.value), version)
	}
	require.NoError(t, txMgr.db.ApplyPrivacyAwareUpdates(updates, version))
}

type testutilPvtdata struct {
	coll, key string
	value     []byte
}
