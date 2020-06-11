/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func TestBuffer(t *testing.T) {
	pb := proto.NewBuffer(nil)
	pb.EncodeVarint(10)
	pos1 := len(pb.Bytes())
	pb.EncodeRawBytes([]byte("JunkText"))
	pos2 := len(pb.Bytes())
	pb.EncodeRawBytes([]byte("YetAnotherJunkText"))
	pos3 := len(pb.Bytes())
	pb.EncodeVarint(1000000)
	pos4 := len(pb.Bytes())

	b := newBuffer(pb.Bytes())
	b.DecodeVarint()
	require.Equal(t, pos1, b.GetBytesConsumed())
	b.DecodeRawBytes(false)
	require.Equal(t, pos2, b.GetBytesConsumed())
	b.DecodeRawBytes(false)
	require.Equal(t, pos3, b.GetBytesConsumed())
	b.DecodeVarint()
	require.Equal(t, pos4, b.GetBytesConsumed())
}
