/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/ledger/queryresult"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/chaincode/implicitcollection"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/confighistory/confighistorytest"
	"github.com/hyperledger/fabric/core/ledger/internal/version"
	kvledgermock "github.com/hyperledger/fabric/core/ledger/kvledger/mock"
	"github.com/hyperledger/fabric/core/ledger/kvledger/msgs"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/statedb/statecouchdb"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/internal/fileutil"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestSnapshotGenerationAndNewLedgerCreation(t *testing.T) {
	conf := testConfig(t)
	snapshotRootDir := conf.SnapshotsConfig.RootDir
	nsCollBtlConfs := []*nsCollBtlConfig{
		{
			namespace: "ns",
			btlConfig: map[string]uint64{"coll": 0},
		},
	}
	provider := testutilNewProviderWithCollectionConfig(
		t,
		nsCollBtlConfs,
		conf,
	)
	defer provider.Close()

	// add the genesis block and generate the snapshot
	blkGenerator, genesisBlk := testutil.NewBlockGenerator(t, "testLedgerid", false)
	lgr, err := provider.CreateFromGenesisBlock(genesisBlk)
	require.NoError(t, err)
	defer lgr.Close()
	kvlgr := lgr.(*kvLedger)
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		&expectedSnapshotOutput{
			snapshotRootDir:   snapshotRootDir,
			ledgerID:          kvlgr.ledgerID,
			lastBlockNumber:   0,
			lastBlockHash:     protoutil.BlockHeaderHash(genesisBlk.Header),
			previousBlockHash: genesisBlk.Header.PreviousHash,
			lastCommitHash:    kvlgr.commitHash,
			stateDBType:       simpleKeyValueDB,
			expectedBinaryFiles: []string{
				"txids.data", "txids.metadata",
			},
		},
	)

	// add block-1 only with public state data and generate the snapshot
	blockAndPvtdata1 := prepareNextBlockForTest(t, kvlgr, blkGenerator, "SimulateForBlk1",
		map[string]string{
			"key1": "value1.1",
			"key2": "value2.1",
			"key3": "value3.1",
		},
		nil,
	)
	require.NoError(t, kvlgr.CommitLegacy(blockAndPvtdata1, &ledger.CommitOptions{}))
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		&expectedSnapshotOutput{
			snapshotRootDir:   snapshotRootDir,
			ledgerID:          kvlgr.ledgerID,
			lastBlockNumber:   1,
			lastBlockHash:     protoutil.BlockHeaderHash(blockAndPvtdata1.Block.Header),
			previousBlockHash: blockAndPvtdata1.Block.Header.PreviousHash,
			lastCommitHash:    kvlgr.commitHash,
			stateDBType:       simpleKeyValueDB,
			expectedBinaryFiles: []string{
				"txids.data", "txids.metadata",
				"public_state.data", "public_state.metadata",
			},
		},
	)

	// add dummy entry in collection config history and commit block-2 and generate the snapshot
	addDummyEntryInCollectionConfigHistory(t, provider, kvlgr.ledgerID, "ns", 1, []*peer.StaticCollectionConfig{{Name: "coll"}})

	// add block-2 only with public and private data and generate the snapshot
	blockAndPvtdata2 := prepareNextBlockForTest(t, kvlgr, blkGenerator, "SimulateForBlk2",
		map[string]string{
			"key1": "value1.2",
			"key2": "value2.2",
			"key3": "value3.2",
		},
		map[string]string{
			"key1": "pvtValue1.2",
			"key2": "pvtValue2.2",
			"key3": "pvtValue3.2",
		},
	)
	require.NoError(t, kvlgr.CommitLegacy(blockAndPvtdata2, &ledger.CommitOptions{}))
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		&expectedSnapshotOutput{
			snapshotRootDir:   snapshotRootDir,
			ledgerID:          kvlgr.ledgerID,
			lastBlockNumber:   2,
			lastBlockHash:     protoutil.BlockHeaderHash(blockAndPvtdata2.Block.Header),
			previousBlockHash: blockAndPvtdata2.Block.Header.PreviousHash,
			lastCommitHash:    kvlgr.commitHash,
			stateDBType:       simpleKeyValueDB,
			expectedBinaryFiles: []string{
				"txids.data", "txids.metadata",
				"public_state.data", "public_state.metadata",
				"private_state_hashes.data", "private_state_hashes.metadata",
				"confighistory.data", "confighistory.metadata",
			},
		},
	)

	blockAndPvtdata3 := prepareNextBlockForTest(t, kvlgr, blkGenerator, "SimulateForBlk3",
		map[string]string{
			"key1": "value1.3",
			"key2": "value2.3",
			"key3": "value3.3",
		},
		nil,
	)
	require.NoError(t, kvlgr.CommitLegacy(blockAndPvtdata3, &ledger.CommitOptions{}))
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		&expectedSnapshotOutput{
			snapshotRootDir:   snapshotRootDir,
			ledgerID:          kvlgr.ledgerID,
			lastBlockNumber:   3,
			lastBlockHash:     protoutil.BlockHeaderHash(blockAndPvtdata3.Block.Header),
			previousBlockHash: blockAndPvtdata3.Block.Header.PreviousHash,
			lastCommitHash:    kvlgr.commitHash,
			stateDBType:       simpleKeyValueDB,
			expectedBinaryFiles: []string{
				"txids.data", "txids.metadata",
				"public_state.data", "public_state.metadata",
				"private_state_hashes.data", "private_state_hashes.metadata",
				"confighistory.data", "confighistory.metadata",
			},
		},
	)

	snapshotDir := SnapshotDirForLedgerBlockNum(snapshotRootDir, kvlgr.ledgerID, 3)

	t.Run("create-ledger-from-snapshot", func(t *testing.T) {
		createdLedger := testCreateLedgerFromSnapshot(t, snapshotDir, kvlgr.ledgerID)
		verifyCreatedLedger(t,
			provider,
			createdLedger,
			&expectedLegderState{
				lastBlockNumber:   3,
				lastBlockHash:     protoutil.BlockHeaderHash(blockAndPvtdata3.Block.Header),
				previousBlockHash: blockAndPvtdata3.Block.Header.PreviousHash,
				lastCommitHash:    kvlgr.commitHash,
				namespace:         "ns",
				publicState: map[string]string{
					"key1": "value1.3",
					"key2": "value2.3",
					"key3": "value3.3",
				},
				collectionConfig: map[uint64]*peer.CollectionConfigPackage{
					1: {
						Config: []*peer.CollectionConfig{
							{
								Payload: &peer.CollectionConfig_StaticCollectionConfig{
									StaticCollectionConfig: &peer.StaticCollectionConfig{
										Name: "coll",
									},
								},
							},
						},
					},
				},
			},
		)
	})

	t.Run("create-ledger-from-snapshot-error-paths", func(t *testing.T) {
		testCreateLedgerFromSnapshotErrorPaths(t, snapshotDir)
	})
}

