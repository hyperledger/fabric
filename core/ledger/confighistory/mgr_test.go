/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package confighistory

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io/ioutil"
	"math"
	"os"
	"path"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/ledger/snapshot"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

var (
	testNewHashFunc = func() (hash.Hash, error) {
		return sha256.New(), nil
	}
)

func TestMain(m *testing.M) {
	flogging.ActivateSpec("confighistory=debug")
	os.Exit(m.Run())
}

func TestWithNoCollectionConfig(t *testing.T) {
	dbPath, err := ioutil.TempDir("", "confighistory")
	if err != nil {
		t.Fatalf("Failed to create config history directory: %s", err)
	}
	defer os.RemoveAll(dbPath)
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mgr, err := NewMgr(dbPath, mockCCInfoProvider)
	require.NoError(t, err)
	testutilEquipMockCCInfoProviderToReturnDesiredCollConfig(mockCCInfoProvider, "chaincode1", nil)
	err = mgr.HandleStateUpdates(&ledger.StateUpdateTrigger{
		LedgerID:           "ledger1",
		CommittingBlockNum: 50},
	)
	require.NoError(t, err)
	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
		info: &common.BlockchainInfo{Height: 100},
		qe:   &mock.QueryExecutor{},
	}
	retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
	collConfig, err := retriever.MostRecentCollectionConfigBelow(90, "chaincode1")
	require.NoError(t, err)
	require.Nil(t, collConfig)
}

func TestWithEmptyCollectionConfig(t *testing.T) {
	dbPath, err := ioutil.TempDir("", "confighistory")
	if err != nil {
		t.Fatalf("Failed to create config history directory: %s", err)
	}
	defer os.RemoveAll(dbPath)
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mgr, err := NewMgr(dbPath, mockCCInfoProvider)
	require.NoError(t, err)
	testutilEquipMockCCInfoProviderToReturnDesiredCollConfig(
		mockCCInfoProvider,
		"chaincode1",
		&peer.CollectionConfigPackage{},
	)
	err = mgr.HandleStateUpdates(&ledger.StateUpdateTrigger{
		LedgerID:           "ledger1",
		CommittingBlockNum: 50},
	)
	require.NoError(t, err)
	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
		info: &common.BlockchainInfo{Height: 100},
		qe:   &mock.QueryExecutor{},
	}
	retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
	collConfig, err := retriever.MostRecentCollectionConfigBelow(90, "chaincode1")
	require.NoError(t, err)
	require.Nil(t, collConfig)
}

