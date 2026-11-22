/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwsetutil

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("rwsetutil=debug")
	os.Exit(m.Run())
}

func TestTxSimulationResultWithOnlyPubData(t *testing.T) {
	rwSetBuilder := NewRWSetBuilder()

	rwSetBuilder.AddToReadSet("ns1", "key2", version.NewHeight(1, 2))
	rwSetBuilder.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToWriteSet("ns1", "key2", []byte("value2"))

	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "bKey", EndKey: "", ItrExhausted: false, ReadsInfo: nil}
	rqi1.EndKey = "eKey"
	SetRawReads(rqi1, []*kvrwset.KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))})
	rqi1.ItrExhausted = true
	rwSetBuilder.AddToRangeQuerySet("ns1", rqi1)

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "bKey", EndKey: "", ItrExhausted: false, ReadsInfo: nil}
	rqi2.EndKey = "eKey"
	SetRawReads(rqi2, []*kvrwset.KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))})
	rqi2.ItrExhausted = true
	rwSetBuilder.AddToRangeQuerySet("ns1", rqi2)

	rqi3 := &kvrwset.RangeQueryInfo{StartKey: "bKey", EndKey: "", ItrExhausted: true, ReadsInfo: nil}
	rqi3.EndKey = "eKey1"
	SetRawReads(rqi3, []*kvrwset.KVRead{NewKVRead("bKey1", version.NewHeight(2, 3)), NewKVRead("bKey2", version.NewHeight(2, 4))})
	rwSetBuilder.AddToRangeQuerySet("ns1", rqi3)

	rwSetBuilder.AddToReadSet("ns2", "key2", version.NewHeight(1, 2))
	rwSetBuilder.AddToWriteSet("ns2", "key3", []byte("value3"))

	txSimulationResults, err := rwSetBuilder.GetTxSimulationResults()
	require.NoError(t, err)

	ns1KVRWSet := &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{NewKVRead("key1", version.NewHeight(1, 1)), NewKVRead("key2", version.NewHeight(1, 2))},
		RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1, rqi3},
		Writes:           []*kvrwset.KVWrite{newKVWrite("key2", []byte("value2"))},
	}

	ns1RWSet := &rwset.NsReadWriteSet{
		Namespace: "ns1",
		Rwset:     serializeTestProtoMsg(t, ns1KVRWSet),
	}

	ns2KVRWSet := &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{NewKVRead("key2", version.NewHeight(1, 2))},
		RangeQueriesInfo: nil,
		Writes:           []*kvrwset.KVWrite{newKVWrite("key3", []byte("value3"))},
	}

	ns2RWSet := &rwset.NsReadWriteSet{
		Namespace: "ns2",
		Rwset:     serializeTestProtoMsg(t, ns2KVRWSet),
	}

	expectedTxRWSet := &rwset.TxReadWriteSet{NsRwset: []*rwset.NsReadWriteSet{ns1RWSet, ns2RWSet}}
	require.Equal(t, expectedTxRWSet, txSimulationResults.PubSimulationResults)
	require.Nil(t, txSimulationResults.PvtSimulationResults)
	require.Nil(t, txSimulationResults.PubSimulationResults.GetNsRwset()[0].GetCollectionHashedRwset())
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

	// pvt rwset ns3
	rwSetBuilder.AddToPvtAndHashedWriteSetForPurge("ns3", "coll1", "key1")

	actualSimRes, err := rwSetBuilder.GetTxSimulationResults()
	require.NoError(t, err)

	// /////////////////////////////////////////////////////
	// construct the expected pvt rwset and compare with the one present in the txSimulationResults
	// /////////////////////////////////////////////////////
	pvtNs1Coll2 := &kvrwset.KVRWSet{
		Writes: []*kvrwset.KVWrite{newKVWrite("key1", []byte("pvt-ns1-coll2-key1-value"))},
	}

	pvtNs2Coll2 := &kvrwset.KVRWSet{
		Writes: []*kvrwset.KVWrite{newKVWrite("key1", []byte("pvt-ns2-coll2-key1-value"))},
	}

	pvtNs3Coll1 := &kvrwset.KVRWSet{
		Writes: []*kvrwset.KVWrite{
			{
				Key:      "key1",
				IsDelete: true,
			},
		},
	}

	expectedPvtRWSet := &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns1",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll2",
						Rwset:          serializeTestProtoMsg(t, pvtNs1Coll2),
					},
				},
			},

			{
				Namespace: "ns2",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll2",
						Rwset:          serializeTestProtoMsg(t, pvtNs2Coll2),
					},
				},
			},
			{
				Namespace: "ns3",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll1",
						Rwset:          serializeTestProtoMsg(t, pvtNs3Coll1),
					},
				},
			},
		},
	}
	require.Equal(t, expectedPvtRWSet, actualSimRes.PvtSimulationResults)

	// /////////////////////////////////////////////////////
	// construct the public rwset (which will be part of the block) and compare with the one present in the txSimulationResults
	// /////////////////////////////////////////////////////
	pubNs1 := &kvrwset.KVRWSet{
		Reads: []*kvrwset.KVRead{NewKVRead("key1", version.NewHeight(1, 1))},
	}

	pubNs2 := &kvrwset.KVRWSet{
		Reads:  []*kvrwset.KVRead{NewKVRead("key1", version.NewHeight(1, 1))},
		Writes: []*kvrwset.KVWrite{newKVWrite("key1", []byte("ns2-key1-value"))},
	}

	pubNs3 := &kvrwset.KVRWSet{}

	hashedNs1Coll1 := &kvrwset.HashedRWSet{
		HashedReads: []*kvrwset.KVReadHash{
			constructTestPvtKVReadHash(t, "key1", version.NewHeight(1, 1)),
		},
	}

	hashedNs1Coll2 := &kvrwset.HashedRWSet{
		HashedReads: []*kvrwset.KVReadHash{
			constructTestPvtKVReadHash(t, "key1", version.NewHeight(1, 1)),
		},
		HashedWrites: []*kvrwset.KVWriteHash{
			constructTestPvtKVWriteHash(t, "key1", []byte("pvt-ns1-coll2-key1-value")),
		},
	}

	hashedNs2Coll1 := &kvrwset.HashedRWSet{
		HashedReads: []*kvrwset.KVReadHash{
			constructTestPvtKVReadHash(t, "key1", version.NewHeight(1, 1)),
			constructTestPvtKVReadHash(t, "key2", version.NewHeight(1, 1)),
		},
	}

	hashedNs2Coll2 := &kvrwset.HashedRWSet{
		HashedWrites: []*kvrwset.KVWriteHash{
			constructTestPvtKVWriteHash(t, "key1", []byte("pvt-ns2-coll2-key1-value")),
		},
	}

	hashedNs3Coll1 := &kvrwset.HashedRWSet{
		HashedWrites: []*kvrwset.KVWriteHash{
			{
				KeyHash:  util.ComputeStringHash("key1"),
				IsDelete: true,
				IsPurge:  true,
			},
		},
	}

	combinedNs1 := &rwset.NsReadWriteSet{
		Namespace: "ns1",
		Rwset:     serializeTestProtoMsg(t, pubNs1),
		CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
			{
				CollectionName: "coll1",
				HashedRwset:    serializeTestProtoMsg(t, hashedNs1Coll1),
			},
			{
				CollectionName: "coll2",
				HashedRwset:    serializeTestProtoMsg(t, hashedNs1Coll2),
				PvtRwsetHash:   util.ComputeHash(serializeTestProtoMsg(t, pvtNs1Coll2)),
			},
		},
	}
	require.Equal(t, combinedNs1, actualSimRes.PubSimulationResults.GetNsRwset()[0])

	combinedNs2 := &rwset.NsReadWriteSet{
		Namespace: "ns2",
		Rwset:     serializeTestProtoMsg(t, pubNs2),
		CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
			{
				CollectionName: "coll1",
				HashedRwset:    serializeTestProtoMsg(t, hashedNs2Coll1),
			},
			{
				CollectionName: "coll2",
				HashedRwset:    serializeTestProtoMsg(t, hashedNs2Coll2),
				PvtRwsetHash:   util.ComputeHash(serializeTestProtoMsg(t, pvtNs2Coll2)),
			},
		},
	}
	require.Equal(t, combinedNs2, actualSimRes.PubSimulationResults.GetNsRwset()[1])

	combinedNs3 := &rwset.NsReadWriteSet{
		Namespace: "ns3",
		Rwset:     serializeTestProtoMsg(t, pubNs3),
		CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
			{
				CollectionName: "coll1",
				HashedRwset:    serializeTestProtoMsg(t, hashedNs3Coll1),
				PvtRwsetHash:   util.ComputeHash(serializeTestProtoMsg(t, pvtNs3Coll1)),
			},
		},
	}
	require.Equal(t, combinedNs3, actualSimRes.PubSimulationResults.GetNsRwset()[2])

	expectedPubRWSet := &rwset.TxReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsRwset:   []*rwset.NsReadWriteSet{combinedNs1, combinedNs2, combinedNs3},
	}
	require.Equal(t, expectedPubRWSet, actualSimRes.PubSimulationResults)
}

