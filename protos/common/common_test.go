/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/golang/protobuf/proto"
	google_protobuf "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/stretchr/testify/assert"
)

func TestCommonEnums(t *testing.T) {
	var status Status

	status = 0
	assert.Equal(t, "UNKNOWN", status.String())
	status = 200
	assert.Equal(t, "SUCCESS", status.String())
	status = 400
	assert.Equal(t, "BAD_REQUEST", status.String())
	status = 403
	assert.Equal(t, "FORBIDDEN", status.String())
	status = 404
	assert.Equal(t, "NOT_FOUND", status.String())
	status = 413
	assert.Equal(t, "REQUEST_ENTITY_TOO_LARGE", status.String())
	status = 500
	assert.Equal(t, "INTERNAL_SERVER_ERROR", status.String())
	status = 503
	assert.Equal(t, "SERVICE_UNAVAILABLE", status.String())
	_, _ = status.EnumDescriptor()

	var header HeaderType
	header = 0
	assert.Equal(t, "MESSAGE", header.String())
	header = 1
	assert.Equal(t, "CONFIG", header.String())
	header = 2
	assert.Equal(t, "CONFIG_UPDATE", header.String())
	header = 3
	assert.Equal(t, "ENDORSER_TRANSACTION", header.String())
	header = 4
	assert.Equal(t, "ORDERER_TRANSACTION", header.String())
	header = 5
	assert.Equal(t, "DELIVER_SEEK_INFO", header.String())
	header = 6
	assert.Equal(t, "CHAINCODE_PACKAGE", header.String())
	_, _ = header.EnumDescriptor()

	var index BlockMetadataIndex
	index = 0
	assert.Equal(t, "SIGNATURES", index.String())
	index = 1
	assert.Equal(t, "LAST_CONFIG", index.String())
	index = 2
	assert.Equal(t, "TRANSACTIONS_FILTER", index.String())
	index = 3
	assert.Equal(t, "ORDERER", index.String())
	_, _ = index.EnumDescriptor()
}

