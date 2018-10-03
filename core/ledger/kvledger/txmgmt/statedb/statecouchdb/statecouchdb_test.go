/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package statecouchdb

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/commontests"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/version"
	ledgertestutil "github.com/hyperledger/fabric/core/ledger/testutil"
	"github.com/hyperledger/fabric/integration/runner"
)

func TestMain(m *testing.M) {
	flogging.SetModuleLevel("statecouchdb", "debug")
	flogging.SetModuleLevel("couchdb", "debug")
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	// Read the core.yaml file for default config.
	ledgertestutil.SetupCoreYAMLConfig()
	viper.Set("peer.fileSystemPath", "/tmp/fabric/ledgertests/kvledger/txmgmt/statedb/statecouchdb")

	// Switch to CouchDB
	couchAddress, cleanup := couchDBSetup()
	defer cleanup()
	viper.Set("ledger.state.stateDatabase", "CouchDB")
	defer viper.Set("ledger.state.stateDatabase", "goleveldb")

	viper.Set("ledger.state.couchDBConfig.couchDBAddress", couchAddress)
	// Replace with correct username/password such as
	// admin/admin if user security is enabled on couchdb.
	viper.Set("ledger.state.couchDBConfig.username", "")
	viper.Set("ledger.state.couchDBConfig.password", "")
	viper.Set("ledger.state.couchDBConfig.maxRetries", 3)
	viper.Set("ledger.state.couchDBConfig.maxRetriesOnStartup", 20)
	viper.Set("ledger.state.couchDBConfig.requestTimeout", time.Second*35)
	// Disable auto warm to avoid error logs when the couchdb database has been dropped
	viper.Set("ledger.state.couchDBConfig.autoWarmIndexes", false)

	flogging.SetModuleLevel("statecouchdb", "debug")
	//run the actual test
	return m.Run()
}

func couchDBSetup() (addr string, cleanup func()) {
	externalCouch, set := os.LookupEnv("COUCHDB_ADDR")
	if set {
		return externalCouch, func() {}
	}

	couchDB := &runner.CouchDB{}
	if err := couchDB.Start(); err != nil {
		err := fmt.Errorf("failed to start couchDB: %s", err)
		panic(err)
	}
	return couchDB.Address(), func() { couchDB.Stop() }
}

func TestBasicRW(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestBasicRW(t, env.DBProvider)

}

func TestMultiDBBasicRW(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestMultiDBBasicRW(t, env.DBProvider)

}

func TestDeletes(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestDeletes(t, env.DBProvider)
}

func TestIterator(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestIterator(t, env.DBProvider)
}

// The following tests are unique to couchdb, they are not used in leveldb
//  query test
func TestQuery(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestQuery(t, env.DBProvider)
}

func TestGetStateMultipleKeys(t *testing.T) {

	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestGetStateMultipleKeys(t, env.DBProvider)
}

func TestGetVersion(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestGetVersion(t, env.DBProvider)
}

func TestSmallBatchSize(t *testing.T) {
	viper.Set("ledger.state.couchDBConfig.maxBatchUpdateSize", 2)
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	defer viper.Set("ledger.state.couchDBConfig.maxBatchUpdateSize", 1000)
	commontests.TestSmallBatchSize(t, env.DBProvider)
}

func TestBatchRetry(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestBatchWithIndividualRetry(t, env.DBProvider)
}

func TestValueAndMetadataWrites(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestValueAndMetadataWrites(t, env.DBProvider)
}

func TestPaginatedRangeQuery(t *testing.T) {
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	commontests.TestPaginatedRangeQuery(t, env.DBProvider)
}