func TestMgr(t *testing.T) {
	dbPath, err := ioutil.TempDir("", "confighistory")
	if err != nil {
		t.Fatalf("Failed to create config history directory: %s", err)
	}
	defer os.RemoveAll(dbPath)
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mgr, err := NewMgr(dbPath, mockCCInfoProvider)
	require.NoError(t, err)
	chaincodeName := "chaincode1"
	maxBlockNumberInLedger := uint64(2000)
	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
		info: &common.BlockchainInfo{Height: maxBlockNumberInLedger + 1},
		qe:   &mock.QueryExecutor{},
	}
	configCommittingBlockNums := []uint64{5, 10, 15, 100}
	ledgerIds := []string{"ledgerid1", "ledger2"}

	// Populate collection config versions
	for _, ledgerid := range ledgerIds {
		for _, committingBlockNum := range configCommittingBlockNums {
			// for each ledgerid and commitHeight combination, construct a unique collConfigPackage and induce a stateUpdate
			collConfigPackage := sampleCollectionConfigPackage(ledgerid, committingBlockNum)
			testutilEquipMockCCInfoProviderToReturnDesiredCollConfig(mockCCInfoProvider, chaincodeName, collConfigPackage)
			mgr.HandleStateUpdates(&ledger.StateUpdateTrigger{
				LedgerID:           ledgerid,
				CommittingBlockNum: committingBlockNum},
			)
		}
	}

	t.Run("test-api-MostRecentCollectionConfigBelow()", func(t *testing.T) {
		// A map that contains entries such that for each of the entries of type <K, V>,
		// we retrieve the 'MostRecentCollectionConfigBelow' for 'K' and the expected value
		// should be configuration committed at 'V'
		m := map[uint64]uint64{math.MaxUint64: 100, 1000: 100, 50: 15, 12: 10, 7: 5}
		for _, ledgerid := range ledgerIds {
			retriever := mgr.GetRetriever(ledgerid, dummyLedgerInfoRetriever)
			for testHeight, expectedHeight := range m {
				retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(testHeight, chaincodeName)
				require.NoError(t, err)
				expectedConfig := sampleCollectionConfigPackage(ledgerid, expectedHeight)
				require.Equal(t, expectedConfig, retrievedConfig.CollectionConfig)
				require.Equal(t, expectedHeight, retrievedConfig.CommittingBlockNum)
			}

			retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(5, chaincodeName)
			require.NoError(t, err)
			require.Nil(t, retrievedConfig)
		}
	})

	t.Run("test-api-CollectionConfigAt()", func(t *testing.T) {
		for _, ledgerid := range ledgerIds {
			retriever := mgr.GetRetriever(ledgerid, dummyLedgerInfoRetriever)
			for _, commitHeight := range configCommittingBlockNums {
				retrievedConfig, err := retriever.CollectionConfigAt(commitHeight, chaincodeName)
				require.NoError(t, err)
				expectedConfig := sampleCollectionConfigPackage(ledgerid, commitHeight)
				require.Equal(t, expectedConfig, retrievedConfig.CollectionConfig)
				require.Equal(t, commitHeight, retrievedConfig.CommittingBlockNum)
			}
		}
	})

	t.Run("test-api-CollectionConfigAt-BoundaryCases()", func(t *testing.T) {
		retriever := mgr.GetRetriever("ledgerid1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.CollectionConfigAt(4, chaincodeName)
		require.NoError(t, err)
		require.Nil(t, retrievedConfig)

		_, err = retriever.CollectionConfigAt(5000, chaincodeName)
		typedErr, ok := err.(*ledger.ErrCollectionConfigNotYetAvailable)
		require.True(t, ok)
		require.Equal(t, maxBlockNumberInLedger, typedErr.MaxBlockNumCommitted)
	})
}

func TestWithImplicitColls(t *testing.T) {
	dbPath, err := ioutil.TempDir("", "confighistory")
	if err != nil {
		t.Fatalf("Failed to create config history directory: %s", err)
	}
	defer os.RemoveAll(dbPath)
	collConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-coll-1", "Explicit-coll-2"})
	mockCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockCCInfoProvider.ImplicitCollectionsReturns(
		[]*peer.StaticCollectionConfig{
			{
				Name: "Implicit-coll-1",
			},
			{
				Name: "Implicit-coll-2",
			},
		},
		nil,
	)
	p, err := newDBProvider(dbPath)
	require.NoError(t, err)

	mgr := &mgr{
		ccInfoProvider: mockCCInfoProvider,
		dbProvider:     p,
	}

	// add explicit collections at height 20
	batch, err := prepareDBBatch(
		map[string]*peer.CollectionConfigPackage{
			"chaincode1": collConfigPackage,
		},
		20,
	)
	require.NoError(t, err)
	dbHandle := mgr.dbProvider.getDB("ledger1")
	require.NoError(t, dbHandle.writeBatch(batch, true))

	onlyImplicitCollections := testutilCreateCollConfigPkg(
		[]string{"Implicit-coll-1", "Implicit-coll-2"},
	)

	explicitAndImplicitCollections := testutilCreateCollConfigPkg(
		[]string{"Explicit-coll-1", "Explicit-coll-2", "Implicit-coll-1", "Implicit-coll-2"},
	)

	dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
		info: &common.BlockchainInfo{Height: 1000},
		qe:   &mock.QueryExecutor{},
	}

	t.Run("CheckQueryExecutorCalls", func(t *testing.T) {
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		// function MostRecentCollectionConfigBelow calls Done on query executor
		_, err := retriever.MostRecentCollectionConfigBelow(50, "chaincode1")
		require.NoError(t, err)
		require.Equal(t, 1, dummyLedgerInfoRetriever.qe.DoneCallCount())
		// function CollectionConfigAt calls Done on query executor
		_, err = retriever.CollectionConfigAt(50, "chaincode1")
		require.NoError(t, err)
		require.Equal(t, 2, dummyLedgerInfoRetriever.qe.DoneCallCount())
	})

	t.Run("MostRecentCollectionConfigBelow50", func(t *testing.T) {
		// explicit collections added at height 20 should be merged with the implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(50, "chaincode1")
		require.NoError(t, err)
		require.True(t, proto.Equal(retrievedConfig.CollectionConfig, explicitAndImplicitCollections))
	})

	t.Run("MostRecentCollectionConfigBelow10", func(t *testing.T) {
		// No explicit collections below height 10, should return only implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.MostRecentCollectionConfigBelow(10, "chaincode1")
		require.NoError(t, err)
		require.True(t, proto.Equal(retrievedConfig.CollectionConfig, onlyImplicitCollections))
	})

	t.Run("CollectionConfigAt50", func(t *testing.T) {
		// No explicit collections at height 50, should return only implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.CollectionConfigAt(50, "chaincode1")
		require.NoError(t, err)
		require.True(t, proto.Equal(retrievedConfig.CollectionConfig, onlyImplicitCollections))
	})

	t.Run("CollectionConfigAt20", func(t *testing.T) {
		// Explicit collections at height 20, should be merged with implicit collections
		retriever := mgr.GetRetriever("ledger1", dummyLedgerInfoRetriever)
		retrievedConfig, err := retriever.CollectionConfigAt(20, "chaincode1")
		require.NoError(t, err)
		require.True(t, proto.Equal(retrievedConfig.CollectionConfig, explicitAndImplicitCollections))
	})

}

