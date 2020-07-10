/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgprocessor

import (
	"testing"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/orderer/common/msgprocessor/mocks"
	"github.com/stretchr/testify/require"
)

func TestMaxBytesRule(t *testing.T) {
	dataSize := uint32(100)
	maxBytes := calcMessageBytesForPayloadDataSize(dataSize)
	mockResources := &mocks.Resources{}
	mockOrdererConfig := &mocks.OrdererConfig{}
	mockResources.OrdererConfigReturns(mockOrdererConfig, true)

	mockOrdererConfig.BatchSizeReturns(
		&ab.BatchSize{
			AbsoluteMaxBytes: maxBytes,
		},
	)
	msf := NewSizeFilter(mockResources)

	t.Run("Less Than", func(t *testing.T) {
		require.Nil(t, msf.Apply(makeMessage(make([]byte, dataSize-1))))
	})

	t.Run("Exact", func(t *testing.T) {
		require.Nil(t, msf.Apply(makeMessage(make([]byte, dataSize))))
	})

	t.Run("Too Big", func(t *testing.T) {
		require.NotNil(t, msf.Apply(makeMessage(make([]byte, dataSize+1))))
	})

	t.Run("Dynamic Resources", func(t *testing.T) {
		require.NotNil(t, msf.Apply(makeMessage(make([]byte, dataSize+1))))
		mockOrdererConfig.BatchSizeReturns(
			&ab.BatchSize{
				AbsoluteMaxBytes: maxBytes + 2,
			},
		)
		require.Nil(t, msf.Apply(makeMessage(make([]byte, dataSize+1))))
	})
}

func calcMessageBytesForPayloadDataSize(dataSize uint32) uint32 {
	return messageByteSize(makeMessage(make([]byte, dataSize)))
}

func makeMessage(data []byte) *cb.Envelope {
	data, err := proto.Marshal(&cb.Payload{Data: data})
	if err != nil {
		panic(err)
	}
	return &cb.Envelope{Payload: data}
}
