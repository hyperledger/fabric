/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package common

import (
	"testing"

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
	last = &LastConfig{
		Index: uint64(1),
	}
	last.Reset()
	assert.Equal(t, uint64(0), last.Index)
	_, _ = last.Descriptor()
	_ = last.String()
	last.ProtoMessage()

	var meta *Metadata
	meta = nil
	assert.Nil(t, meta.GetSignatures())
	meta = &Metadata{
		Signatures: []*MetadataSignature{&MetadataSignature{}},
	}
	assert.NotNil(t, meta.GetSignatures())
	meta.Reset()
	assert.Nil(t, meta.GetSignatures())
	_, _ = meta.Descriptor()
	_ = meta.String()
	meta.ProtoMessage()

	var sig *MetadataSignature
	sig = &MetadataSignature{
		Signature: []byte("signature"),
	}
	sig.Reset()
	assert.Nil(t, sig.Signature)
	_, _ = sig.Descriptor()
	_ = sig.String()
	sig.ProtoMessage()

	var header *Header
	header = &Header{
		ChannelHeader: []byte("channel header"),
	}
	header.Reset()
	assert.Nil(t, header.ChannelHeader)
	_, _ = header.Descriptor()
	_ = header.String()
	header.ProtoMessage()

	var chheader *ChannelHeader
	chheader = nil
	assert.Nil(t, chheader.GetTimestamp())
	chheader = &ChannelHeader{
		Timestamp: &google_protobuf.Timestamp{},
	}
	assert.NotNil(t, chheader.GetTimestamp())
	chheader.Reset()
	assert.Nil(t, chheader.GetTimestamp())
	_, _ = chheader.Descriptor()
	_ = chheader.String()
	chheader.ProtoMessage()

	var sigheader *SignatureHeader
	sigheader = &SignatureHeader{
		Creator: []byte("creator"),
	}
	sigheader.Reset()
	assert.Nil(t, sigheader.Creator)
	_, _ = sigheader.Descriptor()
	_ = sigheader.String()
	sigheader.ProtoMessage()

	var payload *Payload
	payload = nil
	assert.Nil(t, payload.GetHeader())
	payload = &Payload{
		Header: &Header{},
	}
	assert.NotNil(t, payload.GetHeader())
	payload.Reset()
	assert.Nil(t, payload.Data)
	_, _ = payload.Descriptor()
	_ = payload.String()
	payload.ProtoMessage()

	var env *Envelope
	env = &Envelope{
		Payload: []byte("payload"),
	}
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

	bh := &BlockHeader{
		PreviousHash: []byte("hash"),
	}
	bh.Reset()
	assert.Nil(t, bh.PreviousHash)
	_, _ = bh.Descriptor()
	_ = bh.String()
	bh.ProtoMessage()

	bd := &BlockData{
		Data: [][]byte{},
	}
	bd.Reset()
	assert.Nil(t, bd.Data)
	_, _ = bd.Descriptor()
	_ = bd.String()
	bd.ProtoMessage()

	bm := &BlockMetadata{
		Metadata: [][]byte{},
	}
	bm.Reset()
	assert.Nil(t, bm.Metadata)
	_, _ = bm.Descriptor()
	_ = bm.String()
	bm.ProtoMessage()

}
