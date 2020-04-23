/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"testing"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/stretchr/testify/require"
)

// StartCouchDB starts the CouchDB if it is not running already
func StartCouchDB(t *testing.T, binds []string) (addr string, stopCouchDBFunc func()) {
	couchDB := &runner.CouchDB{Binds: binds}
	require.NoError(t, couchDB.Start())
	return couchDB.Address(), func() { couchDB.Stop() }
}

// DeleteApplicationDBs deletes all the databases other than fabric internal database
func DeleteApplicationDBs(t testing.TB, config *ledger.CouchDBConfig) {
	couchInstance, err := createCouchInstance(config, &disabled.Provider{})
	require.NoError(t, err)
	dbNames, err := couchInstance.retrieveApplicationDBNames()
	require.NoError(t, err)
	for _, dbName := range dbNames {
		if dbName != fabricInternalDBName {
			dropDB(t, couchInstance, dbName)
		}
	}
}

func dropDB(t testing.TB, couchInstance *couchInstance, dbName string) {
	db := &couchDatabase{
		couchInstance: couchInstance,
		dbName:        dbName,
	}
	response, err := db.dropDatabase()
	require.NoError(t, err)
	require.True(t, response.Ok)
}
