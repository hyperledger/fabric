/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package internal

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/storageutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/ledger/rwset/kvrwset"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		fmt.Printf("could not create temp dir %s", err)
		os.Exit(-1)
		return
	}
	flogging.SetModuleLevel("valinternal", "debug")
	viper.Set("peer.fileSystemPath", tempDir)
	os.Exit(m.Run())
}

func TestTxOps(t *testing.T) {
	assert := assert.New(t)

	txops := txOps{}
	key1 := compositeKey{"ns1", "", "key1"}
	key2 := compositeKey{"ns1", "coll2", "key2"}
	key3 := compositeKey{"ns1", "coll3", "key3"}
	key4 := compositeKey{"ns1", "coll4", "key4"}

	txops.upsert(key1, []byte("key1-value1"))
	assert.True(txops[key1].isOnlyUpsert())

	txops.upsert(key2, []byte("key2-value2"))
	assert.True(txops[key2].isOnlyUpsert())
	txops.metadataUpdate(key2, []byte("key2-metadata"))
	assert.False(txops[key2].isOnlyUpsert())
	assert.True(txops[key2].isUpsertAndMetadataUpdate())

	txops.upsert(key3, []byte("key3-value"))
	assert.True(txops[key3].isOnlyUpsert())
	txops.metadataDelete(key3)
	assert.False(txops[key3].isOnlyUpsert())
	assert.True(txops[key3].isUpsertAndMetadataUpdate())

	txops.delete(key4)
	assert.True(txops[key4].isDelete())
}

func TestTxOpsPreparationValueUpdate(t *testing.T) {
	testDBEnv := privacyenabledstate.LevelDBCommonStorageTestEnv{}
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	ck1, ck2, ck3 :=
		compositeKey{ns: "ns1", key: "key1"},
		compositeKey{ns: "ns1", key: "key2"},
		compositeKey{ns: "ns1", key: "key3"}

	updateBatch := privacyenabledstate.NewUpdateBatch()
	updateBatch.PubUpdates.Put(ck1.ns, ck1.key, []byte("value1"), version.NewHeight(1, 1)) // write key1 with only value
	updateBatch.PubUpdates.PutValAndMetadata(                                              // write key2 with value and metadata
		ck2.ns, ck2.key,
		[]byte("value2"),
		testutilSerializedMetadata(t, map[string][]byte{"metadata2": []byte("metadata2")}),
		version.NewHeight(1, 2))

	db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 2)) //write the above initial state to db
	precedingUpdates := NewPubAndHashUpdates()

	rwset := testutilBuildRwset( // A sample rwset {upsert key1, key2, key3}
		t,
		map[compositeKey][]byte{
			ck1: []byte("value1_new"),
			ck2: []byte("value2_new"),
			ck3: []byte("value3_new"),
		},
		nil,
	)

	txOps, err := prepareTxOps(rwset, version.NewHeight(1, 2), precedingUpdates, db)
	assert.NoError(t, err)
	assert.Len(t, txOps, 3)

	ck1ExpectedKeyOps := &keyOps{ // finally, key1 should have only new value
		flag:  upsertVal,
		value: []byte("value1_new"),
	}

	ck2ExpectedKeyOps := &keyOps{ // key2 should have new value and existing metadata
		flag:     upsertVal,
		value:    []byte("value2_new"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata2": []byte("metadata2")}),
	}

	ck3ExpectedKeyOps := &keyOps{ // key3 should have new value
		flag:  upsertVal,
		value: []byte("value3_new"),
	}

	assert.Equal(t, ck1ExpectedKeyOps, txOps[ck1])
	assert.Equal(t, ck2ExpectedKeyOps, txOps[ck2])
	assert.Equal(t, ck3ExpectedKeyOps, txOps[ck3])
}

