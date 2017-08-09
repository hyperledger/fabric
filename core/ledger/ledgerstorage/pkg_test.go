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

package ledgerstorage

import (
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/ledger/ledgerconfig"
)

type testEnv struct {
	t testing.TB
}

func newTestEnv(t *testing.T) *testEnv {
	testEnv := &testEnv{t}
	testEnv.cleanup()
	return testEnv
}

func (env *testEnv) cleanup() {
	path := ledgerconfig.GetRootPath()
	os.RemoveAll(path)
}
