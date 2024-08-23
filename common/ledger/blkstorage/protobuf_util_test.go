/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blkstorage

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestBuffer(t *testing.T) {
	var pb []byte
	pb = protowire.AppendVarint(pb, 10)
	pos1 := len(pb)
	pb = protowire.AppendBytes(pb, []byte("JunkText"))
	pos2 := len(pb)
	pb = protowire.AppendBytes(pb, []byte("YetAnotherJunkText"))
	pos3 := len(pb)
	pb = protowire.AppendVarint(pb, 1000000)
	pos4 := len(pb)

	b := newBuffer(pb)
	b.DecodeVarint()
	require.Equal(t, pos1, b.GetBytesConsumed())
	b.DecodeRawBytes(false)
	require.Equal(t, pos2, b.GetBytesConsumed())
	b.DecodeRawBytes(false)
	require.Equal(t, pos3, b.GetBytesConsumed())
	b.DecodeVarint()
	require.Equal(t, pos4, b.GetBytesConsumed())
}
