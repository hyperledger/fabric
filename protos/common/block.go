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

package common

import (
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
)

// NewBlock construct a block with no data and no metadata.
func NewBlock(seqNum uint64, previousHash []byte) *Block {
	block := &Block{}
	block.Header = &BlockHeader{}
	block.Header.Number = seqNum
	block.Header.PreviousHash = previousHash
	block.Data = &BlockData{}

	var metadataContents [][]byte
	for i := 0; i < len(BlockMetadataIndex_name); i++ {
		metadataContents = append(metadataContents, []byte{})
	}
	block.Metadata = &BlockMetadata{Metadata: metadataContents}

	return block
}

// Bytes returns the marshaled representation of the block header.
func (b *BlockHeader) Bytes() []byte {
	data, err := proto.Marshal(b) // XXX this is wrong, protobuf is not the right mechanism to serialize for a hash
	if err != nil {
		panic("This should never fail and is generally irrecoverable")
	}
	return data
}

// Hash returns the hash of the block header.
func (b *BlockHeader) Hash() []byte {
	return util.ComputeCryptoHash(b.Bytes())
}

// Hash returns the hash of the marshaled representation of the block data.
func (b *BlockData) Hash() []byte {
	data, err := proto.Marshal(b) // XXX this is wrong, protobuf is not the right mechanism to serialize for a hash, AND, it is not a MerkleTree hash
	if err != nil {
		panic("This should never fail and is generally irrecoverable")
	}

	return util.ComputeCryptoHash(data)
}
