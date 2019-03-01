/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockledger_test

import (
	. "github.com/hyperledger/fabric/common/ledger/blockledger"
	ramledger "github.com/hyperledger/fabric/common/ledger/blockledger/ram"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
)

func init() {
	testables = append(testables, &ramLedgerTestEnv{})
}

type ramledgerTestFactory struct{}

type ramLedgerTestEnv struct{}

func (env *ramLedgerTestEnv) Initialize() (ledgerTestFactory, error) {
	return &ramledgerTestFactory{}, nil
}

func (env *ramLedgerTestEnv) Name() string {
	return "ramledger"
}

func (env *ramledgerTestFactory) Destroy() error {
	return nil
}

func (env *ramledgerTestFactory) Persistent() bool {
	return false
}

func (env *ramledgerTestFactory) New() (Factory, ReadWriter) {
	historySize := 10
	rlf := ramledger.New(historySize)
	rl, err := rlf.GetOrCreate(genesisconfig.TestChainID)
	if err != nil {
		panic(err)
	}
	err = rl.Append(genesisBlock)
	if err != nil {
		panic(err)
	}
	return rlf, rl
}
