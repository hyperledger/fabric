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
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestRollbackKVLedger(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	var blockchainsInfo []*common.BlockchainInfo

	h := newTestHelperCreateLgr("testLedger", t)
	// populate creates 8 blocks
	dataHelper.populateLedger(h)
	dataHelper.verifyLedgerContent(h)
	bcInfo, err := h.lgr.GetBlockchainInfo()
	assert.NoError(t, err)
	blockchainsInfo = append(blockchainsInfo, bcInfo)
	closeLedgerMgmt()

	// Rollback the testLedger (invalid rollback params)
	err = kvledger.RollbackKVLedger("noLedger", 0)
	assert.Equal(t, "ledgerID [noLedger] does not exist", err.Error())
	err = kvledger.RollbackKVLedger("testLedger", bcInfo.Height)
	expectedErr := fmt.Sprintf("target block number [%d] should be less than the biggest block number [%d]",
		bcInfo.Height, bcInfo.Height-1)
	assert.Equal(t, expectedErr, err.Error())

	// Rollback the testLedger (valid rollback params)
	targetBlockNum := bcInfo.Height - 3
	err = kvledger.RollbackKVLedger("testLedger", targetBlockNum)
	assert.NoError(t, err)
	rebuildable := rebuildableStatedb + rebuildableBookkeeper + rebuildableConfigHistory + rebuildableHistoryDB
	env.verifyRebuilableDoesNotExist(rebuildable)
	initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight()
	assert.Equal(t, bcInfo.Height, preResetHt["testLedger"])
	t.Logf("preResetHt = %#v", preResetHt)

	h = newTestHelperOpenLgr("testLedger", t)
	h.verifyLedgerHeight(targetBlockNum + 1)
	targetBlockNumIndex := targetBlockNum - 1
	for _, b := range dataHelper.submittedData["testLedger"].Blocks[targetBlockNumIndex+1:] {
		// if the pvtData is already present in the pvtdata store, the ledger (during commit) should be
		// able to fetch them if not passed along with the block.
		assert.NoError(t, h.lgr.CommitWithPvtData(b, &ledger.CommitOptions{FetchPvtDataFromLedger: true}))
	}
	actualBcInfo, err := h.lgr.GetBlockchainInfo()
	assert.Equal(t, bcInfo, actualBcInfo)
	dataHelper.verifyLedgerContent(h)
	// TODO: extend integration test with BTL support for pvtData. FAB-15704
}

func TestRollbackKVLedgerWithBTL(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 1}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	h.cutBlockAndCommitWithPvtdata()

	// commit pvtdata writes in block 2.
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1") // (key1 would never expire)
		s.setPvtdata("cc1", "coll2", "key2", "value2") // (key2 would expire at block 4)
	})
	blk2 := h.cutBlockAndCommitWithPvtdata()

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
		h.cutBlockAndCommitWithPvtdata()
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
	h.cutBlockAndCommitWithPvtdata()
	closeLedgerMgmt()

	// rebuild statedb and bookkeeper
	err := kvledger.RollbackKVLedger("ledger1", 4)
	assert.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB
	env.verifyRebuilableDoesNotExist(rebuildable)

	initLedgerMgmt()
	h = newTestHelperOpenLgr("ledger1", t)
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
	h.cutBlockAndCommitWithPvtdata()

	// commit pvtdata writes in block 6.
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", "value1") // (key3 would never expire)
		s.setPvtdata("cc1", "coll2", "key4", "value2") // (key4 would expire at block 8)
	})
	h.cutBlockAndCommitWithPvtdata()

	// commit 2 more blocks with some random key/vals
	for i := 0; i < 2; i++ {
		h.simulateDataTx("", func(s *simulator) {
			s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
			s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
		})
		h.cutBlockAndCommitWithPvtdata()
	}

	// After commit of block 8
	h.verifyPvtState("cc1", "coll1", "key3", "value1")                  // key3 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key4", "")                        // key4 should have been purged from the state
	h.verifyBlockAndPvtData(6, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key3", "value1") // key3 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}
