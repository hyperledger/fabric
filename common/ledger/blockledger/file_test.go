/*
Copyright IBM Corp. 2016 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blockledger_test

import (
	"io/ioutil"
	"os"

	. "github.com/hyperledger/fabric/common/ledger/blockledger"
	fileledger "github.com/hyperledger/fabric/common/ledger/blockledger/file"
	genesisconfig "github.com/hyperledger/fabric/internal/configtxgen/localconfig"
)

func init() {
	testables = append(testables, &fileLedgerTestEnv{})
}

type fileLedgerTestFactory struct {
	location string
}

type fileLedgerTestEnv struct {
}

func (env *fileLedgerTestEnv) Initialize() (ledgerTestFactory, error) {
	var err error
	location, err := ioutil.TempDir("", "hyperledger")
	if err != nil {
		return nil, err
	}
	return &fileLedgerTestFactory{location: location}, nil
}

func (env *fileLedgerTestEnv) Name() string {
	return "fileledger"
}

func (env *fileLedgerTestEnv) Close(lf Factory) {
	lf.Close()
}

func (env *fileLedgerTestFactory) Destroy() error {
	err := os.RemoveAll(env.location)
	return err
}

func (env *fileLedgerTestFactory) Persistent() bool {
	return true
}

func (env *fileLedgerTestFactory) New() (Factory, ReadWriter) {
	flf := fileledger.New(env.location)
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
