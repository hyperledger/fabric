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
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger"
	lgr "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestGenerateSnapshot(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	snapshotRootDir := conf.SnapshotsConfig.RootDir
	provider := testutilNewProviderWithCollectionConfig(
		t,
		"ns",
		map[string]uint64{"coll": 0},
		conf,
	)
	defer provider.Close()

	// add the genesis block and generate the snapshot
	blkGenerator, genesisBlk := testutil.NewBlockGenerator(t, "testLedgerid", false)
	lgr, err := provider.Create(genesisBlk)
	require.NoError(t, err)
	defer lgr.Close()
	kvlgr := lgr.(*kvLedger)
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		snapshotRootDir,
		kvlgr.ledgerID,
		1,
		protoutil.BlockHeaderHash(genesisBlk.Header),
		kvlgr.commitHash,
		"txids.data", "txids.metadata",
	)

	// add block-1 only with public state data and generate the snapshot
	blockAndPvtdata1 := prepareNextBlockForTest(t, kvlgr, blkGenerator, "SimulateForBlk1",
		map[string]string{"key1": "value1.1", "key2": "value2.1", "key3": "value3.1"},
		nil,
	)
	require.NoError(t, kvlgr.CommitLegacy(blockAndPvtdata1, &ledger.CommitOptions{}))
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		snapshotRootDir,
		kvlgr.ledgerID,
		2,
		protoutil.BlockHeaderHash(blockAndPvtdata1.Block.Header),
		kvlgr.commitHash,
		"txids.data", "txids.metadata",
		"public_state.data", "public_state.metadata",
	)

	// add block-2 only with public and private data and generate the snapshot
	blockAndPvtdata2 := prepareNextBlockForTest(t, kvlgr, blkGenerator, "SimulateForBlk2",
		map[string]string{"key1": "value1.2", "key2": "value2.2", "key3": "value3.2"},
		map[string]string{"key1": "pvtValue1.2", "key2": "pvtValue2.2", "key3": "pvtValue3.2"},
	)
	require.NoError(t, kvlgr.CommitLegacy(blockAndPvtdata2, &ledger.CommitOptions{}))
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		snapshotRootDir,
		kvlgr.ledgerID,
		3,
		protoutil.BlockHeaderHash(blockAndPvtdata2.Block.Header),
		kvlgr.commitHash,
		"txids.data", "txids.metadata",
		"public_state.data", "public_state.metadata",
		"private_state_hashes.data", "private_state_hashes.metadata",
	)

	// add dummy entry in collection config history and commit block-3 and generate the snapshot
	addDummyEntryInCollectionConfigHistory(t, provider, kvlgr.ledgerID)
	blockAndPvtdata3 := prepareNextBlockForTest(t, kvlgr, blkGenerator, "SimulateForBlk3",
		map[string]string{"key1": "value1.3", "key2": "value2.3", "key3": "value3.3"},
		nil,
	)
	require.NoError(t, kvlgr.CommitLegacy(blockAndPvtdata3, &ledger.CommitOptions{}))
	require.NoError(t, kvlgr.generateSnapshot())
	verifySnapshotOutput(t,
		snapshotRootDir,
		kvlgr.ledgerID,
		4,
		protoutil.BlockHeaderHash(blockAndPvtdata3.Block.Header),
		kvlgr.commitHash,
		"txids.data", "txids.metadata",
		"public_state.data", "public_state.metadata",
		"private_state_hashes.data", "private_state_hashes.metadata",
		"confighistory.data", "confighistory.metadata",
	)
}

func TestSnapshotDirPaths(t *testing.T) {
	require.Equal(t, "/peerFSPath/snapshotRootDir/underConstruction", InProgressSnapshotsPath("/peerFSPath/snapshotRootDir"))
	require.Equal(t, "/peerFSPath/snapshotRootDir/completed", CompletedSnapshotsPath("/peerFSPath/snapshotRootDir"))
	require.Equal(t, "/peerFSPath/snapshotRootDir/completed/myLedger", SnapshotsDirForLedger("/peerFSPath/snapshotRootDir", "myLedger"))
	require.Equal(t, "/peerFSPath/snapshotRootDir/completed/myLedger/2000", SnapshotDirForLedgerHeight("/peerFSPath/snapshotRootDir", "myLedger", 2000))
}

