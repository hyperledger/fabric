/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/blkstorage/fsblkstorage"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/stretchr/testify/require"
)

func TestResetAllLedgers(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)
	var genesisBlocks []*common.Block
	var blockchainsInfo []*common.BlockchainInfo

	// create ledgers and pouplate with sample data
	// Also, retrieve the genesis blocks and blockchain info for matching later
	numLedgers := 10
	ledgerIDs := make([]string, numLedgers, numLedgers)
	for i := 0; i < numLedgers; i++ {
		ledgerIDs[i] = fmt.Sprintf("ledger-%d", i)
		h := env.newTestHelperCreateLgr(ledgerIDs[i], t)
		dataHelper.populateLedger(h)
		dataHelper.verifyLedgerContent(h)
		gb, err := h.lgr.GetBlockByNumber(0)
		require.NoError(t, err)
		genesisBlocks = append(genesisBlocks, gb)
		bcInfo, err := h.lgr.GetBlockchainInfo()
		require.NoError(t, err)
		blockchainsInfo = append(blockchainsInfo, bcInfo)
	}
	env.closeLedgerMgmt()

	// Reset All kv ledgers
	rootFSPath := env.initializer.Config.RootFSPath
	err := kvledger.ResetAllKVLedgers(rootFSPath)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB | rebuildableBlockIndex
	env.verifyRebuilableDoesNotExist(rebuildable)
	env.initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight(rootFSPath, ledgerIDs)
	t.Logf("preResetHt = %#v", preResetHt)
	// open all the ledgers again and verify that
	// - initial height==1
	// - compare the genesis block from the one before reset
	// - resubmit the previous committed blocks from block number 1 onwards
	//   and final blockchainInfo and ledger state same as before reset
	for i := 0; i < 10; i++ {
		ledgerID := fmt.Sprintf("ledger-%d", i)
		h := env.newTestHelperOpenLgr(ledgerID, t)
		h.verifyLedgerHeight(1)
		require.Equal(t, blockchainsInfo[i].Height, preResetHt[ledgerID])
		gb, err := h.lgr.GetBlockByNumber(0)
		require.NoError(t, err)
		require.Equal(t, genesisBlocks[i], gb)
		for _, b := range dataHelper.submittedData[ledgerID].Blocks {
			require.NoError(t, h.lgr.CommitLegacy(b, &ledger.CommitOptions{}))
		}
		bcInfo, err := h.lgr.GetBlockchainInfo()
		require.NoError(t, err)
		require.Equal(t, blockchainsInfo[i], bcInfo)
		dataHelper.verifyLedgerContent(h)
	}

	require.NoError(t, kvledger.ClearPreResetHeight(env.initializer.Config.RootFSPath, ledgerIDs))
	preResetHt, err = kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, ledgerIDs)
	require.NoError(t, err)
	require.Len(t, preResetHt, 0)

	// reset again to test ClearPreResetHeight with different ledgerIDs
	env.closeLedgerMgmt()
	err = kvledger.ResetAllKVLedgers(rootFSPath)
	require.NoError(t, err)
	env.initLedgerMgmt()
	// verify LoadPreResetHeight with different ledgerIDs
	newLedgerIDs := ledgerIDs[:len(ledgerIDs)-3]
	preResetHt, err = kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, newLedgerIDs)
	require.NoError(t, err)
	require.Equal(t, numLedgers-3, len(preResetHt))
	for i := 0; i < len(preResetHt); i++ {
		require.Contains(t, preResetHt, fmt.Sprintf("ledger-%d", i))
	}
	// verify preResetHt after ClearPreResetHeight
	require.NoError(t, kvledger.ClearPreResetHeight(env.initializer.Config.RootFSPath, newLedgerIDs))
	preResetHt, err = kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, ledgerIDs)
	require.NoError(t, err)
	require.Len(t, preResetHt, 3)
	require.Contains(t, preResetHt, fmt.Sprintf("ledger-%d", 7))
	require.Contains(t, preResetHt, fmt.Sprintf("ledger-%d", 8))
	require.Contains(t, preResetHt, fmt.Sprintf("ledger-%d", 9))
}

func TestResetAllLedgersWithBTL(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	h := env.newTestHelperCreateLgr("ledger1", t)
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 1}}

	// deploy cc1 with 'collConf'
	h.simulateDeployTx("cc1", collConf)
	blk1 := h.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 2.
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1") // (key1 would never expire)
		s.setPvtdata("cc1", "coll2", "key2", "value2") // (key2 would expire at block 4)
	})
	blk2 := h.cutBlockAndCommitLegacy()

	// After commit of block 2
	h.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	h.verifyBlockAndPvtDataSameAs(2, blk2)             // key1 and key2 should still exist in the pvtdata storage

	// After commit of block 3
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	blk3 := h.cutBlockAndCommitLegacy()

	// After commit of block 4
	h.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	blk4 := h.cutBlockAndCommitLegacy()

	// After commit of block 4
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	env.closeLedgerMgmt()

	// reset ledgers to genesis block
	err := kvledger.ResetAllKVLedgers(env.initializer.Config.RootFSPath)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB | rebuildableBlockIndex
	env.verifyRebuilableDoesNotExist(rebuildable)
	env.initLedgerMgmt()

	// ensure that the reset is executed correctly
	preResetHt, err := kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, []string{"ledger1"})
	t.Logf("preResetHt = %#v", preResetHt)
	require.Equal(t, uint64(5), preResetHt["ledger1"])
	h = env.newTestHelperOpenLgr("ledger1", t)
	h.verifyLedgerHeight(1)

	// recommit blocks
	require.NoError(t, h.lgr.CommitLegacy(blk1, &ledger.CommitOptions{}))
	require.NoError(t, h.lgr.CommitLegacy(blk2, &ledger.CommitOptions{}))
	// After the recommit of block 2
	h.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	require.NoError(t, h.lgr.CommitLegacy(blk3, &ledger.CommitOptions{}))
	require.NoError(t, h.lgr.CommitLegacy(blk4, &ledger.CommitOptions{}))

	// after the recommit of block 4
	h.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	h.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	h.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})
}

func TestResetLedgerWithoutDroppingDBs(t *testing.T) {
	env := newEnv(t)
	defer env.cleanup()
	env.initLedgerMgmt()
	// populate ledgers with sample data
	dataHelper := newSampleDataHelper(t)

	// create ledgers and pouplate with sample data
	h := env.newTestHelperCreateLgr("ledger-1", t)
	dataHelper.populateLedger(h)
	dataHelper.verifyLedgerContent(h)
	env.closeLedgerMgmt()

	// Reset All kv ledgers
	blockstorePath := kvledger.BlockStorePath(env.initializer.Config.RootFSPath)
	err := fsblkstorage.ResetBlockStore(blockstorePath)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB
	env.verifyRebuilablesExist(rebuildable)
	rebuildable = rebuildableBlockIndex
	env.verifyRebuilableDoesNotExist(rebuildable)
	env.initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, []string{"ledger-1"})
	t.Logf("preResetHt = %#v", preResetHt)
	require.NoError(t, err)
	require.Equal(t, uint64(9), preResetHt["ledger-1"])
	_, err = env.ledgerMgr.OpenLedger("ledger-1")
	require.Error(t, err)
	// populateLedger() stores 8 block in total
	require.EqualError(t, err, "the state database [height=9] is ahead of the block store [height=1]. "+
		"This is possible when the state database is not dropped after a ledger reset/rollback. "+
		"The state database can safely be dropped and will be rebuilt up to block store height upon the next peer start.")
}
