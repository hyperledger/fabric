/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmt

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/cceventmgmt"
	"github.com/hyperledger/fabric/core/ledger/mock"
	"github.com/stretchr/testify/assert"
)

func TestLedgerMgmt(t *testing.T) {
	// Check for error when creating/opening ledger without initialization.
	gb, _ := test.MakeGenesisBlock(constructTestLedgerID(0))
	l, err := CreateLedger(gb)
	assert.Nil(t, l)
	assert.Equal(t, ErrLedgerMgmtNotInitialized, err)

	ledgerID := constructTestLedgerID(2)
	l, err = OpenLedger(ledgerID)
	assert.Nil(t, l)
	assert.Equal(t, ErrLedgerMgmtNotInitialized, err)

	ids, err := GetLedgerIDs()
	assert.Nil(t, ids)
	assert.Equal(t, ErrLedgerMgmtNotInitialized, err)

	Close()

	rootPath, err := ioutil.TempDir("", "lgrmgmt")
	if err != nil {
		t.Fatalf("Failed to create ledger directory: %s", err)
	}

	initializer := &Initializer{
		PlatformRegistry: platforms.NewRegistry(&golang.Platform{}),
		MetricsProvider:  &disabled.Provider{},
		Config: &ledger.Config{
			RootFSPath:    rootPath,
			StateDBConfig: &ledger.StateDBConfig{},
		},
	}

	cleanup, err := InitializeTestEnvWithInitializer(initializer)
	if err != nil {
		t.Fatalf("Failed to initialize test environment: %s", err)
	}
	defer cleanup()

	numLedgers := 10
	ledgers := make([]ledger.PeerLedger, numLedgers)
	for i := 0; i < numLedgers; i++ {
		gb, _ := test.MakeGenesisBlock(constructTestLedgerID(i))
		l, _ := CreateLedger(gb)
		ledgers[i] = l
	}

	ids, _ = GetLedgerIDs()
	assert.Len(t, ids, numLedgers)
	for i := 0; i < numLedgers; i++ {
		assert.Equal(t, constructTestLedgerID(i), ids[i])
	}

	ledgerID = constructTestLedgerID(2)
	t.Logf("Ledger selected for test = %s", ledgerID)
	_, err = OpenLedger(ledgerID)
	assert.Equal(t, ErrLedgerAlreadyOpened, err)

	l = ledgers[2]
	l.Close()
	l, err = OpenLedger(ledgerID)
	assert.NoError(t, err)

	l, err = OpenLedger(ledgerID)
	assert.Equal(t, ErrLedgerAlreadyOpened, err)

	// close all opened ledgers and ledger mgmt
	Close()

	// Restart ledger mgmt with existing ledgers
	Initialize(initializer)
	l, err = OpenLedger(ledgerID)
	assert.NoError(t, err)
	Close()
}

func TestChaincodeInfoProvider(t *testing.T) {
	cleanup := InitializeTestEnv(t)
	defer cleanup()
	gb, _ := test.MakeGenesisBlock("ledger1")
	CreateLedger(gb)

	mockDeployedCCInfoProvider := &mock.DeployedChaincodeInfoProvider{}
	mockDeployedCCInfoProvider.ChaincodeInfoStub = func(channelName, ccName string, qe ledger.SimpleQueryExecutor) (*ledger.DeployedChaincodeInfo, error) {
		return constructTestCCInfo(ccName, ccName, ccName), nil
	}

	ccInfoProvider := &chaincodeInfoProviderImpl{
		platforms.NewRegistry(&golang.Platform{}),
		mockDeployedCCInfoProvider,
	}
	_, err := ccInfoProvider.GetDeployedChaincodeInfo("ledger2", constructTestCCDef("cc2", "1.0", "cc2Hash"))
	t.Logf("Expected error received = %s", err)
	assert.Error(t, err)

	ccInfo, err := ccInfoProvider.GetDeployedChaincodeInfo("ledger1", constructTestCCDef("cc1", "non-matching-version", "cc1"))
	assert.NoError(t, err)
	assert.Nil(t, ccInfo)

	ccInfo, err = ccInfoProvider.GetDeployedChaincodeInfo("ledger1", constructTestCCDef("cc1", "cc1", "non-matching-hash"))
	assert.NoError(t, err)
	assert.Nil(t, ccInfo)

	ccInfo, err = ccInfoProvider.GetDeployedChaincodeInfo("ledger1", constructTestCCDef("cc1", "cc1", "cc1"))
	assert.NoError(t, err)
	assert.Equal(t, constructTestCCInfo("cc1", "cc1", "cc1"), ccInfo)
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
