/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckRegister(t *testing.T) {
	fakeHealthCheckRegistry := &mock.HealthCheckRegistry{}
	dbProvider := &CommonStorageDBProvider{
		VersionedDBProvider: &stateleveldb.VersionedDBProvider{},
		HealthCheckRegistry: fakeHealthCheckRegistry,
	}

	err := dbProvider.RegisterHealthChecker()
	require.NoError(t, err)
	require.Equal(t, 0, fakeHealthCheckRegistry.RegisterCheckerCallCount())

	dbProvider.VersionedDBProvider = &statecouchdb.VersionedDBProvider{}
	err = dbProvider.RegisterHealthChecker()
	require.NoError(t, err)
	require.Equal(t, 1, fakeHealthCheckRegistry.RegisterCheckerCallCount())

	arg1, arg2 := fakeHealthCheckRegistry.RegisterCheckerArgsForCall(0)
	require.Equal(t, "couchdb", arg1)
	require.NotNil(t, arg2)
}

func TestGetIndexInfo(t *testing.T) {
	chaincodeIndexPath := "META-INF/statedb/couchdb/indexes/indexColorSortName.json"
	actualIndexInfo := getIndexInfo(chaincodeIndexPath)
	expectedIndexInfo := &indexInfo{
		hasIndexForChaincode:  true,
		hasIndexForCollection: false,
		collectionName:        "",
	}
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	collectionIndexPath := "META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexCollMarbles.json"
	actualIndexInfo = getIndexInfo(collectionIndexPath)
	expectedIndexInfo = &indexInfo{
		hasIndexForChaincode:  false,
		hasIndexForCollection: true,
		collectionName:        "collectionMarbles",
	}
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	incorrectChaincodeIndexPath := "META-INF/statedb/couchdb/indexColorSortName.json"
	actualIndexInfo = getIndexInfo(incorrectChaincodeIndexPath)
	expectedIndexInfo = &indexInfo{
		hasIndexForChaincode:  false,
		hasIndexForCollection: false,
		collectionName:        "",
	}
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	incorrectCollectionIndexPath := "META-INF/statedb/couchdb/collections/indexes/indexCollMarbles.json"
	actualIndexInfo = getIndexInfo(incorrectCollectionIndexPath)
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	incorrectIndexPath := "META-INF/statedb/"
	actualIndexInfo = getIndexInfo(incorrectIndexPath)
	require.Equal(t, expectedIndexInfo, actualIndexInfo)
}
