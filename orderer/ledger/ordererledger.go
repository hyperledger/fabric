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

package ordererledger

import (
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"

	"github.com/golang/protobuf/proto"
)

// Factory retrieves or creates new ledgers by chainID
type Factory interface {
	// GetOrCreate gets an existing ledger (if it exists) or creates it if it does not
	GetOrCreate(chainID string) (ReadWriter, error)

	// ChainIDs returns the chain IDs the Factory is aware of
	ChainIDs() []string
}

// Iterator is useful for a chain Reader to stream blocks as they are created
type Iterator interface {
	// Next blocks until there is a new block available, or returns an error if the next block is no longer retrievable
	Next() (*cb.Block, cb.Status)
	// ReadyChan supplies a channel which will block until Next will not block
	ReadyChan() <-chan struct{}
}

// Reader allows the caller to inspect the orderer ledger
type Reader interface {
	// Iterator retrieves an Iterator, as specified by an cb.SeekInfo message, returning an iterator, and its starting block number
	Iterator(startType *ab.SeekPosition) (Iterator, uint64)
	// Height returns the highest block number in the chain, plus one
	Height() uint64
}

// Writer allows the caller to modify the orderer ledger
type Writer interface {
	// Append a new block to the ledger
	Append(block *cb.Block) error
}

// ReadWriter encapsulated both the reading and writing functions of the ordererledger
type ReadWriter interface {
	Reader
	Writer
}

// CreateNextBlock provides a utility way to construct the next block from contents and metadata for a given ledger
// XXX this will need to be modified to accept marshaled envelopes to accomodate non-deterministic marshaling
func CreateNextBlock(rl Reader, messages []*cb.Envelope) *cb.Block {
	var nextBlockNumber uint64
	var previousBlockHash []byte

	if rl.Height() > 0 {
		it, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Newest{&ab.SeekNewest{}}})
		<-it.ReadyChan() // Should never block, but just in case
		block, status := it.Next()
		if status != cb.Status_SUCCESS {
			panic("Error seeking to newest block for chain with non-zero height")
		}
		nextBlockNumber = block.Header.Number + 1
		previousBlockHash = block.Header.Hash()
	}

	data := &cb.BlockData{
		Data: make([][]byte, len(messages)),
	}

	var err error
	for i, msg := range messages {
		data.Data[i], err = proto.Marshal(msg)
		if err != nil {
			panic(err)
		}
	}

	block := cb.NewBlock(nextBlockNumber, previousBlockHash)
	block.Header.DataHash = data.Hash()
	block.Data = data

	return block
}

// GetBlock is a utility method for retrieving a single block
func GetBlock(rl Reader, index uint64) *cb.Block {
	i, _ := rl.Iterator(&ab.SeekPosition{Type: &ab.SeekPosition_Specified{Specified: &ab.SeekSpecified{Number: index}}})
	select {
	case <-i.ReadyChan():
		block, status := i.Next()
		if status != cb.Status_SUCCESS {
			return nil
		}
		return block
	default:
		return nil
	}
}
