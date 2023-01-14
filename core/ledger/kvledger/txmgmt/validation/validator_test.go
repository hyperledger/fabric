/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

type keyValue struct {
	namespace  string
	collection string
	key        string
	keyHash    []byte
	value      []byte
	version    *version.Height
}

const (
	levelDBtestEnvName = "levelDB_LockBasedTxMgr"
	couchDBtestEnvName = "couchDB_LockBasedTxMgr"
)

var (
	// Tests will be run against each environment in this array
	// For example, to skip CouchDB tests, remove &couchDBLockBasedEnv{}
	testEnvs = map[string]privacyenabledstate.TestEnv{
		levelDBtestEnvName: &privacyenabledstate.LevelDBTestEnv{},
		couchDBtestEnvName: &privacyenabledstate.CouchDBTestEnv{},
	}

	testHashFunc = func(data []byte) ([]byte, error) {
		h := sha256.New()
		if _, err := h.Write(data); err != nil {
			return nil, err
		}
		return h.Sum(nil), nil
	}
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("statevalidator,statebasedval,statecouchdb=debug")
	exitCode := m.Run()
	for _, testEnv := range testEnvs {
		testEnv.StopExternalResource()
	}
	os.Exit(exitCode)
}

func TestValidatorBulkLoadingOfCache(t *testing.T) {
	testDBEnv := testEnvs[couchDBtestEnvName]
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("testdb")

	testValidator := &validator{db: db, hashFunc: testHashFunc}

	// populate db with initial data
	batch := privacyenabledstate.NewUpdateBatch()

	// Create two public KV pairs
	pubKV1 := keyValue{namespace: "ns1", key: "key1", value: []byte("value1"), version: version.NewHeight(1, 0)}
	pubKV2 := keyValue{namespace: "ns1", key: "key2", value: []byte("value2"), version: version.NewHeight(1, 1)}

	// Create two hashed KV pairs
	hashedKV1 := keyValue{
		namespace: "ns2", collection: "col1", key: "hashedPvtKey1",
		keyHash: util.ComputeStringHash("hashedPvtKey1"), value: []byte("value1"),
		version: version.NewHeight(1, 2),
	}
	hashedKV2 := keyValue{
		namespace: "ns2", collection: "col2", key: "hashedPvtKey2",
		keyHash: util.ComputeStringHash("hashedPvtKey2"), value: []byte("value2"),
		version: version.NewHeight(1, 3),
	}

	// Store the public and hashed KV pairs to DB
	batch.PubUpdates.Put(pubKV1.namespace, pubKV1.key, pubKV1.value, pubKV1.version)
	batch.PubUpdates.Put(pubKV2.namespace, pubKV2.key, pubKV2.value, pubKV2.version)
	batch.HashUpdates.Put(hashedKV1.namespace, hashedKV1.collection, hashedKV1.keyHash, hashedKV1.value, hashedKV1.version)
	batch.HashUpdates.Put(hashedKV2.namespace, hashedKV2.collection, hashedKV2.keyHash, hashedKV2.value, hashedKV2.version)

	require.NoError(t, db.ApplyPrivacyAwareUpdates(batch, version.NewHeight(1, 4)))

	// Construct read set for transaction 1. It contains two public KV pairs (pubKV1, pubKV2) and two
	// hashed KV pairs (hashedKV1, hashedKV2).
	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder1.AddToReadSet(pubKV1.namespace, pubKV1.key, pubKV1.version)
	rwsetBuilder1.AddToReadSet(pubKV2.namespace, pubKV2.key, pubKV2.version)
	rwsetBuilder1.AddToHashedReadSet(hashedKV1.namespace, hashedKV1.collection, hashedKV1.key, hashedKV1.version)
	rwsetBuilder1.AddToHashedReadSet(hashedKV2.namespace, hashedKV2.collection, hashedKV2.key, hashedKV2.version)

	// Construct read set for transaction 1. It contains KV pairs which are not in the state db.
	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder2.AddToReadSet("ns3", "key1", nil)
	rwsetBuilder2.AddToHashedReadSet("ns3", "col1", "hashedPvtKey1", nil)

	// Construct internal block
	transRWSets := getTestPubSimulationRWSet(t, rwsetBuilder1, rwsetBuilder2)
	var trans []*transaction
	for i, tranRWSet := range transRWSets {
		tx := &transaction{
			id:             fmt.Sprintf("txid-%d", i),
			indexInBlock:   i,
			validationCode: peer.TxValidationCode_VALID,
			rwset:          tranRWSet,
		}
		trans = append(trans, tx)
	}
	blk := &block{num: 1, txs: trans}

	if testValidator.db.IsBulkOptimizable() {

		db := testValidator.db
		bulkOptimizable, _ := db.VersionedDB.(statedb.BulkOptimizable)

		// Clear cache loaded during ApplyPrivacyAwareUpdates()
		testValidator.db.ClearCachedVersions()

		require.NoError(t, testValidator.preLoadCommittedVersionOfRSet(blk))

		// pubKV1 should be found in cache
		version, keyFound := bulkOptimizable.GetCachedVersion(pubKV1.namespace, pubKV1.key)
		require.True(t, keyFound)
		require.Equal(t, pubKV1.version, version)

		// pubKV2 should be found in cache
		version, keyFound = bulkOptimizable.GetCachedVersion(pubKV2.namespace, pubKV2.key)
		require.True(t, keyFound)
		require.Equal(t, pubKV2.version, version)

		// [ns3, key1] should be found in cache as it was in the readset of transaction 1 though it is
		// not in the state db but the version would be nil
		version, keyFound = bulkOptimizable.GetCachedVersion("ns3", "key1")
		require.True(t, keyFound)
		require.Nil(t, version)

		// [ns4, key1] should not be found in cache as it was not loaded
		version, keyFound = bulkOptimizable.GetCachedVersion("ns4", "key1")
		require.False(t, keyFound)
		require.Nil(t, version)

		// hashedKV1 should be found in cache
		version, keyFound = testValidator.db.GetCachedKeyHashVersion(hashedKV1.namespace,
			hashedKV1.collection, hashedKV1.keyHash)
		require.True(t, keyFound)
		require.Equal(t, hashedKV1.version, version)

		// hashedKV2 should be found in cache
		version, keyFound = testValidator.db.GetCachedKeyHashVersion(hashedKV2.namespace,
			hashedKV2.collection, hashedKV2.keyHash)
		require.True(t, keyFound)
		require.Equal(t, hashedKV2.version, version)

		// [ns3, col1, hashedPvtKey1] should be found in cache as it was in the readset of transaction 2 though it is
		// not in the state db
		version, keyFound = testValidator.db.GetCachedKeyHashVersion("ns3", "col1", util.ComputeStringHash("hashedPvtKey1"))
		require.True(t, keyFound)
		require.Nil(t, version)

		// [ns4, col, key1] should not be found in cache as it was not loaded
		version, keyFound = testValidator.db.GetCachedKeyHashVersion("ns4", "col1", util.ComputeStringHash("key1"))
		require.False(t, keyFound)
		require.Nil(t, version)

		// Clear cache
		testValidator.db.ClearCachedVersions()

		// pubKV1 should not be found in cache as cahce got emptied
		version, keyFound = bulkOptimizable.GetCachedVersion(pubKV1.namespace, pubKV1.key)
		require.False(t, keyFound)
		require.Nil(t, version)

		// [ns3, col1, key1] should not be found in cache as cahce got emptied
		version, keyFound = testValidator.db.GetCachedKeyHashVersion("ns3", "col1", util.ComputeStringHash("hashedPvtKey1"))
		require.False(t, keyFound)
		require.Nil(t, version)
	}
}

