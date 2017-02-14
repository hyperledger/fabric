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

package handlers

import (
	"testing"

	configtxapi "github.com/hyperledger/fabric/common/configtx/api"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestInterface(t *testing.T) {
	_ = configtxapi.SubInitializer(NewOrgConfig("id", nil))
}

func TestDoubleBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewOrgConfig("id", nil)
	m.BeginConfig()
	m.BeginConfig()
}

func TestCommitWithoutBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewOrgConfig("id", nil)
	m.CommitConfig()
}

func TestRollback(t *testing.T) {
	m := NewOrgConfig("id", nil)
	m.pendingConfig = &orgConfig{}
	m.RollbackConfig()
	if m.pendingConfig != nil {
		t.Fatalf("Should have cleared pending config on rollback")
	}
}