func TestSnapshotDBTypeCouchDB(t *testing.T) {
	conf := testConfig(t)
	fmt.Printf("snapshotRootDir %s\n", conf.SnapshotsConfig.RootDir)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer provider.Close()

	_, genesisBlk := testutil.NewBlockGenerator(t, "testLedgerid", false)
	lgr, err := provider.CreateFromGenesisBlock(genesisBlk)
	require.NoError(t, err)
	defer lgr.Close()
	kvlgr := lgr.(*kvLedger)

	// artificially set the db type
	kvlgr.config.StateDBConfig.StateDatabase = ledger.CouchDB
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		&expectedSnapshotOutput{
			snapshotRootDir: conf.SnapshotsConfig.RootDir,
			ledgerID:        kvlgr.ledgerID,
			stateDBType:     ledger.CouchDB,
			lastBlockHash:   protoutil.BlockHeaderHash(genesisBlk.Header),
			expectedBinaryFiles: []string{
				"txids.data", "txids.metadata",
			},
		},
	)
}

func TestSnapshotCouchDBIndexCreation(t *testing.T) {
	setup := func() (string, *ledger.CouchDBConfig, *Provider) {
		conf := testConfig(t)

		snapshotRootDir := conf.SnapshotsConfig.RootDir
		provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
		t.Cleanup(provider.Close)

		// add the genesis block and generate the snapshot
		blkGenerator, genesisBlk := testutil.NewBlockGenerator(t, "test_ledger", false)
		lgr, err := provider.CreateFromGenesisBlock(genesisBlk)
		require.NoError(t, err)
		t.Cleanup(lgr.Close)
		kvlgr := lgr.(*kvLedger)

		blockAndPvtdata := prepareNextBlockForTest(t, kvlgr, blkGenerator, "SimulateForBlk1",
			map[string]string{
				"key1": `{"asset_name": "marble1", "color": "blue", "size": 1, "owner": "tom"}`,
				"key2": `{"asset_name": "marble2", "color": "red", "size": 2, "owner": "jerry"}`,
			},
			nil,
		)
		require.NoError(t, kvlgr.CommitLegacy(blockAndPvtdata, &ledger.CommitOptions{}))
		require.NoError(t, kvlgr.generateSnapshot())
		snapshotDir := SnapshotDirForLedgerBlockNum(snapshotRootDir, kvlgr.ledgerID, 1)

		cceventmgmt.Initialize(nil)
		if couchDBAddress == "" {
			couchDBAddress, stopCouchDBFunc = statecouchdb.StartCouchDB(t, nil)
		}
		couchDBConfig := &ledger.CouchDBConfig{
			Address:             couchDBAddress,
			Username:            "admin",
			Password:            "adminpw",
			MaxRetries:          3,
			MaxRetriesOnStartup: 3,
			RequestTimeout:      10 * time.Second,
			InternalQueryLimit:  1000,
			RedoLogPath:         filepath.Join(conf.RootFSPath, "couchdbRedoLogs"),
		}

		destConf := testConfig(t)
		destConf.StateDBConfig = &ledger.StateDBConfig{
			StateDatabase: ledger.CouchDB,
			CouchDB:       couchDBConfig,
		}
		destinationProvider := testutilNewProvider(destConf, t, &mock.DeployedChaincodeInfoProvider{})
		return snapshotDir, couchDBConfig, destinationProvider
	}

	dbArtifactsBytes := testutil.CreateTarBytesForTest(
		[]*testutil.TarFileEntry{
			{
				Name: "META-INF/statedb/couchdb/indexes/indexSizeSortName.json",
				Body: `{"index":{"fields":[{"size":"desc"}]},"ddoc":"indexSizeSortName","name":"indexSizeSortName","type":"json"}`,
			},
		},
	)

	verifyIndexCreatedOnMarbleSize := func(lgr ledger.PeerLedger) {
		qe, err := lgr.NewQueryExecutor()
		require.NoError(t, err)
		defer qe.Done()
		iter, err := qe.ExecuteQuery("ns", `{"selector":{"owner":"tom"}, "sort": [{"size": "desc"}]}`)
		require.NoError(t, err)
		defer iter.Close()
		actualResults := []*queryresult.KV{}
		for {
			queryResults, err := iter.Next()
			require.NoError(t, err)
			if queryResults == nil {
				break
			}
			actualResults = append(actualResults, queryResults.(*queryresult.KV))
		}
		require.Len(t, actualResults, 1)
		_, err = qe.ExecuteQuery("ns", `{"selector":{"owner":"tom"}, "sort": [{"color": "desc"}]}`)
		require.Contains(t, err.Error(), "No index exists for this sort")
	}

	t.Run("create_indexes_on_couchdb_for_new_lifecycle", func(t *testing.T) {
		snapshotDir, couchDBConfig, provider := setup()
		defer func() {
			require.NoError(t, statecouchdb.DropApplicationDBs(couchDBConfig))
		}()

		// mimic new lifecycle chaincode "ns" installed and defiend and the package contains an index definition "sort index"
		ccLifecycleEventProvider := provider.initializer.ChaincodeLifecycleEventProvider.(*mock.ChaincodeLifecycleEventProvider)
		ccLifecycleEventProvider.RegisterListenerStub =
			func(
				channelID string,
				listener ledger.ChaincodeLifecycleEventListener,
				callback bool,
			) error {
				if callback {
					err := listener.HandleChaincodeDeploy(
						&ledger.ChaincodeDefinition{
							Name: "ns",
						},
						dbArtifactsBytes,
					)
					require.NoError(t, err)
				}
				return nil
			}

		lgr, _, err := provider.CreateFromSnapshot(snapshotDir)
		require.NoError(t, err)
		verifyIndexCreatedOnMarbleSize(lgr)
	})

	t.Run("create_indexes_on_couchdb_for_legacy_lifecycle", func(t *testing.T) {
		snapshotDir, couchDBConfig, provider := setup()
		defer func() {
			require.NoError(t, statecouchdb.DropApplicationDBs(couchDBConfig))
		}()

		// mimic legacy chaincode "ns" installed and defiend and the package contains an index definition "sort index"
		deployedCCInfoProvider := provider.initializer.DeployedChaincodeInfoProvider.(*mock.DeployedChaincodeInfoProvider)
		deployedCCInfoProvider.AllChaincodesInfoReturns(
			map[string]*ledger.DeployedChaincodeInfo{
				"ns": {
					Name:     "ns",
					Version:  "version",
					Hash:     []byte("hash"),
					IsLegacy: true,
				},
				"anotherNs": {
					Name:     "anotherNs",
					Version:  "version",
					Hash:     []byte("hash"),
					IsLegacy: false,
				},
			},
			nil,
		)

		installedChaincodeInfoProvider := &kvledgermock.ChaincodeInfoProvider{}
		installedChaincodeInfoProvider.RetrieveChaincodeArtifactsReturns(
			true, dbArtifactsBytes, nil,
		)
		cceventmgmt.Initialize(installedChaincodeInfoProvider)
		lgr, _, err := provider.CreateFromSnapshot(snapshotDir)
		require.NoError(t, err)

		require.Equal(t, 1, installedChaincodeInfoProvider.RetrieveChaincodeArtifactsCallCount())
		require.Equal(t,
			&cceventmgmt.ChaincodeDefinition{
				Name:    "ns",
				Version: "version",
				Hash:    []byte("hash"),
			},
			installedChaincodeInfoProvider.RetrieveChaincodeArtifactsArgsForCall(0),
		)
		verifyIndexCreatedOnMarbleSize(lgr)
	})

	t.Run("errors-propagation", func(t *testing.T) {
		snapshotDir, couchDBConfig, provider := setup()
		defer func() {
			require.NoError(t, statecouchdb.DropApplicationDBs(couchDBConfig))
		}()

		t.Run("deployedChaincodeInfoProvider-returns-error", func(t *testing.T) {
			deployedCCInfoProvider := provider.initializer.DeployedChaincodeInfoProvider.(*mock.DeployedChaincodeInfoProvider)
			deployedCCInfoProvider.AllChaincodesInfoReturns(nil, fmt.Errorf("error-retrieving-all-defined-chaincodes"))

			installedChaincodeInfoProvider := &kvledgermock.ChaincodeInfoProvider{}
			installedChaincodeInfoProvider.RetrieveChaincodeArtifactsReturns(
				true, dbArtifactsBytes, nil,
			)
			cceventmgmt.Initialize(installedChaincodeInfoProvider)
			_, _, err := provider.CreateFromSnapshot(snapshotDir)
			require.EqualError(t, err, "error while opening ledger: error while creating statdb indexes after bootstrapping from snapshot: error-retrieving-all-defined-chaincodes")
		})

		t.Run("legacychaincodes-dbartifacts-retriever-returns-error", func(t *testing.T) {
			deployedCCInfoProvider := provider.initializer.DeployedChaincodeInfoProvider.(*mock.DeployedChaincodeInfoProvider)
			deployedCCInfoProvider.AllChaincodesInfoReturns(
				map[string]*ledger.DeployedChaincodeInfo{
					"ns": {
						Name:     "ns",
						Version:  "version",
						Hash:     []byte("hash"),
						IsLegacy: true,
					},
				},
				nil,
			)

			installedChaincodeInfoProvider := &kvledgermock.ChaincodeInfoProvider{}
			installedChaincodeInfoProvider.RetrieveChaincodeArtifactsReturns(false, nil, fmt.Errorf("error-retrieving-db-artifacts"))
			cceventmgmt.Initialize(installedChaincodeInfoProvider)
			_, _, err := provider.CreateFromSnapshot(snapshotDir)
			require.EqualError(t, err, "error while opening ledger: error while creating statdb indexes after bootstrapping from snapshot: error-retrieving-db-artifacts")
		})

		t.Run("chaincodeLifecycleEventProvider-returns-error", func(t *testing.T) {
			chaincodeLifecycleEventProvider := provider.initializer.ChaincodeLifecycleEventProvider.(*mock.ChaincodeLifecycleEventProvider)
			chaincodeLifecycleEventProvider.RegisterListenerReturns(fmt.Errorf("error-calling-back"))
			cceventmgmt.Initialize(nil)
			_, _, err := provider.CreateFromSnapshot(snapshotDir)
			require.EqualError(t, err, "error while opening ledger: error while creating statdb indexes after bootstrapping from snapshot: error-calling-back")
		})
	})
}

