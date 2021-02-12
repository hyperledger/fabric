/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
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
	ledgerIDs := make([]string, numLedgers)
	for i := 0; i < numLedgers; i++ {
		ledgerIDs[i] = fmt.Sprintf("ledger-%d", i)
		l := env.createTestLedgerFromGenesisBlk(ledgerIDs[i])
		dataHelper.populateLedger(l)
		dataHelper.verifyLedgerContent(l)
		gb, err := l.lgr.GetBlockByNumber(0)
		require.NoError(t, err)
		genesisBlocks = append(genesisBlocks, gb)
		bcInfo, err := l.lgr.GetBlockchainInfo()
		require.NoError(t, err)
		blockchainsInfo = append(blockchainsInfo, bcInfo)
	}
	env.closeLedgerMgmt()

	// Reset All kv ledgers
	rootFSPath := env.initializer.Config.RootFSPath
	err := kvledger.ResetAllKVLedgers(rootFSPath)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB | rebuildableBlockIndex
	env.verifyRebuilableDirEmpty(rebuildable)
	env.initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight(rootFSPath, ledgerIDs)
	require.NoError(t, err)
	t.Logf("preResetHt = %#v", preResetHt)
	// open all the ledgers again and verify that
	// - initial height==1
	// - compare the genesis block from the one before reset
	// - resubmit the previous committed blocks from block number 1 onwards
	//   and final blockchainInfo and ledger state same as before reset
	for i := 0; i < 10; i++ {
		ledgerID := fmt.Sprintf("ledger-%d", i)
		l := env.openTestLedger(ledgerID)
		l.verifyLedgerHeight(1)
		require.Equal(t, blockchainsInfo[i].Height, preResetHt[ledgerID])
		gb, err := l.lgr.GetBlockByNumber(0)
		require.NoError(t, err)
		require.Equal(t, genesisBlocks[i], gb)
		for _, b := range dataHelper.submittedData[ledgerID].Blocks {
			require.NoError(t, l.lgr.CommitLegacy(b, &ledger.CommitOptions{}))
		}
		bcInfo, err := l.lgr.GetBlockchainInfo()
		require.NoError(t, err)
		require.Equal(t, blockchainsInfo[i], bcInfo)
		dataHelper.verifyLedgerContent(l)
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
	l := env.createTestLedgerFromGenesisBlk("ledger1")
	collConf := []*collConf{{name: "coll1", btl: 0}, {name: "coll2", btl: 1}}

	// deploy cc1 with 'collConf'
	l.simulateDeployTx("cc1", collConf)
	blk1 := l.cutBlockAndCommitLegacy()

	// commit pvtdata writes in block 2.
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "key1", "value1") // (key1 would never expire)
		s.setPvtdata("cc1", "coll2", "key2", "value2") // (key2 would expire at block 4)
	})
	blk2 := l.cutBlockAndCommitLegacy()

	// After commit of block 2
	l.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	l.verifyBlockAndPvtDataSameAs(2, blk2)             // key1 and key2 should still exist in the pvtdata storage

	// After commit of block 3
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	blk3 := l.cutBlockAndCommitLegacy()

	// After commit of block 4
	l.simulateDataTx("", func(s *simulator) {
		s.setPvtdata("cc1", "coll1", "someOtherKey", "someOtherVal")
		s.setPvtdata("cc1", "coll2", "someOtherKey", "someOtherVal")
	})
	blk4 := l.cutBlockAndCommitLegacy()

	// After commit of block 4
	l.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
		r.pvtdataShouldContain(0, "cc1", "coll1", "key1", "value1") // key1 should still exist in the pvtdata storage
		r.pvtdataShouldNotContain("cc1", "coll2")                   // <cc1, coll2> shold have been purged from the pvtdata storage
	})

	env.closeLedgerMgmt()

	// reset ledgers to genesis block
	err := kvledger.ResetAllKVLedgers(env.initializer.Config.RootFSPath)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB | rebuildableBlockIndex
	env.verifyRebuilableDirEmpty(rebuildable)
	env.initLedgerMgmt()

	// ensure that the reset is executed correctly
	preResetHt, err := kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, []string{"ledger1"})
	require.NoError(t, err)
	t.Logf("preResetHt = %#v", preResetHt)
	require.Equal(t, uint64(5), preResetHt["ledger1"])
	l = env.openTestLedger("ledger1")
	l.verifyLedgerHeight(1)

	// recommit blocks
	require.NoError(t, l.lgr.CommitLegacy(blk1, &ledger.CommitOptions{}))
	require.NoError(t, l.lgr.CommitLegacy(blk2, &ledger.CommitOptions{}))
	// After the recommit of block 2
	l.verifyPvtState("cc1", "coll1", "key1", "value1") // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "value2") // key2 should still exist in the state
	require.NoError(t, l.lgr.CommitLegacy(blk3, &ledger.CommitOptions{}))
	require.NoError(t, l.lgr.CommitLegacy(blk4, &ledger.CommitOptions{}))

	// after the recommit of block 4
	l.verifyPvtState("cc1", "coll1", "key1", "value1")                  // key1 should still exist in the state
	l.verifyPvtState("cc1", "coll2", "key2", "")                        // key2 should have been purged from the state
	l.verifyBlockAndPvtData(2, nil, func(r *retrievedBlockAndPvtdata) { // retrieve the pvtdata for block 2 from pvtdata storage
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
	l := env.createTestLedgerFromGenesisBlk("ledger-1")
	dataHelper.populateLedger(l)
	dataHelper.verifyLedgerContent(l)
	env.closeLedgerMgmt()

	// Reset All kv ledgers
	blockstorePath := kvledger.BlockStorePath(env.initializer.Config.RootFSPath)
	err := blkstorage.ResetBlockStore(blockstorePath)
	require.NoError(t, err)
	rebuildable := rebuildableStatedb | rebuildableBookkeeper | rebuildableConfigHistory | rebuildableHistoryDB
	env.verifyRebuilablesExist(rebuildable)
	rebuildable = rebuildableBlockIndex
	env.verifyRebuilableDirEmpty(rebuildable)
	env.initLedgerMgmt()
	preResetHt, err := kvledger.LoadPreResetHeight(env.initializer.Config.RootFSPath, []string{"ledger-1"})
	t.Logf("preResetHt = %#v", preResetHt)
	require.NoError(t, err)
	require.Equal(t, uint64(9), preResetHt["ledger-1"])
	_, err = env.ledgerMgr.OpenLedger("ledger-1")
	// populateLedger() stores 8 block in total
	require.EqualError(t, err, "the state database [height=9] is ahead of the block store [height=1]. "+
		"This is possible when the state database is not dropped after a ledger reset/rollback. "+
		"The state database can safely be dropped and will be rebuilt up to block store height upon the next peer start")
}
