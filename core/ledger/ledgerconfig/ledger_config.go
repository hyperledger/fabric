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
const couchdbRedoLogPath = "couchdbRedoLogs"
const confTotalQueryLimit = "ledger.state.totalQueryLimit"
const confEnableHistoryDatabase = "ledger.history.enableHistoryDatabase"

var confCollElgProcMaxDbBatchSize = &conf{"ledger.pvtdataStore.collElgProcMaxDbBatchSize", 5000}
var confCollElgProcDbBatchesInterval = &conf{"ledger.pvtdataStore.collElgProcDbBatchesInterval", 1000}

// GetRootPath returns the filesystem path.
// All ledger related contents are expected to be stored under this path
func GetRootPath() string {
	sysPath := config.GetPath(confPeerFileSystemPath)
	return filepath.Join(sysPath, confLedgersData)
}

func GetCouchdbRedologsPath() string {
	return filepath.Join(GetRootPath(), couchdbRedoLogPath)
}

// GetTotalQueryLimit exposes the totalLimit variable
func GetTotalQueryLimit() int {
	totalQueryLimit := viper.GetInt(confTotalQueryLimit)
	// if queryLimit was unset, default to 10000
	if !viper.IsSet(confTotalQueryLimit) {
		totalQueryLimit = 10000
	}
	return totalQueryLimit
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

// GetPvtdataStoreCollElgProcMaxDbBatchSize returns the maximum db batch size for converting
// the ineligible missing data entries to eligible missing data entries
func GetPvtdataStoreCollElgProcMaxDbBatchSize() int {
	collElgProcMaxDbBatchSize := viper.GetInt(confCollElgProcMaxDbBatchSize.Name)
	if collElgProcMaxDbBatchSize <= 0 {
		collElgProcMaxDbBatchSize = confCollElgProcMaxDbBatchSize.DefaultVal
	}
	return collElgProcMaxDbBatchSize
}

// GetPvtdataStoreCollElgProcDbBatchesInterval returns the minimum duration (in milliseconds) between writing
// two consecutive db batches for converting the ineligible missing data entries to eligible missing data entries
func GetPvtdataStoreCollElgProcDbBatchesInterval() int {
	collElgProcDbBatchesInterval := viper.GetInt(confCollElgProcDbBatchesInterval.Name)
	if collElgProcDbBatchesInterval <= 0 {
		collElgProcDbBatchesInterval = confCollElgProcDbBatchesInterval.DefaultVal
	}
	return collElgProcDbBatchesInterval
}

//IsHistoryDBEnabled exposes the historyDatabase variable
func IsHistoryDBEnabled() bool {
	return viper.GetBool(confEnableHistoryDatabase)
}

type conf struct {
	Name       string
	DefaultVal int
}