func TestSnapshotDirPathsCreation(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer func() {
		provider.Close()
	}()

	inProgressSnapshotsPath := InProgressSnapshotsPath(conf.SnapshotsConfig.RootDir)
	completedSnapshotsPath := CompletedSnapshotsPath(conf.SnapshotsConfig.RootDir)

	// verify that upon first time start, kvledgerProvider creates an empty temp dir and an empty final dir for the snapshots
	for _, dir := range [2]string{inProgressSnapshotsPath, completedSnapshotsPath} {
		f, err := ioutil.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, f, 0)
	}

	// add a file in each of the above folders
	for _, dir := range [2]string{inProgressSnapshotsPath, completedSnapshotsPath} {
		ioutil.WriteFile(filepath.Join(dir, "testFile"), []byte("some junk data"), 0644)
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
			&lgr.Initializer{
				DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
				MetricsProvider:               &disabled.Provider{},
				Config:                        conf,
				HashProvider:                  cryptoProvider,
			},
		)
		return err
	}

	t.Run("invalid-path", func(t *testing.T) {
		conf, cleanup := testConfig(t)
		defer cleanup()
		conf.SnapshotsConfig.RootDir = "./a-relative-path"
		err := initKVLedgerProvider(conf)
		require.EqualError(t, err, "invalid path: ./a-relative-path. The path for the snapshot dir is expected to be an absolute path")
	})

	t.Run("snapshots final dir creation returns error", func(t *testing.T) {
		conf, cleanup := testConfig(t)
		defer cleanup()

		completedSnapshotsPath := CompletedSnapshotsPath(conf.SnapshotsConfig.RootDir)
		require.NoError(t, os.MkdirAll(filepath.Dir(completedSnapshotsPath), 0755))
		require.NoError(t, ioutil.WriteFile(completedSnapshotsPath, []byte("some data"), 0644))
		err := initKVLedgerProvider(conf)
		require.Error(t, err)
		require.Contains(t, err.Error(), "while creating the dir: "+completedSnapshotsPath)
	})
}

func TestGenerateSnapshotErrors(t *testing.T) {
	conf, cleanup := testConfig(t)
	defer cleanup()
	provider := testutilNewProvider(conf, t, &mock.DeployedChaincodeInfoProvider{})
	defer func() {
		provider.Close()
	}()

	// create a ledger
	_, genesisBlk := testutil.NewBlockGenerator(t, "testLedgerid", false)
	lgr, err := provider.Create(genesisBlk)
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
			InProgressSnapshotsPath(conf.SnapshotsConfig.RootDir),
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
		require.Contains(t, errStackTrace, "fabric/common/ledger/blkstorage/blockindex.go")
	})

	t.Run("config history mgr returns error", func(t *testing.T) {
		closeAndReopenLedgerProvider()
		provider.configHistoryMgr.Close() // close the configHistoryMgr to trigger the error
		err := kvlgr.generateSnapshot()
		require.Error(t, err)
		errStackTrace := fmt.Sprintf("%+v", err)
		require.Contains(t, errStackTrace, "internal leveldb error while obtaining db iterator")
		require.Contains(t, errStackTrace, "fabric/core/ledger/confighistory/mgr.go")
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
		snapshotFinalDir := SnapshotDirForLedgerHeight(conf.SnapshotsConfig.RootDir, "testLedgerid", 1)
		require.NoError(t, os.MkdirAll(snapshotFinalDir, 0744))
		defer os.RemoveAll(snapshotFinalDir)
		require.NoError(t, ioutil.WriteFile( // make a non-empty snapshotFinalDir to trigger failure on rename
			filepath.Join(snapshotFinalDir, "dummyFile"),
			[]byte("dummy file"), 0444),
		)
		err := kvlgr.generateSnapshot()
		require.Contains(t, err.Error(), "error while renaming dir")
	})
}

