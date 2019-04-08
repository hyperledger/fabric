/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/util/couchdb"

	"github.com/spf13/viper"
)

func ledgerConfig() *ledger.Config {
	// set defaults
	warmAfterNBlocks := 1
	if viper.IsSet("ledger.state.couchDBConfig.warmIndexesAfterNBlocks") {
		warmAfterNBlocks = viper.GetInt("ledger.state.couchDBConfig.warmIndexesAfterNBlocks")
	}
	internalQueryLimit := 1000
	if viper.IsSet("ledger.state.couchDBConfig.internalQueryLimit") {
		internalQueryLimit = viper.GetInt("ledger.state.couchDBConfig.internalQueryLimit")
	}
	maxBatchUpdateSize := 1000
	if viper.IsSet("ledger.state.couchDBConfig.maxBatchUpdateSize") {
		maxBatchUpdateSize = viper.GetInt("ledger.state.couchDBConfig.maxBatchUpdateSize")
	}

	config := &ledger.Config{
		StateDB: &ledger.StateDB{
			StateDatabase: viper.GetString("ledger.state.stateDatabase"),
			CouchDB:       &couchdb.Config{},
		},
	}

	if config.StateDB.StateDatabase == "CouchDB" {
		config.StateDB.CouchDB = &couchdb.Config{
			Address:                 viper.GetString("ledger.state.couchDBConfig.couchDBAddress"),
			Username:                viper.GetString("ledger.state.couchDBConfig.username"),
			Password:                viper.GetString("ledger.state.couchDBConfig.password"),
			MaxRetries:              viper.GetInt("ledger.state.couchDBConfig.maxRetries"),
			MaxRetriesOnStartup:     viper.GetInt("ledger.state.couchDBConfig.maxRetriesOnStartup"),
			RequestTimeout:          viper.GetDuration("ledger.state.couchDBConfig.requestTimeout"),
			InternalQueryLimit:      internalQueryLimit,
			MaxBatchUpdateSize:      maxBatchUpdateSize,
			WarmIndexesAfterNBlocks: warmAfterNBlocks,
			CreateGlobalChangesDB:   viper.GetBool("ledger.state.couchDBConfig.createGlobalChangesDB"),
		}
	}
	return config
}
