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
	"fmt"

	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
)

// sccProviderFactory implements the sysccprovider.SystemChaincodeProviderFactory
// interface and returns instances of sysccprovider.SystemChaincodeProvider
type sccProviderFactory struct {
}

// NewSystemChaincodeProvider returns pointers to sccProviderFactory as an
// implementer of the sysccprovider.SystemChaincodeProvider interface
func (c *sccProviderFactory) NewSystemChaincodeProvider() sysccprovider.SystemChaincodeProvider {
	return &sccProviderImpl{}
}

// init is called when this package is loaded. This implementation registers the factory
func init() {
	sysccprovider.RegisterSystemChaincodeProviderFactory(&sccProviderFactory{})
}

// ccProviderImpl is an implementation of the ccprovider.ChaincodeProvider interface
type sccProviderImpl struct {
}

// IsSysCC returns true if the supplied chaincode is a system chaincode
func (c *sccProviderImpl) IsSysCC(name string) bool {
	return IsSysCC(name)
}

// IsSysCCAndNotInvokableCC2CC returns true if the supplied chaincode is
// ia system chaincode and it NOT nvokable through a cc2cc invocation
func (c *sccProviderImpl) IsSysCCAndNotInvokableCC2CC(name string) bool {
	return IsSysCCAndNotInvokableCC2CC(name)
}

// GetQueryExecutorForLedger returns a query executor for the specified channel
func (c *sccProviderImpl) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	l := peer.GetLedger(cid)
	if l == nil {
		return nil, fmt.Errorf("Could not retrieve ledger for channel %s", cid)
	}

	return l.NewQueryExecutor()
}

// IsSysCCAndNotInvokableExternal returns true if the supplied chaincode is
// ia system chaincode and it NOT nvokable
func (c *sccProviderImpl) IsSysCCAndNotInvokableExternal(name string) bool {
	// call the static method of the same name
	return IsSysCCAndNotInvokableExternal(name)
}
