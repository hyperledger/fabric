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
	"github.com/pkg/errors"
)

// BuiltinSCCs are special system chaincodes, differentiated from other (plugin)
// system chaincodes.  These chaincodes do not need to be initialized in '_lifecycle'
// and may be invoked without a channel context.  It is expected that '_lifecycle'
// will eventually be the only builtin SCCs.
// Note, this field should only be used on _endorsement_ side, never in validation
// as it might change.
type BuiltinSCCs map[string]struct{}

func (bccs BuiltinSCCs) IsSysCC(name string) bool {
	_, ok := bccs[name]
	return ok
}

// Provider implements sysccprovider.SystemChaincodeProvider
type Provider struct {
	Peer      *peer.Peer
	SysCCs    []SelfDescribingSysCC
	Whitelist Whitelist
}

// RegisterSysCC registers a system chaincode with the syscc provider.
func (p *Provider) RegisterSysCC(scc SelfDescribingSysCC) error {
	for _, registeredSCC := range p.SysCCs {
		if scc.Name() == registeredSCC.Name() {
			return errors.Errorf("chaincode with name '%s' already registered", scc.Name())
		}
	}
	p.SysCCs = append(p.SysCCs, scc)
	return nil
}

// GetQueryExecutorForLedger returns a query executor for the specified channel
func (p *Provider) GetQueryExecutorForLedger(cid string) (ledger.QueryExecutor, error) {
	l := p.Peer.GetLedger(cid)
	if l == nil {
		return nil, fmt.Errorf("Could not retrieve ledger for channel %s", cid)
	}

	return l.NewQueryExecutor()
}

// GetApplicationConfig returns the configtxapplication.SharedConfig for the channel
// and whether the Application config exists
func (p *Provider) GetApplicationConfig(cid string) (channelconfig.Application, bool) {
	return p.Peer.GetApplicationConfig(cid)
}

// Returns the policy manager associated to the passed channel
// and whether the policy manager exists
func (p *Provider) PolicyManager(channelID string) (policies.Manager, bool) {
	m := p.Peer.GetPolicyManager(channelID)
	return m, (m != nil)
}

func (p *Provider) isWhitelisted(syscc SelfDescribingSysCC) bool {
	enabled, ok := p.Whitelist[syscc.Name()]
	return ok && enabled
}
