/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwsetutil

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/require"
)

func TestSetRawReads(t *testing.T) {
	rqi := &kvrwset.RangeQueryInfo{StartKey: "start", EndKey: "end"}
	kvReads := []*kvrwset.KVRead{{Key: "key1"}, {Key: "key2"}}

	expected := &kvrwset.RangeQueryInfo{
		StartKey: "start",
		EndKey:   "end",
		ReadsInfo: &kvrwset.RangeQueryInfo_RawReads{
			RawReads: &kvrwset.QueryReads{KvReads: kvReads},
		},
	}

	SetRawReads(rqi, kvReads)
	require.Equal(t, expected, rqi)
}

func TestSetMerkelSummary(t *testing.T) {
	rqi := &kvrwset.RangeQueryInfo{StartKey: "start", EndKey: "end"}
	merkleSummary := &kvrwset.QueryReadsMerkleSummary{MaxDegree: 12, MaxLevel: 99}

	expected := &kvrwset.RangeQueryInfo{
		StartKey:  "start",
		EndKey:    "end",
		ReadsInfo: &kvrwset.RangeQueryInfo_ReadsMerkleHashes{ReadsMerkleHashes: merkleSummary},
	}

	SetMerkelSummary(rqi, merkleSummary)
	require.Equal(t, expected, rqi)
}
