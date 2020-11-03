/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
)

func TestMissingCollConfig(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedger("ledger1", t)

	collConf := []*collConf{{name: "coll1", btl: 5}}

	// deploy cc1 with no coll config
	l.simulateDeployTx("cc1", nil)
	l.cutBlockAndCommitLegacy()

	// pvt data operations should give error as no collection config defined
	l.simulateDataTx("", func(s *simulator) {
		l.assertError(s.GetPrivateData("cc1", "coll1", "key"))
		l.assertError(s.SetPrivateData("cc1", "coll1", "key", []byte("value")))
		l.assertError(s.DeletePrivateData("cc1", "coll1", "key"))
	})

	// upgrade cc1 (add collConf)
	l.simulateUpgradeTx("cc1", collConf)
	l.cutBlockAndCommitLegacy()

	// operations on coll1 should not give error
	// operations on coll2 should give error (because, only coll1 is defined in collConf)
	l.simulateDataTx("", func(s *simulator) {
		l.assertNoError(s.GetPrivateData("cc1", "coll1", "key1"))
		l.assertNoError(s.SetPrivateData("cc1", "coll1", "key2", []byte("value")))
		l.assertNoError(s.DeletePrivateData("cc1", "coll1", "key3"))
		l.assertError(s.GetPrivateData("cc1", "coll2", "key"))
		l.assertError(s.SetPrivateData("cc1", "coll2", "key", []byte("value")))
		l.assertError(s.DeletePrivateData("cc1", "coll2", "key"))
	})
}

func TestTxWithMissingPvtdata(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedger("ledger1", t)

	collConf := []*collConf{{name: "coll1", btl: 5}}

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

	l.causeMissingPvtData(0)
	blk2 := l.cutBlockAndCommitLegacy()

	l.verifyPvtState("cc1", "coll1", "key2", "value2") // key2 should have been committed
	l.simulateDataTx("", func(s *simulator) {
		l.assertError(s.GetPrivateData("cc1", "coll1", "key1")) // key1 would be stale with respect to hashed version
	})

	// verify missing pvtdata info
	l.verifyBlockAndPvtDataSameAs(2, blk2)
	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	expectedMissingPvtDataInfo.Add(2, 0, "cc1", "coll1")
	l.verifyMissingPvtDataSameAs(2, expectedMissingPvtDataInfo)

	// another data tx overwritting key1
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "newvalue1")
	})
	blk3 := l.cutBlockAndCommitLegacy()
	l.verifyPvtState("cc1", "coll1", "key1", "newvalue1") // key1 should have been committed with new value
	l.verifyBlockAndPvtDataSameAs(2, blk2)
	l.verifyBlockAndPvtDataSameAs(3, blk3)
}

func TestTxWithWrongPvtdata(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedger("ledger1", t)

	collConf := []*collConf{{name: "coll1", btl: 5}}

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
	l.simulatedTrans[0].Pvtws = l.simulatedTrans[1].Pvtws // put wrong pvt writeset in first simulation
	// the commit of block is rejected if the hash of collection present in the block does not match with the pvtdata
	l.cutBlockAndCommitExpectError()
	l.verifyPvtState("cc1", "coll1", "key2", "")
}

func TestBTL(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedger("ledger1", t)
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 5}}

	// deploy cc1 with 'collConf'
	l.simulateDeployTx("cc1", collConf)
	l.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 2.
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1") // (key1 would never expire)
		s.setPvtdata("cc1", "coll2", "key2", "value2") // (key2 would expire at block 8)
	})
	blk2 := l.cutBlockAndCommitLegacy()

	// commit 5 more blocks with some random key/vals
	for i := 0; i < 5; i++ {
		l.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
			s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
		})
		l.cutBlockAndCommitLegacy()
	}

	// After commit of block 7
	l.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	l.verifyBlockAndPvtDataSameAs(2, blk2)             // key1 and key2 should still exist in the pvtdata storage

	// commit block 8 with some random key/vals
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	l.cutBlockAndCommitLegacy()

	// After commit of block 8
	l.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}
