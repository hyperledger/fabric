/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"fmt"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger"
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
			assert.NoError(t, h.lgr.CommitWithPvtData(b))
		}
		bcInfo, err := h.lgr.GetBlockchainInfo()
		assert.NoError(t, err)
		assert.Equal(t, blockchainsInfo[i], bcInfo)
		dataHelper.verifyLedgerContent(h)
	}
}
