/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package leveldbhelper

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/ledger/dataformat"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("leveldbhelper=debug")
	os.Exit(m.Run())
}

func TestDBBasicWriteAndReads(t *testing.T) {
	testDBBasicWriteAndReads(t, "db1", "db2", "")
}

func TestIterator(t *testing.T) {
	env := newTestProviderEnv(t, testDBPath)
	defer env.cleanup()
	p := env.provider

	db1 := p.GetDBHandle("db1")
	db2 := p.GetDBHandle("db2")
	db3 := p.GetDBHandle("db3")
	for i := 0; i < 20; i++ {
		db1.Put([]byte(createTestKey(i)), []byte(createTestValue("db1", i)), false)
		db2.Put([]byte(createTestKey(i)), []byte(createTestValue("db2", i)), false)
		db3.Put([]byte(createTestKey(i)), []byte(createTestValue("db3", i)), false)
	}

	itr1 := db2.GetIterator([]byte(createTestKey(2)), []byte(createTestKey(4)))
	defer itr1.Release()
	checkItrResults(t, itr1, createTestKeys(2, 3), createTestValues("db2", 2, 3))

	itr2 := db2.GetIterator([]byte(createTestKey(2)), nil)
	defer itr2.Release()
	checkItrResults(t, itr2, createTestKeys(2, 19), createTestValues("db2", 2, 19))

	itr3 := db2.GetIterator(nil, nil)
	defer itr3.Release()
	checkItrResults(t, itr3, createTestKeys(0, 19), createTestValues("db2", 0, 19))
}

func TestBatchedUpdates(t *testing.T) {
	env := newTestProviderEnv(t, testDBPath)
	defer env.cleanup()
	p := env.provider

	db1 := p.GetDBHandle("db1")
	db2 := p.GetDBHandle("db2")

	dbs := []*DBHandle{db1, db2}
	for _, db := range dbs {
		batch := NewUpdateBatch()
		batch.Put([]byte("key1"), []byte("value1"))
		batch.Put([]byte("key2"), []byte("value2"))
		batch.Put([]byte("key3"), []byte("value3"))
		db.WriteBatch(batch, true)
	}

	for _, db := range dbs {
		batch := NewUpdateBatch()
		batch.Delete([]byte("key2"))
		db.WriteBatch(batch, true)
	}

	for _, db := range dbs {
		val1, _ := db.Get([]byte("key1"))
		assert.Equal(t, "value1", string(val1))

		val2, err2 := db.Get([]byte("key2"))
		assert.NoError(t, err2, "")
		assert.Nil(t, val2)

		val3, _ := db.Get([]byte("key3"))
		assert.Equal(t, "value3", string(val3))
	}
}

func TestFormatCheck(t *testing.T) {
	testCases := []struct {
		dataFormat     string
		dataExists     bool
		expectedFormat string
		expectedErr    *dataformat.ErrVersionMismatch
	}{
		{
			dataFormat:     "",
			dataExists:     true,
			expectedFormat: "",
			expectedErr:    nil,
		},
		{
			dataFormat:     "",
			dataExists:     false,
			expectedFormat: "",
			expectedErr:    nil,
		},
		{
			dataFormat:     "",
			dataExists:     false,
			expectedFormat: "2.0",
			expectedErr:    nil,
		},
		{
			dataFormat:     "",
			dataExists:     true,
			expectedFormat: "2.0",
			expectedErr:    &dataformat.ErrVersionMismatch{Version: "", ExpectedVersion: "2.0"},
		},
		{
			dataFormat:     "2.0",
			dataExists:     true,
			expectedFormat: "2.0",
			expectedErr:    nil,
		},
		{
			dataFormat:     "2.0",
			dataExists:     true,
			expectedFormat: "3.0",
			expectedErr:    &dataformat.ErrVersionMismatch{Version: "2.0", ExpectedVersion: "3.0"},
		},
	}

	for i, testCase := range testCases {
		t.Run(
			fmt.Sprintf("testCase %d", i),
			func(t *testing.T) {
				testFormatCheck(t, testCase.dataFormat, testCase.expectedFormat, testCase.dataExists, testCase.expectedErr)
			})
	}
}

