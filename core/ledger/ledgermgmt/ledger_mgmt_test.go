/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmt

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

func TestLedgerMgmt(t *testing.T) {
	initializer, ledgerMgr, cleanup := setup(t)
	defer cleanup()

	numLedgers := 10
	ledgers := make([]ledger.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		cid := constructTestLedgerID(i)
		gb, _ := test.MakeGenesisBlock(cid)
		l, err := ledgerMgr.CreateLedger(cid, gb)
		require.NoError(t, err)
		ledgers[i] = l
	}

	ids, _ := ledgerMgr.GetLedgerIDs()
	require.Len(t, ids, numLedgers)
	for i := 0; i < numLedgers; i++ {
		require.Equal(t, constructTestLedgerID(i), ids[i])
	}

	ledgerID := constructTestLedgerID(2)
	t.Logf("Ledger selected for test = %s", ledgerID)
	_, err := ledgerMgr.OpenLedger(ledgerID)
	require.Equal(t, ErrLedgerAlreadyOpened, err)

	l := ledgers[2]
	l.Close()
	// attempt to close the same ledger twice and ensure it doesn't panic
	require.NotPanics(t, l.Close)

	_, err = ledgerMgr.OpenLedger(ledgerID)
	require.NoError(t, err)

	_, err = ledgerMgr.OpenLedger(ledgerID)
	require.Equal(t, ErrLedgerAlreadyOpened, err)
	// close all opened ledgers and ledger mgmt
	ledgerMgr.Close()

	// Recreate LedgerMgr with existing ledgers
	ledgerMgr = NewLedgerMgr(initializer)
	_, err = ledgerMgr.OpenLedger(ledgerID)
	require.NoError(t, err)
	ledgerMgr.Close()
}

// TestCreateLedgerFromSnapshot first creates a ledger using a genesis block and generates a snapshot.
// After it, it tests creating ledger from the snapshot.
func TestCreateLedgerFromSnapshot(t *testing.T) {
	initializer, lgrMgr, cleanup := setup(t)
	defer cleanup()

	channelID := "testcreatefromsnapshot"
	snapshotDir, gb := generateSnapshot(t, lgrMgr, initializer, channelID)

	t.Run("create_ledger_from_snapshot_internal", func(t *testing.T) {
		_, ledgerMgr, cleanup := setup(t)
		defer cleanup()

		l, _, err := ledgerMgr.createFromSnapshot(snapshotDir)
		require.NoError(t, err)
		bcInfo, _ := l.GetBlockchainInfo()
		require.Equal(t, &common.BlockchainInfo{
			Height:            1,
			CurrentBlockHash:  protoutil.BlockHeaderHash(gb.Header),
			PreviousBlockHash: nil,
			BootstrappingSnapshotInfo: &common.BootstrappingSnapshotInfo{
				LastBlockInSnapshot: 0,
			},
		}, bcInfo)
	})

	t.Run("create_ledger_from_snapshot_async", func(t *testing.T) {
		_, ledgerMgr, cleanup := setup(t)
		defer cleanup()

		callbackCounter := 0
		callback := func(l ledger.PeerLedger, cid string) { callbackCounter++ }

		require.NoError(t, ledgerMgr.CreateLedgerFromSnapshot(snapshotDir, callback))

		ledgerCreated := func() bool {
			status := ledgerMgr.JoinBySnapshotStatus()
			return !status.InProgress && status.BootstrappingSnapshotDir == ""
		}

		require.Eventually(t, ledgerCreated, time.Minute, time.Second)
		require.Equal(t, 1, callbackCounter)

		ledgerids, err := ledgerMgr.GetLedgerIDs()
		require.NoError(t, err)
		require.Equal(t, ledgerids, []string{channelID})
	})

	t.Run("create_existing_ledger_returns_error", func(t *testing.T) {
		// create the ledger from snapshot under the same rootdir should return error because the ledger already exists
		_, _, err := lgrMgr.createFromSnapshot(snapshotDir)
		require.EqualError(t, err, "error while creating ledger id: ledger [testcreatefromsnapshot] already exists with state [ACTIVE]")
	})

	t.Run("create_ledger_from_nonexist_or_empty_dir_returns_error", func(t *testing.T) {
		testDir := t.TempDir()

		nonExistDir := filepath.Join(testDir, "nonexistdir")
		require.EqualError(t, lgrMgr.CreateLedgerFromSnapshot(nonExistDir, nil),
			fmt.Sprintf("error opening dir [%s]: open %s: no such file or directory", nonExistDir, nonExistDir))

		require.EqualError(t, lgrMgr.CreateLedgerFromSnapshot(testDir, nil),
			fmt.Sprintf("snapshot dir %s is empty", testDir))
	})

	t.Run("callback_func_is_not_called_if_create_ledger_from_snapshot_failed", func(t *testing.T) {
		initializer, ledgerMgr, cleanup := setup(t)
		defer cleanup()

		// copy snapshotDir to a new dir and remove a metadata file so that kvledger.CreateFromSnapshot will fail
		require.NoError(t, os.MkdirAll(initializer.Config.SnapshotsConfig.RootDir, 0o755))
		require.NoError(t, testutil.CopyDir(snapshotDir, initializer.Config.SnapshotsConfig.RootDir, false))
		newSnapshotDir := filepath.Join(initializer.Config.SnapshotsConfig.RootDir, "0")
		require.NoError(t, os.Remove(filepath.Join(newSnapshotDir, "_snapshot_signable_metadata.json")))

		callbackCounter := 0
		callback := func(l ledger.PeerLedger, cid string) { callbackCounter++ }

		require.NoError(t, ledgerMgr.CreateLedgerFromSnapshot(newSnapshotDir, callback))

		// wait until CreateFromSnapshot is done
		ledgerCreated := func() bool {
			status := ledgerMgr.JoinBySnapshotStatus()
			return !status.InProgress && status.BootstrappingSnapshotDir == ""
		}
		require.Eventually(t, ledgerCreated, time.Minute, time.Second)

		// callback should not be called and ledger should not be generated
		require.Equal(t, 0, callbackCounter)

		ids, err := ledgerMgr.GetLedgerIDs()
		require.NoError(t, err)
		require.Equal(t, 0, len(ids))
	})
}

