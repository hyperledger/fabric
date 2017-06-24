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
	"fmt"

	"github.com/golang/protobuf/proto"
)

func (e *Envelope) StaticallyOpaqueFields() []string {
	return []string{"payload"}
}

func (e *Envelope) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	if name != e.StaticallyOpaqueFields()[0] {
		return nil, fmt.Errorf("not a marshaled field: %s", name)
	}
	return &Payload{}, nil
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
	ch := &ChannelHeader{}
	if err := proto.Unmarshal(p.Header.ChannelHeader, ch); err != nil {
		return nil, fmt.Errorf("corrupt channel header: %s", err)
	}

	switch ch.Type {
	case int32(HeaderType_CONFIG):
		return &ConfigEnvelope{}, nil
	case int32(HeaderType_CONFIG_UPDATE):
		return &ConfigUpdateEnvelope{}, nil
	// TODO implement the other well known types, particularly endorser txs
	default:
		return nil, fmt.Errorf("decoding type %v is unimplemented", ch.Type)
	}
}

func (h *Header) StaticallyOpaqueFields() []string {
	return []string{"channel_header", "signature_header"}
}

func (h *Header) StaticallyOpaqueFieldProto(name string) (proto.Message, error) {
	switch name {
	case h.StaticallyOpaqueFields()[0]: // channel_header
		return &ChannelHeader{}, nil
	case h.StaticallyOpaqueFields()[1]: // signature_header
		return &SignatureHeader{}, nil
	default:
		return nil, fmt.Errorf("unknown header field: %s", name)
	}
}

func (bd *BlockData) StaticallyOpaqueSliceFields() []string {
	return []string{"data"}
}

func (bd *BlockData) StaticallyOpaqueSliceFieldProto(fieldName string, index int) (proto.Message, error) {
	if fieldName != bd.StaticallyOpaqueSliceFields()[0] {
		return nil, fmt.Errorf("not an opaque slice field: %s", fieldName)
	}

	return &Envelope{}, nil
}