type testEnvForSnapshot struct {
	mgr             *mgr
	retriever       *Retriever
	testSnapshotDir string
	cleanup         func()
}

func newTestEnvForSnapshot(t *testing.T) *testEnvForSnapshot {
	dbPath, err := ioutil.TempDir("", "confighistory")
	require.NoError(t, err)
	p, err := newDBProvider(dbPath)
	if err != nil {
		os.RemoveAll(dbPath)
		t.Fatalf("Failed to create new leveldb provider: %s", err)
	}
	mgr := &mgr{dbProvider: p}
	retriever := mgr.GetRetriever("ledger1", nil)

	testSnapshotDir, err := ioutil.TempDir("", "confighistorysnapshot")
	if err != nil {
		os.RemoveAll(dbPath)
		t.Fatalf("Failed to create config history snapshot directory: %s", err)
	}
	return &testEnvForSnapshot{
		mgr:             mgr,
		retriever:       retriever,
		testSnapshotDir: testSnapshotDir,
		cleanup: func() {
			os.RemoveAll(dbPath)
			os.RemoveAll(testSnapshotDir)
		},
	}
}

func TestExportConfigHistory(t *testing.T) {
	env := newTestEnvForSnapshot(t)
	defer env.cleanup()

	// config history database is empty
	fileHashes, err := env.retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.NoError(t, err)
	require.Empty(t, fileHashes)
	files, err := ioutil.ReadDir(env.testSnapshotDir)
	require.NoError(t, err)
	require.Len(t, files, 0)

	// config history database has 3 chaincodes each with 1 collection config entry in the
	// collectionConfigNamespace
	dbHandle := env.mgr.dbProvider.getDB("ledger1")
	cc1collConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc1-coll-1", "Explicit-cc1-coll-2"})
	cc2collConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc2-coll-1", "Explicit-cc2-coll-2"})
	cc3collConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc3-coll-1", "Explicit-cc3-coll-2"})
	batch, err := prepareDBBatch(
		map[string]*peer.CollectionConfigPackage{
			"chaincode1": cc1collConfigPackage,
			"chaincode2": cc2collConfigPackage,
			"chaincode3": cc3collConfigPackage,
		},
		50,
	)
	require.NoError(t, err)
	require.NoError(t, dbHandle.writeBatch(batch, true))

	fileHashes, err = env.retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.NoError(t, err)
	cc1configBytes, err := proto.Marshal(cc1collConfigPackage)
	require.NoError(t, err)
	cc2configBytes, err := proto.Marshal(cc2collConfigPackage)
	require.NoError(t, err)
	cc3configBytes, err := proto.Marshal(cc3collConfigPackage)
	require.NoError(t, err)
	expectedCollectionConfigs := []*compositeKV{
		{&compositeKey{ns: "lscc", key: "chaincode1~collection", blockNum: 50}, cc1configBytes},
		{&compositeKey{ns: "lscc", key: "chaincode2~collection", blockNum: 50}, cc2configBytes},
		{&compositeKey{ns: "lscc", key: "chaincode3~collection", blockNum: 50}, cc3configBytes},
	}
	verifyExportedConfigHistory(t, env.testSnapshotDir, fileHashes, expectedCollectionConfigs)
	os.Remove(path.Join(env.testSnapshotDir, snapshotDataFileName))
	os.Remove(path.Join(env.testSnapshotDir, snapshotMetadataFileName))

	// config history database has 3 chaincodes each with 2 collection config entries in the
	// collectionConfigNamespace
	cc1collConfigPackageNew := testutilCreateCollConfigPkg([]string{"Explicit-cc1-coll-1", "Explicit-cc1-coll-2", "Explicit-cc1-coll-3"})
	cc2collConfigPackageNew := testutilCreateCollConfigPkg([]string{"Explicit-cc2-coll-1", "Explicit-cc2-coll-2", "Explicit-cc2-coll-3"})
	cc3collConfigPackageNew := testutilCreateCollConfigPkg([]string{"Explicit-cc3-coll-1", "Explicit-cc3-coll-2", "Explicit-cc3-coll-3"})
	batch, err = prepareDBBatch(
		map[string]*peer.CollectionConfigPackage{
			"chaincode1": cc1collConfigPackageNew,
			"chaincode2": cc2collConfigPackageNew,
			"chaincode3": cc3collConfigPackageNew,
		},
		100,
	)
	require.NoError(t, err)
	require.NoError(t, dbHandle.writeBatch(batch, true))

	fileHashes, err = env.retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.NoError(t, err)

	cc1configBytesNew, err := proto.Marshal(cc1collConfigPackageNew)
	require.NoError(t, err)
	cc2configBytesNew, err := proto.Marshal(cc2collConfigPackageNew)
	require.NoError(t, err)
	cc3configBytesNew, err := proto.Marshal(cc3collConfigPackageNew)
	require.NoError(t, err)
	expectedCollectionConfigs = []*compositeKV{
		{&compositeKey{ns: "lscc", key: "chaincode1~collection", blockNum: 100}, cc1configBytesNew},
		{&compositeKey{ns: "lscc", key: "chaincode1~collection", blockNum: 50}, cc1configBytes},
		{&compositeKey{ns: "lscc", key: "chaincode2~collection", blockNum: 100}, cc2configBytesNew},
		{&compositeKey{ns: "lscc", key: "chaincode2~collection", blockNum: 50}, cc2configBytes},
		{&compositeKey{ns: "lscc", key: "chaincode3~collection", blockNum: 100}, cc3configBytesNew},
		{&compositeKey{ns: "lscc", key: "chaincode3~collection", blockNum: 50}, cc3configBytes},
	}
	verifyExportedConfigHistory(t, env.testSnapshotDir, fileHashes, expectedCollectionConfigs)
}

