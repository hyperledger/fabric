/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Sample snapshot records
var sampleSSRec1 = SnapshotRecord{
	Value:    "v1",
	BlockNum: uint64(3),
	TxNum:    uint64(3),
}

var sampleSSRec2 = SnapshotRecord{
	Value:    "v2",
	BlockNum: uint64(2),
	TxNum:    uint64(4),
}

var sampleSSRec3 = SnapshotRecord{
	Value:    "v3",
	BlockNum: uint64(2),
	TxNum:    uint64(3),
}

// Sample diff records
var sampleDiffRec1 = DiffRecord{
	Namespace: "ns1",
	Key:       "k1",
	Hashed:    false,
	Record1:   &sampleSSRec1,
	Record2:   &sampleSSRec2,
}

var sampleDiffRec2 = DiffRecord{
	Namespace: "ns2",
	Key:       "k2",
	Hashed:    false,
	Record1:   &sampleSSRec3,
	Record2:   nil,
}

var sampleDiffRec3 = DiffRecord{
	Namespace: "ns3",
	Key:       "k3",
	Hashed:    false,
	Record1:   nil,
	Record2:   nil,
}

func TestGetLaterHeight(t *testing.T) {
	blockRes1, txRes1 := sampleDiffRec1.GetLaterHeight()
	require.Equal(t, uint64(3), blockRes1)
	require.Equal(t, uint64(3), txRes1)
	blockRes2, txRes2 := sampleDiffRec2.GetLaterHeight()
	require.Equal(t, uint64(2), blockRes2)
	require.Equal(t, uint64(3), txRes2)
	blockRes3, txRes3 := sampleDiffRec3.GetLaterHeight()
	require.Equal(t, uint64(0), blockRes3)
	require.Equal(t, uint64(0), txRes3)
}

func TestLaterRecord(t *testing.T) {
	result1 := laterRecord(&sampleSSRec1, &sampleSSRec2)
	require.Equal(t, &sampleSSRec1, result1)
	result2 := laterRecord(&sampleSSRec3, &sampleSSRec2)
	require.Equal(t, &sampleSSRec2, result2)
	result3 := laterRecord(&sampleSSRec1, &sampleSSRec3)
	require.Equal(t, &sampleSSRec1, result3)
	result4 := laterRecord(&sampleSSRec3, nil)
	require.Equal(t, &sampleSSRec3, result4)
	result5 := laterRecord(nil, &sampleSSRec2)
	require.Equal(t, &sampleSSRec2, result5)
	result6 := laterRecord(&sampleSSRec2, &sampleSSRec1)
	require.Equal(t, &sampleSSRec1, result6)
	result7 := laterRecord(&sampleSSRec2, &sampleSSRec3)
	require.Equal(t, &sampleSSRec2, result7)
}
