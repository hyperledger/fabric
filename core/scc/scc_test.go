/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	"github.com/hyperledger/fabric/core/container/inproccontroller"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt/ledgermgmttest"
	ccprovider2 "github.com/hyperledger/fabric/core/mocks/ccprovider"
	"github.com/hyperledger/fabric/core/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	viper.Set("peer.fileSystemPath", os.TempDir())
}

func newTestProvider() *Provider {
	p := &Provider{
		Peer:      &peer.Peer{},
		Registrar: inproccontroller.NewRegistry(),
		Whitelist: map[string]bool{
			"invokableExternalButNotCC2CC": true,
			"invokableCC2CCButNotExternal": true,
			"disabled":                     true,
		},
	}
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

	tempdir, err := ioutil.TempDir("", "scc-test")
	require.NoError(t, err, "failed to create temporary directory")
	defer os.RemoveAll(tempdir)
	ledgerMgr := ledgermgmt.NewLedgerMgr(ledgermgmttest.NewInitializer(tempdir))
	defer ledgerMgr.Close()
	p.Peer.LedgerMgr = ledgerMgr

	err = peer.CreateMockChannel(p.Peer, "a")
	if err != nil {
		t.Fatalf("failed to create mock chain: %v", err)
	}
	p.deploySysCC("a", ccp, &SysCCWrapper{SCC: &SystemChaincode{
		Enabled: true,
		Name:    "invokableCC2CCButNotExternal",
	}})
}

func TestDeployInitFailed(t *testing.T) {
	t.Run("shim error status response", func(t *testing.T) {
		ccp := &ccprovider2.MockCcProviderImpl{
			InitResponse: &pb.Response{Status: shim.ERROR, Message: "i-am-a-failure"},
		}
		p := newTestProvider()

		assert.PanicsWithValue(t, "chaincode deployment failed: i-am-a-failure", func() { p.DeploySysCCs("", ccp) })
	})
	t.Run("shim init error", func(t *testing.T) {
		ccp := &ccprovider2.MockCcProviderImpl{
			InitError: errors.New("i-am-more-than-a-disappointment"),
		}
		p := newTestProvider()

		assert.PanicsWithValue(t, "chaincode deployment failed: i-am-more-than-a-disappointment", func() { p.DeploySysCCs("", ccp) })
	})
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
	p := &Provider{
		Peer:      &peer.Peer{},
		Registrar: inproccontroller.NewRegistry(),
	}
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
		Whitelist: map[string]bool{
			"invokableExternalButNotCC2CC": true,
			"invokableCC2CCButNotExternal": true,
		},
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
	assert.Contains(t, "invokableExternalButNotCC2CC:latest already registered", err)
}
