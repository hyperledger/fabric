/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"fmt"
	"os"
	"testing"

	aclmocks "github.com/hyperledger/fabric/core/aclmgmt/mocks"
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
}

func newTestProvider() *Provider {
	ccp := &ccprovider2.MockCcProviderImpl{}
	mockAclProvider := &aclmocks.MockACLProvider{}
	p := NewProvider(peer.Default, peer.DefaultSupport, inproccontroller.NewRegistry())
	for _, cc := range CreateSysCCs(ccp, p, mockAclProvider) {
		p.RegisterSysCC(cc)
	}
	return p
}

func TestDeploy(t *testing.T) {
	p := newTestProvider()
	ccp := &ccprovider2.MockCcProviderImpl{}
	p.DeploySysCCs("", ccp)
	f := func() {
		p.DeploySysCCs("a", ccp)
	}
	assert.Panics(t, f)
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()
	err := peer.MockCreateChain("a")
	fmt.Println(err)
	(&SystemChaincode{
		Enabled: true,
		Name:    "lscc",
	}).deploySysCC("a", ccp)
}

func TestDeDeploySysCC(t *testing.T) {
	p := newTestProvider()
	ccp := &ccprovider2.MockCcProviderImpl{}
	p.DeDeploySysCCs("", ccp)
	f := func() {
		p.DeDeploySysCCs("a", ccp)
	}
	assert.NotPanics(t, f)
}

func TestIsSysCC(t *testing.T) {
	assert.True(t, (newTestProvider()).IsSysCC("lscc"))
	assert.False(t, (newTestProvider()).IsSysCC("noSCC"))
	assert.True(t, (newTestProvider()).IsSysCC("cscc"))
	assert.True(t, (newTestProvider()).IsSysCC("escc"))
	assert.True(t, (newTestProvider()).IsSysCC("vscc"))
}

func TestIsSysCCAndNotInvokableCC2CC(t *testing.T) {
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("lscc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("escc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("vscc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("cscc"))
}

func TestIsSysCCAndNotInvokableExternal(t *testing.T) {
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("cscc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("escc"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("vscc"))
}

func TestSccProviderImpl_GetQueryExecutorForLedger(t *testing.T) {
	qe, err := (newTestProvider()).GetQueryExecutorForLedger("")
	assert.Nil(t, qe)
	assert.Error(t, err)
}

func TestRegisterSysCC(t *testing.T) {
	ccp := &ccprovider2.MockCcProviderImpl{}
	mockAclProvider := &aclmocks.MockACLProvider{}
	assert.NotPanics(t, func() { CreateSysCCs(ccp, newTestProvider(), mockAclProvider) }, "expected successful init")

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
