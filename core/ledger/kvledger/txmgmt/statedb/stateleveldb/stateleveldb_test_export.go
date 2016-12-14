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
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
)

// TestVDBEnv provides a level db backed versioned db for testing
type TestVDBEnv struct {
	DBPath string
	DB     statedb.VersionedDB
}

// NewTestVDBEnv instantiates and new level db backed TestVDB
func NewTestVDBEnv(t testing.TB, dbPath string) *TestVDBEnv {
	os.RemoveAll(dbPath)
	db := NewVersionedDBProvider(&Conf{DBPath: dbPath}).GetDBHandle("testDB")
	db.Open()
	return &TestVDBEnv{dbPath, db}
}

// Cleanup closes the db and removes the db folder
func (env *TestVDBEnv) Cleanup() {
	env.DB.Close()
	os.RemoveAll(env.DBPath)
}