func TestSnapshotDirPaths(t *testing.T) {
	require.Equal(t, "/peerFSPath/snapshotRootDir/temp", SnapshotsTempDirPath("/peerFSPath/snapshotRootDir"))
	require.Equal(t, "/peerFSPath/snapshotRootDir/completed", CompletedSnapshotsPath("/peerFSPath/snapshotRootDir"))
	require.Equal(t, "/peerFSPath/snapshotRootDir/completed/myLedger", SnapshotsDirForLedger("/peerFSPath/snapshotRootDir", "myLedger"))
	require.Equal(t, "/peerFSPath/snapshotRootDir/completed/myLedger/2000", SnapshotDirForLedgerBlockNum("/peerFSPath/snapshotRootDir", "myLedger", 2000))
}

func TestSnapshotDirPathsCreation(t *testing.T) {
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer func() {
		provider.Close()
	}()

	inProgressSnapshotsPath := SnapshotsTempDirPath(conf.SnapshotsConfig.RootDir)
	completedSnapshotsPath := CompletedSnapshotsPath(conf.SnapshotsConfig.RootDir)

	// verify that upon first time start, kvledgerProvider creates an empty temp dir and an empty final dir for the snapshots
	for _, dir := range [2]string{inProgressSnapshotsPath, completedSnapshotsPath} {
		f, err := ioutil.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, f, 0)
	}

	// add a file in each of the above folders
	for _, dir := range [2]string{inProgressSnapshotsPath, completedSnapshotsPath} {
		err := ioutil.WriteFile(filepath.Join(dir, "testFile"), []byte("some junk data"), 0o644)
		require.NoError(t, err)
		f, err := ioutil.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, f, 1)
	}

	// verify that upon subsequent opening, kvledgerProvider removes any under-processing snapshots,
	// potentially from a previous crash, from the temp dir but it does not remove any files from the final dir
	provider.Close()
	provider = testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	f, err := ioutil.ReadDir(inProgressSnapshotsPath)
	require.NoError(t, err)
	require.Len(t, f, 0)
	f, err = ioutil.ReadDir(completedSnapshotsPath)
	require.NoError(t, err)
	require.Len(t, f, 1)
}

