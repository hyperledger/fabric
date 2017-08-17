/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/pkg/errors"
)

// PvtDataCollections data type to encapsulate collections
// of private data
type PvtDataCollections []*ledger.TxPvtData

// Marshal encodes private collection into bytes array
func (pvt *PvtDataCollections) Marshal() ([][]byte, error) {
	pvtDataBytes := make([][]byte, 0)
	for index, each := range *pvt {
		if each == nil {
			err := errors.Errorf("Mallformed private data payload, rwset index %d is nil", index)
			logger.Errorf("%+v", err)
			return nil, err
		}
		pvtBytes, err := proto.Marshal(each.WriteSet)
		if err != nil {
			err = errors.Wrapf(err, "Could not marshal private rwset index %d", index)
			logger.Errorf("%+v", err)
			return nil, err
		}
		// Compose gossip protobuf message with private rwset + index of transaction in the block
		txSeqInBlock := each.SeqInBlock
		pvtDataPayload := &gossip.PvtDataPayload{TxSeqInBlock: txSeqInBlock, Payload: pvtBytes}
		payloadBytes, err := proto.Marshal(pvtDataPayload)
		if err != nil {
			err = errors.Wrapf(err, "Could not marshal private payload with transaction index %d", txSeqInBlock)
			logger.Errorf("%+v", err)
			return nil, err
		}

		pvtDataBytes = append(pvtDataBytes, payloadBytes)
	}
	return pvtDataBytes, nil
}

// Unmarshal read and unmarshal collection of private data
// from given bytes array
func (pvt *PvtDataCollections) Unmarshal(data [][]byte) error {
	for _, each := range data {
		payload := &gossip.PvtDataPayload{}
		if err := proto.Unmarshal(each, payload); err != nil {
			return errors.WithStack(err)
		}
		pvtRWSet := &rwset.TxPvtReadWriteSet{}
		if err := proto.Unmarshal(payload.Payload, pvtRWSet); err != nil {
			return errors.WithStack(err)
		}
		*pvt = append(*pvt, &ledger.TxPvtData{
			SeqInBlock: payload.TxSeqInBlock,
			WriteSet:   pvtRWSet,
		})
	}

	return nil
}

// Coordinator orchestrates the flow of the new
// blocks arrival and in flight transient data, responsible
// to complete missing parts of transient data for given block.
type Coordinator interface {
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data PvtDataCollections) ([]string, error)

	// GetPvtDataAndBlockByNum get block by number and returns also all related private data
	// the order of private data in slice of PvtDataCollections doesn't implies the order of
	// transactions in the block related to these private data, to get the correct placement
	// need to read TxPvtData.SeqInBlock field
	GetPvtDataAndBlockByNum(seqNum uint64) (*common.Block, PvtDataCollections, error)

	// GetBlockByNum returns block and related to the block private data
	GetBlockByNum(seqNum uint64) (*common.Block, error)

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

// StoreBlock stores block with private data into the ledger
func (c *coordinator) StoreBlock(block *common.Block, data PvtDataCollections) ([]string, error) {
	// Need to check whenever there are missing private rwset
	if len(data) > 0 {
		blockAndPvtData := &ledger.BlockAndPvtData{
			Block:        block,
			BlockPvtData: make(map[uint64]*ledger.TxPvtData),
		}

		// Fill private data map with payloads
		for _, item := range data {
			blockAndPvtData.BlockPvtData[item.SeqInBlock] = item
		}

		// commit block and private data
		return nil, c.CommitWithPvtData(blockAndPvtData)
	}
	// TODO: Since this could be a very first time
	// block arrives from ordering service we need
	// to check whenever there is private data available
	// for this block in transient store and
	// if no private data, we can use simple API
	return nil, c.Commit(block)
}

// GetPvtDataAndBlockByNum get block by number and returns also all related private data
// the order of private data in slice of PvtDataCollections doesn't implies the order of
// transactions in the block related to these private data, to get the correct placement
// need to read TxPvtData.SeqInBlock field
func (c *coordinator) GetPvtDataAndBlockByNum(seqNum uint64) (*common.Block, PvtDataCollections, error) {
	blockAndPvtData, err := c.Committer.GetPvtDataAndBlockByNum(seqNum)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "Cannot retreive block number %d", seqNum)
	}

	var blockPvtData PvtDataCollections

	for _, item := range blockAndPvtData.BlockPvtData {
		blockPvtData = append(blockPvtData, item)
	}

	return blockAndPvtData.Block, blockPvtData, nil
}

// GetBlockByNum returns block by sequence number
func (c *coordinator) GetBlockByNum(seqNum uint64) (*common.Block, error) {
	blocks := c.GetBlocks([]uint64{seqNum})
	if len(blocks) == 0 {
		return nil, errors.Errorf("Cannot retreive block number %d", seqNum)
	}
	return blocks[0], nil
}