func TestTxSimulationResultWithMetadata(t *testing.T) {
	rwSetBuilder := NewRWSetBuilder()
	// public rws ns1
	rwSetBuilder.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	rwSetBuilder.AddToMetadataWriteSet(
		"ns1", "key1",
		map[string][]byte{"metadata2": []byte("ns1-key1-metadata2"), "metadata1": []byte("ns1-key1-metadata1")},
	)
	// public rws ns2
	rwSetBuilder.AddToWriteSet("ns2", "key1", []byte("ns2-key1-value"))
	rwSetBuilder.AddToMetadataWriteSet("ns2", "key1", map[string][]byte{}) // nil/empty-map indicates metadata delete

	// pvt rwset <ns1, coll1>
	rwSetBuilder.AddToPvtAndHashedWriteSet("ns1", "coll1", "key1", []byte("pvt-ns1-coll1-key1-value"))
	rwSetBuilder.AddToHashedMetadataWriteSet("ns1", "coll1", "key1",
		map[string][]byte{"metadata1": []byte("ns1-coll1-key1-metadata1")})

	// pvt rwset <ns1, coll2>
	rwSetBuilder.AddToHashedMetadataWriteSet("ns1", "coll2", "key1", nil) // pvt-data metadata delete

	actualSimRes, err := rwSetBuilder.GetTxSimulationResults()
	require.NoError(t, err)

	// construct the expected pvt rwset and compare with the one present in the txSimulationResults
	pvtNs1Coll1 := &kvrwset.KVRWSet{
		Writes:         []*kvrwset.KVWrite{newKVWrite("key1", []byte("pvt-ns1-coll1-key1-value"))},
		MetadataWrites: []*kvrwset.KVMetadataWrite{{Key: "key1"}},
	}

	pvtNs1Coll2 := &kvrwset.KVRWSet{
		MetadataWrites: []*kvrwset.KVMetadataWrite{{Key: "key1"}},
	}

	expectedPvtRWSet := &rwset.TxPvtReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsPvtRwset: []*rwset.NsPvtReadWriteSet{
			{
				Namespace: "ns1",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{
					{
						CollectionName: "coll1",
						Rwset:          serializeTestProtoMsg(t, pvtNs1Coll1),
					},
					{
						CollectionName: "coll2",
						Rwset:          serializeTestProtoMsg(t, pvtNs1Coll2),
					},
				},
			},
		},
	}
	require.Equal(t, expectedPvtRWSet, actualSimRes.PvtSimulationResults)
	// construct the public and hashed rwset (which will be part of the block) and compare with the one present in the txSimulationResults
	pubNs1 := &kvrwset.KVRWSet{
		Reads: []*kvrwset.KVRead{NewKVRead("key1", version.NewHeight(1, 1))},
		MetadataWrites: []*kvrwset.KVMetadataWrite{
			{
				Key: "key1",
				Entries: []*kvrwset.KVMetadataEntry{
					{Name: "metadata1", Value: []byte("ns1-key1-metadata1")},
					{Name: "metadata2", Value: []byte("ns1-key1-metadata2")},
				},
			},
		},
	}

	pubNs2 := &kvrwset.KVRWSet{
		Writes: []*kvrwset.KVWrite{newKVWrite("key1", []byte("ns2-key1-value"))},
		MetadataWrites: []*kvrwset.KVMetadataWrite{
			{
				Key:     "key1",
				Entries: nil,
			},
		},
	}

	hashedNs1Coll1 := &kvrwset.HashedRWSet{
		HashedWrites: []*kvrwset.KVWriteHash{
			constructTestPvtKVWriteHash(t, "key1", []byte("pvt-ns1-coll1-key1-value")),
		},
		MetadataWrites: []*kvrwset.KVMetadataWriteHash{
			{
				KeyHash: util.ComputeStringHash("key1"),
				Entries: []*kvrwset.KVMetadataEntry{
					{Name: "metadata1", Value: []byte("ns1-coll1-key1-metadata1")},
				},
			},
		},
	}

	hashedNs1Coll2 := &kvrwset.HashedRWSet{
		MetadataWrites: []*kvrwset.KVMetadataWriteHash{
			{
				KeyHash: util.ComputeStringHash("key1"),
				Entries: nil,
			},
		},
	}

	pubAndHashCombinedNs1 := &rwset.NsReadWriteSet{
		Namespace: "ns1",
		Rwset:     serializeTestProtoMsg(t, pubNs1),
		CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{
			{
				CollectionName: "coll1",
				HashedRwset:    serializeTestProtoMsg(t, hashedNs1Coll1),
				PvtRwsetHash:   util.ComputeHash(serializeTestProtoMsg(t, pvtNs1Coll1)),
			},
			{
				CollectionName: "coll2",
				HashedRwset:    serializeTestProtoMsg(t, hashedNs1Coll2),
				PvtRwsetHash:   util.ComputeHash(serializeTestProtoMsg(t, pvtNs1Coll2)),
			},
		},
	}
	require.Equal(t, pubAndHashCombinedNs1, actualSimRes.PubSimulationResults.GetNsRwset()[0])
	pubAndHashCombinedNs2 := &rwset.NsReadWriteSet{
		Namespace:             "ns2",
		Rwset:                 serializeTestProtoMsg(t, pubNs2),
		CollectionHashedRwset: nil,
	}
	expectedPubRWSet := &rwset.TxReadWriteSet{
		DataModel: rwset.TxReadWriteSet_KV,
		NsRwset:   []*rwset.NsReadWriteSet{pubAndHashCombinedNs1, pubAndHashCombinedNs2},
	}
	require.Equal(t, expectedPubRWSet, actualSimRes.PubSimulationResults)
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
	require.NoError(t, err)
	return msgBytes
}