func testFormatCheck(t *testing.T, dataFormat, expectedFormat string, dataExists bool, expectedErr *dataformat.ErrVersionMismatch) {
	assert.NoError(t, os.RemoveAll(testDBPath))
	defer func() {
		assert.NoError(t, os.RemoveAll(testDBPath))
	}()

	// setup test pre-conditions (create a db with dbformat)
	p, err := NewProvider(&Conf{DBPath: testDBPath, ExpectedFormatVersion: dataFormat})
	assert.NoError(t, err)
	f, err := p.GetDataFormat()
	assert.NoError(t, err)
	assert.Equal(t, dataFormat, f)
	if dataExists {
		assert.NoError(t, p.GetDBHandle("testdb").Put([]byte("key"), []byte("value"), true))
	}

	// close and reopen with new conf
	p.Close()
	p, err = NewProvider(&Conf{DBPath: testDBPath, ExpectedFormatVersion: expectedFormat})
	if expectedErr != nil {
		expectedErr.DBInfo = fmt.Sprintf("leveldb at [%s]", testDBPath)
		assert.Equal(t, err, expectedErr)
		return
	}
	assert.NoError(t, err)
	f, err = p.GetDataFormat()
	assert.NoError(t, err)
	assert.Equal(t, expectedFormat, f)
}

func testDBBasicWriteAndReads(t *testing.T, dbNames ...string) {
	env := newTestProviderEnv(t, testDBPath)
	defer env.cleanup()
	p := env.provider

	for _, dbName := range dbNames {
		db := p.GetDBHandle(dbName)
		db.Put([]byte("key1"), []byte("value1_"+dbName), false)
		db.Put([]byte("key2"), []byte("value2_"+dbName), false)
		db.Put([]byte("key3"), []byte("value3_"+dbName), false)
	}

	for _, dbName := range dbNames {
		db := p.GetDBHandle(dbName)
		val, err := db.Get([]byte("key1"))
		assert.NoError(t, err, "")
		assert.Equal(t, []byte("value1_"+dbName), val)

		val, err = db.Get([]byte("key2"))
		assert.NoError(t, err, "")
		assert.Equal(t, []byte("value2_"+dbName), val)

		val, err = db.Get([]byte("key3"))
		assert.NoError(t, err, "")
		assert.Equal(t, []byte("value3_"+dbName), val)
	}

	for _, dbName := range dbNames {
		db := p.GetDBHandle(dbName)
		assert.NoError(t, db.Delete([]byte("key1"), false), "")
		val, err := db.Get([]byte("key1"))
		assert.NoError(t, err, "")
		assert.Nil(t, val)

		assert.NoError(t, db.Delete([]byte("key2"), false), "")
		val, err = db.Get([]byte("key2"))
		assert.NoError(t, err, "")
		assert.Nil(t, val)

		assert.NoError(t, db.Delete([]byte("key3"), false), "")
		val, err = db.Get([]byte("key3"))
		assert.NoError(t, err, "")
		assert.Nil(t, val)
	}
}

func checkItrResults(t *testing.T, itr *Iterator, expectedKeys []string, expectedValues []string) {
	defer itr.Release()
	var actualKeys []string
	var actualValues []string
	for itr.Next(); itr.Valid(); itr.Next() {
		actualKeys = append(actualKeys, string(itr.Key()))
		actualValues = append(actualValues, string(itr.Value()))
	}
	assert.Equal(t, expectedKeys, actualKeys)
	assert.Equal(t, expectedValues, actualValues)
	assert.Equal(t, false, itr.Next())
}

func createTestKey(i int) string {
	return fmt.Sprintf("key_%06d", i)
}

func createTestValue(dbname string, i int) string {
	return fmt.Sprintf("value_%s_%06d", dbname, i)
}

func createTestKeys(start int, end int) []string {
	var keys []string
	for i := start; i <= end; i++ {
		keys = append(keys, createTestKey(i))
	}
	return keys
}

func createTestValues(dbname string, start int, end int) []string {
	var values []string
	for i := start; i <= end; i++ {
		values = append(values, createTestValue(dbname, i))
	}
	return values
}
