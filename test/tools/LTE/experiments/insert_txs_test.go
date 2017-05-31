/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package experiments

import (
	"sync"
	"testing"

	"fmt"

	"github.com/hyperledger/fabric/test/tools/LTE/chainmgmt"
	"github.com/hyperledger/fabric/test/tools/LTE/common"
)

// BenchmarkInsertTxs starts fresh chains and inserts the Key-values by simulating writes-only transactions
// For each of the chains, this test launches the parallel clients (based on the configuration) and the clients
// simulate and commit write-only transactions. The keys are divided among clients such that one key is written
// only once and all the keys are inserted.
//
// For instance, if this benchmark is invoked with the following test parameters
// -testParams=-NumChains=2, -NumParallelTxPerChain=2, -NumKVs=100
// then client_1 on chain_1 will insert Key_1 to key_25 and client_2 on chain_1 will insert Key_26 to key_50
// similarly client_3 on chain_2 will insert Key_1 to key_25 and client_4 on chain_2 will insert Key_26 to key_50
// where, client_1 and client_2 run in parallel on chain_1 and client_3 and client_4 run in parallel on chain_2
func BenchmarkInsertTxs(b *testing.B) {
	if b.N != 1 {
		panic(fmt.Errorf(`This benchmark should be called with N=1 only. Run this with more volume of data`))
	}
	testEnv := chainmgmt.InitTestEnv(conf.chainMgrConf, conf.batchConf, chainmgmt.ChainInitOpCreate)
	for _, chain := range testEnv.Chains() {
		go runInsertClientsForChain(chain)
	}
	testEnv.WaitForTestCompletion()
}

func runInsertClientsForChain(chain *chainmgmt.Chain) {
	numKVsForChain := calculateShare(conf.dataConf.numKVs, conf.chainMgrConf.NumChains, int(chain.ID))
	numClients := conf.txConf.numParallelTxsPerChain
	wg := &sync.WaitGroup{}
	wg.Add(numClients)
	startKey := 0
	for i := 0; i < numClients; i++ {
		numKVsForClient := calculateShare(numKVsForChain, numClients, i)
		endKey := startKey + numKVsForClient - 1
		go runInsertClient(chain, startKey, endKey, wg)
		startKey = endKey + 1
	}
	wg.Wait()
	chain.Done()
}

func runInsertClient(chain *chainmgmt.Chain, startKey, endKey int, wg *sync.WaitGroup) {
	numKeysPerTx := conf.txConf.numKeysInEachTx
	kvSize := conf.dataConf.kvSize

	currentKey := startKey
	for currentKey <= endKey {
		simulator, err := chain.NewTxSimulator()
		common.PanicOnError(err)
		for i := 0; i < numKeysPerTx; i++ {
			common.PanicOnError(simulator.SetState(
				chaincodeName, constructKey(currentKey), constructValue(currentKey, kvSize)))
			currentKey++
			if currentKey > endKey {
				break
			}
		}
		simulator.Done()
		sr, err := simulator.GetTxSimulationResults()
		common.PanicOnError(err)
		chain.SubmitTx(sr)
	}
	wg.Done()
}
