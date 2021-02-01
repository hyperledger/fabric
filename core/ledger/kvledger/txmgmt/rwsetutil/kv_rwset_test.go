/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwsetutil

import (
	"io/ioutil"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/require"
)

const kvrwsetV1ProtoBytesFile = "testdata/kvrwsetV1ProtoBytes"

// TestKVRWSetV1BackwardCompatible passes if the 'KVRWSet' messgae declared in the latest version
// is able to unmarshal the protobytes that are produced by the 'KVRWSet' proto message declared in
// v1.0. This is to make sure that any incompatible changes does not go uncaught.
func TestKVRWSetV1BackwardCompatible(t *testing.T) {
	protoBytes, err := ioutil.ReadFile(kvrwsetV1ProtoBytesFile)
	require.NoError(t, err)
	kvrwset1 := &kvrwset.KVRWSet{}
	require.NoError(t, proto.Unmarshal(protoBytes, kvrwset1))
	kvrwset2 := constructSampleKVRWSet()
	t.Logf("kvrwset1=%s, kvrwset2=%s", spew.Sdump(kvrwset1), spew.Sdump(kvrwset2))
	require.Equal(t, kvrwset2, kvrwset1)
}

// PrepareBinaryFileSampleKVRWSetV1 constructs a proto message for kvrwset and marshals its bytes to file 'kvrwsetV1ProtoBytes'.
// this code should be run on fabric version 1.0 so as to produce a sample file of proto message declared in V1
// In order to invoke this function on V1 code, copy this over on to V1 code, make the first letter as 'T', and finally invoke this function
// using golang test framwork
func PrepareBinaryFileSampleKVRWSetV1(t *testing.T) {
	b, err := proto.Marshal(constructSampleKVRWSet())
	require.NoError(t, err)
	require.NoError(t, ioutil.WriteFile(kvrwsetV1ProtoBytesFile, b, 0o644))
}

func constructSampleKVRWSet() *kvrwset.KVRWSet {
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	SetRawReads(rqi1, []*kvrwset.KVRead{
		{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	SetMerkelSummary(rqi2, &kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})
	return &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
		RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1, rqi2},
		Writes:           []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
	}
}
