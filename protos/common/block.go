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
	"encoding/asn1"
	"fmt"
	"math"

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

type asn1Header struct {
	Number       int64
	PreviousHash []byte
	DataHash     []byte
}

// Bytes returns the ASN.1 marshaled representation of the block header.
func (b *BlockHeader) Bytes() []byte {
	asn1Header := asn1Header{
		PreviousHash: b.PreviousHash,
		DataHash:     b.DataHash,
	}
	if b.Number > uint64(math.MaxInt64) {
		panic(fmt.Errorf("Golang does not currently support encoding uint64 to asn1"))
	} else {
		asn1Header.Number = int64(b.Number)
	}
	result, err := asn1.Marshal(asn1Header)
	if err != nil {
		// Errors should only arise for types which cannot be encoded, since the
		// BlockHeader type is known a-priori to contain only encodable types, an
		// error here is fatal and should not be propogated
		panic(err)
	}
	return result
}

// Hash returns the hash of the block header.
// XXX This method will be removed shortly to allow for confgurable hashing algorithms
func (b *BlockHeader) Hash() []byte {
	return util.ComputeSHA256(b.Bytes())
}

// Bytes returns a deterministically serialized version of the BlockData
// eventually, this should be replaced with a true Merkle tree construction,
// but for the moment, we assume a Merkle tree of infinite width (uint32_max)
// which degrades to a flat hash
func (b *BlockData) Bytes() []byte {
	return util.ConcatenateBytes(b.Data...)
}

// Hash returns the hash of the marshaled representation of the block data.
func (b *BlockData) Hash() []byte {
	return util.ComputeSHA256(b.Bytes())
}
