/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmt

import (
	"fmt"
	"io/ioutil"
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
	testDir, err := ioutil.TempDir("", "ledgermgmt")
	if err != nil {
		t.Fatalf("Failed to create ledger directory: %s", err)
	}
	initializer, err := constructDefaultInitializer(testDir)
	if err != nil {
		t.Fatalf("Failed to create default initializer: %s", err)
	}

	ledgerMgr := NewLedgerMgr(initializer)
	defer func() {
		os.RemoveAll(testDir)
	}()

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
	_, err = ledgerMgr.OpenLedger(ledgerID)
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
	testDir, err := ioutil.TempDir("", "createledgerfromsnapshot")
	require.NoError(t, err)
	initializer, err := constructDefaultInitializer(testDir)
	require.NoError(t, err)
	ledgerMgr := NewLedgerMgr(initializer)
	defer func() {
		os.RemoveAll(testDir)
	}()

	channelID := "testcreatefromsnapshot"
	_, gb := testutil.NewBlockGenerator(t, channelID, false)
	l, err := ledgerMgr.CreateLedger(channelID, gb)
	require.NoError(t, err)

	// submit a snapshot request, wait until it is generated (request removed from pending requests)
	require.NoError(t, l.SubmitSnapshotRequest(0))
	snapshotDir := kvledger.SnapshotDirForLedgerBlockNum(initializer.Config.SnapshotsConfig.RootDir, channelID, 0)
	snapshotGenerated := func() bool {
		pendingRequests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return len(pendingRequests) == 0
	}
	require.Eventually(t, snapshotGenerated, 30*time.Second, 100*time.Millisecond)

	ledgerMgr.Close()

	// re-create the ledger from snapshot under the same rootdir should return error because the ledger already exists
	ledgerMgr = NewLedgerMgr(initializer)
	_, _, err = ledgerMgr.CreateLedgerFromSnapshot(snapshotDir)
	require.EqualError(t, err, "error while creating ledger id: ledger [testcreatefromsnapshot] already exists with state [ACTIVE]")

	// re-create the ledger from snapshot under a different root dir should work because the idStore is empty
	testDir2, err := ioutil.TempDir("", "createledgerfromsnapshot2")
	require.NoError(t, err)
	initializer2, err := constructDefaultInitializer(testDir2)
	require.NoError(t, err)

	ledgerMgr2 := NewLedgerMgr(initializer2)
	defer func() {
		ledgerMgr2.Close()
		os.RemoveAll(testDir2)
	}()

	l2, cid, err := ledgerMgr2.CreateLedgerFromSnapshot(snapshotDir)
	require.NoError(t, err)
	require.Equal(t, channelID, cid)
	bcInfo, _ := l2.GetBlockchainInfo()
	require.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: protoutil.BlockHeaderHash(gb.Header), PreviousBlockHash: nil,
	}, bcInfo)
}

func TestChaincodeInfoProvider(t *testing.T) {
	testDir, err := ioutil.TempDir("", "ledgermgmt")
	if err != nil {
		t.Fatalf("Failed to create ledger directory: %s", err)
	}
	initializer, err := constructDefaultInitializer(testDir)
	if err != nil {
		t.Fatalf("Failed to create default initializer: %s", err)
	}

	ledgerMgr := NewLedgerMgr(initializer)
	defer func() {
		ledgerMgr.Close()
		os.RemoveAll(testDir)
	}()

	gb, _ := test.MakeGenesisBlock("ledger1")
	_, err = ledgerMgr.CreateLedger("ledger1", gb)
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
