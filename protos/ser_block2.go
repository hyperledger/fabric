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

package protos

import (
	"encoding/binary"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/core/util"
)

// SerBlock2 is responsible for serialized structure of block
type SerBlock2 struct {
	txOffsets  []int
	blockHash  []byte
	blockBytes []byte
}

// ConstructSerBlock2 constructs `SerBlock2` from `Block2`
func ConstructSerBlock2(block *Block2) (*SerBlock2, error) {
	txOffsets := []int{}
	buf := proto.NewBuffer(nil)
	if err := buf.EncodeRawBytes(block.PreviousBlockHash); err != nil {
		return nil, err
	}
	blockBytes := buf.Bytes()
	numTxs := len(block.Transactions)
	for i := 0; i < numTxs; i++ {
		nextTxOffset := len(blockBytes)
		txOffsets = append(txOffsets, nextTxOffset)
		blockBytes = append(blockBytes, block.Transactions[i]...)
	}
	trailerOffset := len(blockBytes)
	txOffsets = append(txOffsets, trailerOffset)
	buf = proto.NewBuffer(blockBytes)
	if err := buf.EncodeVarint(uint64(numTxs)); err != nil {
		return nil, err
	}
	for i := 0; i < numTxs; i++ {
		if err := buf.EncodeVarint(uint64(txOffsets[i])); err != nil {
			return nil, err
		}
	}
	logger.Debugf("ConstructSerBlock2():TxOffsets=%#v", txOffsets)
	blockBytes = buf.Bytes()
	lastBytes := intToBytes(uint32(trailerOffset))
	blockBytes = append(blockBytes, lastBytes...)
	return &SerBlock2{txOffsets: txOffsets, blockBytes: blockBytes}, nil
}

// NewSerBlock2 constructs `SerBlock2` from serialized block bytes
func NewSerBlock2(blockBytes []byte) *SerBlock2 {
	return &SerBlock2{blockBytes: blockBytes}
}

// GetBytes gives the serialized bytes
func (serBlock *SerBlock2) GetBytes() []byte {
	return serBlock.blockBytes
}

// ComputeHash computes the crypto-hash of block
func (serBlock *SerBlock2) ComputeHash() []byte {
	if serBlock.blockHash == nil {
		serBlock.blockHash = util.ComputeCryptoHash(serBlock.blockBytes)
	}
	return serBlock.blockHash
}

// GetPreviousBlockHash retrieves PreviousBlockHash from serialized bytes
func (serBlock *SerBlock2) GetPreviousBlockHash() ([]byte, error) {
	buf := proto.NewBuffer(serBlock.blockBytes)
	return serBlock.extractPreviousBlockHash(buf)
}

// GetTxOffsets retrieves transactions offsets from serialized bytes.
// Transactions are serialized in an continuous manner - e.g,
// second transaction starts immediately after first transaction ends.
// The last entry in the return value is an extra entry in order to indicate
// the length of the last transaction
func (serBlock *SerBlock2) GetTxOffsets() ([]int, error) {
	if serBlock.txOffsets == nil {
		var err error
		buf := proto.NewBuffer(serBlock.blockBytes)
		if _, err = serBlock.extractPreviousBlockHash(buf); err != nil {
			return nil, err
		}
		serBlock.txOffsets, err = serBlock.extractTxOffsets()
		return serBlock.txOffsets, err
	}
	return serBlock.txOffsets, nil
}

// ToBlock2 reconstructs `Block2` from `serBlock`
func (serBlock *SerBlock2) ToBlock2() (*Block2, error) {
	block := &Block2{}
	buf := proto.NewBuffer(serBlock.blockBytes)
	var err error
	var previousBlockHash []byte
	var txOffsets []int
	if previousBlockHash, err = serBlock.extractPreviousBlockHash(buf); err != nil {
		return nil, err
	}
	if txOffsets, err = serBlock.extractTxOffsets(); err != nil {
		return nil, err
	}
	block.PreviousBlockHash = previousBlockHash
	block.Transactions = make([][]byte, len(txOffsets)-1)
	for i := 0; i < len(txOffsets)-1; i++ {
		block.Transactions[i] = serBlock.blockBytes[txOffsets[i]:txOffsets[i+1]]
	}
	return block, nil
}

func (serBlock *SerBlock2) extractPreviousBlockHash(buf *proto.Buffer) ([]byte, error) {
	var err error
	var previousBlockHash []byte
	if previousBlockHash, err = buf.DecodeRawBytes(false); err != nil {
		return nil, err
	}
	return previousBlockHash, err
}

func (serBlock *SerBlock2) extractTxOffsets() ([]int, error) {
	lastBytes := serBlock.blockBytes[len(serBlock.blockBytes)-4:]
	trailerOffset := int(bytesToInt(lastBytes))
	trailerBytes := serBlock.blockBytes[trailerOffset:]
	buf := proto.NewBuffer(trailerBytes)
	txOffsets := []int{}
	var err error
	var numTxs uint64
	if numTxs, err = buf.DecodeVarint(); err != nil {
		return nil, err
	}
	var nextTxOffset uint64
	for i := 0; i < int(numTxs); i++ {
		if nextTxOffset, err = buf.DecodeVarint(); err != nil {
			return nil, err
		}
		txOffsets = append(txOffsets, int(nextTxOffset))
	}
	txOffsets = append(txOffsets, trailerOffset)
	logger.Debugf("extractTxOffsets():TxOffsets=%#v", txOffsets)
	return txOffsets, nil
}

func intToBytes(i uint32) []byte {
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, i)
	return bs
}

func bytesToInt(b []byte) uint32 {
	return binary.LittleEndian.Uint32(b)
}
