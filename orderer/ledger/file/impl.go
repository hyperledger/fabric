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

package fileledger

import (
	"github.com/hyperledger/fabric/common/ledger/blkstorage"
	ledger "github.com/hyperledger/fabric/orderer/ledger"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/fileledger")
var closedChan chan struct{}

func init() {
	closedChan = make(chan struct{})
	close(closedChan)
}

type fileLedger struct {
	blockStore blkstorage.BlockStore
	signal     chan struct{}
}

type fileLedgerIterator struct {
	ledger      *fileLedger
	blockNumber uint64
}

// Next blocks until there is a new block available, or returns an error if the
// next block is no longer retrievable
func (i *fileLedgerIterator) Next() (*cb.Block, cb.Status) {
	for {
		if i.blockNumber < i.ledger.Height() {
			block, err := i.ledger.blockStore.RetrieveBlockByNumber(i.blockNumber)
			if err != nil {
				return nil, cb.Status_SERVICE_UNAVAILABLE
			}
			i.blockNumber++
			return block, cb.Status_SUCCESS
		}
		<-i.ledger.signal
	}
}

// ReadyChan supplies a channel which will block until Next will not block
func (i *fileLedgerIterator) ReadyChan() <-chan struct{} {
	signal := i.ledger.signal
	if i.blockNumber > i.ledger.Height()-1 {
		return signal
	}
	return closedChan
}

// Iterator returns an Iterator, as specified by a cb.SeekInfo message, and its
// starting block number
func (fl *fileLedger) Iterator(startPosition *ab.SeekPosition) (ledger.Iterator, uint64) {
	switch start := startPosition.Type.(type) {
	case *ab.SeekPosition_Oldest:
		return &fileLedgerIterator{ledger: fl, blockNumber: 0}, 0
	case *ab.SeekPosition_Newest:
		info, err := fl.blockStore.GetBlockchainInfo()
		if err != nil {
			logger.Panic(err)
		}
		newestBlockNumber := info.Height - 1
		return &fileLedgerIterator{ledger: fl, blockNumber: newestBlockNumber}, newestBlockNumber
	case *ab.SeekPosition_Specified:
		height := fl.Height()
		if start.Specified.Number > height {
			return &ledger.NotFoundErrorIterator{}, 0
		}
		return &fileLedgerIterator{ledger: fl, blockNumber: start.Specified.Number}, start.Specified.Number
	default:
		return &ledger.NotFoundErrorIterator{}, 0
	}
}

// Height returns the number of blocks on the ledger
func (fl *fileLedger) Height() uint64 {
	info, err := fl.blockStore.GetBlockchainInfo()
	if err != nil {
		logger.Panic(err)
	}
	return info.Height
}

// Append a new block to the ledger
func (fl *fileLedger) Append(block *cb.Block) error {
	err := fl.blockStore.AddBlock(block)
	if err == nil {
		close(fl.signal)
		fl.signal = make(chan struct{})
	}
	return err
}
