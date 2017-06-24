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
	"fmt"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/committer/txvalidator"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/events/producer"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
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
	ledger    ledger.PeerLedger
	validator txvalidator.Validator
	eventer   ConfigBlockEventer
}

// ConfigBlockEventer callback function proto type to define action
// upon arrival on new configuaration update block
type ConfigBlockEventer func(block *common.Block) error

// NewLedgerCommitter is a factory function to create an instance of the committer
// which passes incoming blocks via validation and commits them into the ledger.
func NewLedgerCommitter(ledger ledger.PeerLedger, validator txvalidator.Validator) *LedgerCommitter {
	return NewLedgerCommitterReactive(ledger, validator, func(_ *common.Block) error { return nil })
}

// NewLedgerCommitterReactive is a factory function to create an instance of the committer
// same as way as NewLedgerCommitter, while also provides an option to specify callback to
// be called upon new configuration block arrival and commit event
func NewLedgerCommitterReactive(ledger ledger.PeerLedger, validator txvalidator.Validator, eventer ConfigBlockEventer) *LedgerCommitter {
	return &LedgerCommitter{ledger: ledger, validator: validator, eventer: eventer}
}

// Commit commits block to into the ledger
// Note, it is important that this always be called serially
func (lc *LedgerCommitter) Commit(block *common.Block) error {

	// Validate and mark invalid transactions
	logger.Debug("Validating block")
	if err := lc.validator.Validate(block); err != nil {
		return err
	}

	// Updating CSCC with new configuration block
	if utils.IsConfigBlock(block) {
		logger.Debug("Received configuration update, calling CSCC ConfigUpdate")
		if err := lc.eventer(block); err != nil {
			return fmt.Errorf("Could not update CSCC with new configuration update due to %s", err)
		}
	}

	if err := lc.ledger.Commit(block); err != nil {
		return err
	}

	// send block event *after* the block has been committed
	if err := producer.SendProducerBlockEvent(block); err != nil {
		logger.Errorf("Error publishing block %d, because: %v", block.Header.Number, err)
	}

	return nil
}

// LedgerHeight returns recently committed block sequence number
func (lc *LedgerCommitter) LedgerHeight() (uint64, error) {
	var info *common.BlockchainInfo
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
