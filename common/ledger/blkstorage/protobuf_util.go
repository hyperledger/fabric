/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// buffer provides a wrapper on top of proto.Buffer.
// The purpose of this wrapper is to get to know the current position in the []byte
type buffer struct {
	buf      *proto.Buffer
	position int
}

// newBuffer constructs a new instance of Buffer
func newBuffer(b []byte) *buffer {
	return &buffer{proto.NewBuffer(b), 0}
}

// DecodeVarint wraps the actual method and updates the position
func (b *buffer) DecodeVarint() (uint64, error) {
	val, err := b.buf.DecodeVarint()
	if err == nil {
		b.position += proto.SizeVarint(val)
	} else {
		err = errors.Wrap(err, "error decoding varint with proto.Buffer")
	}
	return val, err
}

// DecodeRawBytes wraps the actual method and updates the position
func (b *buffer) DecodeRawBytes(alloc bool) ([]byte, error) {
	val, err := b.buf.DecodeRawBytes(alloc)
	if err == nil {
		b.position += proto.SizeVarint(uint64(len(val))) + len(val)
	} else {
		err = errors.Wrap(err, "error decoding raw bytes with proto.Buffer")
	}
	return val, err
}

// GetBytesConsumed returns the offset of the current position in the underlying []byte
func (b *buffer) GetBytesConsumed() int {
	return b.position
}
