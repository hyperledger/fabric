/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"math"
	"path/filepath"
	"strconv"
	"time"

	coreconfig "github.com/hyperledger/fabric/core/config"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/spf13/viper"
)

func ledgerConfig() *ledger.Config {
	// set defaults
	internalQueryLimit := 1000
	if viper.IsSet("ledger.state.couchDBConfig.internalQueryLimit") {
		internalQueryLimit = viper.GetInt("ledger.state.couchDBConfig.internalQueryLimit")
	}
	maxBatchUpdateSize := 500
	if viper.IsSet("ledger.state.couchDBConfig.maxBatchUpdateSize") {
		maxBatchUpdateSize = viper.GetInt("ledger.state.couchDBConfig.maxBatchUpdateSize")
	}
	collElgProcMaxDbBatchSize := 5000
	if viper.IsSet("ledger.pvtdataStore.collElgProcMaxDbBatchSize") {
		collElgProcMaxDbBatchSize = viper.GetInt("ledger.pvtdataStore.collElgProcMaxDbBatchSize")
	}
	collElgProcDbBatchesInterval := 1000
	if viper.IsSet("ledger.pvtdataStore.collElgProcDbBatchesInterval") {
		collElgProcDbBatchesInterval = viper.GetInt("ledger.pvtdataStore.collElgProcDbBatchesInterval")
	}
	purgeInterval := 100
	if viper.IsSet("ledger.pvtdataStore.purgeInterval") {
		purgeInterval = viper.GetInt("ledger.pvtdataStore.purgeInterval")
	}
	deprioritizedDataReconcilerInterval := 60 * time.Minute
	if viper.IsSet("ledger.pvtdataStore.deprioritizedDataReconcilerInterval") {
		deprioritizedDataReconcilerInterval = viper.GetDuration("ledger.pvtdataStore.deprioritizedDataReconcilerInterval")
	}
	purgedKeyAuditLogging := true
	if viper.IsSet("ledger.pvtdataStore.purgedKeyAuditLogging") {
		purgedKeyAuditLogging = viper.GetBool("ledger.pvtdataStore.purgedKeyAuditLogging")
	}
	// The default equals the default peer.maxSendMsgSize (100MB), the largest
	// payload a peer emits; it is a memory-safety ceiling per simulation, not
	// a deliverability guarantee - the proposal response and the ordering
	// BatchSize limits bound deliverable transactions more tightly.
	maxRWSetSize := int64(100 * 1024 * 1024)
	if viper.IsSet("ledger.state.maxRWSetSize") {
		maxRWSetSize = parseMaxRWSetSize(viper.Get("ledger.state.maxRWSetSize"), maxRWSetSize)
	}
	if maxRWSetSize > 0 {
		logger.Infof("Transaction simulation read-write set size limit = %d bytes", maxRWSetSize)
	} else {
		logger.Info("Transaction simulation read-write set size limit is disabled")
	}

	fsPath := coreconfig.GetPath("peer.fileSystemPath")
	ledgersDataRootDir := filepath.Join(fsPath, "ledgersData")
	snapshotsRootDir := viper.GetString("ledger.snapshots.rootDir")
	if snapshotsRootDir == "" {
		snapshotsRootDir = filepath.Join(fsPath, "snapshots")
	}
	// The type of the internal KV stores is not configurable: leveldb is the
	// only store available, so the type is decided here while the
	// configuration is parsed, and not read from it.
	commonDBType := ledger.GoLevelDB

	// The state database is a separate choice, and it is resolved here so that
	// an unset choice does not travel to the ledger as an empty type.
	stateDBType := viper.GetString("ledger.state.stateDatabase")
	if stateDBType == "" {
		stateDBType = ledger.GoLevelDB
	}

	conf := &ledger.Config{
		RootFSPath:    ledgersDataRootDir,
		StateDatabase: commonDBType,
		StateDBConfig: &ledger.StateDBConfig{
			StateDatabase: stateDBType,
			CouchDB:       &ledger.CouchDBConfig{},
		},
		PrivateDataConfig: &ledger.PrivateDataConfig{
			MaxBatchSize:                        collElgProcMaxDbBatchSize,
			BatchesInterval:                     collElgProcDbBatchesInterval,
			PurgeInterval:                       purgeInterval,
			DeprioritizedDataReconcilerInterval: deprioritizedDataReconcilerInterval,
			PurgedKeyAuditLogging:               purgedKeyAuditLogging,
		},
		HistoryDBConfig: &ledger.HistoryDBConfig{
			Enabled: viper.GetBool("ledger.history.enableHistoryDatabase"),
		},
		SnapshotsConfig: &ledger.SnapshotsConfig{
			RootDir: snapshotsRootDir,
		},
		MaxRWSetSize: maxRWSetSize,
	}

	if conf.StateDBConfig.StateDatabase == ledger.CouchDB {
		conf.StateDBConfig.CouchDB = &ledger.CouchDBConfig{
			Address:               viper.GetString("ledger.state.couchDBConfig.couchDBAddress"),
			Username:              viper.GetString("ledger.state.couchDBConfig.username"),
			Password:              viper.GetString("ledger.state.couchDBConfig.password"),
			MaxRetries:            viper.GetInt("ledger.state.couchDBConfig.maxRetries"),
			MaxRetriesOnStartup:   viper.GetInt("ledger.state.couchDBConfig.maxRetriesOnStartup"),
			RequestTimeout:        viper.GetDuration("ledger.state.couchDBConfig.requestTimeout"),
			InternalQueryLimit:    internalQueryLimit,
			MaxBatchUpdateSize:    maxBatchUpdateSize,
			CreateGlobalChangesDB: viper.GetBool("ledger.state.couchDBConfig.createGlobalChangesDB"),
			RedoLogPath:           filepath.Join(ledgersDataRootDir, "couchdbRedoLogs"),
			RedoLogDBType:         commonDBType,
			UserCacheSizeMBs:      viper.GetInt("ledger.state.couchDBConfig.cacheSize"),
		}
	}
	return conf
}

// parseMaxRWSetSize converts the configured value to bytes; a non-numeric
// value in a security-relevant setting must not silently disable the limit,
// so it falls back to the default with a warning
func parseMaxRWSetSize(value any, defaultValue int64) int64 {
	var parsed int64
	switch v := value.(type) {
	case int:
		parsed = int64(v)
	case int64:
		parsed = v
	case float64:
		// non-finite or fractional values would convert to an arbitrary or
		// zero limit and could silently disable it
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v >= float64(math.MaxInt64) {
			logger.Warningf("Invalid ledger.state.maxRWSetSize value %v, using default %d bytes", value, defaultValue)
			return defaultValue
		}
		parsed = int64(v)
	case uint64:
		if v > uint64(math.MaxInt64) {
			logger.Warningf("Invalid ledger.state.maxRWSetSize value %v, using default %d bytes", value, defaultValue)
			return defaultValue
		}
		parsed = int64(v)
	case string:
		var err error
		if parsed, err = strconv.ParseInt(v, 10, 64); err != nil {
			logger.Warningf("Invalid ledger.state.maxRWSetSize value %v, using default %d bytes", value, defaultValue)
			return defaultValue
		}
	default:
		logger.Warningf("Invalid ledger.state.maxRWSetSize value %v, using default %d bytes", value, defaultValue)
		return defaultValue
	}
	if parsed < 0 {
		// 0 is the documented way to disable; a negative value is likely a typo
		logger.Warningf("Negative ledger.state.maxRWSetSize value %d disables the limit; set 0 to disable explicitly", parsed)
	}
	return parsed
}