func TestValidator(t *testing.T) {
	testDBEnv := testEnvs[levelDBtestEnvName]
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	// populate db with initial data
	batch := privacyenabledstate.NewUpdateBatch()
	batch.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 0))
	batch.PubUpdates.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 1))
	batch.PubUpdates.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 2))
	batch.PubUpdates.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 3))
	batch.PubUpdates.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 4))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(batch, version.NewHeight(1, 4)))

	testValidator := &validator{db: db, hashFunc: testHashFunc}

	// rwset1 should be valid
	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder1.AddToReadSet("ns1", "key1", version.NewHeight(1, 0))
	rwsetBuilder1.AddToReadSet("ns2", "key2", nil)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder1), []int{})

	// rwset2 should not be valid
	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder2.AddToReadSet("ns1", "key1", version.NewHeight(1, 1))
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder2), []int{0})

	// rwset3 should not be valid
	rwsetBuilder3 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder3.AddToReadSet("ns1", "key1", nil)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder3), []int{0})

	// rwset4 and rwset5 within same block - rwset4 should be valid and makes rwset5 as invalid
	rwsetBuilder4 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder4.AddToReadSet("ns1", "key1", version.NewHeight(1, 0))
	rwsetBuilder4.AddToWriteSet("ns1", "key1", []byte("value1_new"))

	rwsetBuilder5 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder5.AddToReadSet("ns1", "key1", version.NewHeight(1, 0))
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder4, rwsetBuilder5), []int{1})
}

