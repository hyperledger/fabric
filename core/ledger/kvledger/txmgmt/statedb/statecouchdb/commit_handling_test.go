/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"strconv"
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

func TestBuildCommittersForNs(t *testing.T) {
	env := testEnv
	env.init(t, &statedb.Cache{})
	defer env.cleanup()

	versionedDB, err := testEnv.DBProvider.GetDBHandle("test-build-committers-for-ns")
	assert.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	nsUpdates := map[string]*statedb.VersionedValue{
		"bad-key": {},
	}

	_, err = db.buildCommittersForNs("ns", nsUpdates)
	assert.EqualError(t, err, "nil version not supported")

	nsUpdates = make(map[string]*statedb.VersionedValue)
	// populate updates with maxBatchSize + 1.
	dummyHeight := version.NewHeight(1, 1)
	for i := 0; i <= env.config.MaxBatchUpdateSize; i++ {
		nsUpdates[strconv.Itoa(i)] = &statedb.VersionedValue{
			Value:    nil,
			Metadata: nil,
			Version:  dummyHeight,
		}
	}

	committers, err := db.buildCommittersForNs("ns", nsUpdates)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(committers))
	assert.Equal(t, "ns", committers[0].namespace)
	assert.Equal(t, "ns", committers[1].namespace)

}

func TestBuildCommitters(t *testing.T) {
	env := testEnv
	env.init(t, &statedb.Cache{})
	defer env.cleanup()

	versionedDB, err := testEnv.DBProvider.GetDBHandle("test-build-committers")
	assert.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	dummyHeight := version.NewHeight(1, 1)
	batch := statedb.NewUpdateBatch()
	batch.Put("ns-1", "key1", []byte("value1"), dummyHeight)
	batch.Put("ns-2", "key1", []byte("value2"), dummyHeight)
	for i := 0; i <= env.config.MaxBatchUpdateSize; i++ {
		batch.Put("maxBatch", "key1", []byte("value3"), dummyHeight)
	}
	namespaceSet := map[string]bool{
		"ns-1": true, "ns-2": true, "maxBatch": true,
	}

	committer, err := db.buildCommitters(batch)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(committer))
	for _, commit := range committer {
		assert.True(t, namespaceSet[commit.namespace])
	}

	badBatch := statedb.NewUpdateBatch()
	badBatch.Put("bad-ns", "bad-key", []byte("bad-value"), nil)

	committer, err = db.buildCommitters(badBatch)
	assert.EqualError(t, err, "nil version not supported")
}
