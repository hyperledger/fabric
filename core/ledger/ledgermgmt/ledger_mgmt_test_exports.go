/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmt

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/chaincode/platforms"
	"github.com/hyperledger/fabric/core/chaincode/platforms/golang"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/mock"
)

//TODO:  Remove all of these functions and create ledger provider instances

// InitializeTestEnv initializes ledgermgmt for tests
func InitializeTestEnv(t *testing.T) (cleanup func()) {
	cleanup, err := InitializeTestEnvWithInitializer(nil)
	if err != nil {
		t.Fatalf("Failed to initialize test environment: %s", err)
	}
	return cleanup
}

// InitializeTestEnvWithInitializer initializes ledgermgmt for tests with the supplied Initializer
func InitializeTestEnvWithInitializer(initializer *Initializer) (cleanup func(), err error) {
	return InitializeExistingTestEnvWithInitializer(initializer)
}

// InitializeExistingTestEnvWithInitializer initializes ledgermgmt for tests with existing ledgers
// This function does not remove the existing ledgers and is used in upgrade tests
// TODO ledgermgmt should be reworked to move the package scoped functions to a struct
func InitializeExistingTestEnvWithInitializer(initializer *Initializer) (cleanup func(), err error) {
	if initializer == nil {
		initializer = &Initializer{}
	}
	if initializer.DeployedChaincodeInfoProvider == nil {
		initializer.DeployedChaincodeInfoProvider = &mock.DeployedChaincodeInfoProvider{}
	}
	if initializer.MetricsProvider == nil {
		initializer.MetricsProvider = &disabled.Provider{}
	}
	if initializer.PlatformRegistry == nil {
		initializer.PlatformRegistry = platforms.NewRegistry(&golang.Platform{})
	}
	if initializer.Config == nil {
		rootPath, err := ioutil.TempDir("", "ltestenv")
		if err != nil {
			return nil, err
		}
		initializer.Config = &ledger.Config{
			RootFSPath:    rootPath,
			StateDBConfig: &ledger.StateDBConfig{},
		}
	}
	if initializer.Config.PrivateDataConfig == nil {
		initializer.Config.PrivateDataConfig = &ledger.PrivateDataConfig{
			MaxBatchSize:    5000,
			BatchesInterval: 1000,
			PurgeInterval:   100,
		}
	}
	if initializer.Config.HistoryDBConfig == nil {
		initializer.Config.HistoryDBConfig = &ledger.HistoryDBConfig{
			Enabled: true,
		}
	}
	initialize(initializer)
	cleanup = func() {
		Close()
		os.RemoveAll(initializer.Config.RootFSPath)
	}
	return cleanup, nil
}
