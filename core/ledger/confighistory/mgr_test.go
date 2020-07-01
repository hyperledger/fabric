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
	"path/filepath"
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

	mgr := &Mgr{
		ccInfoProvider: mockCCInfoProvider,
		dbProvider:     p,
	}
	dbHandle := mgr.dbProvider.getDB("ledger1")
	batch := dbHandle.newBatch()
	// add explicit collections at height 20
	err = prepareDBBatch(
		batch,
		map[string]*peer.CollectionConfigPackage{
			"chaincode1": collConfigPackage,
		},
		20,
	)
	require.NoError(t, err)
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
	mgr             *Mgr
	testSnapshotDir string
	cleanup         func()
}

func newTestEnvForSnapshot(t *testing.T) *testEnvForSnapshot {
	dbPath, err := ioutil.TempDir("", "confighistory")
	require.NoError(t, err)
	mgr, err := NewMgr(dbPath, &mock.DeployedChaincodeInfoProvider{})
	if err != nil {
		os.RemoveAll(dbPath)
		t.Fatalf("Failed to create new config history manager: %s", err)
	}

	testSnapshotDir, err := ioutil.TempDir("", "confighistorysnapshot")
	if err != nil {
		os.RemoveAll(dbPath)
		t.Fatalf("Failed to create config history snapshot directory: %s", err)
	}
	return &testEnvForSnapshot{
		mgr:             mgr,
		testSnapshotDir: testSnapshotDir,
		cleanup: func() {
			os.RemoveAll(dbPath)
			os.RemoveAll(testSnapshotDir)
		},
	}
}

