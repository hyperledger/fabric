/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lifecycle

import (
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Protobuf defines the subset of protobuf lifecycle needs and allows
// for injection of mocked marshaling errors.
type Protobuf interface {
	Marshal(msg proto.Message) (marshaled []byte, err error)
	Unmarshal(marshaled []byte, msg proto.Message) error
}

// ProtobufImpl is the standard implementation to use for Protobuf
type ProtobufImpl struct{}

// Marshal passes through to proto.Marshal
func (p ProtobufImpl) Marshal(msg proto.Message) ([]byte, error) {
	res, err := proto.Marshal(msg)
	return res, errors.WithStack(err)
}

// Unmarshal passes through to proto.Unmarshal
func (p ProtobufImpl) Unmarshal(marshaled []byte, msg proto.Message) error {
	return errors.WithStack(proto.Unmarshal(marshaled, msg))
}
