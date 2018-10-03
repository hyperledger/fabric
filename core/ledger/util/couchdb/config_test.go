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
	expectedAddress := viper.GetString("ledger.state.couchDBConfig.couchDBAddress")

	couchDBDef := GetCouchDBDefinition()
	assert.Equal(t, expectedAddress, couchDBDef.URL)
	assert.Equal(t, "", couchDBDef.Username)
	assert.Equal(t, "", couchDBDef.Password)
	assert.Equal(t, 3, couchDBDef.MaxRetries)
	assert.Equal(t, 20, couchDBDef.MaxRetriesOnStartup)
	assert.Equal(t, time.Second*35, couchDBDef.RequestTimeout)
}
