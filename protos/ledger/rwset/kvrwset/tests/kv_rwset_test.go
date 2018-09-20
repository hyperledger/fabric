/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package kvrwset

import (
	"io/ioutil"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
)

const (
	binaryTestFileName = "kvrwsetV1ProtoBytes"
)

// TestKVRWSetV1BackwardCompatible passes if the 'KVRWSet' messgae declared in the latest version
// is able to unmarshal the protobytes that are produced by the 'KVRWSet' proto message declared in
// v1.0. This is to make sure that any incompatible changes does not go uncaught.
func TestKVRWSetV1BackwardCompatible(t *testing.T) {
	protoBytes, err := ioutil.ReadFile(binaryTestFileName)
	assert.NoError(t, err)
	kvrwset1 := &kvrwset.KVRWSet{}
	assert.NoError(t, proto.Unmarshal(protoBytes, kvrwset1))
	kvrwset2 := constructSampleKVRWSet()
	t.Logf("kvrwset1=%s, kvrwset2=%s", spew.Sdump(kvrwset1), spew.Sdump(kvrwset2))
	assert.Equal(t, kvrwset2, kvrwset1)
}

// testPrepareBinaryFileSampleKVRWSetV1 constructs a proto message for kvrwset and marshals its bytes to file 'kvrwsetV1ProtoBytes'.
// this code should be run on fabric version 1.0 so as to produce a sample file of proto message declared in V1
// In order to invoke this function on V1 code, copy this over on to V1 code, make the first letter as 'T', and finally invoke this function
// using golang test framwork
func testPrepareBinaryFileSampleKVRWSetV1(t *testing.T) {
	b, err := proto.Marshal(constructSampleKVRWSet())
	assert.NoError(t, err)
	assert.NoError(t, ioutil.WriteFile(binaryTestFileName, b, 0775))
}

func constructSampleKVRWSet() *kvrwset.KVRWSet {
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	rqi1.SetRawReads([]*kvrwset.KVRead{
		{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	rqi2.SetMerkelSummary(&kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})
	return &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
		RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1, rqi2},
		Writes:           []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
	}
}