// TestUtilityFunctions tests utility functions
func TestUtilityFunctions(t *testing.T) {

	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testutilityfunctions")
	assert.NoError(t, err)

	// BytesKeySuppoted should be false for CouchDB
	byteKeySupported := db.BytesKeySuppoted()
	assert.False(t, byteKeySupported)

	// ValidateKeyValue should return nil for a valid key and value
	err = db.ValidateKeyValue("testKey", []byte("Some random bytes"))
	assert.Nil(t, err)

	// ValidateKey should return an error for an invalid key
	err = db.ValidateKeyValue(string([]byte{0xff, 0xfe, 0xfd}), []byte("Some random bytes"))
	assert.Error(t, err, "ValidateKey should have thrown an error for an invalid utf-8 string")

	reservedFields := []string{"~version", "_id", "_test"}

	// ValidateKeyValue should return an error for a json value that contains one of the reserved fields
	// at the top level
	for _, reservedField := range reservedFields {
		testVal := fmt.Sprintf(`{"%s":"dummyVal"}`, reservedField)
		err = db.ValidateKeyValue("testKey", []byte(testVal))
		assert.Error(t, err, fmt.Sprintf(
			"ValidateKey should have thrown an error for a json value %s, as contains one of the reserved fields", testVal))
	}

	// ValidateKeyValue should not return an error for a json value that contains one of the reserved fields
	// if not at the top level
	for _, reservedField := range reservedFields {
		testVal := fmt.Sprintf(`{"data.%s":"dummyVal"}`, reservedField)
		err = db.ValidateKeyValue("testKey", []byte(testVal))
		assert.NoError(t, err, fmt.Sprintf(
			"ValidateKey should not have thrown an error the json value %s since the reserved field was not at the top level", testVal))
	}

	// ValidateKeyValue should return an error for a key that begins with an underscore
	err = db.ValidateKeyValue("_testKey", []byte("testValue"))
	assert.Error(t, err, "ValidateKey should have thrown an error for a key that begins with an underscore")

}

