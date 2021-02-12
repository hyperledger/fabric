/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validation

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/stretchr/testify/require"
)

func TestCombinedIterator(t *testing.T) {
	testDBEnv := stateleveldb.NewTestVDBEnv(t)
	defer testDBEnv.Cleanup()

	db, err := testDBEnv.DBProvider.GetDBHandle("TestDB", nil)
	require.NoError(t, err)

	// populate db with initial data
	batch := statedb.NewUpdateBatch()
	batch.Put("ns", "key1", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns", "key4", []byte("value4"), version.NewHeight(1, 1))
	batch.Put("ns", "key6", []byte("value6"), version.NewHeight(1, 1))
	batch.Put("ns", "key8", []byte("value8"), version.NewHeight(1, 1))
	require.NoError(t, db.ApplyUpdates(batch, version.NewHeight(1, 5)))

	// prepare batch1
	batch1 := statedb.NewUpdateBatch()
	batch1.Put("ns", "key3", []byte("value3"), version.NewHeight(1, 1))
	batch1.Delete("ns", "key5", version.NewHeight(1, 1))
	batch1.Put("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1))
	batch1.Put("ns", "key7", []byte("value7"), version.NewHeight(1, 1))

	// prepare batch2 (empty)
	batch2 := statedb.NewUpdateBatch()

	// Test db + batch1 updates (exclude endKey)
	itr1, _ := newCombinedIterator(db, batch1, "ns", "key2", "key8", false)
	defer itr1.Close()
	checkItrResults(t, "ExcludeEndKey", itr1, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
	})

	// Test db + batch1 updates (include endKey)
	itr1WithEndKey, _ := newCombinedIterator(db, batch1, "ns", "key2", "key8", true)
	defer itr1WithEndKey.Close()
	checkItrResults(t, "IncludeEndKey", itr1WithEndKey, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key8", []byte("value8"), version.NewHeight(1, 1)),
	})

	// Test db + batch1 updates (include endKey) for extra range
	itr1WithEndKeyExtraRange, _ := newCombinedIterator(db, batch1, "ns", "key0", "key9", true)
	defer itr1WithEndKeyExtraRange.Close()
	checkItrResults(t, "IncludeEndKey_ExtraRange", itr1WithEndKeyExtraRange, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key1", []byte("value1"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key8", []byte("value8"), version.NewHeight(1, 1)),
	})

	// Test db + batch1 updates with full range query
	itr3, _ := newCombinedIterator(db, batch1, "ns", "", "", false)
	defer itr3.Close()
	checkItrResults(t, "ExcludeEndKey_FullRange", itr3, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key1", []byte("value1"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key3", []byte("value3"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6_new"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key7", []byte("value7"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key8", []byte("value8"), version.NewHeight(1, 1)),
	})

	// Test db + batch2 updates
	itr2, _ := newCombinedIterator(db, batch2, "ns", "key2", "key8", false)
	defer itr2.Close()
	checkItrResults(t, "ExcludeEndKey_EmptyUpdates", itr2, []*statedb.VersionedKV{
		constructVersionedKV("ns", "key4", []byte("value4"), version.NewHeight(1, 1)),
		constructVersionedKV("ns", "key6", []byte("value6"), version.NewHeight(1, 1)),
	})
}

func checkItrResults(t *testing.T, testName string, itr statedb.ResultsIterator, expectedResults []*statedb.VersionedKV) {
	t.Run(testName, func(t *testing.T) {
		for i := 0; i < len(expectedResults); i++ {
			res, _ := itr.Next()
			require.Equal(t, expectedResults[i], res)
		}
		lastRes, err := itr.Next()
		require.NoError(t, err)
		require.Nil(t, lastRes)
	})
}

func constructVersionedKV(ns string, key string, value []byte, version *version.Height) *statedb.VersionedKV {
	return &statedb.VersionedKV{
		CompositeKey: &statedb.CompositeKey{
			Namespace: ns,
			Key:       key,
		},
		VersionedValue: &statedb.VersionedValue{
			Value:   value,
			Version: version,
		},
	}
}
