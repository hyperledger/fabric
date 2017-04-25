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

package sysccprovider

import (
	"github.com/hyperledger/fabric/core/ledger"
)

// SystemChaincodeProvider provides an abstraction layer that is
// used for different packages to interact with code in the
// system chaincode package without importing it; more methods
// should be added below if necessary
type SystemChaincodeProvider interface {
	// IsSysCC returns true if the supplied chaincode is a system chaincode
	IsSysCC(name string) bool

	// IsSysCCAndNotInvokableCC2CC returns true if the supplied chaincode
	// is a system chaincode and is not invokable through a cc2cc invocation
	IsSysCCAndNotInvokableCC2CC(name string) bool

	// IsSysCCAndNotInvokable returns true if the supplied chaincode
	// is a system chaincode and is not invokable through a proposal
	IsSysCCAndNotInvokableExternal(name string) bool

	// GetQueryExecutorForLedger returns a query executor for the
	// ledger of the supplied channel.
	// That's useful for system chaincodes that require unfettered
	// access to the ledger
	GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error)
}

var sccFactory SystemChaincodeProviderFactory

// SystemChaincodeProviderFactory defines a factory interface so
// that the actual implementation can be injected
type SystemChaincodeProviderFactory interface {
	NewSystemChaincodeProvider() SystemChaincodeProvider
}

// RegisterSystemChaincodeProviderFactory is to be called once to set
// the factory that will be used to obtain instances of ChaincodeProvider
func RegisterSystemChaincodeProviderFactory(sccfact SystemChaincodeProviderFactory) {
	sccFactory = sccfact
}

// GetSystemChaincodeProvider returns instances of SystemChaincodeProvider;
// the actual implementation is controlled by the factory that
// is registered via RegisterSystemChaincodeProviderFactory
func GetSystemChaincodeProvider() SystemChaincodeProvider {
	if sccFactory == nil {
		panic("The factory must be set first via RegisterSystemChaincodeProviderFactory")
	}
	return sccFactory.NewSystemChaincodeProvider()
}

// ChaincodeInstance is unique identifier of chaincode instance
type ChaincodeInstance struct {
	ChainID          string
	ChaincodeName    string
	ChaincodeVersion string
}

func (ci *ChaincodeInstance) String() string {
	return ci.ChainID + "." + ci.ChaincodeName + "#" + ci.ChaincodeVersion
}
