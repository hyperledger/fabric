/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"
)

func TestRebuildComponents(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()

	l1, l2 := env.createTestLedgerFromGenesisBlk("ledger1"), env.createTestLedgerFromGenesisBlk("ledger2")
	dataHelper := newSampleDataHelper(t)

	dataHelper.populateLedger(l1)
	dataHelper.populateLedger(l2)

	dataHelper.verifyLedgerContent(l1)
	dataHelper.verifyLedgerContent(l2)

	t.Run("rebuild only statedb",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb)
			l1, l2 := env.openTestLedger("ledger1"), env.openTestLedger("ledger2")
			dataHelper.verifyLedgerContent(l1)
			dataHelper.verifyLedgerContent(l2)
		},
	)

	t.Run("rebuild statedb and config history",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableConfigHistory)
			l1, l2 := env.openTestLedger("ledger1"), env.openTestLedger("ledger2")
			dataHelper.verifyLedgerContent(l1)
			dataHelper.verifyLedgerContent(l2)
		},
	)

	t.Run("rebuild statedb and block index",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableBlockIndex)
			l1, l2 := env.openTestLedger("ledger1"), env.openTestLedger("ledger2")
			dataHelper.verifyLedgerContent(l1)
			dataHelper.verifyLedgerContent(l2)
		},
	)

	t.Run("rebuild statedb and historydb",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableHistoryDB)
			l1, l2 := env.openTestLedger("ledger1"), env.openTestLedger("ledger2")
			dataHelper.verifyLedgerContent(l1)
			dataHelper.verifyLedgerContent(l2)
		},
	)
}

func TestRebuildComponentsWithBTL(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedgerFromGenesisBlk("ledger1")
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 1}}

	// deploy cc1 with 'collConf'
	l.simulateDeployTx("cc1", collConf)
	l.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 2.
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1") // (key1 would never expire)
		s.setPvtdata("cc1", "coll2", "key2", "value2") // (key2 would expire at block 4)
	})
	blk2 := l.cutBlockAndCommitLegacy()

	// After commit of block 2
	l.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	l.verifyBlockAndPvtDataSameAs(2, blk2)             // key1 and key2 should still exist in the pvtdata storage

	// commit 2 more blocks with some random key/vals

	for i := 0; i < 2; i++ {
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
			s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
		})
		l.cutBlockAndCommitLegacy()
	}

	// After commit of block 4
	l.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	// rebuild statedb and bookkeeper
	env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableBookkeeper)

	l = env.openTestLedger("ledger1")
	l.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	// commit pvtdata writes in block 5.
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", "value1") // (key3 would never expire)
		s.setPvtdata("cc1", "coll2", "key4", "value2") // (key4 would expire at block 7)
	})
	l.cutBlockAndCommitLegacy()

	// commit 2 more blocks with some random key/vals
	for i := 0; i < 2; i++ {
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
			s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
		})
		l.cutBlockAndCommitLegacy()
	}

	// After commit of block 7
	l.verifyPvtState("cc1", "coll1", "key3", "value1")                  // key3 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key4", "")                        // key4 should have been purged from the state
	l.verifyBlockAndPvtData(5, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key3", "value1") // key3 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}
