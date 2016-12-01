/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package committer

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/op/go-logging"
)

//--------!!!IMPORTANT!!-!!IMPORTANT!!-!!IMPORTANT!!---------
// This is used merely to complete the loop for the "skeleton"
// path so we can reason about and  modify committer component
// more effectively using code.

var logger *logging.Logger // package-level logger

func init() {
	logger = logging.MustGetLogger("committer")
}

// LedgerCommitter is the implementation of  Committer interface
// it keeps the reference to the ledger to commit blocks and retreive
// chain information
type LedgerCommitter struct {
	ledger ledger.ValidatedLedger
}

// NewLedgerCommitter is a factory function to create an instance of the committer
func NewLedgerCommitter(ledger ledger.ValidatedLedger) *LedgerCommitter {
	return &LedgerCommitter{ledger}
}

// CommitBlock commits block to into the ledger
func (lc *LedgerCommitter) CommitBlock(block *common.Block) error {
	if _, _, err := lc.ledger.RemoveInvalidTransactionsAndPrepare(block); err != nil {
		return err
	}
	if err := lc.ledger.Commit(); err != nil {
		return err
	}
	return nil
}

// LedgerHeight returns recently committed block sequence number
func (lc *LedgerCommitter) LedgerHeight() (uint64, error) {
	var info *pb.BlockchainInfo
	var err error
	if info, err = lc.ledger.GetBlockchainInfo(); err != nil {
		logger.Errorf("Cannot get blockchain info, %s\n", info)
		return uint64(0), err
	}

	return info.Height, nil
}

// GetBlocks used to retrieve blocks with sequence numbers provided in the slice
func (lc *LedgerCommitter) GetBlocks(blockSeqs []uint64) []*common.Block {
	var blocks []*common.Block

	for _, seqNum := range blockSeqs {
		if blck, err := lc.ledger.GetBlockByNumber(seqNum); err != nil {
			logger.Errorf("Not able to acquire block num %d, from the ledger skipping...\n", seqNum)
			continue
		} else {
			logger.Debug("Appending next block with seqNum = ", seqNum, " to the resulting set")
			blocks = append(blocks, blck)
		}
	}

	return blocks
}

// Close the ledger
func (lc *LedgerCommitter) Close() {
	lc.ledger.Close()
}
