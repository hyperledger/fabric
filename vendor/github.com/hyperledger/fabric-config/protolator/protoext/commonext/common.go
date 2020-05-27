/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commonext

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric-protos-go/peer"
)

type Envelope struct{ *common.Envelope }

func (e *Envelope) Underlying() proto.Message {
	return e.Envelope
}

func (e *Envelope) StaticallyOpaqueFields() []string {
	return []string{"payload"}
}

func (e *Envelope) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != e.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	return &common.Payload{}, nil
}

type Payload struct{ *common.Payload }

func (p *Payload) Underlying() proto.Message {
	return p.Payload
}

func (p *Payload) VariablyOpaqueFields() []string {
	return []string{"data"}
}

func (p *Payload) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != p.VariablyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	if p.Header == nil {
		return nil, fmt.Errorf("cannot determine payload type when header is missing")
	}
	ch := &common.ChannelHeader{}
	if err := proto.Unmarshal(p.Header.ChannelHeader, ch); err != nil {
		return nil, fmt.Errorf("corrupt channel header: %s", err)
	}

	switch ch.Type {
	case int32(common.HeaderType_CONFIG):
		return &common.ConfigEnvelope{}, nil
	case int32(common.HeaderType_ORDERER_TRANSACTION):
		return &common.Envelope{}, nil
	case int32(common.HeaderType_CONFIG_UPDATE):
		return &common.ConfigUpdateEnvelope{}, nil
	case int32(common.HeaderType_MESSAGE):
		// Only used by broadcast_msg sample client
		return &common.ConfigValue{}, nil
	case int32(common.HeaderType_ENDORSER_TRANSACTION):
		return &peer.Transaction{}, nil
	default:
		return nil, fmt.Errorf("decoding type %v is unimplemented", ch.Type)
	}
}

type ChannelHeader struct{ *common.ChannelHeader }

func (ch *ChannelHeader) Underlying() proto.Message {
	return ch.ChannelHeader
}

func (ch *ChannelHeader) VariablyOpaqueFields() []string {
	return []string{"extension"}
}

func (ch *ChannelHeader) VariablyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != "extension" {
		return nil, fmt.Errorf("not an opaque field")
	}

	switch ch.Type {
	case int32(common.HeaderType_ENDORSER_TRANSACTION):
		return &peer.ChaincodeHeaderExtension{}, nil
	default:
		return nil, fmt.Errorf("channel header extension only valid for endorser transactions")
	}
}

type Header struct{ *common.Header }

func (h *Header) Underlying() proto.Message {
	return h.Header
}

func (h *Header) StaticallyOpaqueFields() []string {
	return []string{"channel_header", "signature_header"}
}

func (h *Header) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	switch name {
	case h.StaticallyOpaqueFields()[0]: // channel_header
		return &common.ChannelHeader{}, nil
	case h.StaticallyOpaqueFields()[1]: // signature_header
		return &common.SignatureHeader{}, nil
	default:
		return nil, fmt.Errorf("unknown header field: %s", name)
	}
}

type SignatureHeader struct{ *common.SignatureHeader }

func (sh *SignatureHeader) Underlying() proto.Message {
	return sh.SignatureHeader
}

func (sh *SignatureHeader) StaticallyOpaqueFields() []string {
	return []string{"creator"}
}

func (sh *SignatureHeader) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	switch name {
	case sh.StaticallyOpaqueFields()[0]: // creator
		return &msp.SerializedIdentity{}, nil
	default:
		return nil, fmt.Errorf("unknown header field: %s", name)
	}
}

type BlockData struct{ *common.BlockData }

func (bd *BlockData) Underlying() proto.Message {
	return bd.BlockData
}

func (bd *BlockData) StaticallyOpaqueSliceFields() []string {
	return []string{"data"}
}

func (bd *BlockData) StaticallyOpaqueSliceFieldProto(fieldName string, index int) (proto.Message, error) {
	if fieldName != bd.StaticallyOpaqueSliceFields()[0] {
		return nil, fmt.Errorf("not an opaque slice field: %s", fieldName)
	}

	return &common.Envelope{}, nil
}
