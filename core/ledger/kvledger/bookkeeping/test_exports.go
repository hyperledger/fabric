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

package bookkeeping

import (
	"os"
	"testing"
)

// TestEnv provides the bookkeeper provider env for testing
type TestEnv struct {
	t            testing.TB
	TestProvider Provider
}

// NewTestEnv construct a TestEnv for testing
func NewTestEnv(t testing.TB) *TestEnv {
	removePath(t)
	provider := NewProvider()
	return &TestEnv{t, provider}
}

// Cleanup cleansup the  store env after testing
func (env *TestEnv) Cleanup() {
	env.TestProvider.Close()
	removePath(env.t)
}

func removePath(t testing.TB) {
	dbPath := getInternalBookkeeperPath()
	if err := os.RemoveAll(dbPath); err != nil {
		t.Fatalf("Err: %s", err)
		t.FailNow()
	}
}
