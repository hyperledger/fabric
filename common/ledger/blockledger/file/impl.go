/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fileledger

import (
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/common/ledger/blockledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var logger = flogging.MustGetLogger("common.ledger.blockledger.file")

// FileLedger is a struct used to interact with a node's ledger
type FileLedger struct {
	blockStore FileLedgerBlockStore
	signal     chan struct{}
}

// FileLedgerBlockStore defines the interface to interact with deliver when using a
// file ledger
type FileLedgerBlockStore interface {
	AddBlock(block *cb.Block) error
	GetBlockchainInfo() (*cb.BlockchainInfo, error)
	RetrieveBlocks(startBlockNumber uint64) (ledger.ResultsIterator, error)
}

// NewFileLedger creates a new FileLedger for interaction with the ledger
func NewFileLedger(blockStore FileLedgerBlockStore) *FileLedger {
	return &FileLedger{blockStore: blockStore, signal: make(chan struct{})}
}

type fileLedgerIterator struct {
	ledger         *FileLedger
	blockNumber    uint64
	commonIterator ledger.ResultsIterator
}

// Next blocks until there is a new block available, or until Close is called.
// It returns an error if the next block is no longer retrievable.
func (i *fileLedgerIterator) Next() (*cb.Block, cb.Status) {
	result, err := i.commonIterator.Next()
	if err != nil {
		logger.Error(err)
		return nil, cb.Status_SERVICE_UNAVAILABLE
	}
	// Cover the case where another thread calls Close on the iterator.
	if result == nil {
		return nil, cb.Status_SERVICE_UNAVAILABLE
	}
	return result.(*cb.Block), cb.Status_SUCCESS
}

// Close releases resources acquired by the Iterator
func (i *fileLedgerIterator) Close() {
	i.commonIterator.Close()
}

// Iterator returns an Iterator, as specified by an ab.SeekInfo message, and its
// starting block number
func (fl *FileLedger) Iterator(startPosition *ab.SeekPosition) (blockledger.Iterator, uint64) {
	var startingBlockNumber uint64
	switch start := startPosition.Type.(type) {
	case *ab.SeekPosition_Oldest:
		startingBlockNumber = 0
	case *ab.SeekPosition_Newest:
		info, err := fl.blockStore.GetBlockchainInfo()
		if err != nil {
			logger.Panic(err)
		}
		newestBlockNumber := info.Height - 1
		startingBlockNumber = newestBlockNumber
	case *ab.SeekPosition_Specified:
		startingBlockNumber = start.Specified.Number
		height := fl.Height()
		if startingBlockNumber > height {
			return &blockledger.NotFoundErrorIterator{}, 0
		}
	default:
		return &blockledger.NotFoundErrorIterator{}, 0
	}

	iterator, err := fl.blockStore.RetrieveBlocks(startingBlockNumber)
	if err != nil {
		return &blockledger.NotFoundErrorIterator{}, 0
	}

	return &fileLedgerIterator{ledger: fl, blockNumber: startingBlockNumber, commonIterator: iterator}, startingBlockNumber
}

// Height returns the number of blocks on the ledger
func (fl *FileLedger) Height() uint64 {
	info, err := fl.blockStore.GetBlockchainInfo()
	if err != nil {
		logger.Panic(err)
	}
	return info.Height
}

// Append a new block to the ledger
func (fl *FileLedger) Append(block *cb.Block) error {
	err := fl.blockStore.AddBlock(block)
	if err == nil {
		close(fl.signal)
		fl.signal = make(chan struct{})
	}
	return err
}
