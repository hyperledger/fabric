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

package application

import (
	"testing"

	api "github.com/hyperledger/fabric/common/configvalues"

	logging "github.com/op/go-logging"
)

func init() {
	logging.SetLevel(logging.DEBUG, "")
}

func TestApplicationInterface(t *testing.T) {
	_ = api.Application(NewSharedConfigImpl(nil))
}

func TestApplicationDoubleBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewSharedConfigImpl(nil)
	m.BeginValueProposals(nil)
	m.BeginValueProposals(nil)
}

func TestApplicationCommitWithoutBegin(t *testing.T) {
	defer func() {
		if err := recover(); err == nil {
			t.Fatalf("Should have panicked on multiple begin configs")
		}
	}()

	m := NewSharedConfigImpl(nil)
	m.CommitProposals()
}

func TestApplicationRollback(t *testing.T) {
	m := NewSharedConfigImpl(nil)
	m.pendingConfig = &sharedConfig{}
	m.RollbackProposals()
	if m.pendingConfig != nil {
		t.Fatalf("Should have cleared pending config on rollback")
	}
}
