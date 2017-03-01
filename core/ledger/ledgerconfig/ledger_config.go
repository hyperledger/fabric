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

package ledgerconfig

import (
	"path/filepath"

	"github.com/spf13/viper"
)

// TODO remove all these config variables, they are never used as defaults
var stateDatabase = "goleveldb"
var couchDBAddress = "127.0.0.1:5984"
var username = ""
var password = ""
var historyDatabase = true

var maxBlockFileSize = 0

// CouchDBDef contains parameters
type CouchDBDef struct {
	URL      string
	Username string
	Password string
}

//IsCouchDBEnabled exposes the useCouchDB variable
func IsCouchDBEnabled() bool {
	stateDatabase = viper.GetString("ledger.state.stateDatabase")
	if stateDatabase == "CouchDB" {
		return true
	}
	return false
}

// GetRootPath returns the filesystem path.
// All ledger related contents are expected to be stored under this path
func GetRootPath() string {
	sysPath := viper.GetString("peer.fileSystemPath")
	return filepath.Join(sysPath, "ledgersData")
}

// GetLedgerProviderPath returns the filesystem path for stroing ledger ledgerProvider contents
func GetLedgerProviderPath() string {
	return filepath.Join(GetRootPath(), "ledgerProvider")
}

// GetStateLevelDBPath returns the filesystem path that is used to maintain the state level db
func GetStateLevelDBPath() string {
	return filepath.Join(GetRootPath(), "stateLeveldb")
}

// GetHistoryLevelDBPath returns the filesystem path that is used to maintain the history level db
func GetHistoryLevelDBPath() string {
	return filepath.Join(GetRootPath(), "historyLeveldb")
}

// GetBlockStorePath returns the filesystem path that is used for the chain block stores
func GetBlockStorePath() string {
	return filepath.Join(GetRootPath(), "chains")
}

// GetMaxBlockfileSize returns maximum size of the block file
func GetMaxBlockfileSize() int {
	return 64 * 1024 * 1024
}

//GetCouchDBDefinition exposes the useCouchDB variable
func GetCouchDBDefinition() *CouchDBDef {

	couchDBAddress = viper.GetString("ledger.state.couchDBConfig.couchDBAddress")
	username = viper.GetString("ledger.state.couchDBConfig.username")
	password = viper.GetString("ledger.state.couchDBConfig.password")

	return &CouchDBDef{couchDBAddress, username, password}
}

//GetQueryLimit exposes the queryLimit variable
func GetQueryLimit() int {
	return viper.GetInt("ledger.state.queryLimit")
}

//IsHistoryDBEnabled exposes the historyDatabase variable
func IsHistoryDBEnabled() bool {
	return viper.GetBool("ledger.state.historyDatabase")
}

// IsQueryReadsHashingEnabled enables or disables computing of hash
// of range query results for phantom item validation
func IsQueryReadsHashingEnabled() bool {
	return true
}

// GetMaxDegreeQueryReadsHashing return the maximum degree of the merkle tree for hashes of
// of range query results for phantom item validation
// For more details - see description in kvledger/txmgmt/rwset/query_results_helper.go
func GetMaxDegreeQueryReadsHashing() int {
	return 50
}
