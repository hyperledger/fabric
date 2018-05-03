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
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/ledger/rwset"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("rwsetutil", "debug")
	os.Exit(m.Run())
}

func TestTxSimulationResultWithOnlyPubData(t *testing.T) {
	rwSetBuilder := NewRWSetBuilder()

	rwSetBuilder.AddToReadSet("ns1", "key2", version.NewHeight(1, 2))
	rwSetBuilder.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToWriteSet("ns1", "key2", []byte("value2"))

	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "bKey", EndKey: "", ItrExhausted: false, ReadsInfo: nil}
	rqi1.EndKey = "eKey"
	rqi1.SetRawReads([]*kvrwset.KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))})
	rqi1.ItrExhausted = true
	rwSetBuilder.AddToRangeQuerySet("ns1", rqi1)

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "bKey", EndKey: "", ItrExhausted: false, ReadsInfo: nil}
	rqi2.EndKey = "eKey"
	rqi2.SetRawReads([]*kvrwset.KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))})
	rqi2.ItrExhausted = true
	rwSetBuilder.AddToRangeQuerySet("ns1", rqi2)

	rqi3 := &kvrwset.RangeQueryInfo{StartKey: "bKey", EndKey: "", ItrExhausted: true, ReadsInfo: nil}
	rqi3.EndKey = "eKey1"
	rqi3.SetRawReads([]*kvrwset.KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))})
	rwSetBuilder.AddToRangeQuerySet("ns1", rqi3)

	rwSetBuilder.AddToReadSet("ns2", "key2", version.NewHeight(1, 2))
	rwSetBuilder.AddToWriteSet("ns2", "key3", []byte("value3"))

	txSimulationResults, err := rwSetBuilder.GetTxSimulationResults()
	testutil.AssertNoError(t, err, "")

	ns1KVRWSet := &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{NewKVRead("key1", version.NewHeight(1, 1)), NewKVRead("key2", version.NewHeight(1, 2))},
		RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1, rqi3},
		Writes:           []*kvrwset.KVWrite{newKVWrite("key2", []byte("value2"))}}

	ns1RWSet := &rwset.NsReadWriteSet{
		Namespace: "ns1",
		Rwset:     serializeTestProtoMsg(t, ns1KVRWSet),
	}

	ns2KVRWSet := &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{NewKVRead("key2", version.NewHeight(1, 2))},
		RangeQueriesInfo: nil,
		Writes:           []*kvrwset.KVWrite{newKVWrite("key3", []byte("value3"))}}

	ns2RWSet := &rwset.NsReadWriteSet{
		Namespace: "ns2",
		Rwset:     serializeTestProtoMsg(t, ns2KVRWSet),
	}

	expectedTxRWSet := &rwset.TxReadWriteSet{NsRwset: []*rwset.NsReadWriteSet{ns1RWSet, ns2RWSet}}
	testutil.AssertEquals(t, txSimulationResults.PubSimulationResults, expectedTxRWSet)
	testutil.AssertNil(t, txSimulationResults.PvtSimulationResults)
	testutil.AssertNil(t, txSimulationResults.PubSimulationResults.NsRwset[0].CollectionHashedRwset)
}