func TestSnapshotsDirInitializingErrors(t *testing.T) {
	initKVLedgerProvider := func(conf *ledger.Config) error {
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		_, err = NewProvider(
			&ledger.Initializer{
				DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
				MetricsProvider:               &disabled.Provider{},
				Config:                        conf,
				HashProvider:                  cryptoProvider,
			},
		)
		return err
	}

	t.Run("invalid-path", func(t *testing.T) {
		conf := testConfig(t)
		conf.SnapshotsConfig.RootDir = "./a-relative-path"
		err := initKVLedgerProvider(conf)
		require.EqualError(t, err, "invalid path: ./a-relative-path. The path for the snapshot dir is expected to be an absolute path")
	})

	t.Run("snapshots final dir creation returns error", func(t *testing.T) {
		conf := testConfig(t)

		completedSnapshotsPath := CompletedSnapshotsPath(conf.SnapshotsConfig.RootDir)
		require.NoError(t, os.MkdirAll(filepath.Dir(completedSnapshotsPath), 0o755))
		require.NoError(t, ioutil.WriteFile(completedSnapshotsPath, []byte("some data"), 0o644))
		err := initKVLedgerProvider(conf)
		require.Error(t, err)
		require.Contains(t, err.Error(), "while creating the dir: "+completedSnapshotsPath)
	})
}

