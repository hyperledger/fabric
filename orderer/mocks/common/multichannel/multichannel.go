/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multichannel

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/orderer/common/blockcutter"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor"
	mockblockcutter "github.com/hyperledger/fabric/orderer/mocks/common/blockcutter"
	"github.com/hyperledger/fabric/protoutil"
)

// ConsenterSupport is used to mock the multichannel.ConsenterSupport interface
// Whenever a block is written, it writes to the Batches channel to allow for synchronization
type ConsenterSupport struct {
	// SharedConfigVal is the value returned by SharedConfig()
	SharedConfigVal channelconfig.Orderer

	// ChannelConfigVal is the value returned by ChannelConfig()
	ChannelConfigVal channelconfig.Channel

	// BlockCutterVal is the value returned by BlockCutter()
	BlockCutterVal *mockblockcutter.Receiver

	// BlockByIndex maps block numbers to retrieved values of these blocks
	BlockByIndex map[uint64]*cb.Block

	// Blocks is the channel where WriteBlock writes the most recently created block,
	Blocks chan *cb.Block

	// ChannelIDVal is the value returned by ChannelID()
	ChannelIDVal string

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

	// BlockVerificationErr is returned by VerifyBlockSignature
	BlockVerificationErr error
}

// Block returns the block with the given number or nil if not found
func (mcs *ConsenterSupport) Block(number uint64) *cb.Block {
	return mcs.BlockByIndex[number]
}

// BlockCutter returns BlockCutterVal
func (mcs *ConsenterSupport) BlockCutter() blockcutter.Receiver {
	return mcs.BlockCutterVal
}

// SharedConfig returns SharedConfigVal
func (mcs *ConsenterSupport) SharedConfig() channelconfig.Orderer {
	return mcs.SharedConfigVal
}

// ChannelConfig returns ChannelConfigVal
func (mcs *ConsenterSupport) ChannelConfig() channelconfig.Channel {
	return mcs.ChannelConfigVal
}

// CreateNextBlock creates a simple block structure with the given data
func (mcs *ConsenterSupport) CreateNextBlock(data []*cb.Envelope) *cb.Block {
	block := protoutil.NewBlock(0, nil)
	mtxs := make([][]byte, len(data))
	for i := range data {
		mtxs[i] = protoutil.MarshalOrPanic(data[i])
	}
	block.Data = &cb.BlockData{Data: mtxs}
	mcs.NextBlockVal = block
	return block
}

// WriteBlock writes data to the Blocks channel
func (mcs *ConsenterSupport) WriteBlock(block *cb.Block, encodedMetadataValue []byte) {
	if encodedMetadataValue != nil {
		block.Metadata.Metadata[cb.BlockMetadataIndex_ORDERER] = protoutil.MarshalOrPanic(&cb.Metadata{Value: encodedMetadataValue})
	}
	mcs.Append(block)
}

// WriteConfigBlock calls WriteBlock
func (mcs *ConsenterSupport) WriteConfigBlock(block *cb.Block, encodedMetadataValue []byte) {
	mcs.WriteBlock(block, encodedMetadataValue)
}

// ChannelID returns the channel ID this specific consenter instance is associated with
func (mcs *ConsenterSupport) ChannelID() string {
	return mcs.ChannelIDVal
}

// Height returns the number of blocks of the chain this specific consenter instance is associated with
func (mcs *ConsenterSupport) Height() uint64 {
	return mcs.HeightVal
}

// Sign returns the bytes passed in
func (mcs *ConsenterSupport) Sign(message []byte) ([]byte, error) {
	return message, nil
}

// Serialize returns bytes
func (mcs *ConsenterSupport) Serialize() ([]byte, error) {
	return []byte("creator"), nil
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

func (mcs *ConsenterSupport) SignatureVerifier() protoutil.BlockVerifierFunc {
	return func(header *cb.BlockHeader, metadata *cb.BlockMetadata) error {
		return mcs.BlockVerificationErr
	}
}

// Append appends a new block to the ledger in its raw form,
// unlike WriteBlock that also mutates its metadata.
func (mcs *ConsenterSupport) Append(block *cb.Block) error {
	mcs.HeightVal++
	mcs.Blocks <- block
	return nil
}
