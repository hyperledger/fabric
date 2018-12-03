/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	cb "github.com/hyperledger/fabric/protos/common"
)

// This governs the max number of created blocks in-flight; i.e. blocks
// that were created but not written.
// CreateNextBLock returns nil once this number of blocks are in-flight.
const createdBlocksBuffersize = 20

// blockCreator optimistically creates blocks in a chain. The created
// blocks may not be written out eventually. This enables us to pipeline
// the creation of blocks with achieving consensus on them leading to
// performance improvements. The created chain is discarded if a
// diverging block is committed
// To safely use blockCreator, only one thread should interact with it.
type blockCreator struct {
	CreatedBlocks      chan *cb.Block
	LastCreatedBlock   *cb.Block
	LastCommittedBlock *cb.Block
	logger             *flogging.FabricLogger
}

func newBlockCreator(lastBlock *cb.Block, logger *flogging.FabricLogger) *blockCreator {
	if lastBlock == nil {
		logger.Panic("block creator initialized with nil last block")
	}
	bc := &blockCreator{
		CreatedBlocks:      make(chan *cb.Block, createdBlocksBuffersize),
		LastCreatedBlock:   lastBlock,
		LastCommittedBlock: lastBlock,
		logger:             logger,
	}

	logger.Debugf("Initialized block creator with (lastblockNumber=%d)", lastBlock.Header.Number)
	return bc
}

// CreateNextBlock creates a new block with the next block number, and the
// given contents.
// Returns the created block if the block could be created else nil.
// It can fail when the there are `createdBlocksBuffersize` blocks already
// created and no more can be accomodated in the `CreatedBlocks` channel.
func (bc *blockCreator) createNextBlock(messages []*cb.Envelope) *cb.Block {
	previousBlockHash := bc.LastCreatedBlock.Header.Hash()

	data := &cb.BlockData{
		Data: make([][]byte, len(messages)),
	}

	var err error
	for i, msg := range messages {
		data.Data[i], err = proto.Marshal(msg)
		if err != nil {
			bc.logger.Panicf("Could not marshal envelope: %s", err)
		}
	}

	block := cb.NewBlock(bc.LastCreatedBlock.Header.Number+1, previousBlockHash)
	block.Header.DataHash = data.Hash()
	block.Data = data

	select {
	case bc.CreatedBlocks <- block:
		bc.LastCreatedBlock = block
		bc.logger.Debugf("Created block %d", bc.LastCreatedBlock.Header.Number)
		return block
	default:
		return nil
	}
}

// ResetCreatedBlocks resets the queue of created blocks.
// Subsequent blocks will be created over the block that was last committed
// using CommitBlock.
func (bc *blockCreator) resetCreatedBlocks() {
	// We should not recreate CreatedBlocks channel since it can lead to
	// data races on its access
	for len(bc.CreatedBlocks) > 0 {
		// empties the channel
		<-bc.CreatedBlocks
	}
	bc.LastCreatedBlock = bc.LastCommittedBlock
	bc.logger.Debug("Reset created blocks")
}

// commitBlock should be invoked for all blocks to let the blockCreator know
// which blocks have been committed. If the committed block is divergent from
// the stack of created blocks then the stack is reset.
func (bc *blockCreator) commitBlock(block *cb.Block) {
	bc.LastCommittedBlock = block

	// check if the committed block diverges from the created blocks
	select {
	case b := <-bc.CreatedBlocks:
		if !bytes.Equal(b.Header.Bytes(), block.Header.Bytes()) {
			// the written block is diverging from the createBlocks stack
			// discard the created blocks
			bc.resetCreatedBlocks()
		}
		// else the written block is part of the createBlocks stack; nothing to be done
	default:
		// No created blocks; set this block as the last created block.
		// This happens when calls to WriteBlock are being made without a CreateNextBlock being called.
		// For example, in the case of etcd/raft, the leader proposes blocks and the followers
		// only write the proposed blocks.
		bc.LastCreatedBlock = block
	}
}
