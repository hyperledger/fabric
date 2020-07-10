/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package ledger

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/stretchr/testify/require"
)

func TestTxPvtData(t *testing.T) {
	txPvtData := &TxPvtData{}
	require.False(t, txPvtData.Has("ns", "coll"))

	txPvtData.WriteSet = &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll-1",
						Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll1"),
					},
					{
						CollectionName: "coll-2",
						Rwset:          []byte("RandomBytes-PvtRWSet-ns1-coll2"),
					},
				},
			},
		},
	}

	require.True(t, txPvtData.Has("ns", "coll-1"))
	require.True(t, txPvtData.Has("ns", "coll-2"))
	require.False(t, txPvtData.Has("ns", "coll-3"))
	require.False(t, txPvtData.Has("ns1", "coll-1"))
}

func TestPvtNsCollFilter(t *testing.T) {
	filter := NewPvtNsCollFilter()
	filter.Add("ns", "coll-1")
	filter.Add("ns", "coll-2")
	require.True(t, filter.Has("ns", "coll-1"))
	require.True(t, filter.Has("ns", "coll-2"))
	require.False(t, filter.Has("ns", "coll-3"))
	require.False(t, filter.Has("ns1", "coll-3"))
}
