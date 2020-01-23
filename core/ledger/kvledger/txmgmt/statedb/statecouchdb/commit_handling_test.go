/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	"github.com/stretchr/testify/assert"
)

func TestGetRevision(t *testing.T) {
	env := testEnv
	env.init(t, &statedb.Cache{})
	defer env.cleanup()

	versionedDB, err := testEnv.DBProvider.GetDBHandle("test-get-revisions")
	assert.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	// initializing data in couchdb
	batch := statedb.NewUpdateBatch()
	batch.Put("ns", "key-in-db", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns", "key-in-both-db-cache", []byte("value2"), version.NewHeight(1, 2))
	savePoint := version.NewHeight(1, 2)
	assert.NoError(t, db.ApplyUpdates(batch, savePoint))

	// load revision cache with couchDB revision number.
	keys := []*statedb.CompositeKey{
		{
			Namespace: "ns",
			Key:       "key-in-both-db-cache",
		},
	}
	db.LoadCommittedVersions(keys)
	// change cache reivision number for test purpose. (makes sure only read from cache but not db)
	db.committedDataCache.setVerAndRev("ns", "key-in-both-db-cache", version.NewHeight(1, 2), "revision-db-number")

	// Set test revision number only in cache, but not in db.
	db.committedDataCache.setVerAndRev("ns", "key-in-cache", version.NewHeight(1, 1), "revision-cache-number")

	nsUpdates := map[string]*statedb.VersionedValue{
		"key-in-cache": {
			Value:    []byte("test-value"),
			Metadata: nil,
			Version:  version.NewHeight(1, 1),
		},
		"key-in-db": {
			Value:    []byte("value3"),
			Metadata: nil,
			Version:  version.NewHeight(1, 1),
		},
		"key-in-both-db-cache": {
			Value:    []byte("value4"),
			Metadata: nil,
			Version:  version.NewHeight(1, 2),
		},
		"bad-key": {
			Value:    []byte("bad-key-value"),
			Metadata: nil,
			Version:  version.NewHeight(1, 5),
		},
	}

	revisionsMap, err := db.getRevisions("ns", nsUpdates)
	assert.NoError(t, err)
	assert.Equal(t, "revision-cache-number", revisionsMap["key-in-cache"])
	assert.NotEqual(t, "", revisionsMap["key-in-db"])
	assert.Equal(t, "revision-db-number", revisionsMap["key-in-both-db-cache"])
	assert.Equal(t, "", revisionsMap["bad-key"])

	// Get revisions of non-existing nameSpace.
	revisionsMap, err = db.getRevisions("bad-namespace", nsUpdates)
	assert.NoError(t, err)
	assert.Equal(t, "", revisionsMap["key-in-db"])

}
