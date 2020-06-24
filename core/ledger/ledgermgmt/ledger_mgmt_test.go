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

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/mock"
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
	ledgerMgr.CreateLedger("ledger1", gb)

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
