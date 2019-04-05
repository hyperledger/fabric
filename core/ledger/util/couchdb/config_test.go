/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetCouchDBDefinition(t *testing.T) {
	couchAddress := "localhost:5984"
	viper.Set("ledger.state.couchDBConfig.couchDBAddress", couchAddress)
	viper.Set("ledger.state.couchDBConfig.username", "")
	viper.Set("ledger.state.couchDBConfig.password", "")
	viper.Set("ledger.state.couchDBConfig.maxRetries", 3)
	viper.Set("ledger.state.couchDBConfig.maxRetriesOnStartup", 20)
	viper.Set("ledger.state.couchDBConfig.requestTimeout", time.Second*35)
	viper.Set("ledger.state.couchDBConfig.createGlobalChangesDB", true)

	couchDBDef := GetCouchDBDefinition()
	assert.Equal(t, couchAddress, couchDBDef.Address)
	assert.Equal(t, "", couchDBDef.Username)
	assert.Equal(t, "", couchDBDef.Password)
	assert.Equal(t, 3, couchDBDef.MaxRetries)
	assert.Equal(t, 20, couchDBDef.MaxRetriesOnStartup)
	assert.Equal(t, time.Second*35, couchDBDef.RequestTimeout)
}
