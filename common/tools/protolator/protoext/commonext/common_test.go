/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commonext

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/stretchr/testify/assert"
)

func TestCommonProtolator(t *testing.T) {
	// Envelope
	env := &Envelope{Envelope: &common.Envelope{}}
	assert.Equal(t, []string{"payload"}, env.StaticallyOpaqueFields())
	msg, err := env.StaticallyOpaqueFieldProto("badproto")
	assert.Nil(t, msg)
	assert.Error(t, err)
	msg, err = env.StaticallyOpaqueFieldProto("payload")
	assert.NoError(t, err)
	assert.Equal(t, &common.Payload{}, msg)

	// Payload
	payload := &Payload{Payload: &common.Payload{}}
	assert.Equal(t, []string{"data"}, payload.VariablyOpaqueFields())
	msg, err = payload.VariablyOpaqueFieldProto("badproto")
	assert.Nil(t, msg)
	assert.Error(t, err)
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Nil(t, msg)
	assert.Error(t, err)

	payload = &Payload{
		Payload: &common.Payload{
			Header: &common.Header{
				ChannelHeader: []byte("badbytes"),
			},
		},
	}
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Nil(t, msg)
	assert.Error(t, err)

	ch := &common.ChannelHeader{
		Type: int32(common.HeaderType_CONFIG),
	}
	chbytes, _ := proto.Marshal(ch)
	payload = &Payload{
		Payload: &common.Payload{
			Header: &common.Header{
				ChannelHeader: chbytes,
			},
		},
	}
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Equal(t, &common.ConfigEnvelope{}, msg)
	assert.NoError(t, err)

	ch = &common.ChannelHeader{
		Type: int32(common.HeaderType_CONFIG_UPDATE),
	}
	chbytes, _ = proto.Marshal(ch)
	payload = &Payload{
		Payload: &common.Payload{
			Header: &common.Header{
				ChannelHeader: chbytes,
			},
		},
	}
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Equal(t, &common.ConfigUpdateEnvelope{}, msg)
	assert.NoError(t, err)

	ch = &common.ChannelHeader{
		Type: int32(common.HeaderType_CHAINCODE_PACKAGE),
	}
	chbytes, _ = proto.Marshal(ch)
	payload = &Payload{
		Payload: &common.Payload{
			Header: &common.Header{
				ChannelHeader: chbytes,
			},
		},
	}
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Nil(t, msg)
	assert.Error(t, err)

	// Header
	var header *Header
	assert.Equal(t, []string{"channel_header", "signature_header"},
		header.StaticallyOpaqueFields())

	msg, err = header.StaticallyOpaqueFieldProto("badproto")
	assert.Nil(t, msg)
	assert.Error(t, err)

	msg, err = header.StaticallyOpaqueFieldProto("channel_header")
	assert.Equal(t, &common.ChannelHeader{}, msg)
	assert.NoError(t, err)

	msg, err = header.StaticallyOpaqueFieldProto("signature_header")
	assert.Equal(t, &common.SignatureHeader{}, msg)
	assert.NoError(t, err)

	// BlockData
	var bd *BlockData
	assert.Equal(t, []string{"data"}, bd.StaticallyOpaqueSliceFields())

	msg, err = bd.StaticallyOpaqueSliceFieldProto("badslice", 0)
	assert.Nil(t, msg)
	assert.Error(t, err)
	msg, err = bd.StaticallyOpaqueSliceFieldProto("data", 0)
	assert.Equal(t, &common.Envelope{}, msg)
	assert.NoError(t, err)
}