func TestPhantomValidation(t *testing.T) {
	testDBEnv := testEnvs[levelDBtestEnvName]
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	// populate db with initial data
	batch := privacyenabledstate.NewUpdateBatch()
	batch.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 0))
	batch.PubUpdates.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 1))
	batch.PubUpdates.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 2))
	batch.PubUpdates.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 3))
	batch.PubUpdates.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 4))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(batch, version.NewHeight(1, 4)))

	testValidator := &validator{db: db, hashFunc: testHashFunc}

	// rwset1 should be valid
	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: true}
	rwsetutil.SetRawReads(rqi1, []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
	})
	rwsetBuilder1.AddToRangeQuerySet("ns1", rqi1)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder1), []int{})

	// rwset2 should not be valid - Version of key4 changed
	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rwsetutil.SetRawReads(rqi2, []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 2)),
	})
	rwsetBuilder2.AddToRangeQuerySet("ns1", rqi2)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder2), []int{0})

	// rwset3 should not be valid - simulate key3 got committed to db
	rwsetBuilder3 := rwsetutil.NewRWSetBuilder()
	rqi3 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rwsetutil.SetRawReads(rqi3, []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3)),
	})
	rwsetBuilder3.AddToRangeQuerySet("ns1", rqi3)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder3), []int{0})

	// //Remove a key in rwset4 and rwset5 should become invalid
	rwsetBuilder4 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder4.AddToWriteSet("ns1", "key3", nil)
	rwsetBuilder5 := rwsetutil.NewRWSetBuilder()
	rqi5 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rwsetutil.SetRawReads(rqi5, []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3)),
	})
	rwsetBuilder5.AddToRangeQuerySet("ns1", rqi5)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder4, rwsetBuilder5), []int{1})

	// Add a key in rwset6 and rwset7 should become invalid
	rwsetBuilder6 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder6.AddToWriteSet("ns1", "key2_1", []byte("value2_1"))

	rwsetBuilder7 := rwsetutil.NewRWSetBuilder()
	rqi7 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key4", ItrExhausted: false}
	rwsetutil.SetRawReads(rqi7, []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3)),
	})
	rwsetBuilder7.AddToRangeQuerySet("ns1", rqi7)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder6, rwsetBuilder7), []int{1})
}

func TestPhantomHashBasedValidation(t *testing.T) {
	testDBEnv := testEnvs[levelDBtestEnvName]
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()
	db := testDBEnv.GetDBHandle("TestDB")

	// populate db with initial data
	batch := privacyenabledstate.NewUpdateBatch()
	batch.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 0))
	batch.PubUpdates.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 1))
	batch.PubUpdates.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 2))
	batch.PubUpdates.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 3))
	batch.PubUpdates.Put("ns1", "key5", []byte("value5"), version.NewHeight(1, 4))
	batch.PubUpdates.Put("ns1", "key6", []byte("value6"), version.NewHeight(1, 5))
	batch.PubUpdates.Put("ns1", "key7", []byte("value7"), version.NewHeight(1, 6))
	batch.PubUpdates.Put("ns1", "key8", []byte("value8"), version.NewHeight(1, 7))
	batch.PubUpdates.Put("ns1", "key9", []byte("value9"), version.NewHeight(1, 8))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(batch, version.NewHeight(1, 8)))

	testValidator := &validator{db: db, hashFunc: testHashFunc}

	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "key2", EndKey: "key9", ItrExhausted: true}
	kvReadsDuringSimulation1 := []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 2)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3)),
		rwsetutil.NewKVRead("key5", version.NewHeight(1, 4)),
		rwsetutil.NewKVRead("key6", version.NewHeight(1, 5)),
		rwsetutil.NewKVRead("key7", version.NewHeight(1, 6)),
		rwsetutil.NewKVRead("key8", version.NewHeight(1, 7)),
	}
	rwsetutil.SetMerkelSummary(rqi1, buildTestHashResults(t, 2, kvReadsDuringSimulation1))
	rwsetBuilder1.AddToRangeQuerySet("ns1", rqi1)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder1), []int{})

	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "key1", EndKey: "key9", ItrExhausted: false}
	kvReadsDuringSimulation2 := []*kvrwset.KVRead{
		rwsetutil.NewKVRead("key1", version.NewHeight(1, 0)),
		rwsetutil.NewKVRead("key2", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key3", version.NewHeight(1, 1)),
		rwsetutil.NewKVRead("key4", version.NewHeight(1, 3)),
		rwsetutil.NewKVRead("key5", version.NewHeight(1, 4)),
		rwsetutil.NewKVRead("key6", version.NewHeight(1, 5)),
		rwsetutil.NewKVRead("key7", version.NewHeight(1, 6)),
		rwsetutil.NewKVRead("key8", version.NewHeight(1, 7)),
		rwsetutil.NewKVRead("key9", version.NewHeight(1, 8)),
	}
	rwsetutil.SetMerkelSummary(rqi2, buildTestHashResults(t, 2, kvReadsDuringSimulation2))
	rwsetBuilder2.AddToRangeQuerySet("ns1", rqi2)
	checkValidation(t, testValidator, getTestPubSimulationRWSet(t, rwsetBuilder2), []int{0})
}

