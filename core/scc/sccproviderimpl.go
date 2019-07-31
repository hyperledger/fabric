/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scc

import (
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

func (p *Provider) isWhitelisted(syscc SelfDescribingSysCC) bool {
	enabled, ok := p.Whitelist[syscc.Name()]
	return ok && enabled
}