func TestNilOrZeroLengthByteArrayValueConvertedToDelete(t *testing.T) {
	t.Run("public_writeset", func(t *testing.T) {
		rwsetBuilder := NewRWSetBuilder()
		rwsetBuilder.AddToWriteSet("ns", "key1", nil)
		rwsetBuilder.AddToWriteSet("ns", "key2", []byte{})

		simulationResults, err := rwsetBuilder.GetTxSimulationResults()
		require.NoError(t, err)
		pubRWSet := &kvrwset.KVRWSet{}
		require.NoError(
			t,
			proto.Unmarshal(simulationResults.PubSimulationResults.GetNsRwset()[0].GetRwset(), pubRWSet),
		)
		require.True(t, proto.Equal(
			&kvrwset.KVRWSet{
				Writes: []*kvrwset.KVWrite{
					{Key: "key1", IsDelete: true},
					{Key: "key2", IsDelete: true},
				},
			},
			pubRWSet,
		))
	})

	t.Run("pvtdata_and_hashes_writesets", func(t *testing.T) {
		rwsetBuilder := NewRWSetBuilder()
		rwsetBuilder.AddToPvtAndHashedWriteSet("ns", "coll", "key1", nil)
		rwsetBuilder.AddToPvtAndHashedWriteSet("ns", "coll", "key2", []byte{})

		simulationResults, err := rwsetBuilder.GetTxSimulationResults()
		require.NoError(t, err)

		t.Run("hashed_writeset", func(t *testing.T) {
			hashedRWSet := &kvrwset.HashedRWSet{}
			require.NoError(
				t,
				proto.Unmarshal(simulationResults.PubSimulationResults.GetNsRwset()[0].GetCollectionHashedRwset()[0].GetHashedRwset(), hashedRWSet),
			)
			require.True(t, proto.Equal(
				&kvrwset.HashedRWSet{
					HashedWrites: []*kvrwset.KVWriteHash{
						{KeyHash: util.ComputeStringHash("key1"), IsDelete: true},
						{KeyHash: util.ComputeStringHash("key2"), IsDelete: true},
					},
				},
				hashedRWSet,
			))
		})

		t.Run("pvtdata_writeset", func(t *testing.T) {
			pvtWSet := &kvrwset.KVRWSet{}
			require.NoError(
				t,
				proto.Unmarshal(simulationResults.PvtSimulationResults.GetNsPvtRwset()[0].GetCollectionPvtRwset()[0].GetRwset(), pvtWSet),
			)
			require.True(t, proto.Equal(
				&kvrwset.KVRWSet{
					Writes: []*kvrwset.KVWrite{
						{Key: "key1", IsDelete: true},
						{Key: "key2", IsDelete: true},
					},
				},
				pvtWSet,
			))
		})
	})
}

