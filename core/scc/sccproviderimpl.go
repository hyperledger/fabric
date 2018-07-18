/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
	"fmt"

	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/peer"
)

// NewProvider creates a new Provider instance
func NewProvider(pOps peer.Operations, pSup peer.Support, r Registrar) *Provider {
	return &Provider{
		Peer:        pOps,
		PeerSupport: pSup,
		Registrar:   r,
	}
}

// Provider implements sysccprovider.SystemChaincodeProvider
type Provider struct {
	Peer        peer.Operations
	PeerSupport peer.Support
	Registrar   Registrar
	SysCCs      []SelfDescribingSysCC
}

// RegisterSysCC registers a system chaincode with the syscc provider.
func (p *Provider) RegisterSysCC(scc SelfDescribingSysCC) {
	p.SysCCs = append(p.SysCCs, scc)
	_, err := p.registerSysCC(scc)
	if err != nil {
		sysccLogger.Panicf("Could not register system chaincode: %s", err)
	}
}

// IsSysCC returns true if the supplied chaincode is a system chaincode
func (p *Provider) IsSysCC(name string) bool {
	for _, sysCC := range p.SysCCs {
		if sysCC.Name() == name {
			return true
		}
	}
	if isDeprecatedSysCC(name) {
		return true
	}
	return false
}

// IsSysCCAndNotInvokableCC2CC returns true if the chaincode
// is a system chaincode and *CANNOT* be invoked through
// a cc2cc invocation
func (p *Provider) IsSysCCAndNotInvokableCC2CC(name string) bool {
	for _, sysCC := range p.SysCCs {
		if sysCC.Name() == name {
			return !sysCC.InvokableCC2CC()
		}
	}

	if isDeprecatedSysCC(name) {
		return true
	}

	return false
}

// GetQueryExecutorForLedger returns a query executor for the specified channel
func (p *Provider) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	l := p.Peer.GetLedger(cid)
	if l == nil {
		return nil, fmt.Errorf("Could not retrieve ledger for channel %s", cid)
	}

	return l.NewQueryExecutor()
}

// IsSysCCAndNotInvokableExternal returns true if the chaincode
// is a system chaincode and *CANNOT* be invoked through
// a proposal to this peer
func (p *Provider) IsSysCCAndNotInvokableExternal(name string) bool {
	for _, sysCC := range p.SysCCs {
		if sysCC.Name() == name {
			return !sysCC.InvokableExternal()
		}
	}

	if isDeprecatedSysCC(name) {
		return true
	}

	return false
}

// GetApplicationConfig returns the configtxapplication.SharedConfig for the channel
// and whether the Application config exists
func (p *Provider) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return p.PeerSupport.GetApplicationConfig(cid)
}

// Returns the policy manager associated to the passed channel
// and whether the policy manager exists
func (p *Provider) PolicyManager(channelID string) (policies.Manager, bool) {
	m := p.Peer.GetPolicyManager(channelID)
	return m, (m != nil)
}

func isDeprecatedSysCC(name string) bool {
	return name == "vscc" || name == "escc"
}
