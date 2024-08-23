/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
)

// buffer provides a wrapper on top of proto.Buffer.
// The purpose of this wrapper is to get to know the current position in the []byte
type buffer struct {
	buf      []byte
	position int
}

// newBuffer constructs a new instance of Buffer
func newBuffer(b []byte) *buffer {
	return &buffer{b, 0}
}

// DecodeVarint wraps the actual method and updates the position
func (b *buffer) DecodeVarint() (uint64, error) {
	v, n := protowire.ConsumeVarint(b.buf[b.position:])
	if n < 0 {
		return 0, errors.Wrap(protowire.ParseError(n), "error decoding varint with proto.Buffer")
	}
	b.position += n
	return v, nil
}

// DecodeRawBytes wraps the actual method and updates the position
func (b *buffer) DecodeRawBytes(alloc bool) ([]byte, error) {
	v, n := protowire.ConsumeBytes(b.buf[b.position:])
	if n < 0 {
		return nil, errors.Wrap(protowire.ParseError(n), "error decoding raw bytes with proto.Buffer")
	}
	b.position += n
	if alloc {
		v = append([]byte(nil), v...)
	}
	return v, nil
}

// GetBytesConsumed returns the offset of the current position in the underlying []byte
func (b *buffer) GetBytesConsumed() int {
	return b.position
}
