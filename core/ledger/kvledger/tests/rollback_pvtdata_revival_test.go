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

func TestRollbackKVLedgerPvtDataRevival(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	l := env.createTestLedgerFromGenesisBlk("ledgerRevival")
	collConf := []*collConf{{name: "coll1", btl: 1}}

	// Block 1: Deploy cc1 with coll1 (BTL=1)
	l.simulateDeployTx("cc1", collConf)
	l.cutBlockAndCommitLegacy()

	// Block 2: Commit pvt data "key1"="value1" in "coll1"
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1")
	})
	l.cutBlockAndCommitLegacy()

	// Verify key1 exists
	l.verifyPvtState("cc1", "coll1", "key1", "value1")

	// Block 3: Commit empty block. key1 should still be live (BTL=1 means live in block 2 and 3? or just 2?)
	// In TestRollbackKVLedgerWithBTL:
	// block 2 (data) -> block 3 (live) -> block 4 (purged).
	l.cutBlockAndCommitLegacy()
	// Verify key1 still exists
	l.verifyPvtState("cc1", "coll1", "key1", "value1")

	// Block 4: Commit empty block. key1 should be purged.
	l.cutBlockAndCommitLegacy()

	// Verify key1 is purged
	l.verifyPvtState("cc1", "coll1", "key1", "")

	env.closeLedgerMgmt()

	// Rollback to Block 2.
	// This makes the ledger height 2.
	// The state should reflect Block 2 (key1 present).
	// However, the pvt data for Block 2 *might* have been purged from pvtstore at Block 4 commit.
	// Let's see if Rollback handles this.
	err := kvledger.RollbackKVLedger(env.initializer.Config.RootFSPath, "ledgerRevival", 2)
	require.NoError(t, err)

	env.initLedgerMgmt()
	l = env.openTestLedger("ledgerRevival")

	// Verify key1 is present
	// If this fails, then Rollback does not restore purged private data.
	l.verifyPvtState("cc1", "coll1", "key1", "value1")
}
