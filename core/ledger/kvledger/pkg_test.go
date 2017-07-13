/*
Copyright IBM Corp. 2016 All Rights Reserved.

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

package kvledger

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
	"github.com/spf13/viper"
)

type testEnv struct {
	t testing.TB
}

func newTestEnv(t testing.TB) *testEnv {
	return createTestEnv(t, "/tmp/fabric/ledgertests/kvledger")
}

func createTestEnv(t testing.TB, path string) *testEnv {
	viper.Set("peer.fileSystemPath", path)
	env := &testEnv{t}
	env.cleanup()
	return env
}

func (env *testEnv) cleanup() {
	path := ledgerconfig.GetRootPath()
	os.RemoveAll(path)
}
