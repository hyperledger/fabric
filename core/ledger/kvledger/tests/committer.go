/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

// committer helps in cutting a block and commits the block (with pvt data) to the ledger
type committer struct {
	lgr    ledger.PeerLedger
	blkgen *blkGenerator
	assert *assert.Assertions
}

func newCommitter(lgr ledger.PeerLedger, t *testing.T) *committer {
	return &committer{lgr, newBlockGenerator(lgr, t), assert.New(t)}
}

// cutBlockAndCommitWithPvtdata cuts the next block from the given 'txAndPvtdata' and commits the block (with pvt data) to the ledger
// This function return a copy of 'ledger.BlockAndPvtData' that was submitted to the ledger to commit.
// A copy is returned instead of the actual one because, ledger makes some changes to the submitted block before commit
// (such as setting the metadata) and the test code would want to have the exact copy of the block that was submitted to
// the ledger
func (c *committer) cutBlockAndCommitWithPvtdata(trans ...*txAndPvtdata) *ledger.BlockAndPvtData {
	blk := c.blkgen.nextBlockAndPvtdata(trans...)
	blkCopy := c.copyOfBlockAndPvtdata(blk)
	c.assert.NoError(
		c.lgr.CommitWithPvtData(blk),
	)
	return blkCopy
}

func (c *committer) cutBlockAndCommitExpectError(trans ...*txAndPvtdata) (*ledger.BlockAndPvtData, error) {
	blk := c.blkgen.nextBlockAndPvtdata(trans...)
	blkCopy := c.copyOfBlockAndPvtdata(blk)
	err := c.lgr.CommitWithPvtData(blk)
	c.assert.Error(err)
	return blkCopy, err
}

func (c *committer) copyOfBlockAndPvtdata(blk *ledger.BlockAndPvtData) *ledger.BlockAndPvtData {
	blkBytes, err := proto.Marshal(blk.Block)
	c.assert.NoError(err)
	blkCopy := &common.Block{}
	c.assert.NoError(proto.Unmarshal(blkBytes, blkCopy))
	return &ledger.BlockAndPvtData{Block: blkCopy, BlockPvtData: blk.BlockPvtData}
}

/////////////////   block generation code  ///////////////////////////////////////////
// blkGenerator helps creating the next block for the ledger
type blkGenerator struct {
	lastNum  uint64
	lastHash []byte
	assert   *assert.Assertions
}

// newBlockGenerator constructs a 'blkGenerator' and initializes the 'blkGenerator'
// from the last block available in the ledger so that the next block can be populated
// with the correct block number and previous block hash
func newBlockGenerator(lgr ledger.PeerLedger, t *testing.T) *blkGenerator {
	assert := assert.New(t)
	info, err := lgr.GetBlockchainInfo()
	assert.NoError(err)
	return &blkGenerator{info.Height - 1, info.CurrentBlockHash, assert}
}

// nextBlockAndPvtdata cuts the next block
func (g *blkGenerator) nextBlockAndPvtdata(trans ...*txAndPvtdata) *ledger.BlockAndPvtData {
	block := common.NewBlock(g.lastNum+1, g.lastHash)
	blockPvtdata := make(map[uint64]*ledger.TxPvtData)
	for i, tran := range trans {
		seq := uint64(i)
		envelopeBytes, _ := proto.Marshal(tran.Envelope)
		block.Data.Data = append(block.Data.Data, envelopeBytes)
		if tran.Pvtws != nil {
			blockPvtdata[seq] = &ledger.TxPvtData{SeqInBlock: seq, WriteSet: tran.Pvtws}
		}
	}
	block.Header.DataHash = block.Data.Hash()
	g.lastNum++
	g.lastHash = block.Header.Hash()
	setBlockFlagsToValid(block)
	return &ledger.BlockAndPvtData{Block: block, BlockPvtData: blockPvtdata}
}
