/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/stretchr/testify/assert"
)

func TestGetMissingPvtDataAfterRollback(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)

	collConf := []*collConf{{name: "coll1", btl: 5}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	h.cutBlockAndCommitWithPvtdata()

	// pvtdata simulation
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1")
	})
	// another pvtdata simulation
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key2", "value2")
	})

	h.causeMissingPvtData(0)
	blk2 := h.cutBlockAndCommitWithPvtdata()

	h.verifyPvtState("cc1", "coll1", "key2", "value2") // key2 should have been committed
	h.simulateDataTx("", func(s *simulator) {
		h.assertError(s.GetPrivateData("cc1", "coll1", "key1")) // key1 would be stale with respect to hashed version
	})

	// verify missing pvtdata info
	h.verifyBlockAndPvtDataSameAs(2, blk2)
	expectedMissingPvtDataInfo := make(ledger.MissingPvtDataInfo)
	expectedMissingPvtDataInfo.Add(2, 0, "cc1", "coll1")
	h.verifyMissingPvtDataSameAs(2, expectedMissingPvtDataInfo)

	// commit block 3
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", "value2")
	})
	blk3 := h.cutBlockAndCommitWithPvtdata()

	// commit block 4
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key3", "value2")
	})
	blk4 := h.cutBlockAndCommitWithPvtdata()

	// verify missing pvtdata info
	h.verifyMissingPvtDataSameAs(5, expectedMissingPvtDataInfo)

	// rollback ledger to block 2
	h.verifyLedgerHeight(5)
	closeLedgerMgmt()
	err := kvledger.RollbackKVLedger("ledger1", 2)
	assert.NoError(t, err)
	initLedgerMgmt()

	h = newTestHelperOpenLgr("ledger1", t)
	h.verifyLedgerHeight(3)

	// verify block & pvtdata
	h.verifyBlockAndPvtDataSameAs(2, blk2)
	// when the pvtdata store is ahead of blockstore,
	// missing pvtdata info for block 2 would not be returned.
	h.verifyMissingPvtDataSameAs(5, nil)

	// recommit block 3
	assert.NoError(t, h.lgr.CommitWithPvtData(blk3, &ledger.CommitOptions{}))
	// when the pvtdata store is ahead of blockstore,
	// missing pvtdata info for block 2 would not be returned.
	h.verifyMissingPvtDataSameAs(5, nil)

	// recommit block 4
	assert.NoError(t, h.lgr.CommitWithPvtData(blk4, &ledger.CommitOptions{}))
	// once the pvtdata store and blockstore becomes equal,
	// missing pvtdata info for block 2 would be returned.
	h.verifyMissingPvtDataSameAs(5, expectedMissingPvtDataInfo)
}
