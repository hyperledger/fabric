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

package db

import (
	"os"
	"testing"

	"fmt"

	"github.com/hyperledger/fabric/core/ledger/testutil"
)

const testDBPath = "/tmp/test/hyperledger/fabric/core/ledger/util/db"

func TestDBBasicWriteAndReads(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()
	db.Put([]byte("key1"), []byte("value1"), false)
	db.Put([]byte("key2"), []byte("value2"), false)
	db.Put([]byte("key3"), []byte("value3"), false)
	val, err := db.Get([]byte("key1"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value1"))

	val, err = db.Get([]byte("key2"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value2"))

	val, err = db.Get([]byte("key3"))
	testutil.AssertNoError(t, err, "")
	testutil.AssertEquals(t, val, []byte("value3"))
}

func TestIterator(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()
	for i := 0; i < 10; i++ {
		db.Put(createTestKey(i), createTestValue(i), false)
	}

	startKey := 2
	endKey := 7
	itr := db.GetIterator(createTestKey(startKey), createTestKey(endKey))
	defer itr.Release()
	var count = 0
	itr.Next()
	for i := startKey; itr.Valid(); itr.Next() {
		k := itr.Key()
		v := itr.Value()
		t.Logf("Key=%s, value=%s", string(k), string(v))
		testutil.AssertEquals(t, k, createTestKey(i))
		testutil.AssertEquals(t, v, createTestValue(i))
		i++
		count++
	}
	testutil.AssertEquals(t, count, endKey-startKey)
}

func createTestKey(i int) []byte {
	return []byte(fmt.Sprintf("key_%d", i))
}

func createTestValue(i int) []byte {
	return []byte(fmt.Sprintf("value_%d", i))
}

func createTestDB(t *testing.T) *DB {
	if err := os.RemoveAll(testDBPath); err != nil {
		t.Fatalf("Error:%s", err)
	}
	dbConf := &Conf{testDBPath}
	db := CreateDB(dbConf)
	db.Open()
	return db
}
