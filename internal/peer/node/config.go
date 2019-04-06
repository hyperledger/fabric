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
	return &ledger.Config{
		StateDB: &ledger.StateDB{
			StateDatabase: viper.GetString("ledger.state.stateDatabase"),
			CouchDB: &couchdb.Config{
				Address:               viper.GetString("ledger.state.couchDBConfig.couchDBAddress"),
				Username:              viper.GetString("ledger.state.couchDBConfig.username"),
				Password:              viper.GetString("ledger.state.couchDBConfig.password"),
				MaxRetries:            viper.GetInt("ledger.state.couchDBConfig.maxRetries"),
				MaxRetriesOnStartup:   viper.GetInt("ledger.state.couchDBConfig.maxRetriesOnStartup"),
				RequestTimeout:        viper.GetDuration("ledger.state.couchDBConfig.requestTimeout"),
				CreateGlobalChangesDB: viper.GetBool("ledger.state.couchDBConfig.createGlobalChangesDB"),
			},
		},
	}
}
