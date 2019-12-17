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

package stateleveldb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

// TestVDBEnv provides a level db backed versioned db for testing
type TestVDBEnv struct {
	t          testing.TB
	DBProvider statedb.VersionedDBProvider
	dbPath     string
}

// NewTestVDBEnv instantiates and new level db backed TestVDB
func NewTestVDBEnv(t testing.TB) *TestVDBEnv {
	t.Logf("Creating new TestVDBEnv")
	dbPath, err := ioutil.TempDir("", "statelvldb")
	if err != nil {
		t.Fatalf("Failed to create leveldb directory: %s", err)
	}
	dbProvider, err := NewVersionedDBProvider(dbPath)
	assert.NoError(t, err)
	return &TestVDBEnv{t, dbProvider, dbPath}
}

// Cleanup closes the db and removes the db folder
func (env *TestVDBEnv) Cleanup() {
	env.t.Logf("Cleaningup TestVDBEnv")
	env.DBProvider.Close()
	os.RemoveAll(env.dbPath)
}
