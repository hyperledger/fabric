/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	configtxtest "github.com/hyperledger/fabric/common/configtx/test"
	mockchannelconfig "github.com/hyperledger/fabric/common/mocks/config"
	mockconfigtx "github.com/hyperledger/fabric/common/mocks/configtx"
	mockpolicies "github.com/hyperledger/fabric/common/mocks/policies"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
)

//MockInitialize resets chains for test env
func MockInitialize() (cleanup func(), err error) {
	cleanup, err = ledgermgmt.InitializeTestEnvWithInitializer(
		&ledgermgmt.Initializer{},
	)
	Default.mutex.Lock()
	Default.channels = make(map[string]*Channel)
	Default.mutex.Unlock()
	return cleanup, err
}

// MockCreateChain used for creating a ledger for a chain for tests
// without having to join
func MockCreateChain(cid string) error {
	var ledger ledger.PeerLedger
	var err error

	if ledger = Default.GetLedger(cid); ledger == nil {
		gb, _ := configtxtest.MakeGenesisBlock(cid)
		if ledger, err = ledgermgmt.CreateLedger(gb); err != nil {
			return err
		}
	}

	Default.mutex.Lock()
	defer Default.mutex.Unlock()

	if Default.channels == nil {
		Default.channels = map[string]*Channel{}
	}

	Default.channels[cid] = &Channel{
		ledger: ledger,
		resources: &mockchannelconfig.Resources{
			PolicyManagerVal: &mockpolicies.Manager{
				Policy: &mockpolicies.Policy{},
			},
			ConfigtxValidatorVal: &mockconfigtx.Validator{},
			ApplicationConfigVal: &mockchannelconfig.MockApplication{CapabilitiesRv: &mockchannelconfig.MockApplicationCapabilities{}},
		},
	}

	return nil
}
