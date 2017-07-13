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

package ledger

import (
	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric/protos/common"
	ab "github.com/hyperledger/fabric/protos/orderer"
)

var closedChan chan struct{}

func init() {
	closedChan = make(chan struct{})
	close(closedChan)
}

// NotFoundErrorIterator simply always returns an error of cb.Status_NOT_FOUND,
// and is generally useful for implementations of the Reader interface
type NotFoundErrorIterator struct{}

// Next returns nil, cb.Status_NOT_FOUND
func (nfei *NotFoundErrorIterator) Next() (*cb.Block, cb.Status) {
	return nil, cb.Status_NOT_FOUND
}

// ReadyChan returns a closed channel
func (nfei *NotFoundErrorIterator) ReadyChan() <-chan struct{} {
	return closedChan
}

// CreateNextBlock provides a utility way to construct the next block from
// contents and metadata for a given ledger
// XXX This will need to be modified to accept marshaled envelopes
//     to accommodate non-deterministic marshaling
func CreateNextBlock(rl Reader, messages []*cb.Envelope) *cb.Block {
	var nextBlockNumber uint64
	var previousBlockHash []byte

	if rl.Height() > 0 {
		it, _ := rl.Iterator(&ab.SeekPosition{
			Type: &ab.SeekPosition_Newest{
				&ab.SeekNewest{},
			},
		})
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
	i, _ := rl.Iterator(&ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{Number: index},
		},
	})
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
