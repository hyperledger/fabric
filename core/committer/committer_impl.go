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
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/events/producer"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
)

//--------!!!IMPORTANT!!-!!IMPORTANT!!-!!IMPORTANT!!---------
// This is used merely to complete the loop for the "skeleton"
// path so we can reason about and  modify committer component
// more effectively using code.

var logger *logging.Logger // package-level logger

func init() {
	logger = flogging.MustGetLogger("committer")
}

// LedgerCommitter is the implementation of  Committer interface
// it keeps the reference to the ledger to commit blocks and retrieve
// chain information
type LedgerCommitter struct {
	ledger.PeerLedger
	eventer ConfigBlockEventer
}

// ConfigBlockEventer callback function proto type to define action
// upon arrival on new configuaration update block
type ConfigBlockEventer func(block *common.Block) error

// NewLedgerCommitter is a factory function to create an instance of the committer
// which passes incoming blocks via validation and commits them into the ledger.
func NewLedgerCommitter(ledger ledger.PeerLedger) *LedgerCommitter {
	return NewLedgerCommitterReactive(ledger, func(_ *common.Block) error { return nil })
}

// NewLedgerCommitterReactive is a factory function to create an instance of the committer
// same as way as NewLedgerCommitter, while also provides an option to specify callback to
// be called upon new configuration block arrival and commit event
func NewLedgerCommitterReactive(ledger ledger.PeerLedger, eventer ConfigBlockEventer) *LedgerCommitter {
	return &LedgerCommitter{PeerLedger: ledger, eventer: eventer}
}

// preCommit takes care to validate the block and update based on its
// content
func (lc *LedgerCommitter) preCommit(block *common.Block) error {
	// Updating CSCC with new configuration block
	if utils.IsConfigBlock(block) {
		logger.Debug("Received configuration update, calling CSCC ConfigUpdate")
		if err := lc.eventer(block); err != nil {
			return errors.WithMessage(err, "could not update CSCC with new configuration update")
		}
	}
	return nil
}

// CommitWithPvtData commits blocks atomically with private data
func (lc *LedgerCommitter) CommitWithPvtData(blockAndPvtData *ledger.BlockAndPvtData) error {
	// Do validation and whatever needed before
	// committing new block
	if err := lc.preCommit(blockAndPvtData.Block); err != nil {
		return err
	}

	// Committing new block
	if err := lc.PeerLedger.CommitWithPvtData(blockAndPvtData); err != nil {
		return err
	}

	// post commit actions, such as event publishing
	lc.postCommit(blockAndPvtData.Block)

	return nil
}

// GetPvtDataAndBlockByNum retrieves private data and block for given sequence number
func (lc *LedgerCommitter) GetPvtDataAndBlockByNum(seqNum uint64) (*ledger.BlockAndPvtData, error) {
	return lc.PeerLedger.GetPvtDataAndBlockByNum(seqNum, nil)
}

// postCommit publish event or handle other tasks once block committed to the ledger
func (lc *LedgerCommitter) postCommit(block *common.Block) {
	// create/send block events *after* the block has been committed
	bevent, fbevent, channelID, err := producer.CreateBlockEvents(block)
	if err != nil {
		logger.Errorf("Channel [%s] Error processing block events for block number [%d]: %+v", channelID, block.Header.Number, err)
	} else {
		if err := producer.Send(bevent); err != nil {
			logger.Errorf("Channel [%s] Error sending block event for block number [%d]: %+v", channelID, block.Header.Number, err)
		}
		if err := producer.Send(fbevent); err != nil {
			logger.Errorf("Channel [%s] Error sending filtered block event for block number [%d]: %+v", channelID, block.Header.Number, err)
		}
	}
}

// LedgerHeight returns recently committed block sequence number
func (lc *LedgerCommitter) LedgerHeight() (uint64, error) {
	var info *common.BlockchainInfo
	var err error
	if info, err = lc.GetBlockchainInfo(); err != nil {
		logger.Errorf("Cannot get blockchain info, %s", info)
		return uint64(0), err
	}

	return info.Height, nil
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
