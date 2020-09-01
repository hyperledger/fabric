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

	h1, h2 := env.newTestHelperCreateLgr("ledger1", t), env.newTestHelperCreateLgr("ledger2", t)
	dataHelper := newSampleDataHelper(t)

	dataHelper.populateLedger(h1)
	dataHelper.populateLedger(h2)

	dataHelper.verifyLedgerContent(h1)
	dataHelper.verifyLedgerContent(h2)

	t.Run("rebuild only statedb",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb)
			h1, h2 := env.newTestHelperOpenLgr("ledger1", t), env.newTestHelperOpenLgr("ledger2", t)
			dataHelper.verifyLedgerContent(h1)
			dataHelper.verifyLedgerContent(h2)
		},
	)

	t.Run("rebuild statedb and config history",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableConfigHistory)
			h1, h2 := env.newTestHelperOpenLgr("ledger1", t), env.newTestHelperOpenLgr("ledger2", t)
			dataHelper.verifyLedgerContent(h1)
			dataHelper.verifyLedgerContent(h2)
		},
	)

	t.Run("rebuild statedb and block index",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableBlockIndex)
			h1, h2 := env.newTestHelperOpenLgr("ledger1", t), env.newTestHelperOpenLgr("ledger2", t)
			dataHelper.verifyLedgerContent(h1)
			dataHelper.verifyLedgerContent(h2)
		},
	)

	t.Run("rebuild statedb and historydb",
		func(t *testing.T) {
			env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableHistoryDB)
			h1, h2 := env.newTestHelperOpenLgr("ledger1", t), env.newTestHelperOpenLgr("ledger2", t)
			dataHelper.verifyLedgerContent(h1)
			dataHelper.verifyLedgerContent(h2)
		},
	)
}

func TestRebuildComponentsWithBTL(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	h := env.newTestHelperCreateLgr("ledger1", t)
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 1}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	h.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 2.
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1") // (key1 would never expire)
		s.setPvtdata("cc1", "coll2", "key2", "value2") // (key2 would expire at block 4)
	})
	blk2 := h.cutBlockAndCommitLegacy()

	// After commit of block 2
	h.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	h.verifyBlockAndPvtDataSameAs(2, blk2)             // key1 and key2 should still exist in the pvtdata storage

	// commit 2 more blocks with some random key/vals

	for i := 0; i < 2; i++ {
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
			s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
		})
		h.cutBlockAndCommitLegacy()
	}

	// After commit of block 4
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	// rebuild statedb and bookkeeper
	env.closeAllLedgersAndRemoveDirContents(rebuildableStatedb | rebuildableBookkeeper)

	h = env.newTestHelperOpenLgr("ledger1", t)
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	// commit pvtdata writes in block 5.
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", "value1") // (key3 would never expire)
		s.setPvtdata("cc1", "coll2", "key4", "value2") // (key4 would expire at block 7)
	})
	h.cutBlockAndCommitLegacy()

	// commit 2 more blocks with some random key/vals
	for i := 0; i < 2; i++ {
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
			s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
		})
		h.cutBlockAndCommitLegacy()
	}

	// After commit of block 7
	h.verifyPvtState("cc1", "coll1", "key3", "value1")                  // key3 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key4", "")                        // key4 should have been purged from the state
	h.verifyBlockAndPvtData(5, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key3", "value1") // key3 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}
