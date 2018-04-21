/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package couchdb

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/spf13/viper"
)

func TestGetCouchDBDefinition(t *testing.T) {
	expectedAddress := viper.GetString("ledger.state.couchDBConfig.couchDBAddress")

	couchDBDef := GetCouchDBDefinition()
	testutil.AssertEquals(t, couchDBDef.URL, expectedAddress)
	testutil.AssertEquals(t, couchDBDef.Username, "")
	testutil.AssertEquals(t, couchDBDef.Password, "")
	testutil.AssertEquals(t, couchDBDef.MaxRetries, 3)
	testutil.AssertEquals(t, couchDBDef.MaxRetriesOnStartup, 10)
	testutil.AssertEquals(t, couchDBDef.RequestTimeout, time.Second*35)
}
