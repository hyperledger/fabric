/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package example

import (
	"github.com/hyperledger/fabric/core/ledger"

	"github.com/hyperledger/fabric/protos/common"
)

// Committer a toy committer
type Committer struct {
	ledger ledger.PeerLedger
}

// ConstructCommitter constructs a committer for the example
func ConstructCommitter(ledger ledger.PeerLedger) *Committer {
	return &Committer{ledger}
}

// Commit commits the block
func (c *Committer) Commit(rawBlock *common.Block) error {
	logger.Debugf("Committer validating the block...")
	if err := c.ledger.CommitWithPvtData(&ledger.BlockAndPvtData{Block: rawBlock}); err != nil {
		return err
	}
	return nil
}