func TestConcurrentCreateLedgerFromGB(t *testing.T) {
	_, ledgerMgr, cleanup := setup(t)
	defer cleanup()

	var err error
	gbs := make([]*common.Block, 0, 5)
	for i := 0; i < len(gbs); i++ {
		gbs[i], err = test.MakeGenesisBlock(fmt.Sprintf("l%d", i))
		require.NoError(t, err)
	}

	// verify CreateLedger (from genesisblock) can be called concurrently
	for i := 0; i < len(gbs); i++ {
		gb := gbs[i]
		ledgerID := fmt.Sprintf("l%d", i)
		go func() {
			_, err := ledgerMgr.CreateLedger(ledgerID, gb)
			require.NoError(t, err)
		}()
	}

	ledgersGenerated := func() bool {
		ledgerIds, err := ledgerMgr.GetLedgerIDs()
		require.NoError(t, err)
		return len(ledgerIds) == len(gbs)
	}
	require.Eventually(t, ledgersGenerated, time.Minute, time.Second)
}

func TestConcurrentCreateLedgerFromSnapshot(t *testing.T) {
	initializer, ledgerMgr, cleanup := setup(t)
	defer cleanup()

	// generate 2 snapshots for 2 channels
	channelID1 := "testcreatefromsnapshot1"
	snapshotDir1, _ := generateSnapshot(t, ledgerMgr, initializer, channelID1)
	channelID2 := "testcreatefromsnapshot2"
	snapshotDir2, _ := generateSnapshot(t, ledgerMgr, initializer, channelID2)

	ledgerMgr.Close()

	// create a new ledger mgr to import snapshot
	_, ledgerMgr2, cleanup2 := setup(t)
	defer cleanup2()

	// use a channel to keep the callback func waiting so that we can test concurrent CreateLedger/CreateLedgerBySnapshot calls
	waitCh := make(chan struct{})
	callback := func(l ledger.PeerLedger, cid string) {
		<-waitCh
	}
	require.NoError(t, ledgerMgr2.CreateLedgerFromSnapshot(snapshotDir1, callback))

	// concurrent CreateLedger call should fail
	channelID3 := "ledgerfromgb"
	gb, err := test.MakeGenesisBlock(channelID3)
	require.NoError(t, err)
	_, err = ledgerMgr2.CreateLedger(channelID3, gb)
	require.EqualError(t, err, fmt.Sprintf("a ledger is being created from a snapshot at %s. Call ledger creation again after it is done.", snapshotDir1))

	// concurrent CreateLedgerBySnapshot call should fail
	err = ledgerMgr2.CreateLedgerFromSnapshot(snapshotDir2, callback)
	require.EqualError(t, err, fmt.Sprintf("a ledger is being created from a snapshot at %s. Call ledger creation again after it is done.", snapshotDir1))

	waitCh <- struct{}{}
	ledgerCreated := func() bool {
		status := ledgerMgr.JoinBySnapshotStatus()
		return !status.InProgress && status.BootstrappingSnapshotDir == ""
	}
	require.Eventually(t, ledgerCreated, time.Minute, time.Second)

	ledgerIDs, err := ledgerMgr2.GetLedgerIDs()
	require.NoError(t, err)
	require.Equal(t, ledgerIDs, []string{channelID1})

	// CreateLedger should work after the previous CreateLedgerFromSnapshot is done
	_, err = ledgerMgr2.CreateLedger(channelID3, gb)
	require.NoError(t, err, "creating ledger for %s should have succeeded", channelID3)

	// CreateLedgerFromSnapshot should work after the previous CreateLedgerFromSnapshot is done
	callback = func(l ledger.PeerLedger, cid string) {}
	require.NoError(t, ledgerMgr2.CreateLedgerFromSnapshot(snapshotDir2, callback))

	// wait until ledger is created from snapshotDir2
	ledgerCreated = func() bool {
		status := ledgerMgr2.JoinBySnapshotStatus()
		return !status.InProgress && status.BootstrappingSnapshotDir == ""
	}
	require.Eventually(t, ledgerCreated, time.Minute, time.Second)

	ledgerIDs, err = ledgerMgr2.GetLedgerIDs()
	require.NoError(t, err)
	require.ElementsMatch(t, ledgerIDs, []string{channelID1, channelID2, channelID3})
}

