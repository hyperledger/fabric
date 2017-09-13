/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	mockconfig "github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/common/blockcutter"
	cb "github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/utils"
)

// ConsenterSupport is used to mock the multichannel.ConsenterSupport interface
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

	// ClassifyMsgVal is returned by ClassifyMsg
	ClassifyMsgVal msgprocessor.Classification

	// ConfigSeqVal is returned as the configSeq for Process*Msg
	ConfigSeqVal uint64

	// ProcessNormalMsgErr is returned as the error for ProcessNormalMsg
	ProcessNormalMsgErr error

	// ProcessConfigUpdateMsgVal is returned as the error for ProcessConfigUpdateMsg
	ProcessConfigUpdateMsgVal *cb.Envelope

	// ProcessConfigUpdateMsgErr is returned as the error for ProcessConfigUpdateMsg
	ProcessConfigUpdateMsgErr error

	// ProcessConfigMsgVal is returned as the error for ProcessConfigMsg
	ProcessConfigMsgVal *cb.Envelope

	// ProcessConfigMsgErr is returned by ProcessConfigMsg
	ProcessConfigMsgErr error

	// SequenceVal is returned by Sequence
	SequenceVal uint64
}

// BlockCutter returns BlockCutterVal
func (mcs *ConsenterSupport) BlockCutter() blockcutter.Receiver {
	return mcs.BlockCutterVal
}

// SharedConfig returns SharedConfigVal
func (mcs *ConsenterSupport) SharedConfig() channelconfig.Orderer {
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
func (mcs *ConsenterSupport) WriteBlock(block *cb.Block, encodedMetadataValue []byte) {
	if encodedMetadataValue != nil {
		block.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER] = utils.MarshalOrPanic(&cb.Metadata{Value: encodedMetadataValue})
	}
	mcs.HeightVal++
	mcs.Blocks <- block
}

// WriteConfigBlock calls WriteBlock
func (mcs *ConsenterSupport) WriteConfigBlock(block *cb.Block, encodedMetadataValue []byte) {
	mcs.WriteBlock(block, encodedMetadataValue)
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

// ClassifyMsg returns ClassifyMsgVal, ClassifyMsgErr
func (mcs *ConsenterSupport) ClassifyMsg(chdr *cb.ChannelHeader) msgprocessor.Classification {
	return mcs.ClassifyMsgVal
}

// ProcessNormalMsg returns ConfigSeqVal, ProcessNormalMsgErr
func (mcs *ConsenterSupport) ProcessNormalMsg(env *cb.Envelope) (configSeq uint64, err error) {
	return mcs.ConfigSeqVal, mcs.ProcessNormalMsgErr
}

// ProcessConfigUpdateMsg returns ProcessConfigUpdateMsgVal, ConfigSeqVal, ProcessConfigUpdateMsgErr
func (mcs *ConsenterSupport) ProcessConfigUpdateMsg(env *cb.Envelope) (config *cb.Envelope, configSeq uint64, err error) {
	return mcs.ProcessConfigUpdateMsgVal, mcs.ConfigSeqVal, mcs.ProcessConfigUpdateMsgErr
}

// ProcessConfigMsg returns ProcessConfigMsgVal, ConfigSeqVal, ProcessConfigMsgErr
func (mcs *ConsenterSupport) ProcessConfigMsg(env *cb.Envelope) (*cb.Envelope, uint64, error) {
	return mcs.ProcessConfigMsgVal, mcs.ConfigSeqVal, mcs.ProcessConfigMsgErr
}

// Sequence returns SequenceVal
func (mcs *ConsenterSupport) Sequence() uint64 {
	return mcs.SequenceVal
}