func TestPrvtdataPurgeUpdates(t *testing.T) {
	testDBEnv := testEnvs[levelDBtestEnvName]
	testDBEnv.Init(t)
	defer testDBEnv.Cleanup()

	db := testDBEnv.GetDBHandle("TestDB")

	rwsetBuilder1 := rwsetutil.NewRWSetBuilder()
	// key1 and key2 are purged, and key3 is written
	rwsetBuilder1.AddToPvtAndHashedWriteSetForPurge("ns1", "coll1", "key1")
	rwsetBuilder1.AddToPvtAndHashedWriteSetForPurge("ns1", "coll1", "key2")
	rwsetBuilder1.AddToPvtAndHashedWriteSet("ns1", "coll1", "key3", []byte("value3"))
	txRWset1 := rwsetBuilder1.GetTxReadWriteSet()

	rwsetBuilder2 := rwsetutil.NewRWSetBuilder()
	rwsetBuilder2.AddToPvtAndHashedWriteSet("ns1", "coll1", "key1", []byte("value1"))
	txRWset2 := rwsetBuilder2.GetTxReadWriteSet()

	createBlock := func(transRWSets []*rwsetutil.TxRwSet) *block {
		var trans []*transaction
		for i, tranRWSet := range transRWSets {
			tx := &transaction{
				id:             fmt.Sprintf("txid-%d", i),
				indexInBlock:   i,
				validationCode: peer.TxValidationCode_VALID,
				rwset:          tranRWSet,
			}
			trans = append(trans, tx)
		}
		return &block{num: 1, txs: trans}
	}

	t.Run("simple-case", func(t *testing.T) {
		block := createBlock([]*rwsetutil.TxRwSet{
			txRWset1,
		})
		validator := &validator{db: db, hashFunc: testHashFunc}
		_, appInitiatedPurgeUpdates, err := validator.validateAndPrepareBatch(block, true)
		require.NoError(t, err)
		require.ElementsMatch(
			t,
			appInitiatedPurgeUpdates,
			[]*AppInitiatedPurgeUpdate{
				{
					CompositeKey: &privacyenabledstate.HashedCompositeKey{
						Namespace:      "ns1",
						CollectionName: "coll1",
						KeyHash:        string(util.ComputeStringHash("key1")),
					},
					Version: version.NewHeight(1, 0),
				},
				{
					CompositeKey: &privacyenabledstate.HashedCompositeKey{
						Namespace:      "ns1",
						CollectionName: "coll1",
						KeyHash:        string(util.ComputeStringHash("key2")),
					},
					Version: version.NewHeight(1, 0),
				},
			},
		)
	})

	t.Run("Next transaction in block overwrites new value to a purged key", func(t *testing.T) {
		block := createBlock([]*rwsetutil.TxRwSet{
			txRWset1, txRWset2,
		})
		validator := &validator{db: db, hashFunc: testHashFunc}
		_, appInitiatedPurgeUpdates, err := validator.validateAndPrepareBatch(block, true)
		require.NoError(t, err)
		require.ElementsMatch(
			t,
			appInitiatedPurgeUpdates,
			[]*AppInitiatedPurgeUpdate{
				{
					CompositeKey: &privacyenabledstate.HashedCompositeKey{
						Namespace:      "ns1",
						CollectionName: "coll1",
						KeyHash:        string(util.ComputeStringHash("key1")),
					},
					Version: version.NewHeight(1, 0),
				},
				{
					CompositeKey: &privacyenabledstate.HashedCompositeKey{
						Namespace:      "ns1",
						CollectionName: "coll1",
						KeyHash:        string(util.ComputeStringHash("key2")),
					},
					Version: version.NewHeight(1, 0),
				},
			},
		)
	})

	t.Run("Next transaction in block purges a key written in a previous transaction in the block", func(t *testing.T) {
		block := createBlock([]*rwsetutil.TxRwSet{
			txRWset2, txRWset1,
		})
		validator := &validator{db: db, hashFunc: testHashFunc}
		_, appInitiatedPurgeUpdates, err := validator.validateAndPrepareBatch(block, true)
		require.NoError(t, err)
		require.ElementsMatch(
			t,
			appInitiatedPurgeUpdates,
			[]*AppInitiatedPurgeUpdate{
				{
					CompositeKey: &privacyenabledstate.HashedCompositeKey{
						Namespace:      "ns1",
						CollectionName: "coll1",
						KeyHash:        string(util.ComputeStringHash("key1")),
					},
					Version: version.NewHeight(1, 1),
				},
				{
					CompositeKey: &privacyenabledstate.HashedCompositeKey{
						Namespace:      "ns1",
						CollectionName: "coll1",
						KeyHash:        string(util.ComputeStringHash("key2")),
					},
					Version: version.NewHeight(1, 1),
				},
			},
		)
	})
}

