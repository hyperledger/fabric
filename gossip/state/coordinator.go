/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/gossip"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/pkg/errors"
)

// PvtData a placeholder to represent private data
type PvtData struct {
	Payload *ledger.TxPvtData
}

// PvtDataCollections data type to encapsulate collections
// of private data
type PvtDataCollections []*PvtData

// Marshal encodes private collection into bytes array
func (pvt *PvtDataCollections) Marshal() ([][]byte, error) {
	pvtDataBytes := make([][]byte, 0)
	for index, each := range *pvt {
		if each.Payload == nil {
			errMsg := fmt.Sprintf("Mallformed private data payload, rwset index %d, payload is nil", index)
			logger.Errorf(errMsg)
			return nil, errors.New(errMsg)
		}
		pvtBytes, err := proto.Marshal(each.Payload.WriteSet)
		if err != nil {
			errMsg := fmt.Sprintf("Could not marshal private rwset index %d, due to %s", index, err)
			logger.Errorf(errMsg)
			return nil, errors.New(errMsg)
		}
		// Compose gossip protobuf message with private rwset + index of transaction in the block
		txSeqInBlock := each.Payload.SeqInBlock
		pvtDataPayload := &gossip.PvtDataPayload{TxSeqInBlock: txSeqInBlock, Payload: pvtBytes}
		payloadBytes, err := proto.Marshal(pvtDataPayload)
		if err != nil {
			errMsg := fmt.Sprintf("Could not marshal private payload with transaction index %d, due to %s", txSeqInBlock, err)
			logger.Errorf(errMsg)
			return nil, errors.New(errMsg)
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
			return err
		}
		pvtRWSet := &rwset.TxPvtReadWriteSet{}
		if err := proto.Unmarshal(payload.Payload, pvtRWSet); err != nil {
			return err
		}
		*pvt = append(*pvt, &PvtData{Payload: &ledger.TxPvtData{
			SeqInBlock: payload.TxSeqInBlock,
			WriteSet:   pvtRWSet,
		}})
	}

	return nil
}

// PvtDataFilter predicate which used to filter block
// private data
type PvtDataFilter func(data *PvtData) bool

// Coordinator orchestrates the flow of the new
// blocks arrival and in flight transient data, responsible
// to complete missing parts of transient data for given block.
type Coordinator interface {
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data ...PvtDataCollections) ([]string, error)

	// GetPvtDataAndBlockByNum returns block and related to the block private data
	GetPvtDataAndBlockByNum(seqNum uint64, filter PvtDataFilter) (*common.Block, PvtDataCollections, error)

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

func (c *coordinator) StoreBlock(block *common.Block, data ...PvtDataCollections) ([]string, error) {
	// Need to check whenever there are missing private rwset
	if len(data) == 0 {
		return nil, c.Commit(block)
	}
	return nil, c.Commit(block)
}

func (c *coordinator) GetPvtDataAndBlockByNum(seqNum uint64, filter PvtDataFilter) (*common.Block, PvtDataCollections, error) {
	blocks := c.GetBlocks([]uint64{seqNum})
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("Cannot retreive block number %d", seqNum)
	}
	return blocks[0], nil, nil
}

func (c *coordinator) GetBlockByNum(seqNum uint64) (*common.Block, error) {
	blocks := c.GetBlocks([]uint64{seqNum})
	if len(blocks) == 0 {
		return nil, fmt.Errorf("Cannot retreive block number %d", seqNum)
	}
	return blocks[0], nil
}
