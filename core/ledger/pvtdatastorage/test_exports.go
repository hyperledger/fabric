/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/stretchr/testify/require"
)

func pvtDataConf() *PrivateDataConfig {
	return &PrivateDataConfig{
		PrivateDataConfig: &ledger.PrivateDataConfig{
			BatchesInterval:                     1000,
			MaxBatchSize:                        5000,
			PurgeInterval:                       2,
			DeprioritizedDataReconcilerInterval: 120 * time.Minute,
		},
		StorePath: "",
	}
}

// StoreEnv provides the  store env for testing
type StoreEnv struct {
	t                 testing.TB
	TestStoreProvider *Provider
	TestStore         *Store
	ledgerid          string
	btlPolicy         pvtdatapolicy.BTLPolicy
	conf              *PrivateDataConfig
}

// NewTestStoreEnv construct a StoreEnv for testing
func NewTestStoreEnv(
	t *testing.T,
	ledgerid string,
	btlPolicy pvtdatapolicy.BTLPolicy,
	conf *PrivateDataConfig) *StoreEnv {
	conf.StorePath = t.TempDir()
	testStoreProvider, err := NewProvider(conf)
	require.NoError(t, err)
	testStore, err := testStoreProvider.OpenStore(ledgerid)
	testStore.Init(btlPolicy)
	require.NoError(t, err)
	return &StoreEnv{t, testStoreProvider, testStore, ledgerid, btlPolicy, conf}
}

// CloseAndReopen closes and opens the store provider
func (env *StoreEnv) CloseAndReopen() {
	var err error
	env.TestStoreProvider.Close()
	env.TestStoreProvider, err = NewProvider(env.conf)
	require.NoError(env.t, err)
	env.TestStore, err = env.TestStoreProvider.OpenStore(env.ledgerid)
	env.TestStore.Init(env.btlPolicy)
	require.NoError(env.t, err)
}

// Cleanup cleansup the  store env after testing
func (env *StoreEnv) Cleanup() {
	env.TestStoreProvider.Close()
}
