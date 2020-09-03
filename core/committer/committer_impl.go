/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
)

var logger = flogging.MustGetLogger("committer")

//--------!!!IMPORTANT!!-!!IMPORTANT!!-!!IMPORTANT!!---------
// This is used merely to complete the loop for the "skeleton"
// path so we can reason about and  modify committer component
// more effectively using code.

// PeerLedgerSupport abstract out the API's of ledger.PeerLedger interface
// required to implement LedgerCommitter
type PeerLedgerSupport interface {
	GetPvtDataAndBlockByNum(blockNum uint64, filter ledger.PvtNsCollFilter) (*ledger.BlockAndPvtData, error)

	GetPvtDataByNum(blockNum uint64, filter ledger.PvtNsCollFilter) ([]*ledger.TxPvtData, error)

	CommitLegacy(blockAndPvtdata *ledger.BlockAndPvtData, commitOpts *ledger.CommitOptions) error

	CommitPvtDataOfOldBlocks(reconciledPvtdata []*ledger.ReconciledPvtdata, unreconciled ledger.MissingPvtDataInfo) ([]*ledger.PvtdataHashMismatch, error)

	GetBlockchainInfo() (*common.BlockchainInfo, error)

	DoesPvtDataInfoExist(blockNum uint64) (bool, error)

	GetBlockByNumber(blockNumber uint64) (*common.Block, error)

	GetConfigHistoryRetriever() (ledger.ConfigHistoryRetriever, error)

	GetMissingPvtDataTracker() (ledger.MissingPvtDataTracker, error)

	Close()
}

// LedgerCommitter is the implementation of  Committer interface
// it keeps the reference to the ledger to commit blocks and retrieve
// chain information
type LedgerCommitter struct {
	PeerLedgerSupport
}

// NewLedgerCommitter is a factory function to create an instance of the committer
// which passes incoming blocks via validation and commits them into the ledger.
func NewLedgerCommitter(ledger PeerLedgerSupport) *LedgerCommitter {
	return &LedgerCommitter{PeerLedgerSupport: ledger}
}

// CommitLegacy commits blocks atomically with private data
func (lc *LedgerCommitter) CommitLegacy(blockAndPvtData *ledger.BlockAndPvtData, commitOpts *ledger.CommitOptions) error {
	// Committing new block
	if err := lc.PeerLedgerSupport.CommitLegacy(blockAndPvtData, commitOpts); err != nil {
		return err
	}

	return nil
}

// GetPvtDataAndBlockByNum retrieves private data and block for given sequence number
func (lc *LedgerCommitter) GetPvtDataAndBlockByNum(seqNum uint64) (*ledger.BlockAndPvtData, error) {
	return lc.PeerLedgerSupport.GetPvtDataAndBlockByNum(seqNum, nil)
}

// LedgerHeight returns recently committed block sequence number
func (lc *LedgerCommitter) LedgerHeight() (uint64, error) {
	info, err := lc.GetBlockchainInfo()
	if err != nil {
		logger.Errorf("Cannot get blockchain info, %s", info)
		return 0, err
	}

	return info.Height, nil
}

// DoesPvtDataInfoExistInLedger returns true if the ledger has pvtdata info
// about a given block number.
func (lc *LedgerCommitter) DoesPvtDataInfoExistInLedger(blockNum uint64) (bool, error) {
	return lc.DoesPvtDataInfoExist(blockNum)
}

// GetBlocks used to retrieve blocks with sequence numbers provided in the slice
func (lc *LedgerCommitter) GetBlocks(blockSeqs []uint64) []*common.Block {
	var blocks []*common.Block

	for _, seqNum := range blockSeqs {
		if blck, err := lc.GetBlockByNumber(seqNum); err != nil {
			logger.Errorf("Not able to acquire block num %d, from the ledger skipping...", seqNum)
			continue
		} else {
			logger.Debug("Appending next block with seqNum = ", seqNum, " to the resulting set")
			blocks = append(blocks, blck)
		}
	}

	return blocks
}