func TestChaincodeInfoProvider(t *testing.T) {
	_, ledgerMgr, cleanup := setup(t)
	defer cleanup()

	gb, _ := test.MakeGenesisBlock("ledger1")
	_, err := ledgerMgr.CreateLedger("ledger1", gb)
	require.NoError(t, err)

	mockDeployedCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockDeployedCCInfoProvider.ChaincodeInfoStub = func(channelName, ccName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
		return constructTestCCInfo(ccName, ccName, ccName), nil
	}

	ccInfoProvider := &chaincodeInfoProviderImpl{
		ledgerMgr,
		mockDeployedCCInfoProvider,
	}
	_, err = ccInfoProvider.GetDeployedChaincodeInfo("ledger2", constructTestCCDef("cc2", "1.0", "cc2Hash"))
	t.Logf("Expected error received = %s", err)
	require.Error(t, err)

	ccInfo, err := ccInfoProvider.GetDeployedChaincodeInfo("ledger1", constructTestCCDef("cc1", "non-matching-version", "cc1"))
	require.NoError(t, err)
	require.Nil(t, ccInfo)

	ccInfo, err = ccInfoProvider.GetDeployedChaincodeInfo("ledger1", constructTestCCDef("cc1", "cc1", "non-matching-hash"))
	require.NoError(t, err)
	require.Nil(t, ccInfo)

	ccInfo, err = ccInfoProvider.GetDeployedChaincodeInfo("ledger1", constructTestCCDef("cc1", "cc1", "cc1"))
	require.NoError(t, err)
	require.Equal(t, constructTestCCInfo("cc1", "cc1", "cc1"), ccInfo)
}

func setup(t *testing.T) (*Initializer, *LedgerMgr, func()) {
	testDir := t.TempDir()
	initializer, err := constructDefaultInitializer(testDir)
	require.NoError(t, err)
	ledgerMgr := NewLedgerMgr(initializer)
	cleanup := func() {
		ledgerMgr.Close()
	}
	return initializer, ledgerMgr, cleanup
}

func constructDefaultInitializer(testDir string) (*Initializer, error) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}
	return &Initializer{
		Config: &ledger.Config{
			RootFSPath:    testDir,
			StateDBConfig: &ledger.StateDBConfig{},
			PrivateDataConfig: &ledger.PrivateDataConfig{
				MaxBatchSize:    5000,
				BatchesInterval: 1000,
				PurgeInterval:   100,
			},
			HistoryDBConfig: &ledger.HistoryDBConfig{
				Enabled: true,
			},
			SnapshotsConfig: &ledger.SnapshotsConfig{
				RootDir: filepath.Join(testDir, "snapshots"),
			},
		},

		MetricsProvider:               &disabled.Provider{},
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
		HashProvider:                  cryptoProvider,
	}, nil
}

func constructTestLedgerID(i int) string {
	return fmt.Sprintf("ledger_%06d", i)
}

func constructTestCCInfo(ccName, version, hash string) *ledger.DeployedChaincodeInfo {
	return &ledger.DeployedChaincodeInfo{
		Name:    ccName,
		Hash:    []byte(hash),
		Version: version,
	}
}

func constructTestCCDef(ccName, version, hash string) *cceventmgmt.ChaincodeDefinition {
	return &cceventmgmt.ChaincodeDefinition{
		Name:    ccName,
		Hash:    []byte(hash),
		Version: version,
	}
}

// generateSnapshot creates a ledger with genesis block and generates a snapshot when the ledger only has genesisblock
func generateSnapshot(t *testing.T, ledgerMgr *LedgerMgr, initializer *Initializer, channelID string) (string, *common.Block) {
	_, gb := testutil.NewBlockGenerator(t, channelID, false)
	l, err := ledgerMgr.CreateLedger(channelID, gb)
	require.NoError(t, err)

	require.NoError(t, l.SubmitSnapshotRequest(0))

	snapshotDir := kvledger.SnapshotDirForLedgerBlockNum(initializer.Config.SnapshotsConfig.RootDir, channelID, 0)
	snapshotGenerated := func() bool {
		pendingRequests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return len(pendingRequests) == 0
	}
	require.Eventually(t, snapshotGenerated, 30*time.Second, 100*time.Millisecond)

	return snapshotDir, gb
}
