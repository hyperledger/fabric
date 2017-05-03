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

package rwsetutil

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/kr/pretty"
)

func TestTxRWSetMarshalUnmarshal(t *testing.T) {
	txRwSet := &TxRwSet{}

	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	rqi1.SetRawReads([]*kvrwset.KVRead{
		{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	rqi2.SetMerkelSummary(&kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})

	txRwSet.NsRwSets = []*NsRwSet{
		{NameSpace: "ns1", KvRwSet: &kvrwset.KVRWSet{
			Reads:            []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1},
			Writes:           []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
		}},

		{NameSpace: "ns2", KvRwSet: &kvrwset.KVRWSet{
			Reads:            []*kvrwset.KVRead{{Key: "key3", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi2},
			Writes:           []*kvrwset.KVWrite{{Key: "key3", IsDelete: false, Value: []byte("value3")}},
		}},

		{NameSpace: "ns3", KvRwSet: &kvrwset.KVRWSet{
			Reads:            []*kvrwset.KVRead{{Key: "key4", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			RangeQueriesInfo: nil,
			Writes:           []*kvrwset.KVWrite{{Key: "key4", IsDelete: false, Value: []byte("value4")}},
		}},
	}

	protoBytes, err := txRwSet.ToProtoBytes()
	testutil.AssertNoError(t, err, "")
	txRwSet1 := &TxRwSet{}
	testutil.AssertNoError(t, txRwSet1.FromProtoBytes(protoBytes), "")
	t.Logf("txRwSet=%s, txRwSet1=%s", spew.Sdump(txRwSet), spew.Sdump(txRwSet1))
	testutil.AssertEquals(t, txRwSet, txRwSet1)
}

func TestTxRwSetConversion(t *testing.T) {
	txRwSet := sampleTxRwSet()
	protoMsg, err := txRwSet.toProtoMsg()
	testutil.AssertNoError(t, err, "")
	txRwSet1, err := TxRwSetFromProtoMsg(protoMsg)
	testutil.AssertNoError(t, err, "")
	t.Logf("txRwSet=%s, txRwSet1=%s", spew.Sdump(txRwSet), spew.Sdump(txRwSet1))
	testutil.AssertEquals(t, txRwSet1, txRwSet)
}

func TestNsRwSetConversion(t *testing.T) {
	nsRwSet := sampleNsRwSet("ns-1")
	protoMsg, err := nsRwSet.toProtoMsg()
	testutil.AssertNoError(t, err, "")
	nsRwSet1, err := nsRwSetFromProtoMsg(protoMsg)
	testutil.AssertNoError(t, err, "")
	t.Logf("nsRwSet=%s, nsRwSet1=%s", spew.Sdump(nsRwSet), spew.Sdump(nsRwSet1))
	testutil.AssertEquals(t, nsRwSet1, nsRwSet)
}

func TestNsRWSetConversionNoCollHashedRWs(t *testing.T) {
	nsRwSet := sampleNsRwSetWithNoCollHashedRWs("ns-1")
	protoMsg, err := nsRwSet.toProtoMsg()
	testutil.AssertNoError(t, err, "")
	testutil.AssertNil(t, protoMsg.CollectionHashedRwset)
}

func TestCollHashedRwSetConversion(t *testing.T) {
	collHashedRwSet := sampleCollHashedRwSet("coll-1")
	protoMsg, err := collHashedRwSet.toProtoMsg()
	testutil.AssertNoError(t, err, "")
	collHashedRwSet1, err := collHashedRwSetFromProtoMsg(protoMsg)
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, collHashedRwSet1, collHashedRwSet)
}

func sampleTxRwSet() *TxRwSet {
	txRwSet := &TxRwSet{}
	txRwSet.NsRwSets = append(txRwSet.NsRwSets, sampleNsRwSet("ns-1"))
	txRwSet.NsRwSets = append(txRwSet.NsRwSets, sampleNsRwSet("ns-2"))
	return txRwSet
}

func sampleNsRwSet(ns string) *NsRwSet {
	nsRwSet := &NsRwSet{NameSpace: ns,
		KvRwSet: sampleKvRwSet()}
	nsRwSet.CollHashedRwSets = append(nsRwSet.CollHashedRwSets, sampleCollHashedRwSet("coll-1"))
	nsRwSet.CollHashedRwSets = append(nsRwSet.CollHashedRwSets, sampleCollHashedRwSet("coll-2"))
	return nsRwSet
}

func sampleNsRwSetWithNoCollHashedRWs(ns string) *NsRwSet {
	return &NsRwSet{NameSpace: ns, KvRwSet: sampleKvRwSet()}
}

func sampleKvRwSet() *kvrwset.KVRWSet {
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	rqi1.SetRawReads([]*kvrwset.KVRead{
		{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	rqi2.SetMerkelSummary(&kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})
	return &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
		RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1},
		Writes:           []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
	}
}

func sampleCollHashedRwSet(collectionName string) *CollHashedRwSet {
	collHashedRwSet := &CollHashedRwSet{
		CollectionName: collectionName,
		HashedRwSet: &kvrwset.HashedRWSet{
			HashedReads: []*kvrwset.KVReadHash{
				{KeyHash: []byte("Key-1-hash"), Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
				{KeyHash: []byte("Key-2-hash"), Version: &kvrwset.Version{BlockNum: 2, TxNum: 3}},
			},
			HashedWrites: []*kvrwset.KVWriteHash{
				{KeyHash: []byte("Key-3-hash"), ValueHash: []byte("value-3-hash"), IsDelete: false},
				{KeyHash: []byte("Key-4-hash"), ValueHash: []byte("value-4-hash"), IsDelete: true},
			},
		},
		PvtRwSetHash: []byte(collectionName + "-pvt-rwset-hash"),
	}
	return collHashedRwSet
}

///////////////////////////////////////////////////////////////////////////////
// tests for private read-write set
///////////////////////////////////////////////////////////////////////////////

func TestTxPvtRwSetConversion(t *testing.T) {
	txPvtRwSet := sampleTxPvtRwSet()
	protoMsg, err := txPvtRwSet.toProtoMsg()
	testutil.AssertNoError(t, err, "")
	txPvtRwSet1, err := TxPvtRwSetFromProtoMsg(protoMsg)
	testutil.AssertNoError(t, err, "")
	t.Logf("txPvtRwSet=%s, txPvtRwSet1=%s, Diff:%s", spew.Sdump(txPvtRwSet), spew.Sdump(txPvtRwSet1), pretty.Diff(txPvtRwSet, txPvtRwSet1))
	testutil.AssertEquals(t, txPvtRwSet1, txPvtRwSet)
}

func sampleTxPvtRwSet() *TxPvtRwSet {
	txPvtRwSet := &TxPvtRwSet{}
	txPvtRwSet.NsPvtRwSet = append(txPvtRwSet.NsPvtRwSet, sampleNsPvtRwSet("ns-1"))
	txPvtRwSet.NsPvtRwSet = append(txPvtRwSet.NsPvtRwSet, sampleNsPvtRwSet("ns-2"))
	return txPvtRwSet
}

func sampleNsPvtRwSet(ns string) *NsPvtRwSet {
	nsRwSet := &NsPvtRwSet{NameSpace: ns}
	nsRwSet.CollPvtRwSets = append(nsRwSet.CollPvtRwSets, sampleCollPvtRwSet("coll-1"))
	nsRwSet.CollPvtRwSets = append(nsRwSet.CollPvtRwSets, sampleCollPvtRwSet("coll-2"))
	return nsRwSet
}

func sampleCollPvtRwSet(collectionName string) *CollPvtRwSet {
	return &CollPvtRwSet{CollectionName: collectionName,
		KvRwSet: &kvrwset.KVRWSet{
			Reads:  []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			Writes: []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
		}}
}

func TestVersionConversion(t *testing.T) {
	protoVer := &kvrwset.Version{BlockNum: 5, TxNum: 2}
	internalVer := version.NewHeight(5, 2)
	// convert proto to internal
	testutil.AssertNil(t, NewVersion(nil))
	testutil.AssertEquals(t, NewVersion(protoVer), internalVer)

	// convert internal to proto
	testutil.AssertNil(t, newProtoVersion(nil))
	testutil.AssertEquals(t, newProtoVersion(internalVer), protoVer)
}
