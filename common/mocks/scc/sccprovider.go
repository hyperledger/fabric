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
	lm "github.com/hyperledger/fabric/common/mocks/ledger"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
)

type MocksccProviderFactory struct {
	Qe   *lm.MockQueryExecutor
	QErr error
}

func (c *MocksccProviderFactory) NewSystemChaincodeProvider() sysccprovider.SystemChaincodeProvider {
	return &mocksccProviderImpl{Qe: c.Qe, QErr: c.QErr}
}

type mocksccProviderImpl struct {
	Qe   *lm.MockQueryExecutor
	QErr error
}

func (c *mocksccProviderImpl) IsSysCC(name string) bool {
	return (name == "lscc") || (name == "escc") || (name == "vscc") || (name == "notext")
}

func (c *mocksccProviderImpl) IsSysCCAndNotInvokableCC2CC(name string) bool {
	return (name == "escc") || (name == "vscc")
}

func (c *mocksccProviderImpl) IsSysCCAndNotInvokableExternal(name string) bool {
	return (name == "escc") || (name == "vscc") || (name == "notext")
}

func (c *mocksccProviderImpl) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	return c.Qe, c.QErr
}
