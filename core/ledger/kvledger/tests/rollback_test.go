/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/stretchr/testify/require"
)

func TestRollbackKVLedger(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)

	l := env.createTestLedgerFromGenesisBlk("testLedger")
	// populate creates 8 blocks
	dataHelper.populateLedger(l)
	dataHelper.verifyLedgerContent(l)
	bcInfo, err := l.lgr.GetBlockchainInfo()
	require.NoError(t, err)
	env.closeLedgerMgmt()

	// Rollback the testLedger (invalid rollback params)
	err = kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "noLedger", 0)
	require.Equal(t, "ledgerID [noLedger] does not exist", err.Error())
	err = kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "testLedger", bcInfo.Height)
	expectedErr := fmt.Sprintf("target block number [%d] should be less than the biggest block number [%d]",
		bcInfo.Height, bcInfo.Height-1)
	require.Equal(t, expectedErr, err.Error())

	// Rollback the testLedger (valid rollback params)
	targetBlockNum := bcInfo.Height - 3
	err = kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "testLedger", targetBlockNum)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb + rebuildableBookkeeper + rebuildableConfigHistory + rebuildableHistoryDB
	env.verifyRebuilableDirEmpty(rebuildable)
	env.initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, []string{"testLedger"})
	require.NoError(t, err)
	require.Equal(t, bcInfo.Height, preResetHt["testLedger"])
	t.Logf("preResetHt = %#v", preResetHt)

	l = env.openTestLedger("testLedger")
	l.verifyLedgerHeight(targetBlockNum + 1)
	targetBlockNumIndex := targetBlockNum - 1
	for _, b := range dataHelper.submittedData["testLedger"].Blocks[targetBlockNumIndex+1:] {
		// if the pvtData is already present in the pvtdata store, the ledger (during commit) should be
		// able to fetch them if not passed along with the block.
		require.NoError(t, l.lgr.CommitLegacy(b, &ledger.CommitOptions{FetchPvtDataFromLedger: true}))
	}
	actualBcInfo, err := l.lgr.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, bcInfo, actualBcInfo)
	dataHelper.verifyLedgerContent(l)
	// TODO: extend integration test with BTL support for pvtData. FAB-15704
}

func TestRollbackKVLedgerWithBTL(t *testing.T) {
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

	// commit block 5 with some random key/vals
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	l.cutBlockAndCommitLegacy()
	env.closeLedgerMgmt()

	// rebuild statedb and bookkeeper
	err := kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "ledger1", 4)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB
	env.verifyRebuilableDirEmpty(rebuildable)

	env.initLedgerMgmt()
	l = env.openTestLedger("ledger1")
	l.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	// commit block 5 with some random key/vals
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	l.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 6.
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", "value1") // (key3 would never expire)
		s.setPvtdata("cc1", "coll2", "key4", "value2") // (key4 would expire at block 8)
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

	// After commit of block 8
	l.verifyPvtState("cc1", "coll1", "key3", "value1")                  // key3 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key4", "")                        // key4 should have been purged from the state
	l.verifyBlockAndPvtData(6, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key3", "value1") // key3 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}

func TestRollbackKVLedgerErrorCases(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	env.closeLedgerMgmt()

	err := kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "non-existing-ledger", 4)
	require.EqualError(t, err, "ledgerID [non-existing-ledger] does not exist")
}