func TestFileUtilFunctionsErrors(t *testing.T) {
	t.Run("syncDir-openfile", func(t *testing.T) {
		err := syncDir("non-existent-dir")
		require.Error(t, err)
		require.Contains(t, err.Error(), "error while opening dir:non-existent-dir")
	})

	t.Run("createAndSyncFile-openfile", func(t *testing.T) {
		path, err := ioutil.TempDir("", "kvledger")
		err = createAndSyncFile(path, []byte("dummy content"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "error while creating file:"+path)
	})
}

func verifySnapshotOutput(
	t *testing.T,
	snapshotRootDir string,
	ledgerID string,
	ledgerHeight uint64,
	lastBlockHash []byte,
	lastCommitHash []byte,
	expectedBinaryFiles ...string,
) {
	inProgressSnapshotsPath := InProgressSnapshotsPath(snapshotRootDir)
	f, err := ioutil.ReadDir(inProgressSnapshotsPath)
	require.NoError(t, err)
	require.Len(t, f, 0)

	snapshotDir := SnapshotDirForLedgerHeight(snapshotRootDir, ledgerID, ledgerHeight)
	files, err := ioutil.ReadDir(snapshotDir)
	require.NoError(t, err)
	require.Len(t, files, len(expectedBinaryFiles)+2) // + 2 JSON files

	filesAndHashes := map[string]string{}
	for _, f := range expectedBinaryFiles {
		c, err := ioutil.ReadFile(filepath.Join(snapshotDir, f))
		require.NoError(t, err)
		filesAndHashes[f] = hex.EncodeToString(util.ComputeSHA256(c))
	}

	// verify the contents of the file snapshot_metadata.json
	m := &snapshotSignableMetadata{}
	mJSON, err := ioutil.ReadFile(filepath.Join(snapshotDir, snapshotMetadataFileName))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(mJSON, m))
	require.Equal(t,
		&snapshotSignableMetadata{
			ChannelName:        ledgerID,
			ChannelHeight:      ledgerHeight,
			LastBlockHashInHex: hex.EncodeToString(lastBlockHash),
			FilesAndHashes:     filesAndHashes,
		},
		m,
	)

	// verify the contents of the file snapshot_metadata_hash.json
	mh := &snapshotAdditionalInfo{}
	mhJSON, err := ioutil.ReadFile(filepath.Join(snapshotDir, snapshotMetadataHashFileName))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(mhJSON, mh))
	require.Equal(t,
		&snapshotAdditionalInfo{
			SnapshotHashInHex:        hex.EncodeToString(util.ComputeSHA256(mJSON)),
			LastBlockCommitHashInHex: hex.EncodeToString(lastCommitHash),
		},
		mh,
	)
}

func addDummyEntryInCollectionConfigHistory(t *testing.T, provider *Provider, ledgerID string) {
	// configure mock to cause data entry in collection config history
	ccInfoProviderMock := provider.initializer.DeployedChaincodeInfoProvider.(*mock.DeployedChaincodeInfoProvider)
	ccInfoProviderMock.UpdatedChaincodesReturns(
		[]*ledger.ChaincodeLifecycleInfo{
			{
				Name: "ns",
			},
		},
		nil,
	)

	ccInfoProviderMock.ChaincodeInfoReturns(
		&ledger.DeployedChaincodeInfo{
			Name: "ns",
			ExplicitCollectionConfigPkg: &peer.CollectionConfigPackage{
				Config: []*peer.CollectionConfig{
					{
						Payload: &peer.CollectionConfig_StaticCollectionConfig{
							StaticCollectionConfig: &peer.StaticCollectionConfig{
								Name: "coll1",
							},
						},
					},
				},
			},
		},
		nil,
	)
	provider.configHistoryMgr.HandleStateUpdates(
		&ledger.StateUpdateTrigger{
			LedgerID: ledgerID,
			StateUpdates: map[string]*ledger.KVStateUpdates{
				"ns": {},
			},
		},
	)
}
