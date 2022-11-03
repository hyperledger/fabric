/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwsetutil

import (
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
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
	require.Nil(t, txSimulationResults.PubSimulationResults.NsRwset[0].CollectionHashedRwset)
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

	///////////////////////////////////////////////////////
	// construct the expected pvt rwset and compare with the one present in the txSimulationResults
	///////////////////////////////////////////////////////
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

	///////////////////////////////////////////////////////
	// construct the public rwset (which will be part of the block) and compare with the one present in the txSimulationResults
	///////////////////////////////////////////////////////
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
	require.Equal(t, combinedNs1, actualSimRes.PubSimulationResults.NsRwset[0])

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
	require.Equal(t, combinedNs2, actualSimRes.PubSimulationResults.NsRwset[1])

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
	require.Equal(t, combinedNs3, actualSimRes.PubSimulationResults.NsRwset[2])

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
	rwSetBuilder.AddToMetadataWriteSet("ns1", "key1",
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
	require.Equal(t, pubAndHashCombinedNs1, actualSimRes.PubSimulationResults.NsRwset[0])
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
			proto.Unmarshal(simulationResults.PubSimulationResults.NsRwset[0].Rwset, pubRWSet),
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
				proto.Unmarshal(simulationResults.PubSimulationResults.NsRwset[0].CollectionHashedRwset[0].HashedRwset, hashedRWSet),
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
				proto.Unmarshal(simulationResults.PvtSimulationResults.NsPvtRwset[0].CollectionPvtRwset[0].Rwset, pvtWSet),
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
