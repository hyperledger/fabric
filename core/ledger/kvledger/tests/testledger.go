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
	lgr    ledger.PeerLedger
	lgrid  string
	assert *require.Assertions
}

// createTestLedger creates a new ledger and retruns a 'testhelper' for the ledger
func (env *env) createTestLedger(id string, t *testing.T) *testLedger {
	genesisBlk, err := constructTestGenesisBlock(id)
	require.NoError(t, err)
	lgr, err := env.ledgerMgr.CreateLedger(id, genesisBlk)
	require.NoError(t, err)
	client, committer, verifier := newClient(lgr, id, t), newCommitter(lgr, t), newVerifier(lgr, t)
	return &testLedger{client, committer, verifier, lgr, id, require.New(t)}
}

// openTestLedger opens an existing ledger and retruns a 'testhelper' for the ledger
func (env *env) openTestLedger(id string, t *testing.T) *testLedger {
	lgr, err := env.ledgerMgr.OpenLedger(id)
	require.NoError(t, err)
	client, committer, verifier := newClient(lgr, id, t), newCommitter(lgr, t), newVerifier(lgr, t)
	return &testLedger{client, committer, verifier, lgr, id, require.New(t)}
}

// cutBlockAndCommitLegacy gathers all the transactions simulated by the test code (by calling
// the functions available in the 'client') and cuts the next block and commits to the ledger
func (h *testLedger) cutBlockAndCommitLegacy() *ledger.BlockAndPvtData {
	defer func() {
		h.simulatedTrans = nil
		h.missingPvtData = make(ledger.TxMissingPvtData)
	}()
	return h.committer.cutBlockAndCommitLegacy(h.simulatedTrans, h.missingPvtData)
}

func (h *testLedger) cutBlockAndCommitExpectError() *ledger.BlockAndPvtData {
	defer func() {
		h.simulatedTrans = nil
		h.missingPvtData = make(ledger.TxMissingPvtData)
	}()
	return h.committer.cutBlockAndCommitExpectError(h.simulatedTrans, h.missingPvtData)
}

func (h *testLedger) commitPvtDataOfOldBlocks(blocksPvtData []*ledger.ReconciledPvtdata, unreconciled ledger.MissingPvtDataInfo) ([]*ledger.PvtdataHashMismatch, error) {
	return h.lgr.CommitPvtDataOfOldBlocks(blocksPvtData, unreconciled)
}

// assertError is a helper function that can be called as assertError(f()) where 'f' is some other function
// this function assumes that the last return type of function 'f' is of type 'error'
func (h *testLedger) assertError(output ...interface{}) {
	lastParam := output[len(output)-1]
	require.NotNil(h.t, lastParam)
	h.assert.Error(lastParam.(error))
}

// assertNoError see comment on function 'assertError'
func (h *testLedger) assertNoError(output ...interface{}) {
	h.assert.Nil(output[len(output)-1])
}
