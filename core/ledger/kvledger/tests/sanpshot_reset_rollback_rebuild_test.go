/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/stretchr/testify/require"
)

func TestResetRollbackRebuildFailsIfAnyLedgerBootstrappedFromSnapshot(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()

	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	l := env.createTestLedgerFromGenesisBlk("ledger-1")
	dataHelper.populateLedger(l)
	dataHelper.verifyLedgerContent(l)
	snapshotDir := l.generateSnapshot()
	env.closeLedgerMgmt()

	// creates a new env with two ledgers, one from genesis block and one from a snapshot
	env2 := newEnv(t)
	defer env2.cleanup()
	env2.initLedgerMgmt()
	env2.createTestLedgerFromGenesisBlk("ledger-2")
	env2.createTestLedgerFromSnapshot(snapshotDir)
	env2.closeLedgerMgmt()

	config := env2.initializer.Config
	rootFSPath := config.RootFSPath

	t.Run("reset_fails", func(t *testing.T) {
		err := kvledger.ResetAllKVLedgers(rootFSPath)
		require.EqualError(t, err, "cannot reset channels because the peer contains channel(s) [ledger-1] that were bootstrapped from snapshot")
	})

	t.Run("rollback_a_channel_fails", func(t *testing.T) {
		err := kvledger.RollbackKVLedger(rootFSPath, "ledger_from_genesis_block", 1)
		require.EqualError(t, err, "cannot rollback any channel because the peer contains channel(s) [ledger-1] that were bootstrapped from snapshot")
	})

	t.Run("rebuild_fails", func(t *testing.T) {
		err := kvledger.RebuildDBs(config)
		require.EqualError(t, err, "cannot rebuild databases because the peer contains channel(s) [ledger-1] that were bootstrapped from snapshot")
	})

	t.Run("manually_dropping_dbs_returns_error_on_start", func(t *testing.T) {
		env2.closeAllLedgersAndRemoveDirContents(rebuildableStatedb)
		_, err := env2.ledgerMgr.OpenLedger("ledger-1")
		require.EqualError(t, err, "recovery for DB [state] not possible. Ledger [ledger-1] is created from a snapshot. Last block in snapshot = [8], DB needs block [0] onward")
	})
}
