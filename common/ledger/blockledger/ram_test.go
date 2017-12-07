/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blockledger_test

import (
	. "github.com/hyperledger/fabric/common/ledger/blockledger"
	ramledger "github.com/hyperledger/fabric/common/ledger/blockledger/ram"
	genesisconfig "github.com/hyperledger/fabric/common/tools/configtxgen/localconfig"
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