func checkValidation(t *testing.T, val *validator, transRWSets []*rwsetutil.TxRwSet, expectedInvalidTxIndexes []int) {
	var trans []*transaction
	for i, tranRWSet := range transRWSets {
		tx := &transaction{
			id:             fmt.Sprintf("txid-%d", i),
			indexInBlock:   i,
			validationCode: peer.TxValidationCode_VALID,
			rwset:          tranRWSet,
		}
		trans = append(trans, tx)
	}
	blk := &block{num: 1, txs: trans}
	_, _, err := val.validateAndPrepareBatch(blk, true)
	require.NoError(t, err)
	t.Logf("block.Txs[0].ValidationCode = %d", blk.txs[0].validationCode)
	var invalidTxs []int
	for _, tx := range blk.txs {
		if tx.validationCode != peer.TxValidationCode_VALID {
			invalidTxs = append(invalidTxs, tx.indexInBlock)
		}
	}
	require.Equal(t, len(expectedInvalidTxIndexes), len(invalidTxs))
	require.ElementsMatch(t, invalidTxs, expectedInvalidTxIndexes)
}

func buildTestHashResults(t *testing.T, maxDegree int, kvReads []*kvrwset.KVRead) *kvrwset.QueryReadsMerkleSummary {
	if len(kvReads) <= maxDegree {
		t.Fatal("This method should be called with number of KVReads more than maxDegree; Else, hashing won't be performedrwset")
	}
	helper, _ := rwsetutil.NewRangeQueryResultsHelper(true, uint32(maxDegree), testHashFunc)
	for _, kvRead := range kvReads {
		require.NoError(t, helper.AddResult(kvRead))
	}
	_, h, err := helper.Done()
	require.NoError(t, err)
	require.NotNil(t, h)
	return h
}

func getTestPubSimulationRWSet(t *testing.T, builders ...*rwsetutil.RWSetBuilder) []*rwsetutil.TxRwSet {
	var pubRWSets []*rwsetutil.TxRwSet
	for _, b := range builders {
		s, e := b.GetTxSimulationResults()
		require.NoError(t, e)
		sBytes, err := s.GetPubSimulationBytes()
		require.NoError(t, err)
		pubRWSet := &rwsetutil.TxRwSet{}
		require.NoError(t, pubRWSet.FromProtoBytes(sBytes))
		pubRWSets = append(pubRWSets, pubRWSet)
	}
	return pubRWSets
}