func TestCommonStructs(t *testing.T) {
	var last *LastConfig
	last = nil
	assert.Equal(t, uint64(0), last.GetIndex())
	last = &LastConfig{
		Index: uint64(1),
	}
	assert.Equal(t, uint64(1), last.GetIndex())
	last.Reset()
	assert.Equal(t, uint64(0), last.GetIndex())
	_, _ = last.Descriptor()
	_ = last.String()
	last.ProtoMessage()

	var meta *Metadata
	meta = nil
	assert.Nil(t, meta.GetSignatures())
	assert.Nil(t, meta.GetValue())
	meta = &Metadata{
		Value:      []byte("value"),
		Signatures: []*MetadataSignature{&MetadataSignature{}},
	}
	assert.NotNil(t, meta.GetSignatures())
	assert.NotNil(t, meta.GetValue())
	meta.Reset()
	assert.Nil(t, meta.GetSignatures())
	_, _ = meta.Descriptor()
	_ = meta.String()
	meta.ProtoMessage()

	var sig *MetadataSignature
	sig = nil
	assert.Nil(t, sig.GetSignature())
	assert.Nil(t, sig.GetSignatureHeader())
	sig = &MetadataSignature{
		SignatureHeader: []byte("header"),
		Signature:       []byte("signature"),
	}
	assert.NotNil(t, sig.GetSignature())
	assert.NotNil(t, sig.GetSignatureHeader())
	sig.Reset()
	assert.Nil(t, sig.GetSignature())
	assert.Nil(t, sig.GetSignatureHeader())
	_, _ = sig.Descriptor()
	_ = sig.String()
	sig.ProtoMessage()

	var header *Header
	header = nil
	assert.Nil(t, header.GetChannelHeader())
	assert.Nil(t, header.GetSignatureHeader())
	header = &Header{
		ChannelHeader:   []byte("channel header"),
		SignatureHeader: []byte("signature header"),
	}
	assert.NotNil(t, header.GetChannelHeader())
	assert.NotNil(t, header.GetSignatureHeader())
	header.Reset()
	assert.Nil(t, header.GetChannelHeader())
	assert.Nil(t, header.GetSignatureHeader())
	_, _ = header.Descriptor()
	_ = header.String()
	header.ProtoMessage()

	var chheader *ChannelHeader
	chheader = nil
	assert.Equal(t, "", chheader.GetChannelId())
	assert.Equal(t, "", chheader.GetTxId())
	assert.Equal(t, uint64(0), chheader.GetEpoch())
	assert.Equal(t, int32(0), chheader.GetType())
	assert.Equal(t, int32(0), chheader.GetVersion())
	assert.Nil(t, chheader.GetExtension())
	assert.Nil(t, chheader.GetTimestamp())
	chheader = &ChannelHeader{
		ChannelId: "ChannelId",
		TxId:      "TxId",
		Timestamp: &google_protobuf.Timestamp{},
		Extension: []byte{},
	}
	assert.Equal(t, "ChannelId", chheader.GetChannelId())
	assert.Equal(t, "TxId", chheader.GetTxId())
	assert.Equal(t, uint64(0), chheader.GetEpoch())
	assert.Equal(t, int32(0), chheader.GetType())
	assert.Equal(t, int32(0), chheader.GetVersion())
	assert.NotNil(t, chheader.GetExtension())
	assert.NotNil(t, chheader.GetTimestamp())
	chheader.Reset()
	assert.Nil(t, chheader.GetTimestamp())
	_, _ = chheader.Descriptor()
	_ = chheader.String()
	chheader.ProtoMessage()

	var sigheader *SignatureHeader
	sigheader = nil
	assert.Nil(t, sigheader.GetCreator())
	assert.Nil(t, sigheader.GetNonce())
	sigheader = &SignatureHeader{
		Creator: []byte("creator"),
		Nonce:   []byte("nonce"),
	}
	assert.NotNil(t, sigheader.GetCreator())
	assert.NotNil(t, sigheader.GetNonce())
	sigheader.Reset()
	assert.Nil(t, sigheader.Creator)
	_, _ = sigheader.Descriptor()
	_ = sigheader.String()
	sigheader.ProtoMessage()

	var payload *Payload
	payload = nil
	assert.Nil(t, payload.GetHeader())
	assert.Nil(t, payload.GetData())
	payload = &Payload{
		Header: &Header{},
		Data:   []byte("data"),
	}
	assert.NotNil(t, payload.GetHeader())
	assert.NotNil(t, payload.GetData())
	payload.Reset()
	assert.Nil(t, payload.Data)
	_, _ = payload.Descriptor()
	_ = payload.String()
	payload.ProtoMessage()

	var env *Envelope
	env = nil
	assert.Nil(t, env.GetPayload())
	assert.Nil(t, env.GetSignature())
	env = &Envelope{
		Payload:   []byte("payload"),
		Signature: []byte("signature"),
	}
	assert.NotNil(t, env.GetPayload())
	assert.NotNil(t, env.GetSignature())
	env.Reset()
	assert.Nil(t, env.Payload)
	_, _ = env.Descriptor()
	_ = env.String()
	env.ProtoMessage()

	b := &Block{
		Data: &BlockData{},
	}
	b.Reset()
	assert.Nil(t, b.GetData())
	_, _ = b.Descriptor()
	_ = b.String()
	b.ProtoMessage()

	var bh *BlockHeader
	bh = nil
	assert.Nil(t, bh.GetDataHash())
	assert.Nil(t, bh.GetPreviousHash())
	assert.Equal(t, uint64(0), bh.GetNumber())
	bh = &BlockHeader{
		PreviousHash: []byte("hash"),
		DataHash:     []byte("dataHash"),
		Number:       uint64(1),
	}
	assert.NotNil(t, bh.GetDataHash())
	assert.NotNil(t, bh.GetPreviousHash())
	assert.Equal(t, uint64(1), bh.GetNumber())
	bh.Reset()
	assert.Nil(t, bh.GetPreviousHash())
	_, _ = bh.Descriptor()
	_ = bh.String()
	bh.ProtoMessage()

	var bd *BlockData
	bd = nil
	assert.Nil(t, bd.GetData())
	bd = &BlockData{
		Data: [][]byte{},
	}
	assert.NotNil(t, bd.GetData())
	bd.Reset()
	assert.Nil(t, bd.GetData())
	_, _ = bd.Descriptor()
	_ = bd.String()
	bd.ProtoMessage()

	var bm *BlockMetadata
	bm = nil
	assert.Nil(t, bm.GetMetadata())
	bm = &BlockMetadata{
		Metadata: [][]byte{},
	}
	assert.NotNil(t, bm.GetMetadata())
	bm.Reset()
	assert.Nil(t, bm.GetMetadata())
	_, _ = bm.Descriptor()
	_ = bm.String()
	bm.ProtoMessage()

}

