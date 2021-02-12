/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/stretchr/testify/require"
)

func TestGetMissingPvtData(t *testing.T) {
	setup := func(l *testLedger) (*ledger.BlockAndPvtData, ledger.MissingPvtDataInfo) {
		collConf := []*collConf{{
			name: "coll1",
			btl:  5,
		}}

		// deploy cc1 with 'collConf'
		l.simulateDeployTx("cc1", collConf)
		l.cutBlockAndCommitLegacy()

		// pvtdata simulation
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key1", "value1")
		})
		// another pvtdata simulation
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key2", "value2")
		})
		// another pvtdata simulation
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key3", "value3")
		})

		// two transactions are missing some pvtdata
		l.causeMissingPvtData(0)
		l.causeMissingPvtData(2)
		blk2 := l.cutBlockAndCommitLegacy()

		l.verifyPvtState("cc1", "coll1", "key2", "value2") // key2 should have been committed

		l.simulateDataTx("", func(s *simulator) {
			// key1 would be stale with respect to hashed version
			_, err := s.GetPrivateData("cc1", "coll1", "key1")
			require.EqualError(t, err, "private data matching public hash version is not available. Public hash version = {BlockNum: 2, TxNum: 0}, Private data version = <nil>")
		})

		l.simulateDataTx("", func(s *simulator) {
			// key3 would be stale with respect to hashed version
			_, err := s.GetPrivateData("cc1", "coll1", "key3")
			require.EqualError(t, err, "private data matching public hash version is not available. Public hash version = {BlockNum: 2, TxNum: 2}, Private data version = <nil>")
		})

		// verify missing pvtdata info
		l.verifyBlockAndPvtDataSameAs(2, blk2)
		expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
		expectedMissingPvtDataInfo.Add(2, 0, "cc1", "coll1")
		expectedMissingPvtDataInfo.Add(2, 2, "cc1", "coll1")
		l.verifyMissingPvtDataSameAs(2, expectedMissingPvtDataInfo)

		return blk2, expectedMissingPvtDataInfo
	}

	t.Run("get missing data after rollback", func(t *testing.T) {
		env := newEnv(t)
		defer env.cleanup()
		env.initLedgerMgmt()
		l := env.createTestLedgerFromGenesisBlk("ledger1")

		blk, expectedMissingPvtDataInfo := setup(l)

		// commit block 3
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key4", "value4")
		})
		blk3 := l.cutBlockAndCommitLegacy()

		// commit block 4
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "key5", "value5")
		})
		blk4 := l.cutBlockAndCommitLegacy()

		// verify missing pvtdata info
		l.verifyMissingPvtDataSameAs(5, expectedMissingPvtDataInfo)

		// rollback ledger to block 2
		l.verifyLedgerHeight(5)
		env.closeLedgerMgmt()
		err := kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "ledger1", 2)
		require.NoError(t, err)
		env.initLedgerMgmt()

		l = env.openTestLedger("ledger1")
		l.verifyLedgerHeight(3)

		// verify block & pvtdata
		l.verifyBlockAndPvtDataSameAs(2, blk)
		// when the pvtdata store is ahead of blockstore,
		// missing pvtdata info for block 2 would not be returned.
		l.verifyMissingPvtDataSameAs(5, nil)

		// recommit block 3
		require.NoError(t, l.lgr.CommitLegacy(blk3, &ledger.CommitOptions{}))
		// when the pvtdata store is ahead of blockstore,
		// missing pvtdata info for block 2 would not be returned.
		l.verifyMissingPvtDataSameAs(5, nil)

		// recommit block 4
		require.NoError(t, l.lgr.CommitLegacy(blk4, &ledger.CommitOptions{}))
		// once the pvtdata store and blockstore becomes equal,
		// missing pvtdata info for block 2 would be returned.
		l.verifyMissingPvtDataSameAs(5, expectedMissingPvtDataInfo)
	})

	t.Run("get deprioritized missing data", func(t *testing.T) {
		initializer := &ledgermgmt.Initializer{
			Config: &ledger.Config{
				PrivateDataConfig: &ledger.PrivateDataConfig{
					MaxBatchSize:                        5000,
					BatchesInterval:                     1000,
					PurgeInterval:                       100,
					DeprioritizedDataReconcilerInterval: 120 * time.Minute,
				},
			},
		}
		env := newEnvWithInitializer(t, initializer)
		defer env.cleanup()
		env.initLedgerMgmt()
		l := env.createTestLedgerFromGenesisBlk("ledger1")

		_, expectedMissingPvtDataInfo := setup(l)

		_, err := l.commitPvtDataOfOldBlocks(nil, expectedMissingPvtDataInfo)
		require.NoError(t, err)
		for i := 0; i < 5; i++ {
			l.verifyMissingPvtDataSameAs(int(2), ledger.MissingPvtDataInfo{})
		}

		env.closeLedgerMgmt()
		env.initializer.Config.PrivateDataConfig.DeprioritizedDataReconcilerInterval = 0 * time.Second
		env.initLedgerMgmt()

		l = env.openTestLedger("ledger1")
		for i := 0; i < 5; i++ {
			l.verifyMissingPvtDataSameAs(2, expectedMissingPvtDataInfo)
		}

		env.closeLedgerMgmt()
		env.initializer.Config.PrivateDataConfig.DeprioritizedDataReconcilerInterval = 120 * time.Minute
		env.initLedgerMgmt()

		l = env.openTestLedger("ledger1")
		for i := 0; i < 5; i++ {
			l.verifyMissingPvtDataSameAs(2, ledger.MissingPvtDataInfo{})
		}
	})
}
