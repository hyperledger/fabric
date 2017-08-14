/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/stretchr/testify/assert"
)

type customTxProcessor struct {
	namespace string
	initData  map[string][]byte
}

func (ctp *customTxProcessor) GenerateSimulationResults(
	txEnvelop *common.Envelope, simulator ledger.TxSimulator) error {
	for k, v := range ctp.initData {
		simulator.SetState(ctp.namespace, k, []byte(v))
	}
	return nil
}

func TestCustomProcessor(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	provider, _ := NewProvider()
	defer provider.Close()

	// create a custom tx processor and register it to handle 'common.HeaderType_CONFIG' type of transaction
	testNamespace := "TestNamespace"
	testKVs := map[string][]byte{"one": []byte("1"), "two": []byte("2")}
	customtx.Initialize(customtx.Processors{
		common.HeaderType_CONFIG: &customTxProcessor{
			testNamespace,
			testKVs,
		}})

	// Create a genesis block with a common.HeaderType_CONFIG transaction
	_, gb := testutil.NewBlockGenerator(t, "testLedger", false)
	ledger, err := provider.Create(gb)
	defer ledger.Close()
	assert.NoError(t, err)

	// verify that the state changes caused by the custom processor took place during ledger creation
	txSim, err := ledger.NewTxSimulator("testTxid")
	assert.NoError(t, err)
	for k, v := range testKVs {
		value, err := txSim.GetState(testNamespace, k)
		assert.NoError(t, err)
		assert.Equal(t, v, value)
	}
}
