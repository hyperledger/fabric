/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package privacyenabledstate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	testmock "github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/privacyenabledstate/mock"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/stateleveldb"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/core/ledger/util"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckRegister(t *testing.T) {
	fakeHealthCheckRegistry := &mock.HealthCheckRegistry{}
	dbProvider := &DBProvider{
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
	chaincodeIndexPath := "META-INF/statedb/couchdb/indexes"
	actualIndexInfo := getIndexInfo(chaincodeIndexPath)
	expectedIndexInfo := &indexInfo{
		hasIndexForChaincode:  true,
		hasIndexForCollection: false,
		collectionName:        "",
	}
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	collectionIndexPath := "META-INF/statedb/couchdb/collections/collectionMarbles/indexes"
	actualIndexInfo = getIndexInfo(collectionIndexPath)
	expectedIndexInfo = &indexInfo{
		hasIndexForChaincode:  false,
		hasIndexForCollection: true,
		collectionName:        "collectionMarbles",
	}
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	incorrectChaincodeIndexPath := "META-INF/statedb/couchdb"
	actualIndexInfo = getIndexInfo(incorrectChaincodeIndexPath)
	expectedIndexInfo = &indexInfo{
		hasIndexForChaincode:  false,
		hasIndexForCollection: false,
		collectionName:        "",
	}
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	incorrectCollectionIndexPath := "META-INF/statedb/couchdb/collections/indexes"
	actualIndexInfo = getIndexInfo(incorrectCollectionIndexPath)
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	incorrectCollectionIndexPath = "META-INF/statedb/couchdb/collections"
	actualIndexInfo = getIndexInfo(incorrectCollectionIndexPath)
	require.Equal(t, expectedIndexInfo, actualIndexInfo)

	incorrectIndexPath := "META-INF/statedb"
	actualIndexInfo = getIndexInfo(incorrectIndexPath)
	require.Equal(t, expectedIndexInfo, actualIndexInfo)
}

func TestDB(t *testing.T) {
	for _, env := range testEnvs {
		t.Run(env.GetName(), func(t *testing.T) {
			testDB(t, env)
		})
	}
}

func testDB(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()
	db := env.GetDBHandle(generateLedgerID(t))

	updates := NewUpdateBatch()

	updates.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	updates.PubUpdates.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	updates.PubUpdates.Put("ns2", "key3", []byte("value3"), version.NewHeight(1, 3))

	putPvtUpdates(t, updates, "ns1", "coll1", "key1", []byte("pvt_value1"), version.NewHeight(1, 4))
	putPvtUpdates(t, updates, "ns1", "coll1", "key2", []byte("pvt_value2"), version.NewHeight(1, 5))
	putPvtUpdates(t, updates, "ns2", "coll1", "key3", []byte("pvt_value3"), version.NewHeight(1, 6))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 6)))
	bulkOptimizable, ok := db.VersionedDB.(statedb.BulkOptimizable)
	if ok {
		bulkOptimizable.ClearCachedVersions()
	}

	vv, err := db.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}, vv)

	vv, err = db.GetPrivateData("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: []byte("pvt_value1"), Version: version.NewHeight(1, 4)}, vv)

	vv, err = db.GetPrivateDataHash("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: util.ComputeStringHash("pvt_value1"), Version: version.NewHeight(1, 4)}, vv)

	vv, err = db.GetValueHash("ns1", "coll1", util.ComputeStringHash("key1"))
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: util.ComputeStringHash("pvt_value1"), Version: version.NewHeight(1, 4)}, vv)

	committedVersion, err := db.GetKeyHashVersion("ns1", "coll1", util.ComputeStringHash("key1"))
	require.NoError(t, err)
	require.Equal(t, version.NewHeight(1, 4), committedVersion)

	updates = NewUpdateBatch()
	updates.PubUpdates.Delete("ns1", "key1", version.NewHeight(2, 7))
	deletePvtUpdates(t, updates, "ns1", "coll1", "key1", version.NewHeight(2, 7))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 7)))

	vv, err = db.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Nil(t, vv)

	vv, err = db.GetPrivateData("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Nil(t, vv)

	vv, err = db.GetValueHash("ns1", "coll1", util.ComputeStringHash("key1"))
	require.NoError(t, err)
	require.Nil(t, vv)
}

