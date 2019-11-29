/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/pvtdatapolicy"
	"github.com/stretchr/testify/assert"
)

func pvtDataConf() *PrivateDataConfig {
	return &PrivateDataConfig{
		PrivateDataConfig: &ledger.PrivateDataConfig{
			BatchesInterval: 1000,
			MaxBatchSize:    5000,
			PurgeInterval:   2,
		},
		StorePath: "",
	}
}

// StoreEnv provides the  store env for testing
type StoreEnv struct {
	t                 testing.TB
	TestStoreProvider Provider
	TestStore         Store
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

	storeDir, err := ioutil.TempDir("", "pdstore")
	if err != nil {
		t.Fatalf("Failed to create private data storage directory: %s", err)
	}
	assert := assert.New(t)
	conf.StorePath = storeDir
	testStoreProvider, err := NewProvider(conf)
	assert.NoError(err)
	testStore, err := testStoreProvider.OpenStore(ledgerid)
	testStore.Init(btlPolicy)
	assert.NoError(err)
	return &StoreEnv{t, testStoreProvider, testStore, ledgerid, btlPolicy, conf}
}

// CloseAndReopen closes and opens the store provider
func (env *StoreEnv) CloseAndReopen() {
	var err error
	env.TestStoreProvider.Close()
	env.TestStoreProvider, err = NewProvider(env.conf)
	assert.NoError(env.t, err)
	env.TestStore, err = env.TestStoreProvider.OpenStore(env.ledgerid)
	env.TestStore.Init(env.btlPolicy)
	assert.NoError(env.t, err)
}

// Cleanup cleansup the  store env after testing
func (env *StoreEnv) Cleanup() {
	os.RemoveAll(env.conf.StorePath)
}