func TestTxSimulationResultWithPvtData(t *testing.T) {
	rwSetBuilder := NewRWSetBuilder()
	// public rws ns1 + ns2
	rwSetBuilder.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToReadSet("ns2", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToWriteSet("ns2", "key1", []byte("ns2-key1-value"))

	// pvt rwset ns1
	rwSetBuilder.AddToHashedReadSet("ns1", "coll1", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToHashedReadSet("ns1", "coll2", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToPvtAndHashedWriteSet("ns1", "coll2", "key1", []byte("pvt-ns1-coll2-key1-value"))

	// pvt rwset ns2
	rwSetBuilder.AddToHashedReadSet("ns2", "coll1", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToHashedReadSet("ns2", "coll1", "key2", version.NewHeight(1, 1))
	rwSetBuilder.AddToPvtAndHashedWriteSet("ns2", "coll2", "key1", []byte("pvt-ns2-coll2-key1-value"))

	actualSimRes, err := rwSetBuilder.GetTxSimulationResults()
	testutil.AssertNoError(t, err, "")

	///////////////////////////////////////////////////////
	// construct the expected pvt rwset and compare with the one present in the txSimulationResults
	///////////////////////////////////////////////////////
	pvt_Ns1_Coll2 := &kvrwset.KVRWSet{
		Writes: []*kvrwset.KVWrite{newKVWrite("key1", []byte("pvt-ns1-coll2-key1-value"))},
	}

	pvt_Ns2_Coll2 := &kvrwset.KVRWSet{
		Writes: []*kvrwset.KVWrite{newKVWrite("key1", []byte("pvt-ns2-coll2-key1-value"))},
	}

	expectedPvtRWSet := &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns1",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll2",
						Rwset:          serializeTestProtoMsg(t, pvt_Ns1_Coll2),
					},
				},
			},

			{
				Namespace: "ns2",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll2",
						Rwset:          serializeTestProtoMsg(t, pvt_Ns2_Coll2),
					},
				},
			},
		},
	}
	assert.Equal(t, expectedPvtRWSet, actualSimRes.PvtSimulationResults)

	///////////////////////////////////////////////////////
	// construct the public rwset (which will be part of the block) and compare with the one present in the txSimulationResults
	///////////////////////////////////////////////////////
	pub_Ns1 := &kvrwset.KVRWSet{
		Reads: []*kvrwset.KVRead{NewKVRead("key1", version.NewHeight(1, 1))},
	}

	pub_Ns2 := &kvrwset.KVRWSet{
		Reads:  []*kvrwset.KVRead{NewKVRead("key1", version.NewHeight(1, 1))},
		Writes: []*kvrwset.KVWrite{newKVWrite("key1", []byte("ns2-key1-value"))},
	}

	hashed_Ns1_Coll1 := &kvrwset.HashedRWSet{
		HashedReads: []*kvrwset.KVReadHash{
			constructTestPvtKVReadHash(t, "key1", version.NewHeight(1, 1))},
	}

	hashed_Ns1_Coll2 := &kvrwset.HashedRWSet{
		HashedReads: []*kvrwset.KVReadHash{
			constructTestPvtKVReadHash(t, "key1", version.NewHeight(1, 1))},
		HashedWrites: []*kvrwset.KVWriteHash{
			constructTestPvtKVWriteHash(t, "key1", []byte("pvt-ns1-coll2-key1-value")),
		},
	}

	hashed_Ns2_Coll1 := &kvrwset.HashedRWSet{
		HashedReads: []*kvrwset.KVReadHash{
			constructTestPvtKVReadHash(t, "key1", version.NewHeight(1, 1)),
			constructTestPvtKVReadHash(t, "key2", version.NewHeight(1, 1)),
		},
	}

	hashed_Ns2_Coll2 := &kvrwset.HashedRWSet{
		HashedWrites: []*kvrwset.KVWriteHash{
			constructTestPvtKVWriteHash(t, "key1", []byte("pvt-ns2-coll2-key1-value")),
		},
	}

	combined_Ns1 := &rwset.NsReadWriteSet{
		Namespace: "ns1",
		Rwset:     serializeTestProtoMsg(t, pub_Ns1),
		CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
			{
				CollectionName: "coll1",
				HashedRwset:    serializeTestProtoMsg(t, hashed_Ns1_Coll1),
			},
			{
				CollectionName: "coll2",
				HashedRwset:    serializeTestProtoMsg(t, hashed_Ns1_Coll2),
				PvtRwsetHash:   util.ComputeHash(serializeTestProtoMsg(t, pvt_Ns1_Coll2)),
			},
		},
	}
	assert.Equal(t, combined_Ns1, actualSimRes.PubSimulationResults.NsRwset[0])

	combined_Ns2 := &rwset.NsReadWriteSet{
		Namespace: "ns2",
		Rwset:     serializeTestProtoMsg(t, pub_Ns2),
		CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
			{
				CollectionName: "coll1",
				HashedRwset:    serializeTestProtoMsg(t, hashed_Ns2_Coll1),
			},
			{
				CollectionName: "coll2",
				HashedRwset:    serializeTestProtoMsg(t, hashed_Ns2_Coll2),
				PvtRwsetHash:   util.ComputeHash(serializeTestProtoMsg(t, pvt_Ns2_Coll2)),
			},
		},
	}
	assert.Equal(t, combined_Ns2, actualSimRes.PubSimulationResults.NsRwset[1])
	expectedPubRWSet := &rwset.TxReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsRwset:   []*rwset.NsReadWriteSet{combined_Ns1, combined_Ns2},
	}
	assert.Equal(t, expectedPubRWSet, actualSimRes.PubSimulationResults)
}

func constructTestPvtKVReadHash(t *testing.T, key string, version *version.Height) *kvrwset.KVReadHash {
	kvReadHash := newPvtKVReadHash(key, version)
	return kvReadHash
}

func constructTestPvtKVWriteHash(t *testing.T, key string, value []byte) *kvrwset.KVWriteHash {
	_, kvWriteHash := newPvtKVWriteAndHash(key, value)
	return kvWriteHash
}

func serializeTestProtoMsg(t *testing.T, protoMsg proto.Message) []byte {
	msgBytes, err := proto.Marshal(protoMsg)
	testutil.AssertNoError(t, err, "")
	return msgBytes
}