func TestExportAndImportConfigHistory(t *testing.T) {
	env := newTestEnvForSnapshot(t)
	defer env.cleanup()

	setup := func(ledgerID string) ([]*compositeKV, map[string][]*ledger.CollectionConfigInfo) {
		cc1CollConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc1-coll-1", "Explicit-cc1-coll-2"})
		cc2CollConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc2-coll-1", "Explicit-cc2-coll-2"})
		cc3CollConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc3-coll-1", "Explicit-cc3-coll-2"})
		cc1CollConfigPackageNew := testutilCreateCollConfigPkg([]string{"Explicit-cc1-coll-1", "Explicit-cc1-coll-2", "Explicit-cc1-coll-3"})
		cc2CollConfigPackageNew := testutilCreateCollConfigPkg([]string{"Explicit-cc2-coll-1", "Explicit-cc2-coll-2", "Explicit-cc2-coll-3"})
		cc3CollConfigPackageNew := testutilCreateCollConfigPkg([]string{"Explicit-cc3-coll-1", "Explicit-cc3-coll-2", "Explicit-cc3-coll-3"})

		ccConfigInfo := map[string][]*ledger.CollectionConfigInfo{
			"chaincode1": {
				{
					CollectionConfig:   cc1CollConfigPackage,
					CommittingBlockNum: 50,
				},
				{
					CollectionConfig:   cc1CollConfigPackageNew,
					CommittingBlockNum: 100,
				},
			},
			"chaincode2": {
				{
					CollectionConfig:   cc2CollConfigPackage,
					CommittingBlockNum: 50,
				},
				{
					CollectionConfig:   cc2CollConfigPackageNew,
					CommittingBlockNum: 100,
				},
			},
			"chaincode3": {
				{
					CollectionConfig:   cc3CollConfigPackage,
					CommittingBlockNum: 50,
				},
				{
					CollectionConfig:   cc3CollConfigPackageNew,
					CommittingBlockNum: 100,
				},
			},
		}

		db := env.mgr.dbProvider.getDB(ledgerID)
		batch := db.newBatch()
		err := prepareDBBatch(
			batch,
			map[string]*peer.CollectionConfigPackage{
				"chaincode1": cc1CollConfigPackage,
				"chaincode2": cc2CollConfigPackage,
				"chaincode3": cc3CollConfigPackage,
			},
			50,
		)
		require.NoError(t, err)
		require.NoError(t, db.writeBatch(batch, true))

		batch = db.newBatch()
		err = prepareDBBatch(
			batch,
			map[string]*peer.CollectionConfigPackage{
				"chaincode1": cc1CollConfigPackageNew,
				"chaincode2": cc2CollConfigPackageNew,
				"chaincode3": cc3CollConfigPackageNew,
			},
			100,
		)
		require.NoError(t, err)
		require.NoError(t, db.writeBatch(batch, true))

		cc1configBytes, err := proto.Marshal(cc1CollConfigPackage)
		require.NoError(t, err)
		cc2configBytes, err := proto.Marshal(cc2CollConfigPackage)
		require.NoError(t, err)
		cc3configBytes, err := proto.Marshal(cc3CollConfigPackage)
		require.NoError(t, err)
		cc1configBytesNew, err := proto.Marshal(cc1CollConfigPackageNew)
		require.NoError(t, err)
		cc2configBytesNew, err := proto.Marshal(cc2CollConfigPackageNew)
		require.NoError(t, err)
		cc3configBytesNew, err := proto.Marshal(cc3CollConfigPackageNew)
		require.NoError(t, err)

		storedKVs := []*compositeKV{
			{&compositeKey{ns: "lscc", key: "chaincode1~collection", blockNum: 100}, cc1configBytesNew},
			{&compositeKey{ns: "lscc", key: "chaincode1~collection", blockNum: 50}, cc1configBytes},
			{&compositeKey{ns: "lscc", key: "chaincode2~collection", blockNum: 100}, cc2configBytesNew},
			{&compositeKey{ns: "lscc", key: "chaincode2~collection", blockNum: 50}, cc2configBytes},
			{&compositeKey{ns: "lscc", key: "chaincode3~collection", blockNum: 100}, cc3configBytesNew},
			{&compositeKey{ns: "lscc", key: "chaincode3~collection", blockNum: 50}, cc3configBytes},
		}

		return storedKVs, ccConfigInfo
	}

	cleanup := func() {
		require.NoError(t, os.RemoveAll(filepath.Join(env.testSnapshotDir, snapshotDataFileName)))
		require.NoError(t, os.RemoveAll(filepath.Join(env.testSnapshotDir, snapshotMetadataFileName)))
	}

	t.Run("confighistory is empty", func(t *testing.T) {
		retriever := env.mgr.GetRetriever("ledger0", nil)
		fileHashes, err := retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
		require.NoError(t, err)
		require.Empty(t, fileHashes)
		files, err := ioutil.ReadDir(env.testSnapshotDir)
		require.NoError(t, err)
		require.Len(t, files, 0)
	})

	t.Run("export confighistory", func(t *testing.T) {
		// setup ledger1 => export ledger1
		storedKVs, _ := setup("ledger1")
		defer cleanup()
		retriever := env.mgr.GetRetriever("ledger1", nil)
		fileHashes, err := retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
		require.NoError(t, err)
		verifyExportedConfigHistory(t, env.testSnapshotDir, fileHashes, storedKVs)
	})

	t.Run("import confighistory and verify queries", func(t *testing.T) {
		// setup ledger2 => export ledger2 => import into ledger3
		_, ccConfigInfo := setup("ledger2")
		defer cleanup()
		retriever := env.mgr.GetRetriever("ledger2", nil)
		_, err := retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
		require.NoError(t, err)

		importConfigsBatchSize = 100
		require.NoError(t, env.mgr.ImportConfigHistory("ledger3", env.testSnapshotDir))
		dummyLedgerInfoRetriever := &dummyLedgerInfoRetriever{
			info: &common.BlockchainInfo{Height: 1000},
			qe:   &mock.QueryExecutor{},
		}

		retriever = env.mgr.GetRetriever("ledger3", dummyLedgerInfoRetriever)
		verifyImportedConfigHistory(t, retriever, ccConfigInfo)
	})

	t.Run("export from an imported confighistory", func(t *testing.T) {
		// setup ledger4 => export ledger4 => import into ledger5 => export ledger5
		storedKVs, _ := setup("ledger4")
		defer cleanup()
		retriever := env.mgr.GetRetriever("ledger4", nil)
		_, err := retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
		require.NoError(t, err)

		importConfigsBatchSize = 100
		require.NoError(t, env.mgr.ImportConfigHistory("ledger5", env.testSnapshotDir))
		cleanup()

		retriever = env.mgr.GetRetriever("ledger5", nil)
		fileHashes, err := retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
		require.NoError(t, err)
		verifyExportedConfigHistory(t, env.testSnapshotDir, fileHashes, storedKVs)
	})

	t.Run("import confighistory error cases", func(t *testing.T) {
		setup("ledger6")
		defer cleanup()
		retriever := env.mgr.GetRetriever("ledger6", nil)
		_, err := retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
		require.NoError(t, err)

		err = env.mgr.ImportConfigHistory("ledger6", env.testSnapshotDir)
		expectedErrStr := "config history for ledger [ledger6] exists. Incremental import is not supported. Remove the existing ledger data before retry"
		require.EqualError(t, err, expectedErrStr)

		require.NoError(t, os.RemoveAll(filepath.Join(env.testSnapshotDir, snapshotDataFileName)))
		err = env.mgr.ImportConfigHistory("ledger8", env.testSnapshotDir)
		require.Contains(t, err.Error(), "confighistory.data: no such file or directory")

		dataFileWriter, err := snapshot.CreateFile(filepath.Join(env.testSnapshotDir, snapshotDataFileName), snapshotFileFormat, testNewHashFunc)
		require.NoError(t, err)
		defer dataFileWriter.Close()
		require.NoError(t, dataFileWriter.EncodeUVarint(1))
		err = env.mgr.ImportConfigHistory("ledger8", env.testSnapshotDir)
		require.Contains(t, err.Error(), "error while reading from the snapshot file")
		require.Contains(t, err.Error(), "confighistory.data: EOF")

		require.NoError(t, os.RemoveAll(filepath.Join(env.testSnapshotDir, snapshotMetadataFileName)))
		err = env.mgr.ImportConfigHistory("ledger8", env.testSnapshotDir)
		require.Contains(t, err.Error(), "confighistory.metadata: no such file or directory")

		dataFileWriter, err = snapshot.CreateFile(filepath.Join(env.testSnapshotDir, snapshotMetadataFileName), snapshotFileFormat, testNewHashFunc)
		require.NoError(t, err)
		defer dataFileWriter.Close()
		require.NoError(t, dataFileWriter.EncodeBytes([]byte("junk")))
		err = env.mgr.ImportConfigHistory("ledger8", env.testSnapshotDir)
		require.Contains(t, err.Error(), "error while reading from the snapshot file")
		require.Contains(t, err.Error(), "confighistory.metadata: EOF")

		env.mgr.dbProvider.Close()
		err = env.mgr.ImportConfigHistory("ledger8", env.testSnapshotDir)
		require.EqualError(t, err, "internal leveldb error while obtaining db iterator: leveldb: closed")
	})
}