func TestTxOpsPreparationMetadataUpdates(t *testing.T) {
	testDBEnv := privacyenabledstate.LevelDBCommonStorageTestEnv{}
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	ck1, ck2, ck3 :=
		compositeKey{ns: "ns1", key: "key1"},
		compositeKey{ns: "ns1", key: "key2"},
		compositeKey{ns: "ns1", key: "key3"}

	updateBatch := privacyenabledstate.NewUpdateBatch()
	updateBatch.PubUpdates.Put(ck1.ns, ck1.key, []byte("value1"), version.NewHeight(1, 1)) // write key1 with only value
	updateBatch.PubUpdates.PutValAndMetadata(                                              // write key2 with value and metadata
		ck2.ns, ck2.key,
		[]byte("value2"),
		testutilSerializedMetadata(t, map[string][]byte{"metadata2": []byte("metadata2")}),
		version.NewHeight(1, 2))

	db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 2)) //write the above initial state to db
	precedingUpdates := NewPubAndHashUpdates()

	rwset := testutilBuildRwset( // A sample rwset {update metadta for the three keys}
		t,
		nil,
		map[compositeKey]map[string][]byte{
			ck1: {"metadata1": []byte("metadata1_new")},
			ck2: {"metadata2": []byte("metadata2_new")},
			ck3: {"metadata3": []byte("metadata3_new")},
		},
	)

	txOps, err := prepareTxOps(rwset, version.NewHeight(1, 2), precedingUpdates, db)
	assert.NoError(t, err)
	assert.Len(t, txOps, 2) // key3 should have been removed from the txOps because, the key3 does not exist and only metadata is being updated

	ck1ExpectedKeyOps := &keyOps{ // finally, key1 should have only existing value and new metadata
		flag:     metadataUpdate,
		value:    []byte("value1"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata1": []byte("metadata1_new")}),
	}

	ck2ExpectedKeyOps := &keyOps{ // key2 should have existing value and new metadata
		flag:     metadataUpdate,
		value:    []byte("value2"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata2": []byte("metadata2_new")}),
	}

	assert.Equal(t, ck1ExpectedKeyOps, txOps[ck1])
	assert.Equal(t, ck2ExpectedKeyOps, txOps[ck2])
}

func TestTxOpsPreparationMetadataDelete(t *testing.T) {
	testDBEnv := privacyenabledstate.LevelDBCommonStorageTestEnv{}
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	ck1, ck2, ck3 :=
		compositeKey{ns: "ns1", key: "key1"},
		compositeKey{ns: "ns1", key: "key2"},
		compositeKey{ns: "ns1", key: "key3"}

	updateBatch := privacyenabledstate.NewUpdateBatch()
	updateBatch.PubUpdates.Put(ck1.ns, ck1.key, []byte("value1"), version.NewHeight(1, 1)) // write key1 with only value
	updateBatch.PubUpdates.PutValAndMetadata(                                              // write key2 with value and metadata
		ck2.ns, ck2.key,
		[]byte("value2"),
		testutilSerializedMetadata(t, map[string][]byte{"metadata2": []byte("metadata2")}),
		version.NewHeight(1, 2))

	db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 2)) //write the above initial state to db
	precedingUpdates := NewPubAndHashUpdates()

	rwset := testutilBuildRwset( // A sample rwset {delete metadata for the three keys}
		t,
		nil,
		map[compositeKey]map[string][]byte{
			ck1: {},
			ck2: {},
			ck3: {},
		},
	)

	txOps, err := prepareTxOps(rwset, version.NewHeight(1, 2), precedingUpdates, db)
	assert.NoError(t, err)
	assert.Len(t, txOps, 2) // key3 should have been removed from the txOps because, the key3 does not exist and only metadata is being updated

	ck1ExpectedKeyOps := &keyOps{ // finally, key1 should have only existing value and no metadata
		flag:  metadataDelete,
		value: []byte("value1"),
	}

	ck2ExpectedKeyOps := &keyOps{ // key2 should have existing value and no metadata
		flag:  metadataDelete,
		value: []byte("value2"),
	}

	assert.Equal(t, ck1ExpectedKeyOps, txOps[ck1])
	assert.Equal(t, ck2ExpectedKeyOps, txOps[ck2])
}

