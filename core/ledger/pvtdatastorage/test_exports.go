/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvtdatastorage

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/stretchr/testify/assert"
)

const testStoreid = "TestStore"

// StoreEnv provides the  store env for testing
type StoreEnv struct {
	t                 testing.TB
	TestStoreProvider Provider
	TestStore         Store
}

// NewTestStoreEnv construct a StoreEnv for testing
func NewTestStoreEnv(t *testing.T) *StoreEnv {
	removeStorePath(t)
	assert := assert.New(t)
	testStoreProvider := NewProvider()
	testStore, err := testStoreProvider.OpenStore(testStoreid)
	assert.NoError(err)
	return &StoreEnv{t, testStoreProvider, testStore}
}

// CloseAndReopen closes and opens the store provider
func (env *StoreEnv) CloseAndReopen() {
	var err error
	env.TestStoreProvider.Close()
	env.TestStoreProvider = NewProvider()
	env.TestStore, err = env.TestStoreProvider.OpenStore(testStoreid)
	assert.NoError(env.t, err)
}

// Cleanup cleansup the  store env after testing
func (env *StoreEnv) Cleanup() {
	env.TestStoreProvider.Close()
	removeStorePath(env.t)
}

func removeStorePath(t testing.TB) {
	dbPath := ledgerconfig.GetPvtdataStorePath()
	if err := os.RemoveAll(dbPath); err != nil {
		t.Fatalf("Err: %s", err)
		t.FailNow()
	}
}