func TestCommonProtolator(t *testing.T) {
	// Envelope
	env := &Envelope{}
	assert.Equal(t, []string{"payload"}, env.StaticallyOpaqueFields())
	msg, err := env.StaticallyOpaqueFieldProto("badproto")
	assert.Nil(t, msg)
	assert.Error(t, err)
	msg, err = env.StaticallyOpaqueFieldProto("payload")
	// Payload
	payload := &Payload{}
	assert.Equal(t, &Payload{}, msg)
	assert.NoError(t, err)
	assert.Equal(t, []string{"data"}, payload.VariablyOpaqueFields())
	msg, err = payload.VariablyOpaqueFieldProto("badproto")
	assert.Nil(t, msg)
	assert.Error(t, err)
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Nil(t, msg)
	assert.Error(t, err)

	payload = &Payload{
		Header: &Header{
			ChannelHeader: []byte("badbytes"),
		},
	}
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Nil(t, msg)
	assert.Error(t, err)

	ch := &ChannelHeader{
		Type: int32(HeaderType_CONFIG),
	}
	chbytes, _ := proto.Marshal(ch)
	payload = &Payload{
		Header: &Header{
			ChannelHeader: chbytes,
		},
	}
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Equal(t, &ConfigEnvelope{}, msg)
	assert.NoError(t, err)

	ch = &ChannelHeader{
		Type: int32(HeaderType_CONFIG_UPDATE),
	}
	chbytes, _ = proto.Marshal(ch)
	payload = &Payload{
		Header: &Header{
			ChannelHeader: chbytes,
		},
	}
	msg, err = payload.VariablyOpaqueFieldProto("data")
	assert.Equal(t, &ConfigUpdateEnvelope{}, msg)
	assert.NoError(t, err)

	ch = &ChannelHeader{
		Type: int32(HeaderType_CHAINCODE_PACKAGE),
	}
	chbytes, _ = proto.Marshal(ch)
	payload = &Payload{
		Header: &Header{
			ChannelHeader: chbytes,
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
	assert.Equal(t, &ChannelHeader{}, msg)
	assert.NoError(t, err)

	msg, err = header.StaticallyOpaqueFieldProto("signature_header")
	assert.Equal(t, &SignatureHeader{}, msg)
	assert.NoError(t, err)

	// BlockData
	var bd *BlockData
	assert.Equal(t, []string{"data"}, bd.StaticallyOpaqueSliceFields())

	msg, err = bd.StaticallyOpaqueSliceFieldProto("badslice", 0)
	assert.Nil(t, msg)
	assert.Error(t, err)
	msg, err = bd.StaticallyOpaqueSliceFieldProto("data", 0)
	assert.Equal(t, &Envelope{}, msg)
	assert.NoError(t, err)

}
