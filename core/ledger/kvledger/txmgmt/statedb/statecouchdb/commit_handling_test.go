/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"strconv"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/internal/version"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/stretchr/testify/require"
)

func TestGetRevision(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	versionedDB, err := vdbEnv.DBProvider.GetDBHandle("test-get-revisions", nil)
	require.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	// initializing data in couchdb
	batch := statedb.NewUpdateBatch()
	batch.Put("ns", "key-in-db", []byte("value1"), version.NewHeight(1, 1))
	batch.Put("ns", "key-in-both-db-cache", []byte("value2"), version.NewHeight(1, 2))
	savePoint := version.NewHeight(1, 2)
	require.NoError(t, db.ApplyUpdates(batch, savePoint))

	// load revision cache with couchDB revision number.
	keys := []*statedb.CompositeKey{
		{
			Namespace: "ns",
			Key:       "key-in-both-db-cache",
		},
	}
	require.NoError(t, db.LoadCommittedVersions(keys))
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
	require.NoError(t, err)
	require.Equal(t, "revision-cache-number", revisionsMap["key-in-cache"])
	require.NotEqual(t, "", revisionsMap["key-in-db"])
	require.Equal(t, "revision-db-number", revisionsMap["key-in-both-db-cache"])
	require.Equal(t, "", revisionsMap["bad-key"])

	// Get revisions of non-existing nameSpace.
	revisionsMap, err = db.getRevisions("bad-namespace", nsUpdates)
	require.NoError(t, err)
	require.Equal(t, "", revisionsMap["key-in-db"])
}

func TestBuildCommittersForNs(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	versionedDB, err := vdbEnv.DBProvider.GetDBHandle("test-build-committers-for-ns", nil)
	require.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	nsUpdates := map[string]*statedb.VersionedValue{
		"bad-key": {},
	}

	_, err = db.buildCommittersForNs("ns", nsUpdates)
	require.EqualError(t, err, "nil version not supported")

	nsUpdates = make(map[string]*statedb.VersionedValue)
	// populate updates with maxBatchSize + 1.
	dummyHeight := version.NewHeight(1, 1)
	for i := 0; i <= vdbEnv.config.MaxBatchUpdateSize; i++ {
		nsUpdates[strconv.Itoa(i)] = &statedb.VersionedValue{
			Value:    nil,
			Metadata: nil,
			Version:  dummyHeight,
		}
	}

	committers, err := db.buildCommittersForNs("ns", nsUpdates)
	require.NoError(t, err)
	require.Equal(t, 2, len(committers))
	require.Equal(t, "ns", committers[0].namespace)
	require.Equal(t, "ns", committers[1].namespace)
}

func TestBuildCommitters(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	versionedDB, err := vdbEnv.DBProvider.GetDBHandle("test-build-committers", nil)
	require.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	dummyHeight := version.NewHeight(1, 1)
	batch := statedb.NewUpdateBatch()
	batch.Put("ns-1", "key1", []byte("value1"), dummyHeight)
	batch.Put("ns-2", "key1", []byte("value2"), dummyHeight)
	for i := 0; i <= vdbEnv.config.MaxBatchUpdateSize; i++ {
		batch.Put("maxBatch", "key1", []byte("value3"), dummyHeight)
	}
	namespaceSet := map[string]bool{
		"ns-1": true, "ns-2": true, "maxBatch": true,
	}

	committer, err := db.buildCommitters(batch)
	require.NoError(t, err)
	require.Equal(t, 3, len(committer))
	for _, commit := range committer {
		require.True(t, namespaceSet[commit.namespace])
	}

	badBatch := statedb.NewUpdateBatch()
	badBatch.Put("bad-ns", "bad-key", []byte("bad-value"), nil)

	_, err = db.buildCommitters(badBatch)
	require.EqualError(t, err, "nil version not supported")
}

