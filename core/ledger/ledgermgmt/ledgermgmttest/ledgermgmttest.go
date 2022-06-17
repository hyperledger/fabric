/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmttest

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

// NewInitializer returns an instance of ledgermgmt Initializer
// with minimum fields populated so as not to cause a failure during construction of LedgerMgr.
// This is intended to be used for creating an instance of LedgerMgr for testing
func NewInitializer(testLedgerDir string) *ledgermgmt.Initializer {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		panic(fmt.Errorf("Failed to initialize cryptoProvider bccsp: %s", err))
	}

	return &ledgermgmt.Initializer{
		Config: &ledger.Config{
			RootFSPath: testLedgerDir,
			// empty StateDBConfig means leveldb
			StateDBConfig: &ledger.StateDBConfig{},
			HistoryDBConfig: &ledger.HistoryDBConfig{
				Enabled: false,
			},
			PrivateDataConfig: &ledger.PrivateDataConfig{
				MaxBatchSize:    5000,
				BatchesInterval: 1000,
				PurgeInterval:   100,
			},
			SnapshotsConfig: &ledger.SnapshotsConfig{
				RootDir: filepath.Join(testLedgerDir, "snapshots"),
			},
		},
		MetricsProvider:                 &disabled.Provider{},
		DeployedChaincodeInfoProvider:   &mock.DeployedChaincodeInfoProvider{},
		HashProvider:                    cryptoProvider,
		HealthCheckRegistry:             &mock.HealthCheckRegistry{},
		ChaincodeLifecycleEventProvider: &mock.ChaincodeLifecycleEventProvider{},
	}
}

// CreateSnapshotWithGenesisBlock creates a snapshot with only genesis block for the given ledgerID and returns
// the snapshot directory. It is intended to be used for creating a ledger by snapshot for testing purpose.
func CreateSnapshotWithGenesisBlock(t *testing.T, testDir string, ledgerID string, configTxProcessor ledger.CustomTxProcessor) string {
	// use a tmpdir to create the ledger for ledgerID so that we can create the snapshot
	tmpDir := t.TempDir()

	initializer := NewInitializer(tmpDir)
	initializer.CustomTxProcessors = map[common.HeaderType]ledger.CustomTxProcessor{
		common.HeaderType_CONFIG: configTxProcessor,
	}
	initializer.Config.SnapshotsConfig.RootDir = testDir
	ledgerMgr := ledgermgmt.NewLedgerMgr(initializer)
	defer ledgerMgr.Close()

	_, gb := testutil.NewBlockGenerator(t, ledgerID, false)
	l, err := ledgerMgr.CreateLedger(ledgerID, gb)
	require.NoError(t, err)

	require.NoError(t, l.SubmitSnapshotRequest(0))
	snapshotDir := kvledger.SnapshotDirForLedgerBlockNum(initializer.Config.SnapshotsConfig.RootDir, ledgerID, 0)
	snapshotGenerated := func() bool {
		pendingRequests, err := l.PendingSnapshotRequests()
		require.NoError(t, err)
		return len(pendingRequests) == 0
	}
	require.Eventually(t, snapshotGenerated, 30*time.Second, 100*time.Millisecond)

	return snapshotDir
}
