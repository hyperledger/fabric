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

	"github.com/stretchr/testify/assert"
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
		assert.Equal(t, k, k1)
	}
}

func TestCompareEncodedHeight(t *testing.T) {
	assert.Equal(t, bytes.Compare(encodeBlockNum(20), encodeBlockNum(40)), 1)
	assert.Equal(t, bytes.Compare(encodeBlockNum(40), encodeBlockNum(10)), -1)
}

func TestQueries(t *testing.T) {
	testDBPath := "/tmp/fabric/core/ledger/confighistory"
	deleteTestPath(t, testDBPath)
	provider := newDBProvider(testDBPath)
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
	}
	populateDBWithSampleData(t, db, sampleData)
	// access most recent entry below ht=[45] - expected item is the one committed at ht = 40
	checkRecentEntryBelow(t, "testcase-query2", db, "ns1", "key1", 45, sampleData[0])
	checkRecentEntryBelow(t, "testcase-query3", db, "ns1", "key1", 35, sampleData[1])
	checkRecentEntryBelow(t, "testcase-query4", db, "ns1", "key1", 30, sampleData[2])
	checkRecentEntryBelow(t, "testcase-query5", db, "ns1", "key1", 10, sampleData[4])

	checkEntryAt(t, "testcase-query6", db, "ns1", "key1", 40, sampleData[0])
	checkEntryAt(t, "testcase-query7", db, "ns1", "key1", 30, sampleData[1])
	checkEntryAt(t, "testcase-query8", db, "ns1", "key1", 0, sampleData[4])
	checkEntryAt(t, "testcase-query9", db, "ns1", "key1", 35, nil)
	checkEntryAt(t, "testcase-query10", db, "ns1", "key1", 45, nil)
}

func populateDBWithSampleData(t *testing.T, db *db, sampledata []*compositeKV) {
	batch := newBatch()
	for _, data := range sampledata {
		batch.add(data.ns, data.key, data.blockNum, data.value)
	}
	assert.NoError(t, db.writeBatch(batch, true))
}

func checkRecentEntryBelow(t *testing.T, testcase string, db *db, ns, key string, commitHt uint64, expectedOutput *compositeKV) {
	t.Run(testcase,
		func(t *testing.T) {
			kv, err := db.mostRecentEntryBelow(commitHt, ns, key)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, kv)
		})
}

func checkEntryAt(t *testing.T, testcase string, db *db, ns, key string, commitHt uint64, expectedOutput *compositeKV) {
	t.Run(testcase,
		func(t *testing.T) {
			kv, err := db.entryAt(commitHt, ns, key)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutput, kv)
		})
}

func deleteTestPath(t *testing.T, dbPath string) {
	err := os.RemoveAll(dbPath)
	assert.NoError(t, err)
}