func TestRWSetSizeLimit(t *testing.T) {
	t.Run("public writes are metered and excess is rejected", func(t *testing.T) {
		// each entry is overhead(512) + key(4) + value(46) = 562 bytes
		b := NewRWSetBuilderWithMaxRWSetSize(1130)
		require.NoError(t, b.AddToWriteSet("ns1", "key1", make([]byte, 46)))
		require.NoError(t, b.AddToWriteSet("ns1", "key2", make([]byte, 46)))
		err := b.AddToWriteSet("ns1", "key3", make([]byte, 46))
		require.EqualError(
			t,
			err,
			"transaction read-write set size 1686 exceeds the maximum allowed size 1130",
		)
		require.NotContains(t, b.pubRwBuilderMap["ns1"].writeMap, "key3")
		// a rejected first write to a new namespace leaves no empty builder behind
		require.Error(t, b.AddToWriteSet("ns2", "key1", make([]byte, 46)))
		require.NotContains(t, b.pubRwBuilderMap, "ns2")
		// once breached, further additions and the results fail with the recorded error
		require.Equal(t, err, b.AddToWriteSet("ns1", "key3", nil))
		_, resErr := b.GetTxSimulationResults()
		require.Equal(t, err, resErr)
	})

	t.Run("overwrites are credited", func(t *testing.T) {
		b := NewRWSetBuilderWithMaxRWSetSize(600)
		require.NoError(t, b.AddToWriteSet("ns1", "key1", make([]byte, 46)))
		// rewriting the same key keeps the metered size at 562 bytes
		require.NoError(t, b.AddToWriteSet("ns1", "key1", make([]byte, 46)))
		// shrinking the value frees budget
		require.NoError(t, b.AddToWriteSet("ns1", "key1", nil))
		require.Error(t, b.AddToWriteSet("ns1", "key2", make([]byte, 80)))
	})

	t.Run("metadata writes are metered", func(t *testing.T) {
		// each entry is overhead(512) + key(4) + entry(128) + name(5) + value(16) = 665 bytes
		b := NewRWSetBuilderWithMaxRWSetSize(1346)
		metadata := map[string][]byte{"meta1": make([]byte, 16)}
		require.NoError(t, b.AddToMetadataWriteSet("ns1", "key1", metadata))
		// deleting metadata frees budget
		require.NoError(t, b.AddToMetadataWriteSet("ns1", "key1", nil))
		require.NoError(t, b.AddToMetadataWriteSet("ns1", "key2", metadata))
		require.Error(t, b.AddToMetadataWriteSet("ns1", "key3", metadata))
	})

	t.Run("private data writes are metered", func(t *testing.T) {
		// each entry is overhead(512) + key(4) + value(10) + keyHash(32) + valueHash(32)
		// + the original key(4) retained by the hashed write map = 594 bytes
		b := NewRWSetBuilderWithMaxRWSetSize(594)
		require.NoError(t, b.AddToPvtAndHashedWriteSet("ns1", "coll1", "key1", make([]byte, 10)))
		// purging the key shrinks the metered size to overhead + key(4) + keyHash(32) + key(4)
		require.NoError(t, b.AddToPvtAndHashedWriteSetForPurge("ns1", "coll1", "key1"))
		require.Error(t, b.AddToPvtAndHashedWriteSet("ns1", "coll1", "key2", make([]byte, 1)))
	})

	t.Run("hashed metadata writes are metered", func(t *testing.T) {
		// each entry is overhead(512) + key(4) + keyHash(32) + entry(128) + name(5) + value(8)
		// + the original key(4) retained by the hashed metadata map = 693 bytes
		b := NewRWSetBuilderWithMaxRWSetSize(693)
		metadata := map[string][]byte{"meta1": make([]byte, 8)}
		require.NoError(t, b.AddToHashedMetadataWriteSet("ns1", "coll1", "key1", metadata))
		require.Error(t, b.AddToHashedMetadataWriteSet("ns1", "coll1", "key2", metadata))
	})

	t.Run("reads are metered", func(t *testing.T) {
		// each read entry is overhead(384) + key(4) = 388 bytes
		b := NewRWSetBuilderWithMaxRWSetSize(776)
		require.NoError(t, b.AddToReadSet("ns1", "key1", version.NewHeight(1, 1)))
		require.NoError(t, b.AddToReadSet("ns1", "key2", version.NewHeight(1, 1)))
		// re-reading a tracked key only replaces the version
		require.NoError(t, b.AddToReadSet("ns1", "key1", version.NewHeight(2, 1)))
		require.Error(t, b.AddToReadSet("ns1", "key3", version.NewHeight(1, 1)))
		// hashed read entries retain the original key plus the 32 byte key hash
		b2 := NewRWSetBuilderWithMaxRWSetSize(420)
		require.NoError(t, b2.AddToHashedReadSet("ns1", "coll1", "key1", version.NewHeight(1, 1)))
		require.Error(t, b2.AddToHashedReadSet("ns1", "coll1", "key2", version.NewHeight(1, 1)))
		// a single oversized key is rejected before any state changes
		b3 := NewRWSetBuilderWithMaxRWSetSize(1024)
		require.Error(t, b3.AddToReadSet("ns1", strings.Repeat("k", 4096), version.NewHeight(1, 1)))
		require.NotContains(t, b3.pubRwBuilderMap, "ns1")
		// the hashed read set retains the original key, not just its hash
		b4 := NewRWSetBuilderWithMaxRWSetSize(1024)
		require.Error(t, b4.AddToHashedReadSet("ns1", "coll1", strings.Repeat("k", 4096), version.NewHeight(1, 1)))
		require.Empty(t, b4.pubRwBuilderMap)
	})

	t.Run("range query bounds are metered at iterator creation", func(t *testing.T) {
		// the bounds are charged when the iterator is created, before any result, so
		// large bounds held by an open iterator are covered
		b := NewRWSetBuilderWithMaxRWSetSize(4096)
		require.Error(t, b.AddRangeQueryKeys(strings.Repeat("s", 3000), strings.Repeat("e", 3000)))
		require.Empty(t, b.pubRwBuilderMap)
	})

	t.Run("compacted range query results release their charge", func(t *testing.T) {
		// a scan whose results exceed the budget in sum fits in peak: only one
		// maxDegree batch is retained at a time
		b := NewRWSetBuilderWithMaxRWSetSize(8192)
		helper, err := NewRangeQueryResultsHelperWithAccounting(true, 50, testHashFunc, b)
		require.NoError(t, err)
		require.NoError(t, b.AddRangeQueryKeys("a", "z")) // 514 bytes
		// 500 results of overhead(128) + key(6) sum to 67k bytes, far over budget
		for i := range 500 {
			require.NoError(t, helper.AddResult(NewKVRead(fmt.Sprintf("key%03d", i), version.NewHeight(1, 2))))
		}
		// only the leftover batch of 41 results is still retained and charged
		require.Equal(t, int64(514+41*(128+6)), b.rwSetBytes)
		results, summary, err := helper.Done()
		require.NoError(t, err)
		require.Nil(t, results)
		require.NotNil(t, summary)
		// the final batch is released once compacted into the summary
		require.Equal(t, int64(514), b.rwSetBytes)

		// a scan whose single retained batch exceeds the budget breaches mid-scan
		b2 := NewRWSetBuilderWithMaxRWSetSize(1024)
		helper2, err := NewRangeQueryResultsHelperWithAccounting(true, 50, testHashFunc, b2)
		require.NoError(t, err)
		require.NoError(t, b2.AddRangeQueryKeys("a", "z"))
		for i := range 3 {
			require.NoError(t, helper2.AddResult(NewKVRead(fmt.Sprintf("key%d", i), version.NewHeight(1, 2))))
		}
		require.Error(t, helper2.AddResult(NewKVRead("key9", version.NewHeight(1, 2))))
		// the rejected result is not retained
		require.Len(t, helper2.pendingResults, 3)

		// results kept as raw reads stay charged
		b3 := NewRWSetBuilderWithMaxRWSetSize(4096)
		helper3, err := NewRangeQueryResultsHelperWithAccounting(true, 50, testHashFunc, b3)
		require.NoError(t, err)
		require.NoError(t, b3.AddRangeQueryKeys("a", "z"))
		for i := range 10 {
			require.NoError(t, helper3.AddResult(NewKVRead(fmt.Sprintf("key%d", i), version.NewHeight(1, 2))))
		}
		results3, summary3, err := helper3.Done()
		require.NoError(t, err)
		require.Len(t, results3, 10)
		require.Nil(t, summary3)
		require.Equal(t, int64(514+10*(128+4)), b3.rwSetBytes)
	})

	t.Run("only the merkle summary is charged when the finalized query info is added", func(t *testing.T) {
		summary := &kvrwset.QueryReadsMerkleSummary{MaxDegree: 1, MaxLevel: 1, MaxLevelHashes: [][]byte{make([]byte, 128)}}
		rqi := &kvrwset.RangeQueryInfo{StartKey: "a", EndKey: "z", ItrExhausted: true}
		SetMerkelSummary(rqi, summary)
		b := NewRWSetBuilderWithMaxRWSetSize(256)
		require.NoError(t, b.AddToRangeQuerySet("ns1", rqi))
		require.Equal(t, int64(proto.Size(summary)), b.rwSetBytes)
		require.NoError(t, b.AddToRangeQuerySet("ns1", rqi))
		require.Equal(t, int64(proto.Size(summary)), b.rwSetBytes)
		b2 := NewRWSetBuilderWithMaxRWSetSize(64)
		require.Error(t, b2.AddToRangeQuerySet("ns1", rqi))
		require.Empty(t, b2.pubRwBuilderMap)
	})

	t.Run("a rejected first addition leaves no builders behind", func(t *testing.T) {
		// each case needs a fresh builder: a breach poisons the builder, so
		// reusing one would prove nothing about the check order
		const tinyLimit = 64
		b := NewRWSetBuilderWithMaxRWSetSize(tinyLimit)
		require.Error(t, b.AddToWriteSet("ns1", "key1", nil))
		require.Empty(t, b.pubRwBuilderMap)

		b = NewRWSetBuilderWithMaxRWSetSize(tinyLimit)
		require.Error(t, b.AddToMetadataWriteSet("ns1", "key1", map[string][]byte{"m": {}}))
		require.Empty(t, b.pubRwBuilderMap)

		b = NewRWSetBuilderWithMaxRWSetSize(tinyLimit)
		require.Error(t, b.AddToReadSet("ns1", "key1", version.NewHeight(1, 1)))
		require.Empty(t, b.pubRwBuilderMap)

		b = NewRWSetBuilderWithMaxRWSetSize(tinyLimit)
		require.Error(t, b.AddToHashedReadSet("ns1", "coll1", "key1", version.NewHeight(1, 1)))
		require.Empty(t, b.pubRwBuilderMap)

		b = NewRWSetBuilderWithMaxRWSetSize(tinyLimit)
		require.Error(t, b.AddToPvtAndHashedWriteSet("ns1", "coll1", "key1", nil))
		require.Empty(t, b.pubRwBuilderMap)
		require.Empty(t, b.pvtRwBuilderMap)

		b = NewRWSetBuilderWithMaxRWSetSize(tinyLimit)
		require.Error(t, b.AddToPvtAndHashedWriteSetForPurge("ns1", "coll1", "key1"))
		require.Empty(t, b.pubRwBuilderMap)
		require.Empty(t, b.pvtRwBuilderMap)

		b = NewRWSetBuilderWithMaxRWSetSize(tinyLimit)
		require.Error(t, b.AddToHashedMetadataWriteSet("ns1", "coll1", "key1", map[string][]byte{"m": {}}))
		require.Empty(t, b.pubRwBuilderMap)
		require.Empty(t, b.pvtRwBuilderMap)
	})

	t.Run("every addition path enforces a recorded breach", func(t *testing.T) {
		// read(388) + hashed read(420) + range query(0) = 808 bytes
		b := NewRWSetBuilderWithMaxRWSetSize(808)
		require.NoError(t, b.AddToReadSet("ns1", "key1", version.NewHeight(1, 1)))
		require.NoError(t, b.AddToHashedReadSet("ns1", "coll1", "key1", version.NewHeight(1, 1)))
		rqi := &kvrwset.RangeQueryInfo{StartKey: "a", EndKey: "z", ItrExhausted: true}
		require.NoError(t, b.AddToRangeQuerySet("ns1", rqi))
		err := b.AddToReadSet("ns1", "key3", version.NewHeight(1, 1)) // breach
		require.Error(t, err)
		// re-adding already tracked entries must not bypass the recorded breach
		require.Equal(t, err, b.AddToReadSet("ns1", "key1", version.NewHeight(1, 1)))
		require.Equal(t, err, b.AddToHashedReadSet("ns1", "coll1", "key1", version.NewHeight(1, 1)))
		require.Equal(t, err, b.AddToRangeQuerySet("ns1", rqi))
	})

	t.Run("tiny metadata entries are bounded by the per-entry overhead", func(t *testing.T) {
		// 1000 entries of name(4)+value(4) with overhead(128) each = 136k bytes
		b := NewRWSetBuilderWithMaxRWSetSize(100_000)
		metadata := make(map[string][]byte, 1000)
		for i := range 1000 {
			metadata[fmt.Sprintf("m%03d", i)] = make([]byte, 4)
		}
		require.ErrorContains(t, b.AddToMetadataWriteSet("ns1", "key1", metadata), "exceeds the maximum allowed size")
		// a rejected metadata write is not retained
		require.Empty(t, b.pubRwBuilderMap)
	})

	t.Run("tiny keys with empty values are bounded by the per-entry overhead", func(t *testing.T) {
		// 8-byte keys with nil values retain map entries and metadata, so the
		// limit must trip on entry count, not just value bytes
		b := NewRWSetBuilderWithMaxRWSetSize(64 * 1024)
		entrySize := int64(writeEntryOverhead + 8)
		expected := int64(64*1024) / entrySize
		for i := range expected {
			require.NoError(t, b.AddToWriteSet("ns1", fmt.Sprintf("k%07d", i), nil))
		}
		require.Error(t, b.AddToWriteSet("ns1", "k9999999", nil))
	})

	t.Run("unlimited builder never rejects", func(t *testing.T) {
		b := NewRWSetBuilder()
		for i := range 1000 {
			key := fmt.Sprintf("key%d", i)
			require.NoError(t, b.AddToWriteSet("ns1", key, make([]byte, 1024)))
			require.NoError(t, b.AddToReadSet("ns1", key, version.NewHeight(1, 1)))
		}
	})

	t.Run("recorded breach is returned on re-read of a tracked key", func(t *testing.T) {
		// limit 1000: two 386-byte reads fit, the third trips
		b := NewRWSetBuilderWithMaxRWSetSize(1000)
		require.NoError(t, b.AddToReadSet("ns1", "k1", version.NewHeight(1, 1)))
		require.NoError(t, b.AddToReadSet("ns1", "k2", version.NewHeight(1, 1)))
		err := b.AddToReadSet("ns1", "k3", version.NewHeight(1, 1))
		require.Error(t, err)
		require.Equal(t, err, b.AddToReadSet("ns1", "k1", version.NewHeight(2, 1)))
	})
}

