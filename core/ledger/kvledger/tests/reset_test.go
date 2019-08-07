/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

func TestResetAllLedgers(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	var genesisBlocks []*common.Block
	var blockchainsInfo []*common.BlockchainInfo

	// create ledgers and pouplate with sample data
	// Also, retrieve the genesis blocks and blockchain info for matching later
	for i := 0; i < 10; i++ {
		h := newTestHelperCreateLgr(fmt.Sprintf("ledger-%d", i), t)
		dataHelper.populateLedger(h)
		dataHelper.verifyLedgerContent(h)
		gb, err := h.lgr.GetBlockByNumber(0)
		assert.NoError(t, err)
		genesisBlocks = append(genesisBlocks, gb)
		bcInfo, err := h.lgr.GetBlockchainInfo()
		assert.NoError(t, err)
		blockchainsInfo = append(blockchainsInfo, bcInfo)
	}
	closeLedgerMgmt()

	// Reset All kv ledgers
	err := kvledger.ResetAllKVLedgers()
	assert.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB | rebuildableBlockIndex
	env.verifyRebuilableDoesNotExist(rebuildable)
	initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight()
	t.Logf("preResetHt = %#v", preResetHt)
	// open all the ledgers again and verify that
	// - initial height==1
	// - compare the genesis block from the one before reset
	// - resubmit the previous committed blocks from block number 1 onwards
	//   and final blockchainInfo and ledger state same as before reset
	for i := 0; i < 10; i++ {
		ledgerID := fmt.Sprintf("ledger-%d", i)
		h := newTestHelperOpenLgr(ledgerID, t)
		h.verifyLedgerHeight(1)
		assert.Equal(t, blockchainsInfo[i].Height, preResetHt[ledgerID])
		gb, err := h.lgr.GetBlockByNumber(0)
		assert.NoError(t, err)
		assert.Equal(t, genesisBlocks[i], gb)
		for _, b := range dataHelper.submittedData[ledgerID].Blocks {
			assert.NoError(t, h.lgr.CommitWithPvtData(b, &ledger.CommitOptions{}))
		}
		bcInfo, err := h.lgr.GetBlockchainInfo()
		assert.NoError(t, err)
		assert.Equal(t, blockchainsInfo[i], bcInfo)
		dataHelper.verifyLedgerContent(h)
	}

	assert.NoError(t, kvledger.ClearPreResetHeight())
	preResetHt, err = kvledger.LoadPreResetHeight()
	assert.NoError(t, err)
	assert.Len(t, preResetHt, 0)
}

func TestResetAllLedgersWithBTL(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	h := newTestHelperCreateLgr("ledger1", t)
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 1}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	blk1 := h.cutBlockAndCommitWithPvtdata()

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

	// After commit of block 3
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	blk3 := h.cutBlockAndCommitWithPvtdata()

	// After commit of block 4
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	blk4 := h.cutBlockAndCommitWithPvtdata()

	// After commit of block 4
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	closeLedgerMgmt()

	// reset ledgers to genesis block
	err := kvledger.ResetAllKVLedgers()
	assert.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB | rebuildableBlockIndex
	env.verifyRebuilableDoesNotExist(rebuildable)
	initLedgerMgmt()

	// ensure that the reset is executed correctly
	preResetHt, err := kvledger.LoadPreResetHeight()
	t.Logf("preResetHt = %#v", preResetHt)
	assert.Equal(t, uint64(5), preResetHt["ledger1"])
	h = newTestHelperOpenLgr("ledger1", t)
	h.verifyLedgerHeight(1)

	// recommit blocks
	assert.NoError(t, h.lgr.CommitWithPvtData(blk1, &ledger.CommitOptions{}))
	assert.NoError(t, h.lgr.CommitWithPvtData(blk2, &ledger.CommitOptions{}))
	// After the recommit of block 2
	h.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	assert.NoError(t, h.lgr.CommitWithPvtData(blk3, &ledger.CommitOptions{}))
	assert.NoError(t, h.lgr.CommitWithPvtData(blk4, &ledger.CommitOptions{}))

	// after the recommit of block 4
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}

func TestResetLedgerWithoutDroppingDBs(t *testing.T) {
	env := newEnv(defaultConfig, t)
	defer env.cleanup()
	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)

	// create ledgers and pouplate with sample data
	h := newTestHelperCreateLgr("ledger-1", t)
	dataHelper.populateLedger(h)
	dataHelper.verifyLedgerContent(h)
	closeLedgerMgmt()

	// Reset All kv ledgers
	blockstorePath := ledgerconfig.GetBlockStorePath()
	err := fsblkstorage.ResetBlockStore(blockstorePath)
	assert.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB
	env.verifyRebuilablesExist(rebuildable)
	rebuildable = rebuildableBlockIndex
	env.verifyRebuilableDoesNotExist(rebuildable)
	initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight()
	t.Logf("preResetHt = %#v", preResetHt)
	_, err = ledgermgmt.OpenLedger("ledger-1")
	assert.Error(t, err)
	// populateLedger() stores 8 block in total
	assert.EqualError(t, err, "the state database [height=9] is ahead of the block store [height=1]. "+
		"This is possible when the state database is not dropped after a ledger reset/rollback. "+
		"The state database can safely be dropped and will be rebuilt up to block store height upon the next peer start.")
}
