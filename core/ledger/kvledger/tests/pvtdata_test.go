/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/stretchr/testify/require"
)

func TestMissingCollConfig(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedgerFromGenesisBlk("ledger1")

	collConf := []*collConf{{name: "coll1", btl: 5}}

	// deploy cc1 with no coll config
	l.simulateDeployTx("cc1", nil)
	l.cutBlockAndCommitLegacy()

	// pvt data operations should give error as no collection config defined
	l.simulateDataTx("", func(s *simulator) {
		expectedErr := "collection config not defined for chaincode [cc1], pass the collection configuration upon chaincode definition/instantiation"
		_, err := s.GetPrivateData("cc1", "coll1", "key")
		require.EqualError(t, err, expectedErr)

		err = s.SetPrivateData("cc1", "coll1", "key", []byte("value"))
		require.EqualError(t, err, expectedErr)

		err = s.DeletePrivateData("cc1", "coll1", "key")
		require.EqualError(t, err, expectedErr)
	})

	// upgrade cc1 (add collConf)
	l.simulateUpgradeTx("cc1", collConf)
	l.cutBlockAndCommitLegacy()

	// operations on coll1 should not give error
	// operations on coll2 should give error (because, only coll1 is defined in collConf)
	l.simulateDataTx("", func(s *simulator) {
		_, err := s.GetPrivateData("cc1", "coll1", "key1")
		require.NoError(t, err)

		err = s.SetPrivateData("cc1", "coll1", "key2", []byte("value"))
		require.NoError(t, err)

		err = s.DeletePrivateData("cc1", "coll1", "key3")
		require.NoError(t, err)

		expectedErr := "collection [coll2] not defined in the collection config for chaincode [cc1]"
		_, err = s.GetPrivateData("cc1", "coll2", "key")
		require.EqualError(t, err, expectedErr)

		err = s.SetPrivateData("cc1", "coll2", "key", []byte("value"))
		require.EqualError(t, err, expectedErr)

		err = s.DeletePrivateData("cc1", "coll2", "key")
		require.EqualError(t, err, expectedErr)
	})
}

func TestTxWithMissingPvtdata(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedgerFromGenesisBlk("ledger1")

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
		_, err := s.GetPrivateData("cc1", "coll1", "key1") // key1 would be stale with respect to hashed version
		require.EqualError(t, err, "private data matching public hash version is not available. Public hash version = {BlockNum: 2, TxNum: 0}, Private data version = <nil>")
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
	l := env.createTestLedgerFromGenesisBlk("ledger1")

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
	l := env.createTestLedgerFromGenesisBlk("ledger1")
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

func TestAppInitiatedPrivateDataPurge(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedgerFromGenesisBlk("ledger1")
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 0}}

	// deploy cc1 with 'collConf'
	l.simulateDeployTx("cc1", collConf)
	l.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 2.
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1")
		s.setPvtdata("cc1", "coll2", "key2", "value2")
		s.setPvtdata("cc1", "coll2", "key3", "value3")
	})
	l.cutBlockAndCommitLegacy()

	// Two transactions in block 3
	l.simulateDataTx("", func(s *simulator) {
		// purge key1 and key2
		s.purgePvtdata("cc1", "coll1", "key1")
		s.purgePvtdata("cc1", "coll2", "key2")
	})

	l.simulateDataTx("", func(s *simulator) {
		// set key2 to a new value, after purge
		s.setPvtdata("cc1", "coll2", "key2", "value2_new")
	})
	l.cutBlockAndCommitLegacy()

	l.verifyPvtState("cc1", "coll1", "key1", "")           // key1 should have been purged from the state
	l.verifyPvtState("cc1", "coll2", "key2", "value2_new") // key2 should be present with new value
	l.verifyPvtState("cc1", "coll2", "key3", "value3")

	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldNotContainKey("cc1", "coll1", "key1")
		r.pvtdataShouldNotContainKey("cc1", "coll2", "key2")
		r.pvtdataShouldContain(0, "cc1", "coll2", "key3", "value3")
	})

	l.verifyBlockAndPvtData(3, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 3 from pvtdata storage
		r.pvtdataShouldNotContainKey("cc1", "coll1", "key1")
		r.pvtdataShouldContain(1, "cc1", "coll2", "key2", "value2_new")
		r.pvtdataShouldNotContainKey("cc1", "coll2", "key3")
	})
}
