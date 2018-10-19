/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package etcdraft

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func getSeedBlock() *cb.Block {
	seedBlock := cb.NewBlock(0, []byte("firsthash"))
	seedBlock.Data.Data = [][]byte{[]byte("somebytes")}
	return seedBlock
}

func TestValidCreatedBlocksQueue(t *testing.T) {
	seedBlock := getSeedBlock()
	logger := flogging.NewFabricLogger(zap.NewNop())
	bc := newBlockCreator(seedBlock, logger)

	t.Run("correct creation of a single block", func(t *testing.T) {
		block := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})

		assert.Equal(t, seedBlock.Header.Number+1, block.Header.Number)
		assert.Equal(t, block.Data.Hash(), block.Header.DataHash)
		assert.Equal(t, seedBlock.Header.Hash(), block.Header.PreviousHash)
		// this created block should be part of the queue of created blocks
		assert.Len(t, bc.CreatedBlocks, 1)

		bc.commitBlock(block)

		assert.Empty(t, bc.CreatedBlocks)
		assert.Equal(t, bc.LastCommittedBlock.Header.Hash(), block.Header.Hash())
	})

	t.Run("ResetCreatedBlocks resets the queue of created blocks", func(t *testing.T) {
		numBlocks := 10
		blocks := []*cb.Block{}
		for i := 0; i < numBlocks; i++ {
			blocks = append(blocks, bc.createNextBlock([]*cb.Envelope{{Payload: []byte("test envelope")}}))
		}

		bc.resetCreatedBlocks()
		assert.True(t,
			bytes.Equal(bc.LastCommittedBlock.Header.Bytes(), bc.LastCreatedBlock.Header.Bytes()),
			"resetting the created blocks queue should leave the lastCommittedBlock and the lastCreatedBlock equal",
		)
		assert.Empty(t, bc.CreatedBlocks)
	})

	t.Run("commit of block without any created blocks sets the lastCreatedBlock correctly", func(t *testing.T) {
		block := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("some other bytes")}})
		bc.resetCreatedBlocks()

		bc.commitBlock(block)

		assert.True(t,
			bytes.Equal(block.Header.Bytes(), bc.LastCommittedBlock.Header.Bytes()),
			"resetting the created blocks queue should leave the lastCommittedBlock and the lastCreatedBlock equal",
		)
		assert.True(t,
			bytes.Equal(block.Header.Bytes(), bc.LastCreatedBlock.Header.Bytes()),
			"resetting the created blocks queue should leave the lastCommittedBlock and the lastCreatedBlock equal",
		)
		assert.Empty(t, bc.CreatedBlocks)
	})
	t.Run("propose multiple blocks without having to commit them; tests the optimistic block creation", func(t *testing.T) {
		/*
		 * Scenario:
		 *   We create five blocks initially and then commit only two of them. We further create five more blocks
		 *   and commit out the remaining 8 blocks in the propose stack. This should succeed since the committed
		 *   blocks are not divergent from the created blocks.
		 */
		blocks := []*cb.Block{}
		// Create five blocks without writing them out
		for i := 0; i < 5; i++ {
			blocks = append(blocks, bc.createNextBlock([]*cb.Envelope{{Payload: []byte("test envelope")}}))
		}
		assert.Len(t, bc.CreatedBlocks, 5)

		// Write two of these out
		for i := 0; i < 2; i++ {
			bc.commitBlock(blocks[i])
		}
		assert.Len(t, bc.CreatedBlocks, 3)

		// Create five more blocks; these should be created over the previous five blocks created
		for i := 0; i < 5; i++ {
			blocks = append(blocks, bc.createNextBlock([]*cb.Envelope{{Payload: []byte("test envelope")}}))
		}
		assert.Len(t, bc.CreatedBlocks, 8)

		// Write out the remaining eight blocks; can only succeed if all the blocks were created in a single stack else will panic
		for i := 2; i < 10; i++ {
			bc.commitBlock(blocks[i])
		}
		assert.Empty(t, bc.CreatedBlocks)

		// Assert that the block were indeed created in the correct sequence
		for i := 0; i < 9; i++ {
			assertNextBlock(t, blocks[i], blocks[i+1])
		}
	})

	t.Run("createNextBlock returns nil after createdBlocksBuffersize blocks have been created", func(t *testing.T) {
		numBlocks := createdBlocksBuffersize
		blocks := []*cb.Block{}

		for i := 0; i < numBlocks; i++ {
			blocks = append(blocks, bc.createNextBlock([]*cb.Envelope{{Payload: []byte("test envelope")}}))
		}

		block := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("test envelope")}})

		assert.Nil(t, block)
	})

	t.Run("created blocks should always be over committed blocks", func(t *testing.T) {
		/*
		 * Scenario:
		 *   We will create
		 *     1. a propose stack with 5 blocks over baseLastCreatedBlock, and
		 *     2. an alternate block over baseLastCreatedBlock.
		 *   We will write out this alternate block and verify that the subsequent block is created over this alternate block
		 *   and not on the existing propose stack.
		 * This scenario fails if commitBlock does not flush the createdBlocks queue when the written block is divergent from the
		 * created blocks.
		 */

		baseLastCreatedBlock := bc.LastCreatedBlock

		// Create the stack of five blocks without writing them out
		blocks := []*cb.Block{}
		for i := 0; i < 5; i++ {
			blocks = append(blocks, bc.createNextBlock([]*cb.Envelope{{Payload: []byte("test envelope")}}))
		}

		// create and write out the alternate block
		alternateBlock := createBlockOverSpecifiedBlock(baseLastCreatedBlock, []*cb.Envelope{{Payload: []byte("alternate test envelope")}})
		bc.commitBlock(alternateBlock)

		// assert that createNextBlock creates the next block over this alternateBlock
		createdBlockOverAlternateBlock := bc.createNextBlock([]*cb.Envelope{{Payload: []byte("test envelope")}})
		synthesizedBlockOverAlternateBlock := createBlockOverSpecifiedBlock(alternateBlock, []*cb.Envelope{{Payload: []byte("test envelope")}})
		assert.True(t,
			bytes.Equal(createdBlockOverAlternateBlock.Header.Bytes(), synthesizedBlockOverAlternateBlock.Header.Bytes()),
			"created and synthesized blocks should be equal",
		)
		bc.commitBlock(createdBlockOverAlternateBlock)
	})

}

func createBlockOverSpecifiedBlock(baseBlock *cb.Block, messages []*cb.Envelope) *cb.Block {
	previousBlockHash := baseBlock.Header.Hash()

	data := &cb.BlockData{
		Data: make([][]byte, len(messages)),
	}

	var err error
	for i, msg := range messages {
		data.Data[i], err = proto.Marshal(msg)
		if err != nil {
			panic(fmt.Sprintf("Could not marshal envelope: %s", err))
		}
	}

	block := cb.NewBlock(baseBlock.Header.Number+1, previousBlockHash)
	block.Header.DataHash = data.Hash()
	block.Data = data

	return block
}

// isNextBlock returns true if `nextBlock` is correctly formed as the next block
// following `block` in a chain.
func assertNextBlock(t *testing.T, block, nextBlock *cb.Block) {
	assert.Equal(t, block.Header.Number+1, nextBlock.Header.Number)
	assert.Equal(t, block.Header.Hash(), nextBlock.Header.PreviousHash)
}
