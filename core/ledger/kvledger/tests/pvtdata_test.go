/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"
)

func TestMissingCollConfig(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)

	collConf := []*collConf{{name: "coll1", btl: 5}}

	// deploy cc1 with no coll config
	h.simulateDeployTx("cc1", nil)
	h.cutBlockAndCommitWithPvtdata()

	// pvt data operations should give error as no collection config defined
	h.simulateDataTx("", func(s *simulator) {
		h.assertError(s.GetPrivateData("cc1", "coll1", "key"))
		h.assertError(s.SetPrivateData("cc1", "coll1", "key", []byte("value")))
		h.assertError(s.DeletePrivateData("cc1", "coll1", "key"))
	})

	// upgrade cc1 (add collConf)
	h.simulateUpgradeTx("cc1", collConf)
	h.cutBlockAndCommitWithPvtdata()

	// operations on coll1 should not give error
	// operations on coll2 should give error (because, only coll1 is defined in collConf)
	h.simulateDataTx("", func(s *simulator) {
		h.assertNoError(s.GetPrivateData("cc1", "coll1", "key1"))
		h.assertNoError(s.SetPrivateData("cc1", "coll1", "key2", []byte("value")))
		h.assertNoError(s.DeletePrivateData("cc1", "coll1", "key3"))
		h.assertError(s.GetPrivateData("cc1", "coll2", "key"))
		h.assertError(s.SetPrivateData("cc1", "coll2", "key", []byte("value")))
		h.assertError(s.DeletePrivateData("cc1", "coll2", "key"))
	})
}

func TestTxWithMissingPvtdata(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)

	collConf := []*collConf{{name: "coll1", btl: 5}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	h.cutBlockAndCommitWithPvtdata()

	// pvtdata simulation
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1")
	})
	// another pvtdata simulation
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key2", "value2")
	})
	h.simulatedTrans[0].Pvtws = nil // drop pvt writeset from first simulation
	blk2 := h.cutBlockAndCommitWithPvtdata()

	h.verifyPvtState("cc1", "coll1", "key2", "value2") // key2 should have been committed
	h.simulateDataTx("", func(s *simulator) {
		h.assertError(s.GetPrivateData("cc1", "coll1", "key1")) // key1 would be stale with respect to hashed version
	})

	// another data tx overwritting key1
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "newvalue1")
	})
	blk3 := h.cutBlockAndCommitWithPvtdata()
	h.verifyPvtState("cc1", "coll1", "key1", "newvalue1") // key1 should have been committed with new value
	h.verifyBlockAndPvtDataSameAs(2, blk2)
	h.verifyBlockAndPvtDataSameAs(3, blk3)
}

func TestTxWithWrongPvtdata(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)

	collConf := []*collConf{{name: "coll1", btl: 5}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	h.cutBlockAndCommitWithPvtdata()

	// pvtdata simulation
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1")
	})
	// another pvtdata simulation
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key2", "value2")
	})
	h.simulatedTrans[0].Pvtws = h.simulatedTrans[1].Pvtws // put wrong pvt writeset in first simulation
	// the commit of block is rejected if the hash of collection present in the block does not match with the pvtdata
	h.cutBlockAndCommitExpectError()
	h.verifyPvtState("cc1", "coll1", "key2", "")
}

func TestBTL(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 5}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	h.cutBlockAndCommitWithPvtdata()

	// commit pvtdata writes in block 2.
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1") // (key1 would never expire)
		s.setPvtdata("cc1", "coll2", "key2", "value2") // (key2 would expire at block 8)
	})
	blk2 := h.cutBlockAndCommitWithPvtdata()

	// commit 5 more blocks with some random key/vals
	for i := 0; i < 5; i++ {
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
			s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
		})
		h.cutBlockAndCommitWithPvtdata()
	}

	// After commit of block 7
	h.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	h.verifyBlockAndPvtDataSameAs(2, blk2)             // key1 and key2 should still exist in the pvtdata storage

	// commit block 8 with some random key/vals
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	h.cutBlockAndCommitWithPvtdata()

	// After commit of block 8
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}
