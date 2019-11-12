/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/hyperledger/fabric/bccsp/sw"
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	mockchannelconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	"github.com/hyperledger/fabric/common/policies"
	"github.com/hyperledger/fabric/core/ledger"
)

func CreateMockChannel(p *Peer, cid string, policyMgr policies.Manager) error {
	var ledger ledger.PeerLedger
	var err error

	if ledger = p.GetLedger(cid); ledger == nil {
		gb, _ := configtxtest.MakeGenesisBlock(cid)
		if ledger, err = p.LedgerMgr.CreateLedger(cid, gb); err != nil {
			return err
		}
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.channels == nil {
		p.channels = map[string]*Channel{}
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return err
	}

	p.channels[cid] = &Channel{
		ledger: ledger,
		resources: &mockchannelconfig.Resources{
			PolicyManagerVal:     policyMgr,
			ConfigtxValidatorVal: &mockconfigtx.Validator{},
			ApplicationConfigVal: &mockchannelconfig.MockApplication{CapabilitiesRv: &mockchannelconfig.MockApplicationCapabilities{}},
		},
		cryptoProvider: cryptoProvider,
	}

	return nil
}