func verifyExportedConfigHistory(t *testing.T, dir string, fileHashes map[string][]byte, expectedCollectionConfigs []*compositeKV) {
	require.Len(t, fileHashes, 2)
	require.Contains(t, fileHashes, snapshotDataFileName)
	require.Contains(t, fileHashes, snapshotMetadataFileName)

	dataFile := path.Join(dir, snapshotDataFileName)
	dataFileContent, err := ioutil.ReadFile(dataFile)
	require.NoError(t, err)
	dataFileHash := sha256.Sum256(dataFileContent)
	require.Equal(t, dataFileHash[:], fileHashes[snapshotDataFileName])

	metadataFile := path.Join(dir, snapshotMetadataFileName)
	metadataFileContent, err := ioutil.ReadFile(metadataFile)
	require.NoError(t, err)
	metadataFileHash := sha256.Sum256(metadataFileContent)
	require.Equal(t, metadataFileHash[:], fileHashes[snapshotMetadataFileName])

	metadataReader, err := snapshot.OpenFile(metadataFile, snapshotFileFormat)
	require.NoError(t, err)
	defer metadataReader.Close()

	dataReader, err := snapshot.OpenFile(dataFile, snapshotFileFormat)
	require.NoError(t, err)
	defer dataReader.Close()

	numCollectionConfigs, err := metadataReader.DecodeUVarInt()
	require.NoError(t, err)

	var retrievedCollectionConfigs []*compositeKV
	for i := uint64(0); i < numCollectionConfigs; i++ {
		key, err := dataReader.DecodeBytes()
		require.NoError(t, err)
		val, err := dataReader.DecodeBytes()
		require.NoError(t, err)
		retrievedCollectionConfigs = append(retrievedCollectionConfigs,
			&compositeKV{decodeCompositeKey(key), val},
		)
	}
	require.Equal(t, expectedCollectionConfigs, retrievedCollectionConfigs)
}

