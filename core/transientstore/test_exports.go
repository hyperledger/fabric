/*
Copyright IBM Corp. 2017 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package transientstore

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// StoreEnv provides the  store env for testing
type StoreEnv struct {
	t                 testing.TB
	TestStoreProvider StoreProvider
	TestStore         Store
}

// NewTestStoreEnv construct a StoreEnv for testing
func NewTestStoreEnv(t *testing.T) *StoreEnv {
	removeStorePath(t)
	assert := assert.New(t)
	testStoreProvider := NewStoreProvider()
	testStore, err := testStoreProvider.OpenStore("TestStore")
	assert.NoError(err)
	return &StoreEnv{t, testStoreProvider, testStore}
}

// Cleanup cleansup the  store env after testing
func (env *StoreEnv) Cleanup() {
	env.TestStoreProvider.Close()
	removeStorePath(env.t)
}

func removeStorePath(t testing.TB) {
	dbPath := GetTransientStorePath()
	if err := os.RemoveAll(dbPath); err != nil {
		t.Fatalf("Err: %s", err)
		t.FailNow()
	}
}