func TestTxOpsPreparationMixedUpdates(t *testing.T) {
	testDBEnv := privacyenabledstate.LevelDBCommonStorageTestEnv{}
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	ck1, ck2, ck3, ck4 :=
		compositeKey{ns: "ns1", key: "key1"},
		compositeKey{ns: "ns1", key: "key2"},
		compositeKey{ns: "ns1", key: "key3"},
		compositeKey{ns: "ns1", key: "key4"}

	updateBatch := privacyenabledstate.NewUpdateBatch()
	updateBatch.PubUpdates.Put(ck1.ns, ck1.key, []byte("value1"), version.NewHeight(1, 1)) // write key1 with only value
	updateBatch.PubUpdates.Put(ck2.ns, ck2.key, []byte("value2"), version.NewHeight(1, 2)) // write key2 with only value
	updateBatch.PubUpdates.PutValAndMetadata(                                              // write key3 with value and metadata
		ck3.ns, ck3.key,
		[]byte("value3"),
		testutilSerializedMetadata(t, map[string][]byte{"metadata3": []byte("metadata3")}),
		version.NewHeight(1, 3))
	updateBatch.PubUpdates.PutValAndMetadata( // write key4 with value and metadata
		ck4.ns, ck4.key,
		[]byte("value4"),
		testutilSerializedMetadata(t, map[string][]byte{"metadata4": []byte("metadata4")}),
		version.NewHeight(1, 4))

	db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 2)) //write the above initial state to db

	precedingUpdates := NewPubAndHashUpdates()

	rwset := testutilBuildRwset( // A sample rwset {key1:only value update, key2: value and metadata update, key3: only metadata update, key4: only value update}
		t,
		map[compositeKey][]byte{
			ck1: []byte("value1_new"),
			ck2: []byte("value2_new"),
			ck4: []byte("value4_new"),
		},
		map[compositeKey]map[string][]byte{
			ck2: {"metadata2": []byte("metadata2_new")},
			ck3: {"metadata3": []byte("metadata3_new")},
		},
	)

	txOps, err := prepareTxOps(rwset, version.NewHeight(1, 2), precedingUpdates, db)
	assert.NoError(t, err)
	assert.Len(t, txOps, 4)

	ck1ExpectedKeyOps := &keyOps{ // finally, key1 should have only new value
		flag:  upsertVal,
		value: []byte("value1_new"),
	}

	ck2ExpectedKeyOps := &keyOps{ // key2 should have new value and new metadata
		flag:     upsertVal + metadataUpdate,
		value:    []byte("value2_new"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata2": []byte("metadata2_new")}),
	}

	ck3ExpectedKeyOps := &keyOps{ // key3 should have existing value and new metadata
		flag:     metadataUpdate,
		value:    []byte("value3"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata3": []byte("metadata3_new")}),
	}

	ck4ExpectedKeyOps := &keyOps{ // key4 should have new value and existing metadata
		flag:     upsertVal,
		value:    []byte("value4_new"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata4": []byte("metadata4")}),
	}

	assert.Equal(t, ck1ExpectedKeyOps, txOps[ck1])
	assert.Equal(t, ck2ExpectedKeyOps, txOps[ck2])
	assert.Equal(t, ck3ExpectedKeyOps, txOps[ck3])
	assert.Equal(t, ck4ExpectedKeyOps, txOps[ck4])
}

func TestTxOpsPreparationPvtdataHashes(t *testing.T) {
	testDBEnv := privacyenabledstate.LevelDBCommonStorageTestEnv{}
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	ck1, ck2, ck3, ck4 :=
		compositeKey{ns: "ns1", coll: "coll1", key: "key1"},
		compositeKey{ns: "ns1", coll: "coll1", key: "key2"},
		compositeKey{ns: "ns1", coll: "coll1", key: "key3"},
		compositeKey{ns: "ns1", coll: "coll1", key: "key4"}

	ck1Hash, ck2Hash, ck3Hash, ck4Hash :=
		compositeKey{ns: "ns1", coll: "coll1", key: string(util.ComputeStringHash("key1"))},
		compositeKey{ns: "ns1", coll: "coll1", key: string(util.ComputeStringHash("key2"))},
		compositeKey{ns: "ns1", coll: "coll1", key: string(util.ComputeStringHash("key3"))},
		compositeKey{ns: "ns1", coll: "coll1", key: string(util.ComputeStringHash("key4"))}

	updateBatch := privacyenabledstate.NewUpdateBatch()

	updateBatch.HashUpdates.Put(ck1.ns, ck1.coll, util.ComputeStringHash(ck1.key),
		util.ComputeStringHash("value1"), version.NewHeight(1, 1)) // write key1 with only value

	updateBatch.HashUpdates.Put(ck2.ns, ck2.coll, util.ComputeStringHash(ck2.key),
		util.ComputeStringHash("value2"), version.NewHeight(1, 2)) // write key2 with only value

	updateBatch.HashUpdates.PutValAndMetadata( // write key3 with value and metadata
		ck3.ns, ck3.coll, string(util.ComputeStringHash(ck3.key)),
		util.ComputeStringHash("value3"),
		testutilSerializedMetadata(t, map[string][]byte{"metadata3": []byte("metadata3")}),
		version.NewHeight(1, 3))

	updateBatch.HashUpdates.PutValAndMetadata( // write key4 with value and metadata
		ck4.ns, ck4.coll, string(util.ComputeStringHash(ck4.key)),
		util.ComputeStringHash("value4"),
		testutilSerializedMetadata(t, map[string][]byte{"metadata4": []byte("metadata4")}),
		version.NewHeight(1, 4))

	db.ApplyPrivacyAwareUpdates(updateBatch, version.NewHeight(1, 2)) //write the above initial state to db

	precedingUpdates := NewPubAndHashUpdates()
	rwset := testutilBuildRwset( // A sample rwset {key1:only value update, key2: value and metadata update, key3: only metadata update, key4: only value update}
		t,
		map[compositeKey][]byte{
			ck1: []byte("value1_new"),
			ck2: []byte("value2_new"),
			ck4: []byte("value4_new"),
		},
		map[compositeKey]map[string][]byte{
			ck2: {"metadata2": []byte("metadata2_new")},
			ck3: {"metadata3": []byte("metadata3_new")},
		},
	)

	txOps, err := prepareTxOps(rwset, version.NewHeight(1, 2), precedingUpdates, db)
	assert.NoError(t, err)
	assert.Len(t, txOps, 4)

	ck1ExpectedKeyOps := &keyOps{ // finally, key1 should have only new value
		flag:  upsertVal,
		value: util.ComputeStringHash("value1_new"),
	}

	ck2ExpectedKeyOps := &keyOps{ // key2 should have new value and new metadata
		flag:     upsertVal + metadataUpdate,
		value:    util.ComputeStringHash("value2_new"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata2": []byte("metadata2_new")}),
	}

	ck3ExpectedKeyOps := &keyOps{ // key3 should have existing value and new metadata
		flag:     metadataUpdate,
		value:    util.ComputeStringHash("value3"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata3": []byte("metadata3_new")}),
	}

	ck4ExpectedKeyOps := &keyOps{ // key4 should have new value and existing metadata
		flag:     upsertVal,
		value:    util.ComputeStringHash("value4_new"),
		metadata: testutilSerializedMetadata(t, map[string][]byte{"metadata4": []byte("metadata4")}),
	}

	assert.Equal(t, ck1ExpectedKeyOps, txOps[ck1Hash])
	assert.Equal(t, ck2ExpectedKeyOps, txOps[ck2Hash])
	assert.Equal(t, ck3ExpectedKeyOps, txOps[ck3Hash])
	assert.Equal(t, ck4ExpectedKeyOps, txOps[ck4Hash])
}

func testutilBuildRwset(t *testing.T,
	kvWrites map[compositeKey][]byte,
	metadataWrites map[compositeKey]map[string][]byte) *rwsetutil.TxRwSet {
	rwsetBuilder := rwsetutil.NewRWSetBuilder()
	for kvwrite, val := range kvWrites {
		if kvwrite.coll == "" {
			rwsetBuilder.AddToWriteSet(kvwrite.ns, kvwrite.key, val)
		} else {
			rwsetBuilder.AddToPvtAndHashedWriteSet(kvwrite.ns, kvwrite.coll, kvwrite.key, val)
		}
	}

	for metadataWrite, metadataVal := range metadataWrites {
		if metadataWrite.coll == "" {
			rwsetBuilder.AddToMetadataWriteSet(metadataWrite.ns, metadataWrite.key, metadataVal)
		} else {
			rwsetBuilder.AddToHashedMetadataWriteSet(metadataWrite.ns, metadataWrite.coll, metadataWrite.key, metadataVal)
		}
	}
	return rwsetBuilder.GetTxReadWriteSet()
}

func testutilSerializedMetadata(t *testing.T, metadataMap map[string][]byte) []byte {
	metadataEntries := []*kvrwset.KVMetadataEntry{}
	for metadataK, metadataV := range metadataMap {
		metadataEntries = append(metadataEntries, &kvrwset.KVMetadataEntry{Name: metadataK, Value: metadataV})
	}
	metadataBytes, err := storageutil.SerializeMetadata(metadataEntries)
	assert.NoError(t, err)
	return metadataBytes
}