func TestExecuteCommitter(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	versionedDB, err := vdbEnv.DBProvider.GetDBHandle("test-execute-committer", nil)
	require.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	committerDB, err := db.getNamespaceDBHandle("ns")
	require.NoError(t, err)
	couchDocKey1, err := keyValToCouchDoc(&keyValue{
		key:            "key1",
		revision:       "",
		VersionedValue: &statedb.VersionedValue{Value: []byte("value1"), Metadata: nil, Version: version.NewHeight(1, 1)},
	})
	require.NoError(t, err)
	couchDocKey2, err := keyValToCouchDoc(&keyValue{
		key:            "key2",
		revision:       "",
		VersionedValue: &statedb.VersionedValue{Value: nil, Metadata: nil, Version: version.NewHeight(1, 1)},
	})
	require.NoError(t, err)

	committers := []*committer{
		{
			db:             committerDB,
			batchUpdateMap: map[string]*batchableDocument{"key1": {CouchDoc: *couchDocKey1}},
			namespace:      "ns",
			cacheKVs:       make(cacheKVs),
			cacheEnabled:   true,
		},
		{
			db:             committerDB,
			batchUpdateMap: map[string]*batchableDocument{"key2": {CouchDoc: *couchDocKey2}},
			namespace:      "ns",
			cacheKVs:       make(cacheKVs),
			cacheEnabled:   true,
		},
	}

	err = db.executeCommitter(committers)
	require.NoError(t, err)
	vv, err := db.GetState("ns", "key1")
	require.NoError(t, err)
	require.Equal(t, vv.Value, []byte("value1"))
	require.Equal(t, vv.Version, version.NewHeight(1, 1))

	committers = []*committer{
		{
			db:             committerDB,
			batchUpdateMap: map[string]*batchableDocument{},
			namespace:      "ns",
			cacheKVs:       make(cacheKVs),
			cacheEnabled:   true,
		},
	}
	err = db.executeCommitter(committers)
	require.EqualError(t, err, "error handling CouchDB request. Error:bad_request,  Status Code:400,  Reason:`docs` parameter must be an array.")
}

func TestCommitUpdates(t *testing.T) {
	vdbEnv.init(t, nil)
	defer vdbEnv.cleanup()

	versionedDB, err := vdbEnv.DBProvider.GetDBHandle("test-commitupdates", nil)
	require.NoError(t, err)
	db := versionedDB.(*VersionedDB)

	nsUpdates := map[string]*statedb.VersionedValue{
		"key1": {
			Value:    nil,
			Metadata: nil,
			Version:  version.NewHeight(1, 1),
		},
		"key2": {
			Value:    []byte("value2"),
			Metadata: nil,
			Version:  version.NewHeight(1, 1),
		},
	}

	committerDB, err := db.getNamespaceDBHandle("ns")
	require.NoError(t, err)
	couchDoc, err := keyValToCouchDoc(&keyValue{key: "key1", revision: "", VersionedValue: nsUpdates["key1"]})
	require.NoError(t, err)

	tests := []struct {
		committer   *committer
		expectedErr string
	}{
		{
			committer: &committer{
				db:             committerDB,
				batchUpdateMap: map[string]*batchableDocument{},
				namespace:      "ns",
				cacheKVs:       make(cacheKVs),
				cacheEnabled:   true,
			},
			expectedErr: "error handling CouchDB request. Error:bad_request,  Status Code:400,  Reason:`docs` parameter must be an array.",
		},
		{
			committer: &committer{
				db:             committerDB,
				batchUpdateMap: map[string]*batchableDocument{"key1": {CouchDoc: *couchDoc}},
				namespace:      "ns",
				cacheKVs:       make(cacheKVs),
				cacheEnabled:   true,
			},
			expectedErr: "",
		},
		{
			committer: &committer{
				db:             committerDB,
				batchUpdateMap: map[string]*batchableDocument{"key1": {CouchDoc: *couchDoc}},
				namespace:      "ns",
				cacheKVs:       make(cacheKVs),
				cacheEnabled:   true,
			},
			expectedErr: "error saving document ID: key1. Error: conflict,  Reason: Document update conflict.: error handling CouchDB request. Error:conflict,  Status Code:409,  Reason:Document update conflict.",
		},
	}

	for _, test := range tests {
		err := test.committer.commitUpdates()
		if test.expectedErr == "" {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, test.expectedErr)
		}
	}

	couchDoc, err = keyValToCouchDoc(&keyValue{key: "key2", revision: "", VersionedValue: nsUpdates["key2"]})
	require.NoError(t, err)

	committer := &committer{
		db:             committerDB,
		batchUpdateMap: map[string]*batchableDocument{"key2": {CouchDoc: *couchDoc}},
		namespace:      "ns",
		cacheKVs:       cacheKVs{"key2": &CacheValue{}},
		cacheEnabled:   true,
	}

	require.Empty(t, committer.cacheKVs["key2"].AdditionalInfo)
	err = committer.commitUpdates()
	require.NoError(t, err)
	require.NotEmpty(t, committer.cacheKVs["key2"].AdditionalInfo)
}
