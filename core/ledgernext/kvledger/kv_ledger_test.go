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

package kvledger

import (
	"testing"

	"github.com/hyperledger/fabric/core/ledgernext/testutil"
	"github.com/hyperledger/fabric/protos"
)

func TestKVLedgerBlockStorage(t *testing.T) {
	env := newTestEnv(t)
	defer env.cleanup()
	ledger, _ := NewKVLedger(env.conf)
	defer ledger.Close()

	bcInfo, _ := ledger.GetBlockchainInfo()
	testutil.AssertEquals(t, bcInfo, &protos.BlockchainInfo{
		Height: 0, CurrentBlockHash: nil, PreviousBlockHash: nil})

	simulator, _ := ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()
	simRes, _ := simulator.GetTxSimulationResults()
	block1 := testutil.ConstructBlockForSimulationResults(t, [][]byte{simRes})
	ledger.RemoveInvalidTransactionsAndPrepare(block1)
	ledger.Commit()

	bcInfo, _ = ledger.GetBlockchainInfo()
	serBlock1, _ := protos.ConstructSerBlock2(block1)
	block1Hash := serBlock1.ComputeHash()
	testutil.AssertEquals(t, bcInfo, &protos.BlockchainInfo{
		Height: 1, CurrentBlockHash: block1Hash, PreviousBlockHash: []byte{}})

	simulator, _ = ledger.NewTxSimulator()
	simulator.SetState("ns1", "key1", []byte("value4"))
	simulator.SetState("ns1", "key2", []byte("value5"))
	simulator.SetState("ns1", "key3", []byte("value6"))
	simulator.Done()
	simRes, _ = simulator.GetTxSimulationResults()
	block2 := testutil.ConstructBlockForSimulationResults(t, [][]byte{simRes})
	ledger.RemoveInvalidTransactionsAndPrepare(block2)
	ledger.Commit()

	bcInfo, _ = ledger.GetBlockchainInfo()
	serBlock2, _ := protos.ConstructSerBlock2(block2)
	block2Hash := serBlock2.ComputeHash()
	testutil.AssertEquals(t, bcInfo, &protos.BlockchainInfo{
		Height: 2, CurrentBlockHash: block2Hash, PreviousBlockHash: []byte{}})

	b1, _ := ledger.GetBlockByHash(block1Hash)
	testutil.AssertEquals(t, b1, block1)

	b2, _ := ledger.GetBlockByHash(block2Hash)
	testutil.AssertEquals(t, b2, block2)

	b1, _ = ledger.GetBlockByNumber(1)
	testutil.AssertEquals(t, b1, block1)

	b2, _ = ledger.GetBlockByNumber(2)
	testutil.AssertEquals(t, b2, block2)
}
