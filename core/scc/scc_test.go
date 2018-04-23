/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/common/sysccprovider"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	ccprovider2 "github.com/hyperledger/fabric/core/mocks/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func init() {
	viper.Set("chaincode.system", map[string]string{"lscc": "enable", "a": "enable"})
	viper.Set("peer.fileSystemPath", os.TempDir())
	ccprovider.RegisterChaincodeProviderFactory(&ccprovider2.MockCcProviderFactory{})
}

func newTestProvider() sysccprovider.SystemChaincodeProvider {
	return (&ProviderFactory{Peer: peer.Default, PeerSupport: peer.DefaultSupport}).NewSystemChaincodeProvider()
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
	assert.NotNil(t, (&ProviderFactory{}).NewSystemChaincodeProvider())
}

func TestIsSysCC(t *testing.T) {
	assert.True(t, IsSysCC("lscc"))
	assert.False(t, IsSysCC("noSCC"))
	assert.True(t, (newTestProvider()).IsSysCC("lscc"))
	assert.False(t, (newTestProvider()).IsSysCC("noSCC"))
}

func TestIsSysCCAndNotInvokableCC2CC(t *testing.T) {
	assert.False(t, IsSysCCAndNotInvokableCC2CC("lscc"))
	assert.True(t, IsSysCC("cscc"))
	assert.True(t, IsSysCCAndNotInvokableCC2CC("cscc"))
	assert.True(t, (newTestProvider()).IsSysCC("cscc"))
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("lscc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("cscc"))
}

func TestIsSysCCAndNotInvokableExternal(t *testing.T) {
	assert.False(t, IsSysCCAndNotInvokableExternal("cscc"))
	assert.True(t, IsSysCC("cscc"))
	assert.True(t, IsSysCCAndNotInvokableExternal("vscc"))
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("cscc"))
	assert.True(t, (newTestProvider()).IsSysCC("cscc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("vscc"))
}

func TestSccProviderImpl_GetQueryExecutorForLedger(t *testing.T) {
	qe, err := (newTestProvider()).GetQueryExecutorForLedger("")
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
	assert.NotPanics(t, func() { RegisterSysCCs() }, "expected successful init")

	_, err := registerSysCC(&SystemChaincode{
		Name:      "lscc",
		Path:      "path",
		Enabled:   true,
		Chaincode: func(sysccprovider.SystemChaincodeProvider) shim.Chaincode { return nil },
	})
	assert.NoError(t, err)
	_, err = registerSysCC(&SystemChaincode{
		Name:      "lscc",
		Path:      "path",
		Enabled:   true,
		Chaincode: func(sysccprovider.SystemChaincodeProvider) shim.Chaincode { return nil },
	})
	assert.Error(t, err)
	assert.Contains(t, "path already registered", err)
}
