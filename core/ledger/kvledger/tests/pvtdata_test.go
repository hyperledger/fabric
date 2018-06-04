/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"
)

func TestTxWithMissingPvtdata(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)

	collConf := []*collConf{{name: "coll1"}}

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

	collConf := []*collConf{{name: "coll1"}}

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