func TestGenerateSnapshotErrors(t *testing.T) {
	conf := testConfig(t)
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer func() {
		provider.Close()
	}()

	// create a ledger
	_, genesisBlk := testutil.NewBlockGenerator(t, "testLedgerid", false)
	lgr, err := provider.CreateFromGenesisBlock(genesisBlk)
	require.NoError(t, err)
	kvlgr := lgr.(*kvLedger)

	closeAndReopenLedgerProvider := func() {
		provider.Close()
		provider = testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
		lgr, err = provider.Open("testLedgerid")
		require.NoError(t, err)
		kvlgr = lgr.(*kvLedger)
	}

	t.Run("snapshot tmp dir creation returns error", func(t *testing.T) {
		closeAndReopenLedgerProvider()
		require.NoError(t, os.RemoveAll( // remove the base tempdir so that the snapshot tempdir creation fails
			SnapshotsTempDirPath(conf.SnapshotsConfig.RootDir),
		))
		err := kvlgr.generateSnapshot()
		require.Error(t, err)
		require.Contains(t, err.Error(), "error while creating temp dir")
	})

	t.Run("block store returns error", func(t *testing.T) {
		closeAndReopenLedgerProvider()
		provider.blkStoreProvider.Close() // close the blockstore provider to trigger the error
		err := kvlgr.generateSnapshot()
		require.Error(t, err)
		errStackTrace := fmt.Sprintf("%+v", err)
		require.Contains(t, errStackTrace, "internal leveldb error while obtaining db iterator")
		require.Contains(t, errStackTrace, "github.com/hyperledger/fabric/common/ledger/blkstorage")
	})

	t.Run("config history mgr returns error", func(t *testing.T) {
		closeAndReopenLedgerProvider()
		provider.configHistoryMgr.Close() // close the configHistoryMgr to trigger the error
		err := kvlgr.generateSnapshot()
		require.Error(t, err)
		errStackTrace := fmt.Sprintf("%+v", err)
		require.Contains(t, errStackTrace, "internal leveldb error while obtaining db iterator")
		require.Contains(t, errStackTrace, "github.com/hyperledger/fabric/core/ledger/confighistory")
	})

	t.Run("statedb returns error", func(t *testing.T) {
		closeAndReopenLedgerProvider()
		provider.dbProvider.Close() // close the dbProvider to trigger the error
		err := kvlgr.generateSnapshot()
		require.Error(t, err)
		errStackTrace := fmt.Sprintf("%+v", err)
		require.Contains(t, errStackTrace, "internal leveldb error while obtaining db iterator")
		require.Contains(t, errStackTrace, "statedb/stateleveldb/stateleveldb.go")
	})

	t.Run("renaming to the final snapshot dir returns error", func(t *testing.T) {
		closeAndReopenLedgerProvider()
		snapshotFinalDir := SnapshotDirForLedgerBlockNum(conf.SnapshotsConfig.RootDir, "testLedgerid", 0)
		require.NoError(t, os.MkdirAll(snapshotFinalDir, 0o744))
		defer os.RemoveAll(snapshotFinalDir)
		require.NoError(t, ioutil.WriteFile( // make a non-empty snapshotFinalDir to trigger failure on rename
			filepath.Join(snapshotFinalDir, "dummyFile"),
			[]byte("dummy file"), 0o444),
		)
		err := kvlgr.generateSnapshot()
		require.Contains(t, err.Error(), "error while renaming dir")
	})

	t.Run("deletes the temp folder upon error", func(t *testing.T) {
		closeAndReopenLedgerProvider()
		provider.blkStoreProvider.Close() // close the blockstore provider to trigger an error
		err := kvlgr.generateSnapshot()
		require.Error(t, err)

		empty, err := fileutil.DirEmpty(SnapshotsTempDirPath(conf.SnapshotsConfig.RootDir))
		require.NoError(t, err)
		require.True(t, empty)
	})
}

