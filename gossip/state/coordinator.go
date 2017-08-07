/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"fmt"

	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
)

// PvtData a placeholder to represent private data
type PvtData struct {
	Payload *rwset.TxPvtReadWriteSet
}

// Coordinator orchestrates the flow of the new
// blocks arrival and in flight transient data, responsible
// to complete missing parts of transient data for given block.
type Coordinator interface {
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data ...[]*PvtData) ([]string, error)

	// GetBlockByNum returns block and related to the block private data
	GetBlockByNum(seqNum uint64) (*common.Block, []*PvtData, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// Close coordinator, shuts down coordinator service
	Close()
}

type coordinator struct {
	committer.Committer
}

// NewCoordinator creates a new instance of coordinator
func NewCoordinator(committer committer.Committer) Coordinator {
	return &coordinator{Committer: committer}
}

func (c *coordinator) StoreBlock(block *common.Block, data ...[]*PvtData) ([]string, error) {
	// Need to check whenever there are missing private rwset
	return nil, c.Committer.Commit(block)
}

func (c *coordinator) GetBlockByNum(seqNum uint64) (*common.Block, []*PvtData, error) {
	blocks := c.Committer.GetBlocks([]uint64{seqNum})
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("Cannot retreive block number %d", seqNum)
	}
	return blocks[0], nil, nil
}
