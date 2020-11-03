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
	lgr   ledger.PeerLedger
	lgrid string
	t     *testing.T
}

// createTestLedger creates a new ledger and retruns a 'testhelper' for the ledger
func (env *env) createTestLedger(id string) *testLedger {
	t := env.t
	genesisBlk, err := constructTestGenesisBlock(id)
	require.NoError(env.t, err)
	lgr, err := env.ledgerMgr.CreateLedger(id, genesisBlk)
	require.NoError(t, err)
	client, committer, verifier := newClient(lgr, id, t), newCommitter(lgr, t), newVerifier(lgr, t)
	return &testLedger{client, committer, verifier, lgr, id, t}
}

// openTestLedger opens an existing ledger and retruns a 'testhelper' for the ledger
func (env *env) openTestLedger(id string) *testLedger {
	t := env.t
	lgr, err := env.ledgerMgr.OpenLedger(id)
	require.NoError(t, err)
	client, committer, verifier := newClient(lgr, id, t), newCommitter(lgr, t), newVerifier(lgr, t)
	return &testLedger{client, committer, verifier, lgr, id, t}
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
