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
	"github.com/stretchr/testify/require"
)

func TestResetRollbackRebuildFailsIfAnyLedgerBootstrappedFromSnapshot(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()

	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	l := env.createTestLedger("ledger_from_snapshot")
	dataHelper.populateLedger(l)
	dataHelper.verifyLedgerContent(l)
	bcInfo, err := l.lgr.GetBlockchainInfo()
	require.NoError(t, err)

	// create a sanapshot
	blockNum := bcInfo.Height - 1
	require.NoError(t, l.lgr.SubmitSnapshotRequest(blockNum))
	// wait until snapshot is generated
	snapshotGenerated := func() bool {
		requests, err := l.lgr.PendingSnapshotRequests()
		require.NoError(t, err)
		return len(requests) == 0
	}
	require.Eventually(t, snapshotGenerated, time.Minute, 100*time.Millisecond)
	snapshotDir := kvledger.SnapshotDirForLedgerBlockNum(env.initializer.Config.SnapshotsConfig.RootDir, "ledger_from_snapshot", blockNum)
	env.closeLedgerMgmt()

	// creates a new env with multiple ledgers, some from genesis block and some from a snapshot
	env2 := newEnv(t)
	defer env2.cleanup()
	env2.initLedgerMgmt()

	env2.createTestLedger("ledger_from_genesis_block")

	callbackCounter := 0
	callback := func(l ledger.PeerLedger, cid string) { callbackCounter++ }
	require.NoError(t, env2.ledgerMgr.CreateLedgerFromSnapshot(snapshotDir, callback))

	// wait until ledger creation is done
	ledgerCreated := func() bool {
		status := env2.ledgerMgr.JoinBySnapshotStatus()
		return !status.InProgress && status.BootstrappingSnapshotDir == ""
	}
	require.Eventually(t, ledgerCreated, time.Minute, 100*time.Microsecond)
	require.Equal(t, 1, callbackCounter)
	env2.closeLedgerMgmt()

	config := env2.initializer.Config
	rootFSPath := config.RootFSPath

	t.Run("reset_fails", func(t *testing.T) {
		err = kvledger.ResetAllKVLedgers(rootFSPath)
		require.EqualError(t, err, "cannot reset channels because the peer contains channel(s) [ledger_from_snapshot] that were bootstrapped from snapshot")
	})

	t.Run("rollback_a_channel_fails", func(t *testing.T) {
		err = kvledger.RollbackKVLedger(rootFSPath, "ledger_from_genesis_block", 1)
		require.EqualError(t, err, "cannot rollback any channel because the peer contains channel(s) [ledger_from_snapshot] that were bootstrapped from snapshot")
	})

	t.Run("rebuild_fails", func(t *testing.T) {
		err = kvledger.RebuildDBs(config)
		require.EqualError(t, err, "cannot rebuild databases because the peer contains channel(s) [ledger_from_snapshot] that were bootstrapped from snapshot")
	})

	t.Run("manually_dropping_dbs_returns_error_on_start", func(t *testing.T) {
		env2.closeAllLedgersAndRemoveDirContents(rebuildableStatedb)
		_, err := env2.ledgerMgr.OpenLedger("ledger_from_snapshot")
		require.EqualError(t, err, "recovery for DB [state] not possible. Ledger [ledger_from_snapshot] is created from a snapshot. Last block in snapshot = [8], DB needs block [0] onward")
	})
}
