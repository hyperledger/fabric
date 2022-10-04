/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tests

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

// committer helps in cutting a block and commits the block (with pvt data) to the ledger
type committer struct {
	lgr    ledger.PeerLedger
	blkgen *blkGenerator
	assert *require.Assertions
}

func newCommitter(lgr ledger.PeerLedger, t *testing.T) *committer {
	return &committer{lgr, newBlockGenerator(lgr, t), require.New(t)}
}

// cutBlockAndCommitLegacy cuts the next block from the given 'txAndPvtdata' and commits the block (with pvt data) to the ledger
// This function return a copy of 'ledger.BlockAndPvtData' that was submitted to the ledger to commit.
// A copy is returned instead of the actual one because, ledger makes some changes to the submitted block before commit
// (such as setting the metadata) and the test code would want to have the exact copy of the block that was submitted to
// the ledger
func (c *committer) cutBlockAndCommitLegacy(trans []*txAndPvtdata, missingPvtData ledger.TxMissingPvtData) *ledger.BlockAndPvtData {
	blk := c.blkgen.nextBlockAndPvtdata(trans, missingPvtData)
	blkCopy := c.copyOfBlockAndPvtdata(blk)
	c.assert.NoError(
		c.lgr.CommitLegacy(blk, &ledger.CommitOptions{}),
	)
	return blkCopy
}

func (c *committer) cutBlockAndCommitExpectError(trans []*txAndPvtdata, missingPvtData ledger.TxMissingPvtData) *ledger.BlockAndPvtData {
	blk := c.blkgen.nextBlockAndPvtdata(trans, missingPvtData)
	blkCopy := c.copyOfBlockAndPvtdata(blk)
	err := c.lgr.CommitLegacy(blk, &ledger.CommitOptions{})
	c.assert.Error(err)
	return blkCopy
}

func (c *committer) copyOfBlockAndPvtdata(blk *ledger.BlockAndPvtData) *ledger.BlockAndPvtData {
	blkBytes, err := proto.Marshal(blk.Block)
	c.assert.NoError(err)
	blkCopy := &common.Block{}
	c.assert.NoError(proto.Unmarshal(blkBytes, blkCopy))
	return &ledger.BlockAndPvtData{
		Block: blkCopy, PvtData: blk.PvtData,
		MissingPvtData: blk.MissingPvtData,
	}
}

// ///////////////   block generation code  ///////////////////////////////////////////
// blkGenerator helps creating the next block for the ledger
type blkGenerator struct {
	lastNum  uint64
	lastHash []byte
	assert   *require.Assertions
}

// newBlockGenerator constructs a 'blkGenerator' and initializes the 'blkGenerator'
// from the last block available in the ledger so that the next block can be populated
// with the correct block number and previous block hash
func newBlockGenerator(lgr ledger.PeerLedger, t *testing.T) *blkGenerator {
	require := require.New(t)
	info, err := lgr.GetBlockchainInfo()
	require.NoError(err)
	return &blkGenerator{info.Height - 1, info.CurrentBlockHash, require}
}

// nextBlockAndPvtdata cuts the next block
func (g *blkGenerator) nextBlockAndPvtdata(trans []*txAndPvtdata, missingPvtData ledger.TxMissingPvtData) *ledger.BlockAndPvtData {
	block := protoutil.NewBlock(g.lastNum+1, g.lastHash)
	blockPvtdata := make(map[uint64]*ledger.TxPvtData)
	for i, tran := range trans {
		seq := uint64(i)
		envelopeBytes, _ := proto.Marshal(tran.Envelope)
		block.Data.Data = append(block.Data.Data, envelopeBytes)
		if tran.Pvtws != nil {
			blockPvtdata[seq] = &ledger.TxPvtData{SeqInBlock: seq, WriteSet: tran.Pvtws}
		}
	}
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
	g.lastNum++
	g.lastHash = protoutil.BlockHeaderHash(block.Header)
	setBlockFlagsToValid(block)
	return &ledger.BlockAndPvtData{
		Block: block, PvtData: blockPvtdata,
		MissingPvtData: missingPvtData,
	}
}