func TestGetStateMultipleKeys(t *testing.T) {
	for _, env := range testEnvs {
		t.Run(env.GetName(), func(t *testing.T) {
			testGetStateMultipleKeys(t, env)
		})
	}
}

func testGetStateMultipleKeys(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()
	db := env.GetDBHandle(generateLedgerID(t))

	updates := NewUpdateBatch()

	updates.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	updates.PubUpdates.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	updates.PubUpdates.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))

	putPvtUpdates(t, updates, "ns1", "coll1", "key1", []byte("pvt_value1"), version.NewHeight(1, 4))
	putPvtUpdates(t, updates, "ns1", "coll1", "key2", []byte("pvt_value2"), version.NewHeight(1, 5))
	putPvtUpdates(t, updates, "ns1", "coll1", "key3", []byte("pvt_value3"), version.NewHeight(1, 6))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 6)))

	versionedVals, err := db.GetStateMultipleKeys("ns1", []string{"key1", "key3"})
	require.NoError(t, err)
	require.Equal(t,
		[]*statedb.VersionedValue{
			{Value: []byte("value1"), Version: version.NewHeight(1, 1)},
			{Value: []byte("value3"), Version: version.NewHeight(1, 3)},
		},
		versionedVals)

	pvtVersionedVals, err := db.GetPrivateDataMultipleKeys("ns1", "coll1", []string{"key1", "key3"})
	require.NoError(t, err)
	require.Equal(t,
		[]*statedb.VersionedValue{
			{Value: []byte("pvt_value1"), Version: version.NewHeight(1, 4)},
			{Value: []byte("pvt_value3"), Version: version.NewHeight(1, 6)},
		},
		pvtVersionedVals)
}

func TestGetStateRangeScanIterator(t *testing.T) {
	for _, env := range testEnvs {
		t.Run(env.GetName(), func(t *testing.T) {
			testGetStateRangeScanIterator(t, env)
		})
	}
}

func testGetStateRangeScanIterator(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()
	db := env.GetDBHandle(generateLedgerID(t))

	updates := NewUpdateBatch()

	updates.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	updates.PubUpdates.Put("ns1", "key2", []byte("value2"), version.NewHeight(1, 2))
	updates.PubUpdates.Put("ns1", "key3", []byte("value3"), version.NewHeight(1, 3))
	updates.PubUpdates.Put("ns1", "key4", []byte("value4"), version.NewHeight(1, 4))
	updates.PubUpdates.Put("ns2", "key5", []byte("value5"), version.NewHeight(1, 5))
	updates.PubUpdates.Put("ns2", "key6", []byte("value6"), version.NewHeight(1, 6))
	updates.PubUpdates.Put("ns3", "key7", []byte("value7"), version.NewHeight(1, 7))

	putPvtUpdates(t, updates, "ns1", "coll1", "key1", []byte("pvt_value1"), version.NewHeight(1, 1))
	putPvtUpdates(t, updates, "ns1", "coll1", "key2", []byte("pvt_value2"), version.NewHeight(1, 2))
	putPvtUpdates(t, updates, "ns1", "coll1", "key3", []byte("pvt_value3"), version.NewHeight(1, 3))
	putPvtUpdates(t, updates, "ns1", "coll1", "key4", []byte("pvt_value4"), version.NewHeight(1, 4))
	putPvtUpdates(t, updates, "ns2", "coll1", "key5", []byte("pvt_value5"), version.NewHeight(1, 5))
	putPvtUpdates(t, updates, "ns2", "coll1", "key6", []byte("pvt_value6"), version.NewHeight(1, 6))
	putPvtUpdates(t, updates, "ns3", "coll1", "key7", []byte("pvt_value7"), version.NewHeight(1, 7))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 7)))

	itr1, _ := db.GetStateRangeScanIterator("ns1", "key1", "")
	testItr(t, itr1, []string{"key1", "key2", "key3", "key4"})

	itr2, _ := db.GetStateRangeScanIterator("ns1", "key2", "key3")
	testItr(t, itr2, []string{"key2"})

	itr3, _ := db.GetStateRangeScanIterator("ns1", "", "")
	testItr(t, itr3, []string{"key1", "key2", "key3", "key4"})

	itr4, _ := db.GetStateRangeScanIterator("ns2", "", "")
	testItr(t, itr4, []string{"key5", "key6"})

	pvtItr1, _ := db.GetPrivateDataRangeScanIterator("ns1", "coll1", "key1", "")
	testItr(t, pvtItr1, []string{"key1", "key2", "key3", "key4"})

	pvtItr2, _ := db.GetPrivateDataRangeScanIterator("ns1", "coll1", "key2", "key3")
	testItr(t, pvtItr2, []string{"key2"})

	pvtItr3, _ := db.GetPrivateDataRangeScanIterator("ns1", "coll1", "", "")
	testItr(t, pvtItr3, []string{"key1", "key2", "key3", "key4"})

	pvtItr4, _ := db.GetPrivateDataRangeScanIterator("ns2", "coll1", "", "")
	testItr(t, pvtItr4, []string{"key5", "key6"})
}