// TestInvalidJSONFields tests for invalid JSON fields
func TestInvalidJSONFields(t *testing.T) {

	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testinvalidfields")
	assert.NoError(t, err)

	db.Open()
	defer db.Close()

	batch := statedb.NewUpdateBatch()
	jsonValue1 := `{"_id":"key1","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint := version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	assert.Error(t, err, "Invalid field _id should have thrown an error")

	batch = statedb.NewUpdateBatch()
	jsonValue1 = `{"_rev":"rev1","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint = version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	assert.Error(t, err, "Invalid field _rev should have thrown an error")

	batch = statedb.NewUpdateBatch()
	jsonValue1 = `{"_deleted":"true","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint = version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	assert.Error(t, err, "Invalid field _deleted should have thrown an error")

	batch = statedb.NewUpdateBatch()
	jsonValue1 = `{"~version":"v1","asset_name":"marble1","color":"blue","size":1,"owner":"tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))

	savePoint = version.NewHeight(1, 2)
	err = db.ApplyUpdates(batch, savePoint)
	assert.Error(t, err, "Invalid field ~version should have thrown an error")
}

func TestDebugFunctions(t *testing.T) {

	//Test printCompositeKeys
	// initialize a key list
	loadKeys := []*statedb.CompositeKey{}
	//create a composite key and add to the key list
	compositeKey := statedb.CompositeKey{Namespace: "ns", Key: "key3"}
	loadKeys = append(loadKeys, &compositeKey)
	compositeKey = statedb.CompositeKey{Namespace: "ns", Key: "key4"}
	loadKeys = append(loadKeys, &compositeKey)
	assert.Equal(t, "[ns,key4],[ns,key4]", printCompositeKeys(loadKeys))

}

func TestHandleChaincodeDeploy(t *testing.T) {

	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testinit")
	assert.NoError(t, err)
	db.Open()
	defer db.Close()
	batch := statedb.NewUpdateBatch()

	jsonValue1 := `{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))
	jsonValue2 := `{"asset_name": "marble2","color": "blue","size": 2,"owner": "jerry"}`
	batch.Put("ns1", "key2", []byte(jsonValue2), version.NewHeight(1, 2))
	jsonValue3 := `{"asset_name": "marble3","color": "blue","size": 3,"owner": "fred"}`
	batch.Put("ns1", "key3", []byte(jsonValue3), version.NewHeight(1, 3))
	jsonValue4 := `{"asset_name": "marble4","color": "blue","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key4", []byte(jsonValue4), version.NewHeight(1, 4))
	jsonValue5 := `{"asset_name": "marble5","color": "blue","size": 5,"owner": "fred"}`
	batch.Put("ns1", "key5", []byte(jsonValue5), version.NewHeight(1, 5))
	jsonValue6 := `{"asset_name": "marble6","color": "blue","size": 6,"owner": "elaine"}`
	batch.Put("ns1", "key6", []byte(jsonValue6), version.NewHeight(1, 6))
	jsonValue7 := `{"asset_name": "marble7","color": "blue","size": 7,"owner": "fred"}`
	batch.Put("ns1", "key7", []byte(jsonValue7), version.NewHeight(1, 7))
	jsonValue8 := `{"asset_name": "marble8","color": "blue","size": 8,"owner": "elaine"}`
	batch.Put("ns1", "key8", []byte(jsonValue8), version.NewHeight(1, 8))
	jsonValue9 := `{"asset_name": "marble9","color": "green","size": 9,"owner": "fred"}`
	batch.Put("ns1", "key9", []byte(jsonValue9), version.NewHeight(1, 9))
	jsonValue10 := `{"asset_name": "marble10","color": "green","size": 10,"owner": "mary"}`
	batch.Put("ns1", "key10", []byte(jsonValue10), version.NewHeight(1, 10))
	jsonValue11 := `{"asset_name": "marble11","color": "cyan","size": 1000007,"owner": "joe"}`
	batch.Put("ns1", "key11", []byte(jsonValue11), version.NewHeight(1, 11))

	//add keys for a separate namespace
	batch.Put("ns2", "key1", []byte(jsonValue1), version.NewHeight(1, 12))
	batch.Put("ns2", "key2", []byte(jsonValue2), version.NewHeight(1, 13))
	batch.Put("ns2", "key3", []byte(jsonValue3), version.NewHeight(1, 14))
	batch.Put("ns2", "key4", []byte(jsonValue4), version.NewHeight(1, 15))
	batch.Put("ns2", "key5", []byte(jsonValue5), version.NewHeight(1, 16))
	batch.Put("ns2", "key6", []byte(jsonValue6), version.NewHeight(1, 17))
	batch.Put("ns2", "key7", []byte(jsonValue7), version.NewHeight(1, 18))
	batch.Put("ns2", "key8", []byte(jsonValue8), version.NewHeight(1, 19))
	batch.Put("ns2", "key9", []byte(jsonValue9), version.NewHeight(1, 20))
	batch.Put("ns2", "key10", []byte(jsonValue10), version.NewHeight(1, 21))

	savePoint := version.NewHeight(2, 22)
	db.ApplyUpdates(batch, savePoint)

	//Create a tar file for test with 4 index definitions and 2 side dbs
	dbArtifactsTarBytes := testutil.CreateTarBytesForTest(
		[]*testutil.TarFileEntry{
			{Name: "META-INF/statedb/couchdb/indexes/indexColorSortName.json", Body: `{"index":{"fields":[{"color":"desc"}]},"ddoc":"indexColorSortName","name":"indexColorSortName","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/indexes/indexSizeSortName.json", Body: `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/collections/collectionMarbles/indexes/indexCollMarbles.json", Body: `{"index":{"fields":["docType","owner"]},"ddoc":"indexCollectionMarbles", "name":"indexCollectionMarbles","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/collections/collectionMarblesPrivateDetails/indexes/indexCollPrivDetails.json", Body: `{"index":{"fields":["docType","price"]},"ddoc":"indexPrivateDetails", "name":"indexPrivateDetails","type":"json"}`},
		},
	)

	//Create a query
	queryString := `{"selector":{"owner":"fred"}}`

	_, err = db.ExecuteQuery("ns1", queryString)
	assert.NoError(t, err)

	//Create a query with a sort
	queryString = `{"selector":{"owner":"fred"}, "sort": [{"size": "desc"}]}`

	_, err = db.ExecuteQuery("ns1", queryString)
	assert.Error(t, err, "Error should have been thrown for a missing index")

	indexCapable, ok := db.(statedb.IndexCapable)

	if !ok {
		t.Fatalf("Couchdb state impl is expected to implement interface `statedb.IndexCapable`")
	}

	fileEntries, errExtract := ccprovider.ExtractFileEntries(dbArtifactsTarBytes, "couchdb")
	assert.NoError(t, errExtract)

	indexCapable.ProcessIndexesForChaincodeDeploy("ns1", fileEntries["META-INF/statedb/couchdb/indexes"])
	//Sleep to allow time for index creation
	time.Sleep(100 * time.Millisecond)
	//Create a query with a sort
	queryString = `{"selector":{"owner":"fred"}, "sort": [{"size": "desc"}]}`

	//Query should complete without error
	_, err = db.ExecuteQuery("ns1", queryString)
	assert.NoError(t, err)

	//Query namespace "ns2", index is only created in "ns1".  This should return an error.
	_, err = db.ExecuteQuery("ns2", queryString)
	assert.Error(t, err, "Error should have been thrown for a missing index")

}

func TestTryCastingToJSON(t *testing.T) {
	sampleJSON := []byte(`{"a":"A", "b":"B"}`)
	isJSON, jsonVal := tryCastingToJSON(sampleJSON)
	assert.True(t, isJSON)
	assert.Equal(t, "A", jsonVal["a"])
	assert.Equal(t, "B", jsonVal["b"])

	sampleNonJSON := []byte(`This is not a json`)
	isJSON, jsonVal = tryCastingToJSON(sampleNonJSON)
	assert.False(t, isJSON)
}

func TestHandleChaincodeDeployErroneousIndexFile(t *testing.T) {
	channelName := "ch1"
	env := NewTestVDBEnv(t)
	defer env.Cleanup()
	db, err := env.DBProvider.GetDBHandle(channelName)
	assert.NoError(t, err)
	db.Open()
	defer db.Close()

	batch := statedb.NewUpdateBatch()
	batch.Put("ns1", "key1", []byte(`{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`), version.NewHeight(1, 1))
	batch.Put("ns1", "key2", []byte(`{"asset_name": "marble2","color": "blue","size": 2,"owner": "jerry"}`), version.NewHeight(1, 2))

	// Create a tar file for test with 2 index definitions - one of them being errorneous
	badSyntaxFileContent := `{"index":{"fields": This is a bad json}`
	dbArtifactsTarBytes := testutil.CreateTarBytesForTest(
		[]*testutil.TarFileEntry{
			{Name: "META-INF/statedb/couchdb/indexes/indexSizeSortName.json", Body: `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`},
			{Name: "META-INF/statedb/couchdb/indexes/badSyntax.json", Body: badSyntaxFileContent},
		},
	)

	indexCapable, ok := db.(statedb.IndexCapable)
	if !ok {
		t.Fatalf("Couchdb state impl is expected to implement interface `statedb.IndexCapable`")
	}

	fileEntries, errExtract := ccprovider.ExtractFileEntries(dbArtifactsTarBytes, "couchdb")
	assert.NoError(t, errExtract)

	indexCapable.ProcessIndexesForChaincodeDeploy("ns1", fileEntries["META-INF/statedb/couchdb/indexes"])

	//Sleep to allow time for index creation
	time.Sleep(100 * time.Millisecond)
	//Query should complete without error
	_, err = db.ExecuteQuery("ns1", `{"selector":{"owner":"fred"}, "sort": [{"size": "desc"}]}`)
	assert.NoError(t, err)
}

func TestIsBulkOptimizable(t *testing.T) {
	var db statedb.VersionedDB = &VersionedDB{}
	_, ok := db.(statedb.BulkOptimizable)
	if !ok {
		t.Fatal("state couch db is expected to implement interface statedb.BulkOptimizable")
	}
}

func printCompositeKeys(keys []*statedb.CompositeKey) string {

	compositeKeyString := []string{}
	for _, key := range keys {
		compositeKeyString = append(compositeKeyString, "["+key.Namespace+","+key.Key+"]")
	}
	return strings.Join(compositeKeyString, ",")
}

// TestPaginatedQuery tests queries with pagination
func TestPaginatedQuery(t *testing.T) {

	env := NewTestVDBEnv(t)
	defer env.Cleanup()

	db, err := env.DBProvider.GetDBHandle("testpaginatedquery")
	assert.NoError(t, err)
	db.Open()
	defer db.Close()

	batch := statedb.NewUpdateBatch()
	jsonValue1 := `{"asset_name": "marble1","color": "blue","size": 1,"owner": "tom"}`
	batch.Put("ns1", "key1", []byte(jsonValue1), version.NewHeight(1, 1))
	jsonValue2 := `{"asset_name": "marble2","color": "red","size": 2,"owner": "jerry"}`
	batch.Put("ns1", "key2", []byte(jsonValue2), version.NewHeight(1, 2))
	jsonValue3 := `{"asset_name": "marble3","color": "red","size": 3,"owner": "fred"}`
	batch.Put("ns1", "key3", []byte(jsonValue3), version.NewHeight(1, 3))
	jsonValue4 := `{"asset_name": "marble4","color": "red","size": 4,"owner": "martha"}`
	batch.Put("ns1", "key4", []byte(jsonValue4), version.NewHeight(1, 4))
	jsonValue5 := `{"asset_name": "marble5","color": "blue","size": 5,"owner": "fred"}`
	batch.Put("ns1", "key5", []byte(jsonValue5), version.NewHeight(1, 5))
	jsonValue6 := `{"asset_name": "marble6","color": "red","size": 6,"owner": "elaine"}`
	batch.Put("ns1", "key6", []byte(jsonValue6), version.NewHeight(1, 6))
	jsonValue7 := `{"asset_name": "marble7","color": "blue","size": 7,"owner": "fred"}`
	batch.Put("ns1", "key7", []byte(jsonValue7), version.NewHeight(1, 7))
	jsonValue8 := `{"asset_name": "marble8","color": "red","size": 8,"owner": "elaine"}`
	batch.Put("ns1", "key8", []byte(jsonValue8), version.NewHeight(1, 8))
	jsonValue9 := `{"asset_name": "marble9","color": "green","size": 9,"owner": "fred"}`
	batch.Put("ns1", "key9", []byte(jsonValue9), version.NewHeight(1, 9))
	jsonValue10 := `{"asset_name": "marble10","color": "green","size": 10,"owner": "mary"}`
	batch.Put("ns1", "key10", []byte(jsonValue10), version.NewHeight(1, 10))

	jsonValue11 := `{"asset_name": "marble11","color": "cyan","size": 11,"owner": "joe"}`
	batch.Put("ns1", "key11", []byte(jsonValue11), version.NewHeight(1, 11))
	jsonValue12 := `{"asset_name": "marble12","color": "red","size": 12,"owner": "martha"}`
	batch.Put("ns1", "key12", []byte(jsonValue12), version.NewHeight(1, 4))
	jsonValue13 := `{"asset_name": "marble13","color": "red","size": 13,"owner": "james"}`
	batch.Put("ns1", "key13", []byte(jsonValue13), version.NewHeight(1, 4))
	jsonValue14 := `{"asset_name": "marble14","color": "red","size": 14,"owner": "fred"}`
	batch.Put("ns1", "key14", []byte(jsonValue14), version.NewHeight(1, 4))
	jsonValue15 := `{"asset_name": "marble15","color": "red","size": 15,"owner": "mary"}`
	batch.Put("ns1", "key15", []byte(jsonValue15), version.NewHeight(1, 4))
	jsonValue16 := `{"asset_name": "marble16","color": "red","size": 16,"owner": "robert"}`
	batch.Put("ns1", "key16", []byte(jsonValue16), version.NewHeight(1, 4))
	jsonValue17 := `{"asset_name": "marble17","color": "red","size": 17,"owner": "alan"}`
	batch.Put("ns1", "key17", []byte(jsonValue17), version.NewHeight(1, 4))
	jsonValue18 := `{"asset_name": "marble18","color": "red","size": 18,"owner": "elaine"}`
	batch.Put("ns1", "key18", []byte(jsonValue18), version.NewHeight(1, 4))
	jsonValue19 := `{"asset_name": "marble19","color": "red","size": 19,"owner": "alan"}`
	batch.Put("ns1", "key19", []byte(jsonValue19), version.NewHeight(1, 4))
	jsonValue20 := `{"asset_name": "marble20","color": "red","size": 20,"owner": "elaine"}`
	batch.Put("ns1", "key20", []byte(jsonValue20), version.NewHeight(1, 4))

	jsonValue21 := `{"asset_name": "marble21","color": "cyan","size": 21,"owner": "joe"}`
	batch.Put("ns1", "key21", []byte(jsonValue21), version.NewHeight(1, 11))
	jsonValue22 := `{"asset_name": "marble22","color": "red","size": 22,"owner": "martha"}`
	batch.Put("ns1", "key22", []byte(jsonValue22), version.NewHeight(1, 4))
	jsonValue23 := `{"asset_name": "marble23","color": "blue","size": 23,"owner": "james"}`
	batch.Put("ns1", "key23", []byte(jsonValue23), version.NewHeight(1, 4))
	jsonValue24 := `{"asset_name": "marble24","color": "red","size": 24,"owner": "fred"}`
	batch.Put("ns1", "key24", []byte(jsonValue24), version.NewHeight(1, 4))
	jsonValue25 := `{"asset_name": "marble25","color": "red","size": 25,"owner": "mary"}`
	batch.Put("ns1", "key25", []byte(jsonValue25), version.NewHeight(1, 4))
	jsonValue26 := `{"asset_name": "marble26","color": "red","size": 26,"owner": "robert"}`
	batch.Put("ns1", "key26", []byte(jsonValue26), version.NewHeight(1, 4))
	jsonValue27 := `{"asset_name": "marble27","color": "green","size": 27,"owner": "alan"}`
	batch.Put("ns1", "key27", []byte(jsonValue27), version.NewHeight(1, 4))
	jsonValue28 := `{"asset_name": "marble28","color": "red","size": 28,"owner": "elaine"}`
	batch.Put("ns1", "key28", []byte(jsonValue28), version.NewHeight(1, 4))
	jsonValue29 := `{"asset_name": "marble29","color": "red","size": 29,"owner": "alan"}`
	batch.Put("ns1", "key29", []byte(jsonValue29), version.NewHeight(1, 4))
	jsonValue30 := `{"asset_name": "marble30","color": "red","size": 30,"owner": "elaine"}`
	batch.Put("ns1", "key30", []byte(jsonValue30), version.NewHeight(1, 4))

	jsonValue31 := `{"asset_name": "marble31","color": "cyan","size": 31,"owner": "joe"}`
	batch.Put("ns1", "key31", []byte(jsonValue31), version.NewHeight(1, 11))
	jsonValue32 := `{"asset_name": "marble32","color": "red","size": 32,"owner": "martha"}`
	batch.Put("ns1", "key32", []byte(jsonValue32), version.NewHeight(1, 4))
	jsonValue33 := `{"asset_name": "marble33","color": "red","size": 33,"owner": "james"}`
	batch.Put("ns1", "key33", []byte(jsonValue33), version.NewHeight(1, 4))
	jsonValue34 := `{"asset_name": "marble34","color": "red","size": 34,"owner": "fred"}`
	batch.Put("ns1", "key34", []byte(jsonValue34), version.NewHeight(1, 4))
	jsonValue35 := `{"asset_name": "marble35","color": "red","size": 35,"owner": "mary"}`
	batch.Put("ns1", "key35", []byte(jsonValue35), version.NewHeight(1, 4))
	jsonValue36 := `{"asset_name": "marble36","color": "orange","size": 36,"owner": "robert"}`
	batch.Put("ns1", "key36", []byte(jsonValue36), version.NewHeight(1, 4))
	jsonValue37 := `{"asset_name": "marble37","color": "red","size": 37,"owner": "alan"}`
	batch.Put("ns1", "key37", []byte(jsonValue37), version.NewHeight(1, 4))
	jsonValue38 := `{"asset_name": "marble38","color": "yellow","size": 38,"owner": "elaine"}`
	batch.Put("ns1", "key38", []byte(jsonValue38), version.NewHeight(1, 4))
	jsonValue39 := `{"asset_name": "marble39","color": "red","size": 39,"owner": "alan"}`
	batch.Put("ns1", "key39", []byte(jsonValue39), version.NewHeight(1, 4))
	jsonValue40 := `{"asset_name": "marble40","color": "red","size": 40,"owner": "elaine"}`
	batch.Put("ns1", "key40", []byte(jsonValue40), version.NewHeight(1, 4))

	savePoint := version.NewHeight(2, 22)
	db.ApplyUpdates(batch, savePoint)

	// Create a tar file for test with an index for size
	dbArtifactsTarBytes := testutil.CreateTarBytesForTest(
		[]*testutil.TarFileEntry{
			{Name: "META-INF/statedb/couchdb/indexes/indexSizeSortName.json", Body: `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`},
		},
	)

	// Create a query
	queryString := `{"selector":{"color":"red"}}`

	_, err = db.ExecuteQuery("ns1", queryString)
	assert.NoError(t, err)

	// Create a query with a sort
	queryString = `{"selector":{"color":"red"}, "sort": [{"size": "asc"}]}`

	indexCapable, ok := db.(statedb.IndexCapable)

	if !ok {
		t.Fatalf("Couchdb state impl is expected to implement interface `statedb.IndexCapable`")
	}

	fileEntries, errExtract := ccprovider.ExtractFileEntries(dbArtifactsTarBytes, "couchdb")
	assert.NoError(t, errExtract)

	indexCapable.ProcessIndexesForChaincodeDeploy("ns1", fileEntries["META-INF/statedb/couchdb/indexes"])
	// Sleep to allow time for index creation
	time.Sleep(100 * time.Millisecond)
	// Create a query with a sort
	queryString = `{"selector":{"color":"red"}, "sort": [{"size": "asc"}]}`

	// Query should complete without error
	_, err = db.ExecuteQuery("ns1", queryString)
	assert.NoError(t, err)

	// Test explicit paging
	// Execute 3 page queries, there are 28 records with color red, use page size 10
	returnKeys := []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15", "key16"}
	bookmark, err := executeQuery(t, db, "ns1", queryString, "", int32(10), returnKeys)
	assert.NoError(t, err)
	returnKeys = []string{"key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29"}
	bookmark, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	assert.NoError(t, err)

	returnKeys = []string{"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40"}
	_, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	assert.NoError(t, err)

	// Test explicit paging
	// Increase pagesize to 50,  should return all values
	returnKeys = []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15",
		"key16", "key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29",
		"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40"}
	_, err = executeQuery(t, db, "ns1", queryString, "", int32(50), returnKeys)
	assert.NoError(t, err)

	//Set queryLimit to 50
	viper.Set("ledger.state.couchDBConfig.internalQueryLimit", 50)

	// Test explicit paging
	// Pagesize is 10, so all 28 records should be return in 3 "pages"
	returnKeys = []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15", "key16"}
	bookmark, err = executeQuery(t, db, "ns1", queryString, "", int32(10), returnKeys)
	assert.NoError(t, err)
	returnKeys = []string{"key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29"}
	bookmark, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	assert.NoError(t, err)
	returnKeys = []string{"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40"}
	_, err = executeQuery(t, db, "ns1", queryString, bookmark, int32(10), returnKeys)
	assert.NoError(t, err)

	// Set queryLimit to 10
	viper.Set("ledger.state.couchDBConfig.internalQueryLimit", 10)

	// Test implicit paging
	returnKeys = []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15",
		"key16", "key17", "key18", "key19", "key20", "key22", "key24", "key25", "key26", "key28", "key29",
		"key30", "key32", "key33", "key34", "key35", "key37", "key39", "key40"}
	_, err = executeQuery(t, db, "ns1", queryString, "", int32(0), returnKeys)
	assert.NoError(t, err)

	//Set queryLimit to 5
	viper.Set("ledger.state.couchDBConfig.internalQueryLimit", 5)

	// pagesize greater than querysize will execute with implicit paging
	returnKeys = []string{"key2", "key3", "key4", "key6", "key8", "key12", "key13", "key14", "key15", "key16"}
	_, err = executeQuery(t, db, "ns1", queryString, "", int32(10), returnKeys)
	assert.NoError(t, err)

	// Set queryLimit to 1000
	viper.Set("ledger.state.couchDBConfig.internalQueryLimit", 1000)
}

func executeQuery(t *testing.T, db statedb.VersionedDB, namespace, query, bookmark string, limit int32, returnKeys []string) (string, error) {

	var itr statedb.ResultsIterator
	var err error

	if limit == int32(0) && bookmark == "" {
		itr, err = db.ExecuteQuery(namespace, query)
		if err != nil {
			return "", err
		}
	} else {
		queryOptions := make(map[string]interface{})
		if bookmark != "" {
			queryOptions["bookmark"] = bookmark
		}
		if limit != 0 {
			queryOptions["limit"] = limit
		}

		itr, err = db.ExecuteQueryWithMetadata(namespace, query, queryOptions)
		if err != nil {
			return "", err
		}
	}

	// Verify the keys returned
	commontests.TestItrWithoutClose(t, itr, returnKeys)

	returnBookmark := ""
	if queryResultItr, ok := itr.(statedb.QueryResultsIterator); ok {
		returnBookmark = queryResultItr.GetBookmarkAndClose()
	}

	return returnBookmark, nil
}

// TestPaginatedQueryValidation tests queries with pagination
func TestPaginatedQueryValidation(t *testing.T) {

	queryOptions := make(map[string]interface{})
	queryOptions["bookmark"] = "Test1"
	queryOptions["limit"] = int32(10)

	err := validateQueryMetadata(queryOptions)
	assert.NoError(t, err, "An error was thrown for a valid options")
	queryOptions = make(map[string]interface{})
	queryOptions["bookmark"] = "Test1"
	queryOptions["limit"] = float64(10.2)

	err = validateQueryMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid options")

	queryOptions = make(map[string]interface{})
	queryOptions["bookmark"] = "Test1"
	queryOptions["limit"] = "10"

	err = validateQueryMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid options")

	queryOptions = make(map[string]interface{})
	queryOptions["bookmark"] = int32(10)
	queryOptions["limit"] = "10"

	err = validateQueryMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid options")

	queryOptions = make(map[string]interface{})
	queryOptions["bookmark"] = "Test1"
	queryOptions["limit1"] = int32(10)

	err = validateQueryMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid options")

	queryOptions = make(map[string]interface{})
	queryOptions["bookmark1"] = "Test1"
	queryOptions["limit1"] = int32(10)

	err = validateQueryMetadata(queryOptions)
	assert.Error(t, err, "An should have been thrown for an invalid options")

}