func verifyExportedConfigHistory(t *testing.T, dir string, fileHashes map[string][]byte, expectedCollectionConfigs []*compositeKV) {
	require.Len(t, fileHashes, 2)
	require.Contains(t, fileHashes, snapshotDataFileName)
	require.Contains(t, fileHashes, snapshotMetadataFileName)

	dataFile := filepath.Join(dir, snapshotDataFileName)
	dataFileContent, err := ioutil.ReadFile(dataFile)
	require.NoError(t, err)
	dataFileHash := sha256.Sum256(dataFileContent)
	require.Equal(t, dataFileHash[:], fileHashes[snapshotDataFileName])

	metadataFile := filepath.Join(dir, snapshotMetadataFileName)
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

func verifyImportedConfigHistory(t *testing.T, retriever *Retriever, expectedCCConfigInfo map[string][]*ledger.CollectionConfigInfo) {
	for chaincodeName, ccConfigInfos := range expectedCCConfigInfo {
		for _, expectedCCConfig := range ccConfigInfos {
			ccConfig, err := retriever.CollectionConfigAt(expectedCCConfig.CommittingBlockNum, chaincodeName)
			require.NoError(t, err)
			require.True(t, proto.Equal(expectedCCConfig.CollectionConfig, ccConfig.CollectionConfig))
			require.Equal(t, expectedCCConfig.CommittingBlockNum, ccConfig.CommittingBlockNum)

			ccConfig, err = retriever.MostRecentCollectionConfigBelow(expectedCCConfig.CommittingBlockNum+1, chaincodeName)
			require.NoError(t, err)
			require.True(t, proto.Equal(expectedCCConfig.CollectionConfig, ccConfig.CollectionConfig))
			require.Equal(t, expectedCCConfig.CommittingBlockNum, ccConfig.CommittingBlockNum)
		}
	}
}

func TestExportConfigHistoryErrorCase(t *testing.T) {
	env := newTestEnvForSnapshot(t)
	defer env.cleanup()

	db := env.mgr.dbProvider.getDB("ledger1")
	cc1collConfigPackage := testutilCreateCollConfigPkg([]string{"Explicit-cc1-coll-1", "Explicit-cc1-coll-2"})
	batch := db.newBatch()
	err := prepareDBBatch(
		batch,
		map[string]*peer.CollectionConfigPackage{
			"chaincode1": cc1collConfigPackage,
		},
		50,
	)
	require.NoError(t, err)
	require.NoError(t, db.writeBatch(batch, true))

	// error during data file creation
	dataFilePath := filepath.Join(env.testSnapshotDir, snapshotDataFileName)
	_, err = os.Create(dataFilePath)
	require.NoError(t, err)

	retriever := env.mgr.GetRetriever("ledger1", nil)
	_, err = retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+dataFilePath)
	os.RemoveAll(env.testSnapshotDir)

	// error during metadata file creation
	require.NoError(t, os.MkdirAll(env.testSnapshotDir, 0700))
	metadataFilePath := filepath.Join(env.testSnapshotDir, snapshotMetadataFileName)
	_, err = os.Create(metadataFilePath)
	require.NoError(t, err)
	_, err = retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
	require.Contains(t, err.Error(), "error while creating the snapshot file: "+metadataFilePath)
	os.RemoveAll(env.testSnapshotDir)

	// error while reading from leveldb
	require.NoError(t, os.MkdirAll(env.testSnapshotDir, 0700))
	env.mgr.dbProvider.Close()
	_, err = retriever.ExportConfigHistory(env.testSnapshotDir, testNewHashFunc)
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
