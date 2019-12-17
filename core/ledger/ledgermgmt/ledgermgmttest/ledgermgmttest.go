/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledgermgmttest

import (
	"fmt"

	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/metrics/disabled"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/mock"
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
		},
		MetricsProvider:               &disabled.Provider{},
		DeployedChaincodeInfoProvider: &mock.DeployedChaincodeInfoProvider{},
		Hasher:                        cryptoProvider,
	}
}
