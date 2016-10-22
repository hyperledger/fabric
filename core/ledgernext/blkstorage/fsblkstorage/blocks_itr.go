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

package fsblkstorage

import (
	"fmt"

	"github.com/hyperledger/fabric/core/ledgernext"
	"github.com/hyperledger/fabric/protos"
)

// BlockHolder holds block bytes
type BlockHolder struct {
	blockBytes []byte
}

// GetBlock serializes Block from block bytes
func (bh *BlockHolder) GetBlock() *protos.Block2 {
	serBlock := protos.NewSerBlock2(bh.blockBytes)
	block, err := serBlock.ToBlock2()
	if err != nil {
		panic(fmt.Errorf("Problem in deserialzing block: %s", err))
	}
	return block
}

// GetBlockBytes returns block bytes
func (bh *BlockHolder) GetBlockBytes() []byte {
	return bh.blockBytes
}

// BlocksItr - an iterator for iterating over a sequence of blocks
type BlocksItr struct {
	stream            *blockStream
	nextBlockBytes    []byte
	err               error
	numTotalBlocks    int
	numIteratedBlocks int
}

func newBlockItr(stream *blockStream, numTotalBlocks int) *BlocksItr {
	return &BlocksItr{stream, nil, nil, numTotalBlocks, 0}
}

// Next moves the cursor to next block and returns true iff the iterator is not exhausted
func (itr *BlocksItr) Next() bool {
	if itr.err != nil || itr.numIteratedBlocks == itr.numTotalBlocks {
		return false
	}
	itr.nextBlockBytes, itr.err = itr.stream.nextBlockBytes()
	itr.numIteratedBlocks++
	return itr.nextBlockBytes != nil
}

// Get returns the block at current cursor
func (itr *BlocksItr) Get() (ledger.QueryResult, error) {
	if itr.err != nil {
		return nil, itr.err
	}
	return &BlockHolder{itr.nextBlockBytes}, nil
}

// Close releases any resources held by the iterator
func (itr *BlocksItr) Close() {
	itr.stream.close()
}