func TestExportConfigHistoryErrorCase(t *testing.T) {
	env := newTestEnvForSnapshot(t)
	defer env.cleanup()

	dbHandle := env.mgr.dbProvider.getDB("ledger1")
	cc1collConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc1-coll-1", "Explicit-cc1-coll-2"})
	batch, err := prepareDBBatch(
		map[string]*peer.CollectionConfigPackage{
			"chaincode1": cc1collConfigPackage,
		},
		50,
	)
	require.NoError(t, err)
	require.NoError(t, dbHandle.writeBatch(batch, true))

	// error during data file creation
	dataFilePath := path.Join(env.testSnapshotDir, snapshotDataFileName)
	_, err = os.Create(dataFilePath)
	require.NoError(t, err)

	_, err = env.retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+dataFilePath)
	os.RemoveAll(env.testSnapshotDir)

	// error during metadata file creation
	require.NoError(t, os.MkdirAll(env.testSnapshotDir, 0700))
	metadataFilePath := path.Join(env.testSnapshotDir, snapshotMetadataFileName)
	_, err = os.Create(metadataFilePath)
	require.NoError(t, err)
	_, err = env.retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+metadataFilePath)
	os.RemoveAll(env.testSnapshotDir)

	// error while reading from leveldb
	require.NoError(t, os.MkdirAll(env.testSnapshotDir, 0700))
	env.mgr.dbProvider.Close()
	_, err = env.retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
	os.RemoveAll(env.testSnapshotDir)
}

func sampleCollectionConfigPackage(collNamePart1 string, collNamePart2 uint64) *peer.CollectionConfigPackage {
	collName := fmt.Sprintf("%s-%d", collNamePart1, collNamePart2)
	return testutilCreateCollConfigPkg([]string{collName})
}

func testutilEquipMockCCInfoProviderToReturnDesiredCollConfig(
	mockCCInfoProvider *mock.DeployedChaincodeInfoProvider,
	chaincodeName string,
	collConfigPackage *peer.CollectionConfigPackage) {
	mockCCInfoProvider.UpdatedChaincodesReturns(
		[]*ledger.ChaincodeLifecycleInfo{
			{Name: chaincodeName},
		},
		nil,
	)
	mockCCInfoProvider.ChaincodeInfoReturns(
		&ledger.DeployedChaincodeInfo{Name: chaincodeName, ExplicitCollectionConfigPkg: collConfigPackage},
		nil,
	)
}

func testutilCreateCollConfigPkg(collNames []string) *peer.CollectionConfigPackage {
	pkg := &peer.CollectionConfigPackage{
		Config: []*peer.CollectionConfig{},
	}
	for _, collName := range collNames {
		pkg.Config = append(pkg.Config,
			&peer.CollectionConfig{
				Payload: &peer.CollectionConfig_StaticCollectionConfig{
					StaticCollectionConfig: &peer.StaticCollectionConfig{
						Name: collName,
					},
				},
			},
		)
	}
	return pkg
}

type dummyLedgerInfoRetriever struct {
	info *common.BlockchainInfo
	qe   *mock.QueryExecutor
}

func (d *dummyLedgerInfoRetriever) GetBlockchainInfo() (*common.BlockchainInfo, error) {
	return d.info, nil
}

func (d *dummyLedgerInfoRetriever) NewQueryExecutor() (ledger.QueryExecutor, error) {
	return d.qe, nil
}
