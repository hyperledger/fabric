/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privdata

import (
	"fmt"

	"github.com/hyperledger/fabric/core/committer"
	"github.com/hyperledger/fabric/core/common/privdata"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/transientstore"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
)

var logger *logging.Logger // package-level logger

func init() {
	logger = util.GetLogger(util.LoggingPrivModule, "")
}

// TransientStore holds private data that the corresponding blocks haven't been committed yet into the ledger
type TransientStore interface {
	// Persist stores the private read-write set of a transaction in the transient store
	Persist(txid string, endorsementBlkHt uint64, privateSimulationResults *rwset.TxPvtReadWriteSet) error
	// GetTxPvtRWSetByTxid returns an iterator due to the fact that the txid may have multiple private
	// RWSets persisted from different endorsers (via Gossip)
	GetTxPvtRWSetByTxid(txid string, filter ledger.PvtNsCollFilter) (*transientstore.RwsetScanner, error)
}

// PrivateDataDistributor distributes private data to peers
type PrivateDataDistributor interface {
	// Distribute distributes a given private data with a specific transactionID
	// to peers according policies that are derived from the given PolicyStore and PolicyParser
	Distribute(privateData *rwset.TxPvtReadWriteSet, txID string, ps privdata.PolicyStore, pp privdata.PolicyParser) error
}

// Coordinator orchestrates the flow of the new
// blocks arrival and in flight transient data, responsible
// to complete missing parts of transient data for given block.
type Coordinator interface {
	PrivateDataDistributor
	// StoreBlock deliver new block with underlined private data
	// returns missing transaction ids
	StoreBlock(block *common.Block, data util.PvtDataCollections) ([]string, error)

	// GetPvtDataAndBlockByNum get block by number and returns also all related private data
	// the order of private data in slice of PvtDataCollections doesn't implies the order of
	// transactions in the block related to these private data, to get the correct placement
	// need to read TxPvtData.SeqInBlock field
	GetPvtDataAndBlockByNum(seqNum uint64) (*common.Block, util.PvtDataCollections, error)

	// GetBlockByNum returns block and related to the block private data
	GetBlockByNum(seqNum uint64) (*common.Block, error)

	// Get recent block sequence number
	LedgerHeight() (uint64, error)

	// Close coordinator, shuts down coordinator service
	Close()
}

type coordinator struct {
	committer.Committer
	TransientStore
}

// NewCoordinator creates a new instance of coordinator
func NewCoordinator(committer committer.Committer, store TransientStore) Coordinator {
	return &coordinator{Committer: committer, TransientStore: store}
}

// Distribute distributes a given private data with a specific transactionID
// to peers according policies that are derived from the given PolicyStore and PolicyParser
func (c *coordinator) Distribute(privateData *rwset.TxPvtReadWriteSet, txID string, ps privdata.PolicyStore, pp privdata.PolicyParser) error {
	// TODO: also need to distribute the data...
	return c.TransientStore.Persist(txID, 0, privateData)
}

// StoreBlock stores block with private data into the ledger
func (c *coordinator) StoreBlock(block *common.Block, data util.PvtDataCollections) ([]string, error) {
	blockAndPvtData := &ledger.BlockAndPvtData{
		Block:        block,
		BlockPvtData: make(map[uint64]*ledger.TxPvtData),
	}

	// Fill private data map with payloads
	for _, item := range data {
		blockAndPvtData.BlockPvtData[item.SeqInBlock] = item
	}

	transientStorePrivateData, err := c.retrievePrivateData(block)
	if err != nil {
		return nil, errors.Wrap(err, "Failed retrieving private data from transientStore")
	}
	// In any case, overwrite the private data from the block with what is stored in the transient store
	// TODO: verify the hashes match
	for seqInBlock, txPvtRWSet := range transientStorePrivateData {
		blockAndPvtData.BlockPvtData[seqInBlock] = txPvtRWSet
	}

	// commit block and private data
	return nil, c.CommitWithPvtData(blockAndPvtData)
}

// GetPvtDataAndBlockByNum get block by number and returns also all related private data
// the order of private data in slice of PvtDataCollections doesn't implies the order of
// transactions in the block related to these private data, to get the correct placement
// need to read TxPvtData.SeqInBlock field
func (c *coordinator) GetPvtDataAndBlockByNum(seqNum uint64) (*common.Block, util.PvtDataCollections, error) {
	blockAndPvtData, err := c.Committer.GetPvtDataAndBlockByNum(seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("Cannot retreive block number %d, due to %s", seqNum, err)
	}

	var blockPvtData util.PvtDataCollections

	for _, item := range blockAndPvtData.BlockPvtData {
		blockPvtData = append(blockPvtData, item)
	}

	return blockAndPvtData.Block, blockPvtData, nil
}

// GetBlockByNum returns block by sequence number
func (c *coordinator) GetBlockByNum(seqNum uint64) (*common.Block, error) {
	blocks := c.GetBlocks([]uint64{seqNum})
	if len(blocks) == 0 {
		return nil, fmt.Errorf("Cannot retreive block number %d", seqNum)
	}
	return blocks[0], nil
}

func (c *coordinator) retrievePrivateData(block *common.Block) (map[uint64]*ledger.TxPvtData, error) {
	pvtdata := make(map[uint64]*ledger.TxPvtData)
	for txIndex, envBytes := range block.Data.Data {
		env, err := utils.GetEnvelopeFromBlock(envBytes)
		if err != nil {
			return nil, err
		}
		payload, err := utils.GetPayload(env)
		if err != nil {
			return nil, err
		}
		chdr, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
		if err != nil {
			return nil, err
		}
		// TODO: For now, we assume that all peers have access to all collections.
		// Once the peer to collection mapping is available via RSCC,
		// we need to build the filter based on the ns/collections that this peer
		// has access to and pass the filter to GetTxPvtRWSetByTxid instead of nil.
		iter, err := c.TransientStore.GetTxPvtRWSetByTxid(chdr.TxId, nil)
		if err != nil {
			return nil, err
		}

		var pvtEndorsement *transientstore.EndorserPvtSimulationResults

		for {
			pvtEndorsement, err = iter.Next()
			if err != nil {
				return nil, err
			}
			if pvtEndorsement == nil {
				break
			}
			// TODO: When we introduce collection filters, we need to compare hashes
			// and collect the correct private write set. For now, we assume that
			// all entries are correct and each entry contains required collections.
			// Hence, with the first entry, we can break this loop.
			break
		}
		iter.Close()

		if pvtEndorsement == nil {
			continue
		}
		txPvtRWSet := pvtEndorsement.PvtSimulationResults
		seqInBlock := uint64(txIndex)
		pvtdata[seqInBlock] = &ledger.TxPvtData{SeqInBlock: seqInBlock, WriteSet: txPvtRWSet}
	}
	return pvtdata, nil
}
