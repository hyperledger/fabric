/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/core/container/externalbuilder"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/stretchr/testify/require"
)

func TestGetMissingPvtData(t *testing.T) {
	setup := func(h *testhelper) (*ledger.BlockAndPvtData, ledger.MissingPvtDataInfo) {
		collConf := []*collConf{{
			name: "coll1",
			btl:  5,
		}}

		// deploy cc1 with 'collConf'
		h.simulateDeployTx("cc1", collConf)
		h.cutBlockAndCommitLegacy()

		// pvtdata simulation
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key1", "value1")
		})
		// another pvtdata simulation
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key2", "value2")
		})
		// another pvtdata simulation
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key3", "value3")
		})

		// two transactions are missing some pvtdata
		h.causeMissingPvtData(0)
		h.causeMissingPvtData(2)
		blk2 := h.cutBlockAndCommitLegacy()

		h.verifyPvtState("cc1", "coll1", "key2", "value2") // key2 should have been committed
		h.simulateDataTx("", func(s *simulator) {
			h.assertError(s.GetPrivateData("cc1", "coll1", "key1")) // key1 would be stale with respect to hashed version
		})
		h.simulateDataTx("", func(s *simulator) {
			h.assertError(s.GetPrivateData("cc1", "coll1", "key3")) // key3 would be stale with respect to hashed version
		})

		// verify missing pvtdata info
		h.verifyBlockAndPvtDataSameAs(2, blk2)
		expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
		expectedMissingPvtDataInfo.Add(2, 0, "cc1", "coll1")
		expectedMissingPvtDataInfo.Add(2, 2, "cc1", "coll1")
		h.verifyMissingPvtDataSameAs(2, expectedMissingPvtDataInfo)

		return blk2, expectedMissingPvtDataInfo
	}

	t.Run("get missing data after rollback", func(t *testing.T) {
		env := newEnv(t)
		defer env.cleanup()
		env.initLedgerMgmt()
		h := env.newTestHelperCreateLgr("ledger1", t)

		blk, expectedMissingPvtDataInfo := setup(h)

		// commit block 3
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key4", "value4")
		})
		blk3 := h.cutBlockAndCommitLegacy()

		// commit block 4
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key5", "value5")
		})
		blk4 := h.cutBlockAndCommitLegacy()

		// verify missing pvtdata info
		h.verifyMissingPvtDataSameAs(5, expectedMissingPvtDataInfo)

		// rollback ledger to block 2
		h.verifyLedgerHeight(5)
		env.closeLedgerMgmt()
		err := kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "ledger1", 2)
		require.NoError(t, err)
		env.initLedgerMgmt()

		h = env.newTestHelperOpenLgr("ledger1", t)
		h.verifyLedgerHeight(3)

		// verify block & pvtdata
		h.verifyBlockAndPvtDataSameAs(2, blk)
		// when the pvtdata store is ahead of blockstore,
		// missing pvtdata info for block 2 would not be returned.
		h.verifyMissingPvtDataSameAs(5, nil)

		// recommit block 3
		require.NoError(t, h.lgr.CommitLegacy(blk3, &ledger.CommitOptions{}))
		// when the pvtdata store is ahead of blockstore,
		// missing pvtdata info for block 2 would not be returned.
		h.verifyMissingPvtDataSameAs(5, nil)

		// recommit block 4
		require.NoError(t, h.lgr.CommitLegacy(blk4, &ledger.CommitOptions{}))
		// once the pvtdata store and blockstore becomes equal,
		// missing pvtdata info for block 2 would be returned.
		h.verifyMissingPvtDataSameAs(5, expectedMissingPvtDataInfo)
	})

	t.Run("get deprioritized missing data", func(t *testing.T) {
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)

		initializer := &ledgermgmt.Initializer{
			Config: &ledger.Config{
				PrivateDataConfig: &ledger.PrivateDataConfig{
					MaxBatchSize:                        5000,
					BatchesInterval:                     1000,
					PurgeInterval:                       100,
					DeprioritizedDataReconcilerInterval: 120 * time.Minute,
				},
			},
			HashProvider: cryptoProvider,
			EbMetadataProvider: &externalbuilder.MetadataProvider{
				DurablePath: "testdata",
			},
		}
		env := newEnvWithInitializer(t, initializer)
		defer env.cleanup()
		env.initLedgerMgmt()
		h := env.newTestHelperCreateLgr("ledger1", t)

		_, expectedMissingPvtDataInfo := setup(h)

		h.commitPvtDataOfOldBlocks(nil, expectedMissingPvtDataInfo)
		for i := 0; i < 5; i++ {
			h.verifyMissingPvtDataSameAs(int(2), ledger.MissingPvtDataInfo{})
		}

		env.closeLedgerMgmt()
		env.initializer.Config.PrivateDataConfig.DeprioritizedDataReconcilerInterval = 0 * time.Second
		env.initLedgerMgmt()

		h = env.newTestHelperOpenLgr("ledger1", t)
		for i := 0; i < 5; i++ {
			h.verifyMissingPvtDataSameAs(2, expectedMissingPvtDataInfo)
		}

		env.closeLedgerMgmt()
		env.initializer.Config.PrivateDataConfig.DeprioritizedDataReconcilerInterval = 120 * time.Minute
		env.initLedgerMgmt()

		h = env.newTestHelperOpenLgr("ledger1", t)
		for i := 0; i < 5; i++ {
			h.verifyMissingPvtDataSameAs(2, ledger.MissingPvtDataInfo{})
		}
	})
}
