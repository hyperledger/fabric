/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistory

import (
	"bytes"
	"math"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/util/leveldbhelper"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeCompositeKey(t *testing.T) {
	sampleKeys := []*compositeKey{
		{ns: "ns0", key: "key0", blockNum: 0},
		{ns: "ns1", key: "key1", blockNum: 1},
		{ns: "ns2", key: "key2", blockNum: 99},
		{ns: "ns3", key: "key3", blockNum: math.MaxUint64},
	}
	for _, k := range sampleKeys {
		k1 := decodeCompositeKey(encodeCompositeKey(k.ns, k.key, k.blockNum))
		require.Equal(t, k, k1)
	}
}

func TestCompareEncodedHeight(t *testing.T) {
	require.Equal(t, bytes.Compare(encodeBlockNum(20), encodeBlockNum(40)), 1)
	require.Equal(t, bytes.Compare(encodeBlockNum(40), encodeBlockNum(10)), -1)
}

func TestQueries(t *testing.T) {
	testDBPath := "/tmp/fabric/core/ledger/confighistory"
	deleteTestPath(t, testDBPath)
	provider, err := newDBProvider(testDBPath)
	require.NoError(t, err)
	defer deleteTestPath(t, testDBPath)

	db := provider.getDB("ledger1")
	// A query on an empty store
	checkEntryAt(t, "testcase-query1", db, "ns1", "key1", 45, nil)
	// test data
	sampleData := []*compositeKV{
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 40}, []byte("val1_40")},
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 30}, []byte("val1_30")},
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 20}, []byte("val1_20")},
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 10}, []byte("val1_10")},
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 0}, []byte("val1_0")},
		{&compositeKey{ns: "ns2", key: "key2", blockNum: 200}, []byte("val200")},
		{&compositeKey{ns: "ns3", key: "key3", blockNum: 300}, []byte("val300")},
		{&compositeKey{ns: "ns3", key: "key4", blockNum: 400}, []byte("val400")},
	}
	populateDBWithSampleData(t, db, sampleData)
	// access most recent entry below ht=[45] - expected item is the one committed at ht = 40
	checkRecentEntryBelow(t, "testcase-query2", db, "ns1", "key1", 45, sampleData[0])
	checkRecentEntryBelow(t, "testcase-query3", db, "ns1", "key1", 35, sampleData[1])
	checkRecentEntryBelow(t, "testcase-query4", db, "ns1", "key1", 30, sampleData[2])
	checkRecentEntryBelow(t, "testcase-query5", db, "ns1", "key1", 10, sampleData[4])
	checkRecentEntryBelow(t, "testcase-query6", db, "ns2", "key2", 2000, sampleData[5])
	checkRecentEntryBelow(t, "testcase-query7", db, "ns2", "key2", 200, nil)
	checkRecentEntryBelow(t, "testcase-query8", db, "ns3", "key3", 299, nil)

	checkEntryAt(t, "testcase-query9", db, "ns1", "key1", 40, sampleData[0])
	checkEntryAt(t, "testcase-query10", db, "ns1", "key1", 30, sampleData[1])
	checkEntryAt(t, "testcase-query11", db, "ns1", "key1", 0, sampleData[4])
	checkEntryAt(t, "testcase-query12", db, "ns1", "key1", 35, nil)
	checkEntryAt(t, "testcase-query13", db, "ns1", "key1", 45, nil)

	t.Run("test-iter-error-path", func(t *testing.T) {
		provider.Close()
		ckv, err := db.mostRecentEntryBelow(45, "ns1", "key1")
		require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
		require.Nil(t, ckv)
	})
}

func TestGetNamespaceIterator(t *testing.T) {
	testDBPath := "/tmp/fabric/core/ledger/confighistory"
	provider, err := newDBProvider(testDBPath)
	require.NoError(t, err)
	defer deleteTestPath(t, testDBPath)

	db := provider.getDB("ledger1")
	nsItr1, err := db.getNamespaceIterator("ns1")
	require.NoError(t, err)
	defer nsItr1.Release()
	verifyNsEntries(t, nsItr1, nil)

	sampleData := []*compositeKV{
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 40}, []byte("val1_40")}, // index 0
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 30}, []byte("val1_30")}, // index 1
		{&compositeKey{ns: "ns1", key: "key1", blockNum: 20}, []byte("val1_20")}, // index 2
		{&compositeKey{ns: "ns2", key: "key1", blockNum: 50}, []byte("val1_50")}, // index 3
		{&compositeKey{ns: "ns2", key: "key1", blockNum: 20}, []byte("val1_20")}, // index 4
		{&compositeKey{ns: "ns2", key: "key1", blockNum: 10}, []byte("val1_10")}, // index 5
	}
	populateDBWithSampleData(t, db, sampleData)

	nsItr2, err := db.getNamespaceIterator("ns1")
	require.NoError(t, err)
	defer nsItr2.Release()
	verifyNsEntries(t, nsItr2, sampleData[:3])

	nsItr3, err := db.getNamespaceIterator("ns2")
	require.NoError(t, err)
	defer nsItr3.Release()
	verifyNsEntries(t, nsItr3, sampleData[3:])

	t.Run("test-iter-error-path", func(t *testing.T) {
		provider.Close()
		itr, err := db.getNamespaceIterator("ns1")
		require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
		require.Nil(t, itr)
	})
}

func verifyNsEntries(t *testing.T, nsItr *leveldbhelper.Iterator, expectedEntries []*compositeKV) {
	var retrievedEntries []*compositeKV
	for nsItr.Next() {
		require.NoError(t, nsItr.Error())
		key := decodeCompositeKey(nsItr.Key())
		val := make([]byte, len(nsItr.Value()))
		copy(val, nsItr.Value())
		retrievedEntries = append(retrievedEntries, &compositeKV{key, val})
	}
	require.Equal(t, expectedEntries, retrievedEntries)
}

func populateDBWithSampleData(t *testing.T, db *db, sampledata []*compositeKV) {
	batch := db.newBatch()
	for _, data := range sampledata {
		batch.add(data.ns, data.key, data.blockNum, data.value)
	}
	require.NoError(t, db.writeBatch(batch, true))
}

func checkRecentEntryBelow(t *testing.T, testcase string, db *db, ns, key string, commitHt uint64, expectedOutput *compositeKV) {
	t.Run(testcase,
		func(t *testing.T) {
			kv, err := db.mostRecentEntryBelow(commitHt, ns, key)
			require.NoError(t, err)
			require.Equal(t, expectedOutput, kv)
		})
}

func checkEntryAt(t *testing.T, testcase string, db *db, ns, key string, commitHt uint64, expectedOutput *compositeKV) {
	t.Run(testcase,
		func(t *testing.T) {
			kv, err := db.entryAt(commitHt, ns, key)
			require.NoError(t, err)
			require.Equal(t, expectedOutput, kv)
		})
}

func deleteTestPath(t *testing.T, dbPath string) {
	err := os.RemoveAll(dbPath)
	require.NoError(t, err)
}
