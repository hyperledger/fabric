/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgerconfig

import (
	"path/filepath"

	"github.com/hyperledger/fabric/core/config"
	"github.com/spf13/viper"
)

//IsCouchDBEnabled exposes the useCouchDB variable
func IsCouchDBEnabled() bool {
	stateDatabase := viper.GetString("ledger.state.stateDatabase")
	if stateDatabase == "CouchDB" {
		return true
	}
	return false
}

const confPeerFileSystemPath = "peer.fileSystemPath"
const confLedgersData = "ledgersData"
const confLedgerProvider = "ledgerProvider"
const confStateleveldb = "stateLeveldb"
const confHistoryLeveldb = "historyLeveldb"
const confBookkeeper = "bookkeeper"
const confConfigHistory = "configHistory"
const confChains = "chains"
const confPvtdataStore = "pvtdataStore"
const confTotalQueryLimit = "ledger.state.totalQueryLimit"
const confInternalQueryLimit = "ledger.state.couchDBConfig.internalQueryLimit"
const confEnableHistoryDatabase = "ledger.history.enableHistoryDatabase"
const confMaxBatchSize = "ledger.state.couchDBConfig.maxBatchUpdateSize"
const confAutoWarmIndexes = "ledger.state.couchDBConfig.autoWarmIndexes"
const confWarmIndexesAfterNBlocks = "ledger.state.couchDBConfig.warmIndexesAfterNBlocks"

// GetRootPath returns the filesystem path.
// All ledger related contents are expected to be stored under this path
func GetRootPath() string {
	sysPath := config.GetPath(confPeerFileSystemPath)
	return filepath.Join(sysPath, confLedgersData)
}

// GetLedgerProviderPath returns the filesystem path for storing ledger ledgerProvider contents
func GetLedgerProviderPath() string {
	return filepath.Join(GetRootPath(), confLedgerProvider)
}

// GetStateLevelDBPath returns the filesystem path that is used to maintain the state level db
func GetStateLevelDBPath() string {
	return filepath.Join(GetRootPath(), confStateleveldb)
}

// GetHistoryLevelDBPath returns the filesystem path that is used to maintain the history level db
func GetHistoryLevelDBPath() string {
	return filepath.Join(GetRootPath(), confHistoryLeveldb)
}

// GetBlockStorePath returns the filesystem path that is used for the chain block stores
func GetBlockStorePath() string {
	return filepath.Join(GetRootPath(), confChains)
}

// GetPvtdataStorePath returns the filesystem path that is used for permanent storage of private write-sets
func GetPvtdataStorePath() string {
	return filepath.Join(GetRootPath(), confPvtdataStore)
}

// GetInternalBookkeeperPath returns the filesystem path that is used for bookkeeping the internal stuff by by KVledger (such as expiration time for pvt)
func GetInternalBookkeeperPath() string {
	return filepath.Join(GetRootPath(), confBookkeeper)
}

// GetConfigHistoryPath returns the filesystem path that is used for maintaining history of chaincodes collection configurations
func GetConfigHistoryPath() string {
	return filepath.Join(GetRootPath(), confConfigHistory)
}

// GetMaxBlockfileSize returns maximum size of the block file
func GetMaxBlockfileSize() int {
	return 64 * 1024 * 1024
}

//GetTotalLimit exposes the totalLimit variable
func GetTotalQueryLimit() int {
	totalQueryLimit := viper.GetInt(confTotalQueryLimit)
	// if queryLimit was unset, default to 10000
	if !viper.IsSet(confTotalQueryLimit) {
		totalQueryLimit = 10000
	}
	return totalQueryLimit
}

//GetQueryLimit exposes the queryLimit variable
func GetInternalQueryLimit() int {
	internalQueryLimit := viper.GetInt(confInternalQueryLimit)
	// if queryLimit was unset, default to 1000
	if !viper.IsSet(confInternalQueryLimit) {
		internalQueryLimit = 1000
	}
	return internalQueryLimit
}

//GetMaxBatchUpdateSize exposes the maxBatchUpdateSize variable
func GetMaxBatchUpdateSize() int {
	maxBatchUpdateSize := viper.GetInt(confMaxBatchSize)
	// if maxBatchUpdateSize was unset, default to 500
	if !viper.IsSet(confMaxBatchSize) {
		maxBatchUpdateSize = 500
	}
	return maxBatchUpdateSize
}

// GetPvtdataStorePurgeInterval returns the interval in the terms of number of blocks
// when the purge for the expired data would be performed
func GetPvtdataStorePurgeInterval() uint64 {
	purgeInterval := viper.GetInt("ledger.pvtdataStore.purgeInterval")
	if purgeInterval <= 0 {
		purgeInterval = 100
	}
	return uint64(purgeInterval)
}

//IsHistoryDBEnabled exposes the historyDatabase variable
func IsHistoryDBEnabled() bool {
	return viper.GetBool(confEnableHistoryDatabase)
}

// IsQueryReadsHashingEnabled enables or disables computing of hash
// of range query results for phantom item validation
func IsQueryReadsHashingEnabled() bool {
	return true
}

// GetMaxDegreeQueryReadsHashing return the maximum degree of the merkle tree for hashes of
// of range query results for phantom item validation
// For more details - see description in kvledger/txmgmt/rwset/query_results_helper.go
func GetMaxDegreeQueryReadsHashing() uint32 {
	return 50
}

//IsAutoWarmIndexesEnabled exposes the autoWarmIndexes variable
func IsAutoWarmIndexesEnabled() bool {
	//Return the value set in core.yaml, if not set, the return true
	if viper.IsSet(confAutoWarmIndexes) {
		return viper.GetBool(confAutoWarmIndexes)
	}
	return true

}

//GetWarmIndexesAfterNBlocks exposes the warmIndexesAfterNBlocks variable
func GetWarmIndexesAfterNBlocks() int {
	warmAfterNBlocks := viper.GetInt(confWarmIndexesAfterNBlocks)
	// if warmIndexesAfterNBlocks was unset, default to 1
	if !viper.IsSet(confWarmIndexesAfterNBlocks) {
		warmAfterNBlocks = 1
	}
	return warmAfterNBlocks
}
