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

// testLedger embeds (1) a client, (2) a committer and (3) a verifier, all three operate on
// a ledger instance and add helping/reusable functionality on top of ledger apis that helps
// in avoiding the repeation in the actual tests code.
// the 'client' adds value to the simulation relation apis, the 'committer' helps in cutting the
// next block and committing the block, and finally, the verifier helps in veryfying that the
// ledger apis returns correct values based on the blocks submitted
type testLedger struct {
	*client
	*committer
	*verifier

	lgrid  string
	config *ledger.Config
	lgr    ledger.PeerLedger

	t *testing.T
}

// createTestLedgerFromGenesisBlk creates a new ledger and retruns a 'testhelper' for the ledger
func (env *env) createTestLedgerFromGenesisBlk(id string) *testLedger {
	t := env.t
	genesisBlk, err := constructTestGenesisBlock(id)
	require.NoError(t, err)
	lgr, err := env.ledgerMgr.CreateLedger(id, genesisBlk)
	require.NoError(t, err)
	return &testLedger{
		client:    newClient(lgr, id, t),
		committer: newCommitter(lgr, t),
		verifier:  newVerifier(lgr, t),
		lgrid:     id,
		lgr:       lgr,
		config:    env.initializer.Config,
		t:         t,
	}
}

func (env *env) createTestLedgerFromSnapshot(snapshotDir string) *testLedger {
	t := env.t
	var lgr ledger.PeerLedger
	var lgrID string
	require.NoError(
		env.t,
		env.ledgerMgr.CreateLedgerFromSnapshot(
			snapshotDir,
			func(l ledger.PeerLedger, id string) {
				lgr = l
				lgrID = id
			},
		),
	)
	require.Eventually(
		env.t,
		func() bool {
			status := env.ledgerMgr.JoinBySnapshotStatus()
			return !status.InProgress && status.BootstrappingSnapshotDir == ""
		},
		time.Minute,
		100*time.Microsecond,
	)
	require.NotNil(env.t, lgr)

	return &testLedger{
		client:    newClient(lgr, lgrID, t),
		committer: newCommitter(lgr, t),
		verifier:  newVerifier(lgr, t),
		lgrid:     lgrID,
		lgr:       lgr,
		config:    env.initializer.Config,
		t:         t,
	}
}

// openTestLedger opens an existing ledger and retruns a 'testhelper' for the ledger
func (env *env) openTestLedger(id string) *testLedger {
	t := env.t
	lgr, err := env.ledgerMgr.OpenLedger(id)
	require.NoError(t, err)
	return &testLedger{
		client:    newClient(lgr, id, t),
		committer: newCommitter(lgr, t),
		verifier:  newVerifier(lgr, t),
		lgrid:     id,
		lgr:       lgr,
		config:    env.initializer.Config,
		t:         t,
	}
}

// cutBlockAndCommitLegacy gathers all the transactions simulated by the test code (by calling
// the functions available in the 'client') and cuts the next block and commits to the ledger
func (l *testLedger) cutBlockAndCommitLegacy() *ledger.BlockAndPvtData {
	defer func() {
		l.simulatedTrans = nil
		l.missingPvtData = make(ledger.TxMissingPvtData)
	}()
	return l.committer.cutBlockAndCommitLegacy(l.simulatedTrans, l.missingPvtData)
}

func (l *testLedger) cutBlockAndCommitExpectError() *ledger.BlockAndPvtData {
	defer func() {
		l.simulatedTrans = nil
		l.missingPvtData = make(ledger.TxMissingPvtData)
	}()
	return l.committer.cutBlockAndCommitExpectError(l.simulatedTrans, l.missingPvtData)
}

func (l *testLedger) commitPvtDataOfOldBlocks(blocksPvtData []*ledger.ReconciledPvtdata, unreconciled ledger.MissingPvtDataInfo) ([]*ledger.PvtdataHashMismatch, error) {
	return l.lgr.CommitPvtDataOfOldBlocks(blocksPvtData, unreconciled)
}

func (l *testLedger) generateSnapshot() string {
	bcInfo, err := l.lgr.GetBlockchainInfo()
	require.NoError(l.t, err)
	blockNum := bcInfo.Height - 1
	require.NoError(l.t, l.lgr.SubmitSnapshotRequest(blockNum))
	require.Eventually(l.t,
		func() bool {
			requests, err := l.lgr.PendingSnapshotRequests()
			require.NoError(l.t, err)
			return len(requests) == 0
		},
		time.Minute,
		100*time.Millisecond,
	)
	return kvledger.SnapshotDirForLedgerBlockNum(l.config.SnapshotsConfig.RootDir, l.lgrID, blockNum)
}
