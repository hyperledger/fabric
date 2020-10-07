/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"
	"time"

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

	h := env.newTestHelperCreateLgr("testLedger", t)
	// populate creates 8 blocks
	dataHelper.populateLedger(h)
	dataHelper.verifyLedgerContent(h)
	bcInfo, err := h.lgr.GetBlockchainInfo()
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

	h = env.newTestHelperOpenLgr("testLedger", t)
	h.verifyLedgerHeight(targetBlockNum + 1)
	targetBlockNumIndex := targetBlockNum - 1
	for _, b := range dataHelper.submittedData["testLedger"].Blocks[targetBlockNumIndex+1:] {
		// if the pvtData is already present in the pvtdata store, the ledger (during commit) should be
		// able to fetch them if not passed along with the block.
		require.NoError(t, h.lgr.CommitLegacy(b, &ledger.CommitOptions{FetchPvtDataFromLedger: true}))
	}
	actualBcInfo, err := h.lgr.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t, bcInfo, actualBcInfo)
	dataHelper.verifyLedgerContent(h)
	// TODO: extend integration test with BTL support for pvtData. FAB-15704
}

func TestRollbackKVLedgerWithBTL(t *testing.T) {
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

	// commit block 5 with some random key/vals
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	h.cutBlockAndCommitLegacy()
	env.closeLedgerMgmt()

	// rebuild statedb and bookkeeper
	err := kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "ledger1", 4)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB
	env.verifyRebuilableDirEmpty(rebuildable)

	env.initLedgerMgmt()
	h = env.newTestHelperOpenLgr("ledger1", t)
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	// commit block 5 with some random key/vals
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	h.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 6.
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", "value1") // (key3 would never expire)
		s.setPvtdata("cc1", "coll2", "key4", "value2") // (key4 would expire at block 8)
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

	// After commit of block 8
	h.verifyPvtState("cc1", "coll1", "key3", "value1")                  // key3 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key4", "")                        // key4 should have been purged from the state
	h.verifyBlockAndPvtData(6, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key3", "value1") // key3 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}

func TestRollbackFailIfLedgerBootstrappedFromSnapshot(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()

	// populate ledgers with sample data
	ledgerID := "testLedgerFromSnapshot"
	dataHelper := newSampleDataHelper(t)
	h := env.newTestHelperCreateLgr(ledgerID, t)
	dataHelper.populateLedger(h)
	dataHelper.verifyLedgerContent(h)
	bcInfo, err := h.lgr.GetBlockchainInfo()
	require.NoError(t, err)

	// create a sanapshot
	blockNum := bcInfo.Height - 1
	require.NoError(t, h.lgr.SubmitSnapshotRequest(blockNum))
	// wait until snapshot is generated
	snapshotGenerated := func() bool {
		requests, err := h.lgr.PendingSnapshotRequests()
		require.NoError(t, err)
		return len(requests) == 0
	}
	require.Eventually(t, snapshotGenerated, time.Minute, 100*time.Millisecond)
	snapshotDir := kvledger.SnapshotDirForLedgerBlockNum(env.initializer.Config.SnapshotsConfig.RootDir, ledgerID, blockNum)
	env.closeLedgerMgmt()

	// bootstrap a ledger from the snapshot
	env2 := newEnv(t)
	defer env2.cleanup()
	env2.initLedgerMgmt()

	callbackCounter := 0
	callback := func(l ledger.PeerLedger, cid string) { callbackCounter++ }
	require.NoError(t, env2.ledgerMgr.CreateLedgerFromSnapshot(snapshotDir, callback))

	// wait until ledger creation is done
	ledgerCreated := func() bool {
		status := env2.ledgerMgr.JoinBySnapshotStatus()
		return !status.InProgress && status.BootstrappingSnapshotDir == ""
	}
	require.Eventually(t, ledgerCreated, time.Minute, 100*time.Millisecond)
	require.Equal(t, 1, callbackCounter)
	env2.closeLedgerMgmt()

	// rollback the ledger should fail
	err = kvledger.RollbackKVLedger(env2.initializer.Config.RootFSPath, ledgerID, 1)
	require.EqualError(t, err, "cannot rollback channel [testLedgerFromSnapshot] because it was bootstrapped from a snapshot")
}
