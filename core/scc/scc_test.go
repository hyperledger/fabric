/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
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

func newTestProvider() *Provider {
	p := NewProvider(peer.Default, peer.DefaultSupport, inproccontroller.NewRegistry())
	for _, cc := range CreateSysCCs(p) {
		p.RegisterSysCC(cc)
	}
	return p
}

func TestDeploy(t *testing.T) {
	p := newTestProvider()
	p.DeploySysCCs("")
	f := func() {
		p.DeploySysCCs("a")
	}
	assert.Panics(t, f)
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()
	err := peer.MockCreateChain("a")
	fmt.Println(err)
	(&SystemChaincode{
		Enabled: true,
		Name:    "lscc",
	}).deploySysCC("a")
}

func TestDeDeploySysCC(t *testing.T) {
	p := newTestProvider()
	p.DeDeploySysCCs("")
	f := func() {
		p.DeDeploySysCCs("a")
	}
	assert.NotPanics(t, f)
}

func TestIsSysCC(t *testing.T) {
	assert.True(t, (newTestProvider()).IsSysCC("lscc"))
	assert.False(t, (newTestProvider()).IsSysCC("noSCC"))
}

func TestIsSysCCAndNotInvokableCC2CC(t *testing.T) {
	assert.True(t, (newTestProvider()).IsSysCC("cscc"))
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("lscc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("cscc"))
}

func TestIsSysCCAndNotInvokableExternal(t *testing.T) {
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("cscc"))
	assert.True(t, (newTestProvider()).IsSysCC("cscc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("vscc"))
}

func TestSccProviderImpl_GetQueryExecutorForLedger(t *testing.T) {
	qe, err := (newTestProvider()).GetQueryExecutorForLedger("")
	assert.Nil(t, qe)
	assert.Error(t, err)
}

func TestRegisterSysCC(t *testing.T) {
	assert.NotPanics(t, func() { CreateSysCCs(newTestProvider()) }, "expected successful init")

	p := &Provider{
		Registrar: inproccontroller.NewRegistry(),
	}
	_, err := p.registerSysCC(&SystemChaincode{
		Name:      "lscc",
		Path:      "path",
		Enabled:   true,
		Chaincode: nil,
	})
	assert.NoError(t, err)
	_, err = p.registerSysCC(&SystemChaincode{
		Name:      "lscc",
		Path:      "path",
		Enabled:   true,
		Chaincode: nil,
	})
	assert.Error(t, err)
	assert.Contains(t, "lscc-latest already registered", err)
}
