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

package multichain

import (
	"github.com/hyperledger/fabric/common/config"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/filter"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/blockcutter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"

	"github.com/op/go-logging"
)

var logger = logging.MustGetLogger("orderer/mocks/multichain")

// ConsenterSupport is used to mock the multichain.ConsenterSupport interface
// Whenever a block is written, it writes to the Batches channel to allow for synchronization
type ConsenterSupport struct {
	// SharedConfigVal is the value returned by SharedConfig()
	SharedConfigVal *mockconfig.Orderer

	// BlockCutterVal is the value returned by BlockCutter()
	BlockCutterVal *mockblockcutter.Receiver

	// Blocks is the channel where WriteBlock writes the most recently created block
	Blocks chan *cb.Block

	// ChainIDVal is the value returned by ChainID()
	ChainIDVal string

	// HeightVal is the value returned by Height()
	HeightVal uint64

	// NextBlockVal stores the block created by the most recent CreateNextBlock() call
	NextBlockVal *cb.Block
}

// BlockCutter returns BlockCutterVal
func (mcs *ConsenterSupport) BlockCutter() blockcutter.Receiver {
	return mcs.BlockCutterVal
}

// SharedConfig returns SharedConfigVal
func (mcs *ConsenterSupport) SharedConfig() config.Orderer {
	return mcs.SharedConfigVal
}

// CreateNextBlock creates a simple block structure with the given data
func (mcs *ConsenterSupport) CreateNextBlock(data []*cb.Envelope) *cb.Block {
	block := cb.NewBlock(0, nil)
	mtxs := make([][]byte, len(data))
	for i := range data {
		mtxs[i] = utils.MarshalOrPanic(data[i])
	}
	block.Data = &cb.BlockData{Data: mtxs}
	mcs.NextBlockVal = block
	return block
}

// WriteBlock writes data to the Blocks channel
// Note that _committers is ignored by this mock implementation
func (mcs *ConsenterSupport) WriteBlock(block *cb.Block, _committers []filter.Committer, encodedMetadataValue []byte) *cb.Block {
	if encodedMetadataValue != nil {
		block.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER] = utils.MarshalOrPanic(&cb.Metadata{Value: encodedMetadataValue})
	}
	mcs.HeightVal++
	mcs.Blocks <- block
	return block
}

// ChainID returns the chain ID this specific consenter instance is associated with
func (mcs *ConsenterSupport) ChainID() string {
	return mcs.ChainIDVal
}

// Height returns the number of blocks of the chain this specific consenter instance is associated with
func (mcs *ConsenterSupport) Height() uint64 {
	return mcs.HeightVal
}

// Sign returns the bytes passed in
func (mcs *ConsenterSupport) Sign(message []byte) ([]byte, error) {
	return message, nil
}

// NewSignatureHeader returns an empty signature header
func (mcs *ConsenterSupport) NewSignatureHeader() (*cb.SignatureHeader, error) {
	return &cb.SignatureHeader{}, nil
}
