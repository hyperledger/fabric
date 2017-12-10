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

package scc

import (
	"github.com/hyperledger/fabric/common/channelconfig"
	lm "github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
)

type MocksccProviderFactory struct {
	Qe                    *lm.MockQueryExecutor
	QErr                  error
	ApplicationConfigRv   channelconfig.Application
	ApplicationConfigBool bool
	PolicyManagerRv       policies.Manager
	PolicyManagerBool     bool
}

func (c *MocksccProviderFactory) NewSystemChaincodeProvider() sysccprovider.SystemChaincodeProvider {
	return &MocksccProviderImpl{
		Qe:                    c.Qe,
		QErr:                  c.QErr,
		ApplicationConfigRv:   c.ApplicationConfigRv,
		ApplicationConfigBool: c.ApplicationConfigBool,
		PolicyManagerBool:     c.PolicyManagerBool,
		PolicyManagerRv:       c.PolicyManagerRv,
	}
}

type MocksccProviderImpl struct {
	Qe                    *lm.MockQueryExecutor
	QErr                  error
	ApplicationConfigRv   channelconfig.Application
	ApplicationConfigBool bool
	PolicyManagerRv       policies.Manager
	PolicyManagerBool     bool
	SysCCMap              map[string]bool
}

func (c *MocksccProviderImpl) IsSysCC(name string) bool {
	if c.SysCCMap != nil {
		return c.SysCCMap[name]
	}

	return (name == "lscc") || (name == "escc") || (name == "vscc") || (name == "notext")
}

func (c *MocksccProviderImpl) IsSysCCAndNotInvokableCC2CC(name string) bool {
	return (name == "escc") || (name == "vscc")
}

func (c *MocksccProviderImpl) IsSysCCAndNotInvokableExternal(name string) bool {
	return (name == "escc") || (name == "vscc") || (name == "notext")
}

func (c *MocksccProviderImpl) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	return c.Qe, c.QErr
}

func (c *MocksccProviderImpl) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return c.ApplicationConfigRv, c.ApplicationConfigBool
}

func (c *MocksccProviderImpl) PolicyManager(channelID string) (policies.Manager, bool) {
	return c.PolicyManagerRv, c.PolicyManagerBool
}