func TestQueryOnCouchDB(t *testing.T) {
	for _, env := range testEnvs {
		_, ok := env.(*CouchDBTestEnv)
		if !ok {
			continue
		}
		t.Run(env.GetName(), func(t *testing.T) {
			testQueryOnCouchDB(t, env)
		})
	}
}

func testQueryOnCouchDB(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()
	db := env.GetDBHandle(generateLedgerID(t))
	updates := NewUpdateBatch()

	jsonValues := []string{
		`{"asset_name": "marble1", "color": "blue", "size": 1, "owner": "tom"}`,
		`{"asset_name": "marble2","color": "blue","size": 2,"owner": "jerry"}`,
		`{"asset_name": "marble3","color": "blue","size": 3,"owner": "fred"}`,
		`{"asset_name": "marble4","color": "blue","size": 4,"owner": "martha"}`,
		`{"asset_name": "marble5","color": "blue","size": 5,"owner": "fred"}`,
		`{"asset_name": "marble6","color": "blue","size": 6,"owner": "elaine"}`,
		`{"asset_name": "marble7","color": "blue","size": 7,"owner": "fred"}`,
		`{"asset_name": "marble8","color": "blue","size": 8,"owner": "elaine"}`,
		`{"asset_name": "marble9","color": "green","size": 9,"owner": "fred"}`,
		`{"asset_name": "marble10","color": "green","size": 10,"owner": "mary"}`,
		`{"asset_name": "marble11","color": "cyan","size": 1000007,"owner": "joe"}`,
	}

	for i, jsonValue := range jsonValues {
		updates.PubUpdates.Put("ns1", testKey(i), []byte(jsonValue), version.NewHeight(1, uint64(i)))
		updates.PubUpdates.Put("ns2", testKey(i), []byte(jsonValue), version.NewHeight(1, uint64(i)))
		putPvtUpdates(t, updates, "ns1", "coll1", testKey(i), []byte(jsonValue), version.NewHeight(1, uint64(i)))
		putPvtUpdates(t, updates, "ns2", "coll1", testKey(i), []byte(jsonValue), version.NewHeight(1, uint64(i)))
	}
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(1, 11)))

	// query for owner=jerry, use namespace "ns1"
	itr, err := db.ExecuteQuery("ns1", `{"selector":{"owner":"jerry"}}`)
	require.NoError(t, err)
	testQueryItr(t, itr, []string{testKey(1)}, []string{"jerry"})

	// query for owner=jerry, use namespace "ns2"
	itr, err = db.ExecuteQuery("ns2", `{"selector":{"owner":"jerry"}}`)
	require.NoError(t, err)
	testQueryItr(t, itr, []string{testKey(1)}, []string{"jerry"})

	// query for pvt data owner=jerry, use namespace "ns1"
	itr, err = db.ExecuteQueryOnPrivateData("ns1", "coll1", `{"selector":{"owner":"jerry"}}`)
	require.NoError(t, err)
	testQueryItr(t, itr, []string{testKey(1)}, []string{"jerry"})

	// query for pvt data owner=jerry, use namespace "ns2"
	itr, err = db.ExecuteQueryOnPrivateData("ns2", "coll1", `{"selector":{"owner":"jerry"}}`)
	require.NoError(t, err)
	testQueryItr(t, itr, []string{testKey(1)}, []string{"jerry"})

	// query using bad query string
	_, err = db.ExecuteQueryOnPrivateData("ns1", "coll1", "this is an invalid query string")
	require.Error(t, err, "Should have received an error for invalid query string")

	// query returns 0 records
	itr, err = db.ExecuteQueryOnPrivateData("ns1", "coll1", `{"selector":{"owner":"not_a_valid_name"}}`)
	require.NoError(t, err)
	testQueryItr(t, itr, []string{}, []string{})

	// query with embedded implicit "AND" and explicit "OR", namespace "ns1"
	itr, err = db.ExecuteQueryOnPrivateData("ns1", "coll1", `{"selector":{"color":"green","$or":[{"owner":"fred"},{"owner":"mary"}]}}`)
	require.NoError(t, err)
	testQueryItr(t, itr, []string{testKey(8), testKey(9)}, []string{"green"}, []string{"green"})

	// query with integer with digit-count equals 7 and response received is also received
	// with same digit-count and there is no float transformation
	itr, err = db.ExecuteQueryOnPrivateData("ns2", "coll1", `{"selector":{"$and":[{"size":{"$eq": 1000007}}]}}`)
	require.NoError(t, err)
	testQueryItr(t, itr, []string{testKey(10)}, []string{"joe", "1000007"})
}

