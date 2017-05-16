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
	"testing"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	ccprovider2 "github.com/hyperledger/fabric/core/mocks/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func init() {
	viper.Set("chaincode.system", map[string]string{"lscc": "enable", "a": "enable"})
	ccprovider.RegisterChaincodeProviderFactory(&ccprovider2.MockCcProviderFactory{})
	RegisterSysCCs()
}

func TestDeploy(t *testing.T) {
	DeploySysCCs("")
	f := func() {
		DeploySysCCs("a")
	}
	assert.Panics(t, f)
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()
	err := peer.MockCreateChain("a")
	fmt.Println(err)
	deploySysCC("a", &SystemChaincode{
		Enabled: true,
		Name:    "lscc",
	})
}

func TestDeDeploySysCC(t *testing.T) {
	DeDeploySysCCs("")
	f := func() {
		DeDeploySysCCs("a")
	}
	assert.NotPanics(t, f)
}

func TestSCCProvider(t *testing.T) {
	assert.NotNil(t, (&sccProviderFactory{}).NewSystemChaincodeProvider())
}

func TestIsSysCC(t *testing.T) {
	assert.True(t, IsSysCC("lscc"))
	assert.False(t, IsSysCC("noSCC"))
	assert.True(t, (&sccProviderImpl{}).IsSysCC("lscc"))
	assert.False(t, (&sccProviderImpl{}).IsSysCC("noSCC"))
}

func TestIsSysCCAndNotInvokableCC2CC(t *testing.T) {
	assert.False(t, IsSysCCAndNotInvokableCC2CC("lscc"))
	assert.True(t, IsSysCC("cscc"))
	assert.True(t, IsSysCCAndNotInvokableCC2CC("cscc"))
	assert.True(t, (&sccProviderImpl{}).IsSysCC("cscc"))
	assert.False(t, (&sccProviderImpl{}).IsSysCCAndNotInvokableCC2CC("lscc"))
	assert.True(t, (&sccProviderImpl{}).IsSysCCAndNotInvokableCC2CC("cscc"))
}

func TestIsSysCCAndNotInvokableExternal(t *testing.T) {
	assert.False(t, IsSysCCAndNotInvokableExternal("cscc"))
	assert.True(t, IsSysCC("cscc"))
	assert.True(t, IsSysCCAndNotInvokableExternal("vscc"))
	assert.False(t, (&sccProviderImpl{}).IsSysCCAndNotInvokableExternal("cscc"))
	assert.True(t, (&sccProviderImpl{}).IsSysCC("cscc"))
	assert.True(t, (&sccProviderImpl{}).IsSysCCAndNotInvokableExternal("vscc"))
}

func TestSccProviderImpl_GetQueryExecutorForLedger(t *testing.T) {
	qe, err := (&sccProviderImpl{}).GetQueryExecutorForLedger("")
	assert.Nil(t, qe)
	assert.Error(t, err)
}

func TestMockRegisterAndResetSysCCs(t *testing.T) {
	orig := MockRegisterSysCCs([]*SystemChaincode{})
	assert.NotEmpty(t, orig)
	MockResetSysCCs(orig)
	assert.Equal(t, len(orig), len(systemChaincodes))
}

func TestRegisterSysCC(t *testing.T) {
	err := RegisterSysCC(&SystemChaincode{
		Name:    "lscc",
		Path:    "path",
		Enabled: true,
	})
	assert.NoError(t, err)
	err = RegisterSysCC(&SystemChaincode{
		Name:    "lscc",
		Path:    "path",
		Enabled: true,
	})
	assert.Error(t, err)
	assert.Contains(t, "path already registered", err)
}
