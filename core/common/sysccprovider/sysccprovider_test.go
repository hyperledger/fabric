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

package sysccprovider

import (
	"testing"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/stretchr/testify/assert"
)

var mockChaincodeProvider = MockChaincodeProvider{}

type MockFactory struct {
}

type MockChaincodeProvider struct {
}

func (p MockChaincodeProvider) GetQueryExecutorForLedger(str string) (ledger.QueryExecutor, error) {
	return nil, nil
}

func (p MockChaincodeProvider) IsSysCC(name string) bool {
	return true
}

func (p MockChaincodeProvider) IsSysCCAndNotInvokableCC2CC(name string) bool {
	return true
}

func (p MockChaincodeProvider) IsSysCCAndNotInvokableExternal(name string) bool {
	return true
}

func (p MockChaincodeProvider) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return nil, false
}

func (p MockChaincodeProvider) PolicyManager(channelID string) (policies.Manager, bool) {
	return nil, false
}

func (f MockFactory) NewSystemChaincodeProvider() SystemChaincodeProvider {
	return mockChaincodeProvider
}

func TestRegisterSystemChaincodeProviderFactory(t *testing.T) {
	factory := MockFactory{}

	RegisterSystemChaincodeProviderFactory(factory)
	assert.NotNil(t, sccFactory, "sccFactory should not be nil")
	assert.Equal(t, sccFactory, factory, "sccFactory should equal mock factory")
}

func TestGetSystemChaincodeProviderError(t *testing.T) {
	sccFactory = nil
	assert.Panics(t, func() { GetSystemChaincodeProvider() }, "Should panic because factory isnt set")
}

func TestGetSystemChaincodeProviderSuccess(t *testing.T) {
	sccFactory = MockFactory{}
	provider := GetSystemChaincodeProvider()
	assert.NotNil(t, provider, "provider should not be nil")
	assert.Equal(t, provider, mockChaincodeProvider, "provider equal mockChaincodeProvider")
}

func TestString(t *testing.T) {
	chaincodeInstance := ChaincodeInstance{
		ChainID:          "ChainID",
		ChaincodeName:    "ChaincodeName",
		ChaincodeVersion: "ChaincodeVersion",
	}

	assert.NotNil(t, chaincodeInstance.String(), "str should not be nil")
	assert.Equal(t, chaincodeInstance.String(), "ChainID.ChaincodeName#ChaincodeVersion", "str should be the correct value")

}
