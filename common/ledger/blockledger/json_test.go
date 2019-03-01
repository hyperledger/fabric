/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockledger_test

import (
	"io/ioutil"
	"os"

	. "github.com/hyperledger/fabric/common/ledger/blockledger"
	jsonledger "github.com/hyperledger/fabric/common/ledger/blockledger/json"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
	"github.com/hyperledger/fabric/protoutil"
)

var genesisBlock = protoutil.NewBlock(0, nil)

func init() {
	testables = append(testables, &jsonLedgerTestEnv{})
}

type jsonLedgerTestFactory struct {
	location string
}

type jsonLedgerTestEnv struct {
}

func (env *jsonLedgerTestEnv) Initialize() (ledgerTestFactory, error) {
	var err error
	location, err := ioutil.TempDir("", "hyperledger")
	if err != nil {
		return nil, err
	}
	return &jsonLedgerTestFactory{location: location}, nil
}

func (env *jsonLedgerTestEnv) Name() string {
	return "jsonledger"
}

func (env *jsonLedgerTestFactory) Destroy() error {
	err := os.RemoveAll(env.location)
	return err
}

func (env *jsonLedgerTestFactory) Persistent() bool {
	return true
}

func (env *jsonLedgerTestFactory) New() (Factory, ReadWriter) {
	flf := jsonledger.New(env.location)
	fl, err := flf.GetOrCreate(genesisconfig.TestChainID)
	if err != nil {
		panic(err)
	}
	if fl.Height() == 0 {
		if err = fl.Append(genesisBlock); err != nil {
			panic(err)
		}
	}
	return flf, fl
}