func testCreateLedgerFromSnapshotErrorPaths(t *testing.T, originalSnapshotDir string) {
	var provider *Provider
	var snapshotDirForTest string
	var cleanup func()

	var metadata *SnapshotMetadata
	var signableMetadataFile string
	var additionalMetadataFile string

	init := func(t *testing.T) {
		conf := testConfig(t)
		// make a copy of originalSnapshotDir
		snapshotDirForTest = filepath.Join(conf.RootFSPath, "snapshot")
		require.NoError(t, os.MkdirAll(snapshotDirForTest, 0o700))
		files, err := ioutil.ReadDir(originalSnapshotDir)
		require.NoError(t, err)
		for _, f := range files {
			content, err := ioutil.ReadFile(filepath.Join(originalSnapshotDir, f.Name()))
			require.NoError(t, err)
			err = ioutil.WriteFile(filepath.Join(snapshotDirForTest, f.Name()), content, 0o600)
			require.NoError(t, err)
		}

		metadataJSONs, err := loadSnapshotMetadataJSONs(snapshotDirForTest)
		require.NoError(t, err)
		metadata, err = metadataJSONs.ToMetadata()
		require.NoError(t, err)

		signableMetadataFile = filepath.Join(snapshotDirForTest, SnapshotSignableMetadataFileName)
		additionalMetadataFile = filepath.Join(snapshotDirForTest, snapshotAdditionalMetadataFileName)

		provider = testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
		cleanup = func() {
			provider.Close()
		}
	}

	overwriteModifiedSignableMetadata := func() {
		signaleMetadataJSON, err := metadata.SnapshotSignableMetadata.ToJSON()
		require.NoError(t, err)
		require.NoError(t, ioutil.WriteFile(signableMetadataFile, signaleMetadataJSON, 0o600))

		metadata.snapshotAdditionalMetadata.SnapshotHashInHex = computeHashForTest(t, provider, signaleMetadataJSON)
		additionalMetadataJSON, err := metadata.snapshotAdditionalMetadata.ToJSON()
		require.NoError(t, err)
		require.NoError(t, ioutil.WriteFile(additionalMetadataFile, additionalMetadataJSON, 0o600))
	}

	overwriteDataFile := func(fileName string, content []byte) {
		filePath := filepath.Join(snapshotDirForTest, fileName)
		require.NoError(t, ioutil.WriteFile(filePath, content, 0o600))
		metadata.SnapshotSignableMetadata.FilesAndHashes[fileName] = computeHashForTest(t, provider, content)
		overwriteModifiedSignableMetadata()
	}

	t.Run("singable-metadata-file-missing", func(t *testing.T) {
		init(t)
		defer cleanup()

		require.NoError(t, os.Remove(filepath.Join(snapshotDirForTest, SnapshotSignableMetadataFileName)))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.EqualError(t,
			err,
			fmt.Sprintf(
				"error while loading metadata: open %s/_snapshot_signable_metadata.json: no such file or directory",
				snapshotDirForTest,
			),
		)
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("additional-metadata-file-missing", func(t *testing.T) {
		init(t)
		defer cleanup()

		require.NoError(t, os.Remove(filepath.Join(snapshotDirForTest, snapshotAdditionalMetadataFileName)))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.EqualError(t,
			err,
			fmt.Sprintf("error while loading metadata: open %s/_snapshot_additional_metadata.json: no such file or directory", snapshotDirForTest),
		)
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("singable-metadata-file-invalid-json", func(t *testing.T) {
		init(t)
		defer cleanup()

		require.NoError(t, ioutil.WriteFile(signableMetadataFile, []byte(""), 0o600))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.EqualError(t,
			err,
			"error while unmarshalling metadata: error while unmarshalling signable metadata: unexpected end of JSON input",
		)
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("additional-metadata-file-invalid-json", func(t *testing.T) {
		init(t)
		defer cleanup()

		require.NoError(t, ioutil.WriteFile(additionalMetadataFile, []byte(""), 0o600))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.EqualError(t,
			err,
			"error while unmarshalling metadata: error while unmarshalling additional metadata: unexpected end of JSON input",
		)
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("snapshot-hash-mismatch", func(t *testing.T) {
		init(t)
		defer cleanup()

		require.NoError(t, ioutil.WriteFile(signableMetadataFile, []byte("{}"), 0o600))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t,
			err.Error(),
			"error while verifying snapshot: hash mismatch for file [_snapshot_signable_metadata.json]",
		)
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("datafile-missing", func(t *testing.T) {
		init(t)
		defer cleanup()

		err := os.Remove(filepath.Join(snapshotDirForTest, "txids.data"))
		require.NoError(t, err)

		_, _, err = provider.CreateFromSnapshot(snapshotDirForTest)
		require.EqualError(t, err,
			fmt.Sprintf(
				"error while verifying snapshot: open %s/txids.data: no such file or directory",
				snapshotDirForTest,
			),
		)
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("datafile-hash-mismatch", func(t *testing.T) {
		init(t)
		defer cleanup()

		err := ioutil.WriteFile(filepath.Join(snapshotDirForTest, "txids.data"), []byte("random content"), 0o600)
		require.NoError(t, err)

		_, _, err = provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while verifying snapshot: hash mismatch for file [txids.data]")
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("hex-decoding-error-for-lastBlkHash", func(t *testing.T) {
		init(t)
		defer cleanup()

		metadata.SnapshotSignableMetadata.LastBlockHashInHex = "invalid-hex"
		overwriteModifiedSignableMetadata()

		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while decoding last block hash")
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("hex-decoding-error-for-previousBlkHash", func(t *testing.T) {
		init(t)
		defer cleanup()

		metadata.SnapshotSignableMetadata.PreviousBlockHashInHex = "invalid-hex"
		overwriteModifiedSignableMetadata()

		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while decoding previous block hash")
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("idStore-returns-error", func(t *testing.T) {
		init(t)
		defer cleanup()

		provider.idStore.close()
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while creating ledger id")
	})

	t.Run("blkstore-provider-returns-error", func(t *testing.T) {
		init(t)
		defer cleanup()

		overwriteDataFile("txids.data", []byte(""))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while importing data into block store")
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("config-history-mgr-returns-error", func(t *testing.T) {
		init(t)
		defer cleanup()

		overwriteDataFile("confighistory.data", []byte(""))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while importing data into config history Mgr")
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("statedb-provider-returns-error", func(t *testing.T) {
		init(t)
		defer cleanup()

		overwriteDataFile("public_state.data", []byte(""))
		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while importing data into state db")
		verifyLedgerDoesNotExist(t, provider, metadata.ChannelName)
	})

	t.Run("error-while-deleting-partially-created-ledger", func(t *testing.T) {
		init(t)
		defer cleanup()

		provider.historydbProvider.Close()

		_, _, err := provider.CreateFromSnapshot(snapshotDirForTest)
		require.Contains(t, err.Error(), "error while preparing history db")
		require.Contains(t, err.Error(), "error while deleting data from ledger")
		verifyLedgerIDExists(t, provider, metadata.ChannelName, msgs.Status_UNDER_CONSTRUCTION)
	})
}

func computeHashForTest(t *testing.T, provider *Provider, content []byte) string {
	hasher, err := provider.initializer.HashProvider.GetHash(snapshotHashOpts)
	require.NoError(t, err)
	_, err = hasher.Write(content)
	require.NoError(t, err)
	return hex.EncodeToString(hasher.Sum(nil))
}

type expectedSnapshotOutput struct {
	snapshotRootDir     string
	ledgerID            string
	lastBlockNumber     uint64
	lastBlockHash       []byte
	previousBlockHash   []byte
	lastCommitHash      []byte
	stateDBType         string
	expectedBinaryFiles []string
}

func verifySnapshotOutput(
	t *testing.T,
	o *expectedSnapshotOutput,
) {
	inProgressSnapshotsPath := SnapshotsTempDirPath(o.snapshotRootDir)
	f, err := ioutil.ReadDir(inProgressSnapshotsPath)
	require.NoError(t, err)
	require.Len(t, f, 0)

	snapshotDir := SnapshotDirForLedgerBlockNum(o.snapshotRootDir, o.ledgerID, o.lastBlockNumber)
	files, err := ioutil.ReadDir(snapshotDir)
	require.NoError(t, err)
	require.Len(t, files, len(o.expectedBinaryFiles)+2) // + 2 JSON files

	filesAndHashes := map[string]string{}
	for _, f := range o.expectedBinaryFiles {
		c, err := ioutil.ReadFile(filepath.Join(snapshotDir, f))
		require.NoError(t, err)
		filesAndHashes[f] = hex.EncodeToString(util.ComputeSHA256(c))
	}

	// verify the contents of the file snapshot_metadata.json
	m := &SnapshotSignableMetadata{}
	mJSON, err := ioutil.ReadFile(filepath.Join(snapshotDir, SnapshotSignableMetadataFileName))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(mJSON, m))

	previousBlockHashHex := ""
	if o.previousBlockHash != nil {
		previousBlockHashHex = hex.EncodeToString(o.previousBlockHash)
	}
	require.Equal(t,
		&SnapshotSignableMetadata{
			ChannelName:            o.ledgerID,
			LastBlockNumber:        o.lastBlockNumber,
			LastBlockHashInHex:     hex.EncodeToString(o.lastBlockHash),
			PreviousBlockHashInHex: previousBlockHashHex,
			StateDBType:            o.stateDBType,
			FilesAndHashes:         filesAndHashes,
		},
		m,
	)

	// verify the contents of the file snapshot_metadata_hash.json
	mh := &snapshotAdditionalMetadata{}
	mhJSON, err := ioutil.ReadFile(filepath.Join(snapshotDir, snapshotAdditionalMetadataFileName))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(mhJSON, mh))
	require.Equal(t,
		&snapshotAdditionalMetadata{
			SnapshotHashInHex:        hex.EncodeToString(util.ComputeSHA256(mJSON)),
			LastBlockCommitHashInHex: hex.EncodeToString(o.lastCommitHash),
		},
		mh,
	)
}

func testCreateLedgerFromSnapshot(t *testing.T, snapshotDir string, expectedChannelID string) *kvLedger {
	conf := testConfig(t)
	p := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	destLedger, channelID, err := p.CreateFromSnapshot(snapshotDir)
	require.NoError(t, err)
	require.Equal(t, expectedChannelID, channelID)
	return destLedger.(*kvLedger)
}

type expectedLegderState struct {
	lastBlockNumber   uint64
	lastBlockHash     []byte
	previousBlockHash []byte
	lastCommitHash    []byte
	namespace         string
	publicState       map[string]string
	collectionConfig  map[uint64]*peer.CollectionConfigPackage
}

func verifyCreatedLedger(t *testing.T,
	p *Provider,
	l *kvLedger,
	e *expectedLegderState,
) {
	verifyLedgerIDExists(t, p, l.ledgerID, msgs.Status_ACTIVE)

	destBCInfo, err := l.GetBlockchainInfo()
	require.NoError(t, err)
	require.Equal(t,
		&common.BlockchainInfo{
			Height:            e.lastBlockNumber + 1,
			CurrentBlockHash:  e.lastBlockHash,
			PreviousBlockHash: e.previousBlockHash,
			BootstrappingSnapshotInfo: &common.BootstrappingSnapshotInfo{
				LastBlockInSnapshot: e.lastBlockNumber,
			},
		},
		destBCInfo,
	)

	statedbSavepoint, err := l.txmgr.GetLastSavepoint()
	require.NoError(t, err)
	require.Equal(t, version.NewHeight(e.lastBlockNumber, math.MaxUint64), statedbSavepoint)

	historydbSavepoint, err := l.historyDB.GetLastSavepoint()
	require.NoError(t, err)
	require.Equal(t, version.NewHeight(e.lastBlockNumber, math.MaxUint64), historydbSavepoint)

	qe, err := l.txmgr.NewQueryExecutor("dummyTxId")
	require.NoError(t, err)
	defer qe.Done()
	for k, v := range e.publicState {
		val, err := qe.GetState(e.namespace, k)
		require.NoError(t, err)
		require.Equal(t, v, string(val))
	}
	for committingBlock, collConfigPkg := range e.collectionConfig {
		collConfigInfo, err := l.configHistoryRetriever.MostRecentCollectionConfigBelow(committingBlock+1, e.namespace)
		require.NoError(t, err)
		require.Equal(t, committingBlock, collConfigInfo.CommittingBlockNum)
		require.True(t, proto.Equal(collConfigPkg, collConfigInfo.CollectionConfig))
	}
}

func addDummyEntryInCollectionConfigHistory(
	t *testing.T,
	provider *Provider,
	ledgerID string,
	namespace string,
	committingBlockNumber uint64,
	collectionConfig []*peer.StaticCollectionConfig,
) {
	configHistory := &confighistorytest.Mgr{
		Mgr:                provider.configHistoryMgr,
		MockCCInfoProvider: provider.initializer.DeployedChaincodeInfoProvider.(*mock.DeployedChaincodeInfoProvider),
	}
	require.NoError(t,
		configHistory.Setup(ledgerID, namespace,
			map[uint64][]*peer.StaticCollectionConfig{
				committingBlockNumber: collectionConfig,
			},
		),
	)
}

func TestMostRecentCollectionConfigFetcher(t *testing.T) {
	conf := testConfig(t)

	ledgerID := "test-ledger"
	chaincodeName := "test-chaincode"

	implicitCollectionName := implicitcollection.NameForOrg("test-org")
	implicitCollection := &peer.StaticCollectionConfig{
		Name: implicitCollectionName,
	}
	mockDeployedCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockDeployedCCInfoProvider.GenerateImplicitCollectionForOrgReturns(implicitCollection)

	provider := testutilNewProvider(conf, t, mockDeployedCCInfoProvider)
	explicitCollectionName := "explicit-coll"
	explicitCollection := &peer.StaticCollectionConfig{
		Name: explicitCollectionName,
	}
	testutilPersistExplicitCollectionConfig(
		t,
		provider,
		mockDeployedCCInfoProvider,
		ledgerID,
		chaincodeName,
		testutilCollConfigPkg(
			[]*peer.StaticCollectionConfig{
				explicitCollection,
			},
		),
		10,
	)

	fetcher := &mostRecentCollectionConfigFetcher{
		DeployedChaincodeInfoProvider: mockDeployedCCInfoProvider,
		Retriever:                     provider.configHistoryMgr.GetRetriever(ledgerID),
	}

	testcases := []struct {
		name                 string
		lookupCollectionName string
		expectedOutput       *peer.StaticCollectionConfig
	}{
		{
			name:                 "lookup-implicit-collection",
			lookupCollectionName: implicitCollectionName,
			expectedOutput:       implicitCollection,
		},

		{
			name:                 "lookup-explicit-collection",
			lookupCollectionName: explicitCollectionName,
			expectedOutput:       explicitCollection,
		},

		{
			name:                 "lookup-non-existing-explicit-collection",
			lookupCollectionName: "non-existing-explicit-collection",
			expectedOutput:       nil,
		},
	}

	for _, testcase := range testcases {
		t.Run(
			testcase.name,
			func(t *testing.T) {
				config, err := fetcher.CollectionInfo(chaincodeName, testcase.lookupCollectionName)
				require.NoError(t, err)
				require.True(t, proto.Equal(testcase.expectedOutput, config))
			},
		)
	}

	t.Run("explicit-collection-lookup-causes-error", func(t *testing.T) {
		provider.configHistoryMgr.Close()
		_, err := fetcher.CollectionInfo(chaincodeName, explicitCollectionName)
		require.Contains(t, err.Error(), "error while fetching most recent collection config")
	})
}
