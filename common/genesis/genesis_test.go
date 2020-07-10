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
	"github.com/stretchr/testify/require"
)

func TestFactory(t *testing.T) {
	impl := NewFactoryImpl(protoutil.NewConfigGroup())
	block := impl.Block("testchannelid")

	t.Run("test for transaction id", func(t *testing.T) {
		configEnv, _ := protoutil.ExtractEnvelope(block, 0)
		configEnvPayload, _ := protoutil.UnmarshalPayload(configEnv.Payload)
		configEnvPayloadChannelHeader, _ := protoutil.UnmarshalChannelHeader(configEnvPayload.GetHeader().ChannelHeader)
		require.NotEmpty(t, configEnvPayloadChannelHeader.TxId, "tx_id of configuration transaction should not be empty")
	})
	t.Run("test for last config in SIGNATURES field", func(t *testing.T) {
		metadata := &cb.Metadata{}
		err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_SIGNATURES], metadata)
		require.NoError(t, err)
		ordererBlockMetadata := &cb.OrdererBlockMetadata{}
		err = proto.Unmarshal(metadata.Value, ordererBlockMetadata)
		require.NoError(t, err)
		require.NotNil(t, ordererBlockMetadata.LastConfig)
		require.Equal(t, uint64(0), ordererBlockMetadata.LastConfig.Index)
	})
	t.Run("test for last config in LAST_CONFIG field", func(t *testing.T) {
		metadata := &cb.Metadata{}
		err := proto.Unmarshal(block.Metadata.Metadata[cb.BlockMetadataIndex_LAST_CONFIG], metadata)
		require.NoError(t, err)
		lastConfig := &cb.LastConfig{}
		err = proto.Unmarshal(metadata.Value, lastConfig)
		require.NoError(t, err)
		require.Equal(t, uint64(0), lastConfig.Index)
	})
}
