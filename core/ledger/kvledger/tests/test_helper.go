/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/stretchr/testify/assert"
)

// testhelper embeds (1) a client, (2) a committer and (3) a verifier, all three operate on
// a ledger instance and add helping/resuable functionality on top of ledger apis that helps
// in avoiding the repeation in the actual tests code.
// the 'client' adds value to the simulation relation apis, the 'committer' helps in cutting the
// next block and committing the block, and finally, the verifier helps in veryfying that the
// ledger apis returns correct values based on the blocks submitted
type testhelper struct {
	*client
	*committer
	*verifier
	lgr    ledger.PeerLedger
	lgrid  string
	assert *assert.Assertions
}

// newTestHelperCreateLgr creates a new ledger and retruns a 'testhelper' for the ledger
func newTestHelperCreateLgr(id string, t *testing.T) *testhelper {
	genesisBlk, err := constructTestGenesisBlock(id)
	assert.NoError(t, err)
	lgr, err := ledgermgmt.CreateLedger(genesisBlk)
	assert.NoError(t, err)
	client, committer, verifier := newClient(lgr, t), newCommitter(lgr, t), newVerifier(lgr, t)
	return &testhelper{client, committer, verifier, lgr, id, assert.New(t)}
}

// newTestHelperOpenLgr opens an existing ledger and retruns a 'testhelper' for the ledger
func newTestHelperOpenLgr(id string, t *testing.T) *testhelper {
	lgr, err := ledgermgmt.OpenLedger(id)
	assert.NoError(t, err)
	client, committer, verifier := newClient(lgr, t), newCommitter(lgr, t), newVerifier(lgr, t)
	return &testhelper{client, committer, verifier, lgr, id, assert.New(t)}
}

// cutBlockAndCommitWithPvtdata gathers all the transactions simulated by the test code (by calling
// the functions available in the 'client') and cuts the next block and commits to the ledger
func (h *testhelper) cutBlockAndCommitWithPvtdata() *ledger.BlockAndPvtData {
	defer func() { h.simulatedTrans = nil }()
	return h.committer.cutBlockAndCommitWithPvtdata(h.simulatedTrans...)
}

func (h *testhelper) cutBlockAndCommitExpectError() (*ledger.BlockAndPvtData, error) {
	defer func() { h.simulatedTrans = nil }()
	return h.committer.cutBlockAndCommitExpectError(h.simulatedTrans...)
}

// assertError is a helper function that can be called as assertError(f()) where 'f' is some other function
// this function assumes that the last return type of function 'f' is of type 'error'
func (h *testhelper) assertError(output ...interface{}) {
	lastParam := output[len(output)-1]
	assert.NotNil(h.t, lastParam)
	h.assert.Error(lastParam.(error))
}

// assertNoError see comment on function 'assertError'
func (h *testhelper) assertNoError(output ...interface{}) {
	h.assert.Nil(output[len(output)-1])
}
