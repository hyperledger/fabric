/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package genesis

import (
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
)

const (
	msgVersion = int32(1)

	// These values are fixed for the genesis block.
	epoch = 0
)

// Factory facilitates the creation of genesis blocks.
type Factory interface {
	// Block returns a genesis block for a given channel ID.
	Block(channelID string) *cb.Block
}

type factory struct {
	channelGroup *cb.ConfigGroup
}

// NewFactoryImpl creates a new Factory.
func NewFactoryImpl(channelGroup *cb.ConfigGroup) Factory {
	return &factory{channelGroup: channelGroup}
}

// Block constructs and returns a genesis block for a given channel ID.
func (f *factory) Block(channelID string) *cb.Block {
	payloadChannelHeader := protoutil.MakeChannelHeader(cb.HeaderType_CONFIG, msgVersion, channelID, epoch)
	payloadSignatureHeader := protoutil.MakeSignatureHeader(nil, protoutil.CreateNonceOrPanic())
	protoutil.SetTxID(payloadChannelHeader, payloadSignatureHeader)
	payloadHeader := protoutil.MakePayloadHeader(payloadChannelHeader, payloadSignatureHeader)
	payload := &cb.Payload{Header: payloadHeader, Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{Config: &cb.Config{ChannelGroup: f.channelGroup}})}
	envelope := &cb.Envelope{Payload: protoutil.MarshalOrPanic(payload), Signature: nil}

	block := protoutil.NewBlock(0, nil)
	block.Data = &cb.BlockData{Data: [][]byte{protoutil.MarshalOrPanic(envelope)}}
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
	block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.LastConfig{Index: 0}),
	})
	block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES] = protoutil.MarshalOrPanic(&cb.Metadata{
		Value: protoutil.MarshalOrPanic(&cb.OrdererBlockMetadata{
			LastConfig: &cb.LastConfig{Index: 0},
		}),
	})
	return block
}