func TestLongDBNameOnCouchDB(t *testing.T) {
	for _, env := range testEnvs {
		_, ok := env.(*CouchDBTestEnv)
		if !ok {
			continue
		}
		t.Run(env.GetName(), func(t *testing.T) {
			testLongDBNameOnCouchDB(t, env)
		})
	}
}

func testLongDBNameOnCouchDB(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()

	// Creates metadataDB (i.e., chainDB)
	// Allowed pattern for chainName: [a-z][a-z0-9.-]
	db := env.GetDBHandle("w1coaii9ck3l8red6a5cf3rwbe1b4wvbzcrrfl7samu7px8b9gf-4hft7wrgdmzzjj9ure4cbffucaj78nbj9ej.kvl3bus1iq1qir9xlhb8a1wipuksgs3g621elzy1prr658087exwrhp-y4j55o9cld242v--oeh3br1g7m8d6l8jobn.y42cgjt1.u1ik8qxnv4ohh9kr2w2zc8hqir5u4ev23s7jygrg....s7.ohp-5bcxari8nji")

	updates := NewUpdateBatch()

	// Allowed pattern for namespace and collection: [a-zA-Z0-9_-]
	ns := "wMCnSXiV9YoIqNQyNvFVTdM8XnUtvrOFFIWsKelmP5NEszmNLl8YhtOKbFu3P_NgwgsYF8PsfwjYCD8f1XRpANQLoErDHwLlweryqXeJ6vzT2x0pS_GwSx0m6tBI0zOmHQOq_2De8A87x6zUOPwufC2T6dkidFxiuq8Sey2-5vUo_iNKCij3WTeCnKx78PUIg_U1gp4_0KTvYVtRBRvH0kz5usizBxPaiFu3TPhB9XLviScvdUVSbSYJ0Z"
	coll := "vWjtfSTXVK8WJus5s6zWoMIciXd7qHRZIusF9SkOS6m8XuHCiJDE9cCRuVerq22Na8qBL2ywDGFpVMIuzfyEXLjeJb0mMuH4cwewT6r1INOTOSYwrikwOLlT_fl0V1L7IQEwUBB8WCvRqSdj6j5-E5aGul_pv_0UeCdwWiyA_GrZmP7ocLzfj2vP8btigrajqdH-irLO2ydEjQUAvf8fiuxru9la402KmKRy457GgI98UHoUdqV3f3FCdR"

	updates.PubUpdates.Put(ns, "key1", []byte("value1"), version.NewHeight(1, 1))
	updates.PvtUpdates.Put(ns, coll, "key1", []byte("pvt_value"), version.NewHeight(1, 2))
	updates.HashUpdates.Put(ns, coll, util.ComputeStringHash("key1"), util.ComputeHash([]byte("pvt_value")), version.NewHeight(1, 2))

	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 6)))

	vv, err := db.GetState(ns, "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}, vv)

	vv, err = db.GetPrivateData(ns, coll, "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: []byte("pvt_value"), Version: version.NewHeight(1, 2)}, vv)
}

func testItr(t *testing.T, itr statedb.ResultsIterator, expectedKeys []string) {
	defer itr.Close()
	for _, expectedKey := range expectedKeys {
		queryResult, err := itr.Next()
		require.NoError(t, err)
		require.Equal(t, expectedKey, queryResult.Key)
	}
	last, err := itr.Next()
	require.NoError(t, err)
	require.Nil(t, last)
}

func testQueryItr(t *testing.T, itr statedb.ResultsIterator, expectedKeys []string, expectedValStrs ...[]string) {
	defer itr.Close()
	for i, expectedKey := range expectedKeys {
		queryResult, err := itr.Next()
		require.NoError(t, err)
		valStr := string(queryResult.Value)
		require.Equal(t, expectedKey, queryResult.Key)
		for _, expectedValStr := range expectedValStrs[i] {
			require.Contains(t, valStr, expectedValStr)
		}
	}
	last, err := itr.Next()
	require.NoError(t, err)
	require.Nil(t, last)
}

func testKey(i int) string {
	return fmt.Sprintf("key%d", i)
}

func TestHandleChainCodeDeployOnCouchDB(t *testing.T) {
	for _, env := range testEnvs {
		_, ok := env.(*CouchDBTestEnv)
		if !ok {
			continue
		}
		t.Run(env.GetName(), func(t *testing.T) {
			testHandleChainCodeDeploy(t, env)
		})
	}
}

func createCollectionConfig(collectionName string) *peer.CollectionConfig {
	return &peer.CollectionConfig{
		Payload: &peer.CollectionConfig_StaticCollectionConfig{
			StaticCollectionConfig: &peer.StaticCollectionConfig{
				Name:              collectionName,
				MemberOrgsPolicy:  nil,
				RequiredPeerCount: 0,
				MaximumPeerCount:  0,
				BlockToLive:       0,
			},
		},
	}
}

func testHandleChainCodeDeploy(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()
	db := env.GetDBHandle(generateLedgerID(t))

	coll1 := createCollectionConfig("collectionMarbles")
	ccp := &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1}}
	chaincodeDef := &cceventmgmt.ChaincodeDefinition{Name: "ns1", Hash: nil, Version: "", CollectionConfigs: ccp}

	// Test indexes for side databases
	dbArtifactsTarBytes := testutil.CreateTarBytesForTest(
		[]*testutil.TarFileEntry{
			{Name: "META-INF/statedb/couchdb/indexes/indexColorSortName.json", Body: `{"index":{"fields":[{"color":"desc"}]},"ddoc":"indexColorSortName","name":"indexColorSortName","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/indexes/indexSizeSortName.json", Body: `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexCollMarbles.json", Body: `{"index":{"fields":["docType","owner"]},"ddoc":"indexCollectionMarbles", "name":"indexCollectionMarbles","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/collections/collectionMarblesPrivateDetails/indexes/indexCollPrivDetails.json", Body: `{"index":{"fields":["docType","price"]},"ddoc":"indexPrivateDetails", "name":"indexPrivateDetails","type":"json"}`},
		},
	)

	// Test the retrieveIndexArtifacts method
	fileEntries, err := ccprovider.ExtractFileEntries(dbArtifactsTarBytes, "couchdb")
	require.NoError(t, err)

	// There should be 3 entries
	require.Len(t, fileEntries, 3)

	// There should be 2 entries for main
	require.Len(t, fileEntries["META-INF/statedb/couchdb/indexes"], 2)

	// There should be 1 entry for collectionMarbles
	require.Len(t, fileEntries["META-INF/statedb/couchdb/collections/collectionMarbles/indexes"], 1)

	// Verify the content of the array item
	expectedJSON := []byte(`{"index":{"fields":["docType","owner"]},"ddoc":"indexCollectionMarbles", "name":"indexCollectionMarbles","type":"json"}`)
	actualJSON := fileEntries["META-INF/statedb/couchdb/collections/collectionMarbles/indexes"][0].FileContent
	require.Equal(t, expectedJSON, actualJSON)

	// The collection config is added to the chaincodeDef but missing collectionMarblesPrivateDetails.
	// Hence, the index on collectionMarblesPrivateDetails cannot be created
	err = db.HandleChaincodeDeploy(chaincodeDef, dbArtifactsTarBytes)
	require.NoError(t, err)

	coll2 := createCollectionConfig("collectionMarblesPrivateDetails")
	ccp = &peer.CollectionConfigPackage{Config: []*peer.CollectionConfig{coll1, coll2}}
	chaincodeDef = &cceventmgmt.ChaincodeDefinition{Name: "ns1", Hash: nil, Version: "", CollectionConfigs: ccp}

	// The collection config is added to the chaincodeDef and it contains all collections
	// including collectionMarblesPrivateDetails which was missing earlier.
	// Hence, the existing indexes must be updated and the new index must be created for
	// collectionMarblesPrivateDetails
	err = db.HandleChaincodeDeploy(chaincodeDef, dbArtifactsTarBytes)
	require.NoError(t, err)

	chaincodeDef = &cceventmgmt.ChaincodeDefinition{Name: "ns1", Hash: nil, Version: "", CollectionConfigs: nil}

	// The collection config is not added to the chaincodeDef. In this case, the index creation
	// process reads the collection config from state db. However, the state db does not contain
	// any collection config for this chaincode. Hence, index creation/update on all collections
	// should fail
	err = db.HandleChaincodeDeploy(chaincodeDef, dbArtifactsTarBytes)
	require.NoError(t, err)

	// Test HandleChaincodeDefinition with a nil tar file
	err = db.HandleChaincodeDeploy(chaincodeDef, nil)
	require.NoError(t, err)

	// Test HandleChaincodeDefinition with a bad tar file
	err = db.HandleChaincodeDeploy(chaincodeDef, []byte(`This is a really bad tar file`))
	require.NoError(t, err, "Error should not have been thrown for a bad tar file")

	// Test HandleChaincodeDefinition with a nil chaincodeDef
	err = db.HandleChaincodeDeploy(nil, dbArtifactsTarBytes)
	require.Error(t, err, "Error should have been thrown for a nil chaincodeDefinition")

	// Create a tar file for test with 2 index definitions - one of them being errorneous
	badSyntaxFileContent := `{"index":{"fields": This is a bad json}`
	dbArtifactsTarBytes = testutil.CreateTarBytesForTest(
		[]*testutil.TarFileEntry{
			{Name: "META-INF/statedb/couchdb/indexes/indexSizeSortName.json", Body: `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/indexes/badSyntax.json", Body: badSyntaxFileContent},
		},
	)

	// Test the retrieveIndexArtifacts method
	fileEntries, err = ccprovider.ExtractFileEntries(dbArtifactsTarBytes, "couchdb")
	require.NoError(t, err)

	// There should be 1 entry
	require.Len(t, fileEntries, 1)

	err = db.HandleChaincodeDeploy(chaincodeDef, dbArtifactsTarBytes)
	require.NoError(t, err)
}

func TestMetadataRetrieval(t *testing.T) {
	for _, env := range testEnvs {
		t.Run(env.GetName(), func(t *testing.T) {
			testMetadataRetrieval(t, env)
		})
	}
}

func testMetadataRetrieval(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()
	db := env.GetDBHandle(generateLedgerID(t))

	updates := NewUpdateBatch()
	updates.PubUpdates.PutValAndMetadata("ns1", "key1", []byte("value1"), []byte("metadata1"), version.NewHeight(1, 1))
	updates.PubUpdates.PutValAndMetadata("ns1", "key2", []byte("value2"), nil, version.NewHeight(1, 2))
	updates.PubUpdates.PutValAndMetadata("ns2", "key3", []byte("value3"), nil, version.NewHeight(1, 3))

	putPvtUpdatesWithMetadata(t, updates, "ns1", "coll1", "key1", []byte("pvt_value1"), []byte("metadata1"), version.NewHeight(1, 4))
	putPvtUpdatesWithMetadata(t, updates, "ns1", "coll1", "key2", []byte("pvt_value2"), nil, version.NewHeight(1, 5))
	putPvtUpdatesWithMetadata(t, updates, "ns2", "coll1", "key3", []byte("pvt_value3"), nil, version.NewHeight(1, 6))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 6)))

	vm, _ := db.GetStateMetadata("ns1", "key1")
	require.Equal(t, vm, []byte("metadata1"))
	vm, _ = db.GetStateMetadata("ns1", "key2")
	require.Nil(t, vm)
	vm, _ = db.GetStateMetadata("ns2", "key3")
	require.Nil(t, vm)

	vm, _ = db.GetPrivateDataMetadataByHash("ns1", "coll1", util.ComputeStringHash("key1"))
	require.Equal(t, vm, []byte("metadata1"))
	vm, _ = db.GetPrivateDataMetadataByHash("ns1", "coll1", util.ComputeStringHash("key2"))
	require.Nil(t, vm)
	vm, _ = db.GetPrivateDataMetadataByHash("ns2", "coll1", util.ComputeStringHash("key3"))
	require.Nil(t, vm)
}

func putPvtUpdates(t *testing.T, updates *UpdateBatch, ns, coll, key string, value []byte, ver *version.Height) {
	updates.PvtUpdates.Put(ns, coll, key, value, ver)
	updates.HashUpdates.Put(ns, coll, util.ComputeStringHash(key), util.ComputeHash(value), ver)
}

func putPvtUpdatesWithMetadata(t *testing.T, updates *UpdateBatch, ns, coll, key string, value []byte, metadata []byte, ver *version.Height) {
	updates.PvtUpdates.Put(ns, coll, key, value, ver)
	updates.HashUpdates.PutValHashAndMetadata(ns, coll, util.ComputeStringHash(key), util.ComputeHash(value), metadata, ver)
}

func deletePvtUpdates(t *testing.T, updates *UpdateBatch, ns, coll, key string, ver *version.Height) {
	updates.PvtUpdates.Delete(ns, coll, key, ver)
	updates.HashUpdates.Delete(ns, coll, util.ComputeStringHash(key), ver)
}

func generateLedgerID(t *testing.T) string {
	bytes := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, bytes)
	require.NoError(t, err)
	return fmt.Sprintf("x%s", hex.EncodeToString(bytes))
}

func TestDrop(t *testing.T) {
	for _, env := range testEnvs {
		t.Run(env.GetName(), func(t *testing.T) {
			testDrop(t, env)
		})

		t.Run("test-drop-error-propagation", func(t *testing.T) {
			env.Init(t)
			defer env.Cleanup()
			ledgerid := generateLedgerID(t)
			env.GetDBHandle(ledgerid)
			env.GetProvider().Close()
			require.EqualError(t, env.GetProvider().Drop(ledgerid), "internal leveldb error while obtaining db iterator: leveldb: closed")
		})
	}
}

func testDrop(t *testing.T, env TestEnv) {
	env.Init(t)
	defer env.Cleanup()
	ledgerid := generateLedgerID(t)
	db := env.GetDBHandle(ledgerid)

	updates := NewUpdateBatch()
	updates.PubUpdates.Put("ns1", "key1", []byte("value1"), version.NewHeight(1, 1))
	putPvtUpdates(t, updates, "ns1", "coll1", "key1", []byte("pvt_value1"), version.NewHeight(1, 2))
	require.NoError(t, db.ApplyPrivacyAwareUpdates(updates, version.NewHeight(2, 2)))

	vv, err := db.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: []byte("value1"), Version: version.NewHeight(1, 1)}, vv)

	vv, err = db.GetPrivateData("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: []byte("pvt_value1"), Version: version.NewHeight(1, 2)}, vv)

	vv, err = db.GetPrivateDataHash("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Equal(t, &statedb.VersionedValue{Value: util.ComputeStringHash("pvt_value1"), Version: version.NewHeight(1, 2)}, vv)

	require.NoError(t, env.GetProvider().Drop(ledgerid))

	vv, err = db.GetState("ns1", "key1")
	require.NoError(t, err)
	require.Nil(t, vv)

	vv, err = db.GetPrivateData("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Nil(t, vv)

	vv, err = db.GetPrivateDataHash("ns1", "coll1", "key1")
	require.NoError(t, err)
	require.Nil(t, vv)
}

//go:generate counterfeiter -o mock/channelinfo_provider.go -fake-name ChannelInfoProvider . channelInfoProvider

func TestPossibleNamespaces(t *testing.T) {
	namespacesAndCollections := map[string][]string{
		"cc1":        {"_implicit_org_Org1MSP", "_implicit_org_Org2MSP", "collectionA", "collectionB"},
		"cc2":        {"_implicit_org_Org1MSP", "_implicit_org_Org2MSP"},
		"_lifecycle": {"_implicit_org_Org1MSP", "_implicit_org_Org2MSP"},
		"lscc":       {},
		"":           {},
	}
	expectedNamespaces := []string{
		"cc1",
		"cc1$$p_implicit_org_Org1MSP",
		"cc1$$h_implicit_org_Org1MSP",
		"cc1$$p_implicit_org_Org2MSP",
		"cc1$$h_implicit_org_Org2MSP",
		"cc1$$pcollectionA",
		"cc1$$hcollectionA",
		"cc1$$pcollectionB",
		"cc1$$hcollectionB",
		"cc2",
		"cc2$$p_implicit_org_Org1MSP",
		"cc2$$h_implicit_org_Org1MSP",
		"cc2$$p_implicit_org_Org2MSP",
		"cc2$$h_implicit_org_Org2MSP",
		"_lifecycle",
		"_lifecycle$$p_implicit_org_Org1MSP",
		"_lifecycle$$h_implicit_org_Org1MSP",
		"_lifecycle$$p_implicit_org_Org2MSP",
		"_lifecycle$$h_implicit_org_Org2MSP",
		"lscc",
		"",
	}

	fakeChannelInfoProvider := &testmock.ChannelInfoProvider{}
	fakeChannelInfoProvider.NamespacesAndCollectionsReturns(namespacesAndCollections, nil)
	nsProvider := &namespaceProvider{fakeChannelInfoProvider}
	namespaces, err := nsProvider.PossibleNamespaces(&statecouchdb.VersionedDB{})
	require.NoError(t, err)
	require.ElementsMatch(t, expectedNamespaces, namespaces)
}
