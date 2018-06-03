/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"fmt"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	ccprovider2 "github.com/hyperledger/fabric/core/mocks/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func init() {
	viper.Set("chaincode.system", map[string]string{"invokableExternalButNotCC2CC": "enable", "invokableCC2CCButNotExternal": "enable", "disabled": "enable"})
	viper.Set("peer.fileSystemPath", os.TempDir())
}

func newTestProvider() *Provider {
	p := NewProvider(peer.Default, peer.DefaultSupport, inproccontroller.NewRegistry())
	for _, cc := range []SelfDescribingSysCC{
		&SysCCWrapper{
			SCC: &SystemChaincode{
				Name:              "invokableExternalButNotCC2CC",
				InvokableExternal: true,
				InvokableCC2CC:    false,
				Enabled:           true,
			},
		},
		&SysCCWrapper{
			SCC: &SystemChaincode{
				Name:              "invokableCC2CCButNotExternal",
				InvokableExternal: false,
				InvokableCC2CC:    true,
				Enabled:           true,
			},
		},
		&SysCCWrapper{
			SCC: &SystemChaincode{
				Name:    "disabled",
				Enabled: false,
			},
		},
	} {
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
	deploySysCC("a", ccp, &SysCCWrapper{SCC: &SystemChaincode{
		Enabled: true,
		Name:    "invokableCC2CCButNotExternal",
	}})
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
	assert.True(t, (newTestProvider()).IsSysCC("invokableExternalButNotCC2CC"))
	assert.False(t, (newTestProvider()).IsSysCC("noSCC"))
	assert.True(t, (newTestProvider()).IsSysCC("invokableCC2CCButNotExternal"))
	assert.True(t, (newTestProvider()).IsSysCC("disabled"))
}

func TestIsSysCCAndNotInvokableCC2CC(t *testing.T) {
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("invokableExternalButNotCC2CC"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableExternal("invokableCC2CCButNotExternal"))
}

func TestIsSysCCAndNotInvokableExternal(t *testing.T) {
	assert.False(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("invokableCC2CCButNotExternal"))
	assert.True(t, (newTestProvider()).IsSysCCAndNotInvokableCC2CC("invokableExternalButNotCC2CC"))
}

func TestSccProviderImpl_GetQueryExecutorForLedger(t *testing.T) {
	p := NewProvider(peer.Default, peer.DefaultSupport, inproccontroller.NewRegistry())
	qe, err := p.GetQueryExecutorForLedger("")
	assert.Nil(t, qe)
	assert.Error(t, err)
}

func TestCreatePluginSysCCs(t *testing.T) {
	assert.NotPanics(t, func() { CreatePluginSysCCs(nil) }, "expected successful init")
}

func TestRegisterSysCC(t *testing.T) {
	p := &Provider{
		Registrar: inproccontroller.NewRegistry(),
	}
	_, err := p.registerSysCC(&SysCCWrapper{
		SCC: &SystemChaincode{
			Name:      "invokableExternalButNotCC2CC",
			Path:      "path",
			Enabled:   true,
			Chaincode: nil,
		},
	})
	assert.NoError(t, err)
	_, err = p.registerSysCC(&SysCCWrapper{
		SCC: &SystemChaincode{
			Name:      "invokableExternalButNotCC2CC",
			Path:      "path",
			Enabled:   true,
			Chaincode: nil,
		},
	})
	assert.Error(t, err)
	assert.Contains(t, "invokableExternalButNotCC2CC-latest already registered", err)
}