func TestBreachWarningRateLimited(t *testing.T) {
	l := newRateLimitedLogger(time.Hour)
	l.warn("first")
	require.Zero(t, l.suppressed)
	l.warn("second")
	require.Equal(t, uint64(1), l.suppressed)
	l.warn("third")
	require.Equal(t, uint64(2), l.suppressed)
}

// TestRWSetSizeLimitBulkScan verifies that a long range scan whose results far
// exceed the budget in sum is not rejected while its peak retention stays
// below it
func TestRWSetSizeLimitBulkScan(t *testing.T) {
	b := NewRWSetBuilderWithMaxRWSetSize(100 * 1024 * 1024)
	helper, err := NewRangeQueryResultsHelperWithAccounting(true, 50, testHashFunc, b)
	require.NoError(t, err)
	require.NoError(t, b.AddRangeQueryKeys("a", "z"))
	// 771k results of overhead(128) + key(7-9) sum to over 100MiB, but only
	// one maxDegree batch is retained at a time
	for i := range 771_000 {
		require.NoError(t, helper.AddResult(NewKVRead(fmt.Sprintf("key%04d", i), version.NewHeight(1, 2))))
	}
	require.Less(t, b.rwSetBytes, int64(514+51*(128+9)))
	results, summary, err := helper.Done()
	require.NoError(t, err)
	require.Nil(t, results)
	require.NotNil(t, summary)
	require.Equal(t, int64(514), b.rwSetBytes)
}

// TestRWSetSizeLimitBulkWrites drives the bulk-write pattern of a chaincode
// performing many large PutState calls in a single simulation and asserts that
// the accumulation is cut off deterministically at the configured budget
func TestRWSetSizeLimitBulkWrites(t *testing.T) {
	const valueSize = 50 * 1024
	const limit = 4 * 1024 * 1024
	b := NewRWSetBuilderWithMaxRWSetSize(limit)
	entrySize := int64(writeEntryOverhead + len("rwset-00000000") + valueSize)
	expected := int(limit / entrySize)
	for i := range expected {
		require.NoError(t, b.AddToWriteSet("ns", fmt.Sprintf("rwset-%08d", i), make([]byte, valueSize)))
	}
	err := b.AddToWriteSet("ns", "rwset-overflow", make([]byte, valueSize))
	require.ErrorContains(t, err, "exceeds the maximum allowed size")
	// the accumulated writes are retained but no partial results can be obtained
	_, resErr := b.GetTxSimulationResults()
	require.ErrorContains(t, resErr, "exceeds the maximum allowed size")
	require.Nil(t, b.GetTxReadWriteSet())
	require.Len(t, b.pubRwBuilderMap["ns"].writeMap, expected)
}
