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

	"github.com/spf13/viper"
	"github.com/tecbot/gorocksdb"
)

// TestDBWrapper wraps the db. Can be used by other modules for testing
type TestDBWrapper struct {
	performCleanup bool
}

// NewTestDBWrapper constructs a new TestDBWrapper
func NewTestDBWrapper() *TestDBWrapper {
	return &TestDBWrapper{}
}

///////////////////////////
// Test db creation and cleanup functions

// CleanDB This method closes existing db, remove the db dir.
// Can be called before starting a test so that data from other tests does not interfere
func (testDB *TestDBWrapper) CleanDB(t testing.TB) {
	// cleaning up test db here so that each test does not have to call it explicitly
	// at the end of the test
	testDB.cleanup()
	testDB.removeDBPath()
	t.Logf("Creating testDB")

	Start()
	testDB.performCleanup = true
}

// CreateFreshDBGinkgo creates a fresh database for ginkgo testing
func (testDB *TestDBWrapper) CreateFreshDBGinkgo() {
	// cleaning up test db here so that each test does not have to call it explicitly
	// at the end of the test
	testDB.cleanup()
	testDB.removeDBPath()
	Start()
	testDB.performCleanup = true
}

func (testDB *TestDBWrapper) cleanup() {
	if testDB.performCleanup {
		Stop()
		testDB.performCleanup = false
	}
}

func (testDB *TestDBWrapper) removeDBPath() {
	dbPath := viper.GetString("peer.fileSystemPath")
	os.RemoveAll(dbPath)
}

// WriteToDB tests can use this method for persisting a given batch to db
func (testDB *TestDBWrapper) WriteToDB(t testing.TB, writeBatch *gorocksdb.WriteBatch) {
	opt := gorocksdb.NewDefaultWriteOptions()
	defer opt.Destroy()
	err := GetDBHandle().DB.Write(opt, writeBatch)
	if err != nil {
		t.Fatalf("Error while writing to db. Error:%s", err)
	}
}

// GetFromDB gets the value for the given key from default column-family
func (testDB *TestDBWrapper) GetFromDB(t testing.TB, key []byte) []byte {
	db := GetDBHandle().DB
	opt := gorocksdb.NewDefaultReadOptions()
	defer opt.Destroy()
	slice, err := db.Get(opt, key)
	defer slice.Free()
	if err != nil {
		t.Fatalf("Error while getting key-value from DB: %s", err)
	}
	value := append([]byte(nil), slice.Data()...)
	return value
}

// GetFromStateCF tests can use this method for getting value from StateCF column-family
func (testDB *TestDBWrapper) GetFromStateCF(t testing.TB, key []byte) []byte {
	openchainDB := GetDBHandle()
	value, err := openchainDB.GetFromStateCF(key)
	if err != nil {
		t.Fatalf("Error while getting from db. Error:%s", err)
	}
	return value
}

// GetFromStateDeltaCF tests can use this method for getting value from StateDeltaCF column-family
func (testDB *TestDBWrapper) GetFromStateDeltaCF(t testing.TB, key []byte) []byte {
	openchainDB := GetDBHandle()
	value, err := openchainDB.GetFromStateDeltaCF(key)
	if err != nil {
		t.Fatalf("Error while getting from db. Error:%s", err)
	}
	return value
}

// CloseDB closes the db
func (testDB *TestDBWrapper) CloseDB(t testing.TB) {
	Stop()
}

// OpenDB opens the db
func (testDB *TestDBWrapper) OpenDB(t testing.TB) {
	Start()
}

// GetEstimatedNumKeys returns estimated number of key-values in db. This is not accurate in all the cases
func (testDB *TestDBWrapper) GetEstimatedNumKeys(t testing.TB) map[string]string {
	openchainDB := GetDBHandle()
	result := make(map[string]string, 5)
	result["stateCF"] = openchainDB.DB.GetPropertyCF("rocksdb.estimate-num-keys", openchainDB.StateCF)
	result["stateDeltaCF"] = openchainDB.DB.GetPropertyCF("rocksdb.estimate-num-keys", openchainDB.StateDeltaCF)
	result["blockchainCF"] = openchainDB.DB.GetPropertyCF("rocksdb.estimate-num-keys", openchainDB.BlockchainCF)
	result["indexCF"] = openchainDB.DB.GetPropertyCF("rocksdb.estimate-num-keys", openchainDB.IndexesCF)
	return result
}

// GetDBStats returns statistics for the database
func (testDB *TestDBWrapper) GetDBStats() string {
	openchainDB := GetDBHandle()
	return openchainDB.DB.GetProperty("rocksdb.stats")
}
