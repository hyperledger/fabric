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

package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/db"
	"github.com/spf13/viper"
	"github.com/tecbot/gorocksdb"
)

func TestMain(m *testing.M) {
	setupTestConfig()
	os.Exit(m.Run())
}

var numOversizedKeyValues int

func testDetailPrinter(data []byte) {
	numOversizedKeyValues++
}

func TestDBStatsOversizedKV(t *testing.T) {
	dbTestWrapper := db.NewTestDBWrapper()
	dbTestWrapper.CleanDB(t)
	defer dbTestWrapper.CloseDB(t)
	defer deleteTestDBDir()

	openchainDB := db.GetDBHandle()
	writeBatch := gorocksdb.NewWriteBatch()
	writeBatch.PutCF(openchainDB.BlockchainCF, []byte("key1"), []byte("value1"))
	writeBatch.PutCF(openchainDB.BlockchainCF, []byte("key2"), generateOversizedValue(0))
	writeBatch.PutCF(openchainDB.BlockchainCF, []byte("key3"), generateOversizedValue(100))
	writeBatch.PutCF(openchainDB.BlockchainCF, []byte("key4"), []byte("value4"))
	dbTestWrapper.WriteToDB(t, writeBatch)

	totalKVs, numOverSizedKVs := scan(openchainDB, "blockchainCF", openchainDB.BlockchainCF, testDetailPrinter)

	if totalKVs != 4 {
		t.Fatalf("totalKVs is not correct. Expected [%d], found [%d]", 4, totalKVs)
	}

	if numOverSizedKVs != 2 {
		t.Fatalf("numOverSizedKVs is not correct. Expected [%d], found [%d]", 2, numOverSizedKVs)
	}

	if numOversizedKeyValues != 2 {
		t.Fatalf("numOversizedKeyValues is not correct. Expected [%d], found [%d]", 2, numOversizedKeyValues)
	}
}

func setupTestConfig() {
	tempDir, err := ioutil.TempDir("", "db-stats-test")
	if err != nil {
		panic(err)
	}
	viper.Set("peer.fileSystemPath", tempDir)
}

func deleteTestDBDir() {
	path := viper.GetString("peer.fileSystemPath")
	os.RemoveAll(path)
}

func generateOversizedValue(extraSize int) []byte {
	return make([]byte, MaxValueSize+extraSize)
}
