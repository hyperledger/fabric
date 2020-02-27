/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package genesis

import (
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

func TestFactory(t *testing.T) {
	impl := NewFactoryImpl(protoutil.NewConfigGroup())
	block := impl.Block("testchannelid")

	t.Run("test for transaction id", func(t *testing.T) {
		configEnv, _ := protoutil.ExtractEnvelope(block, 0)
		configEnvPayload, _ := protoutil.UnmarshalPayload(configEnv.Payload)
		configEnvPayloadChannelHeader, _ := protoutil.UnmarshalChannelHeader(configEnvPayload.GetHeader().ChannelHeader)
		assert.NotEmpty(t, configEnvPayloadChannelHeader.TxId, "tx_id of configuration transaction should not be empty")
	})
	t.Run("test for last config in SIGNATURES field", func(t *testing.T) {
		metadata := &cb.Metadata{}
		err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES], metadata)
		assert.NoError(t, err)
		ordererBlockMetadata := &cb.OrdererBlockMetadata{}
		err = proto.Unmarshal(metadata.Value, ordererBlockMetadata)
		assert.NoError(t, err)
		assert.NotNil(t, ordererBlockMetadata.LastConfig)
		assert.Equal(t, uint64(0), ordererBlockMetadata.LastConfig.Index)
	})
	t.Run("test for last config in LAST_CONFIG field", func(t *testing.T) {
		metadata := &cb.Metadata{}
		err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG], metadata)
		assert.NoError(t, err)
		lastConfig := &cb.LastConfig{}
		err = proto.Unmarshal(metadata.Value, lastConfig)
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), lastConfig.Index)
	})
}
