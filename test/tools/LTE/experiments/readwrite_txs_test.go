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
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/test/tools/LTE/chainmgmt"
	"github.com/hyperledger/fabric/test/tools/LTE/common"
)

// BenchmarkReadWriteTxs opens the existing chains and modifies the Key-values by simulating read-write transactions
// For each of the chains, this test launches the parallel clients (based on the configuration) and the clients
// simulate and commit read-write transactions. This test assumes the pre-populated chain by previously running
// BenchmarkInsertTxs. Each transaction simultated by this benchmark randomly selects a configurable number of keys
// and modifies their values
//
// For instance, if this benchmark is invoked with the following test parameters
// -testParams=-NumChains=2, -NumParallelTxPerChain=2, -NumKVs=100, -NumTotalTx=200
// then client_1, and client_2 both execute 50 transactions on chain_1 in parallel. Similarly,
// client_3, and client_4 both execute 50 transactions on chain_2 in parallel
// In each of the transactions executed by any client, the transaction expects
// and modifies any key(s) between Key_1 to key_50 (because, total keys are to be 100 across two chains)
func BenchmarkReadWriteTxs(b *testing.B) {
	if b.N != 1 {
		panic(fmt.Errorf(`This benchmark should be called with N=1 only. Run this with more volume of data`))
	}
	testEnv := chainmgmt.InitTestEnv(conf.chainMgrConf, conf.batchConf, chainmgmt.ChainInitOpOpen)
	for _, chain := range testEnv.Chains() {
		go runReadWriteClientsForChain(chain)
	}
	testEnv.WaitForTestCompletion()
}

func runReadWriteClientsForChain(chain *chainmgmt.Chain) {
	numClients := conf.txConf.numParallelTxsPerChain
	numTxForChain := calculateShare(conf.txConf.numTotalTxs, conf.chainMgrConf.NumChains, int(chain.ID))
	wg := &sync.WaitGroup{}
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		numTxForClient := calculateShare(numTxForChain, numClients, i)
		randomNumGen := rand.New(rand.NewSource(int64(time.Now().Nanosecond()) + int64(chain.ID)))
		go runReadWriteClient(chain, randomNumGen, numTxForClient, wg)
	}
	wg.Wait()
	chain.Done()
}

func runReadWriteClient(chain *chainmgmt.Chain, rand *rand.Rand, numTx int, wg *sync.WaitGroup) {
	numKeysPerTx := conf.txConf.numKeysInEachTx
	maxKeyNumber := calculateShare(conf.dataConf.numKVs, conf.chainMgrConf.NumChains, int(chain.ID))

	for i := 0; i < numTx; i++ {
		simulator, err := chain.NewTxSimulator()
		common.PanicOnError(err)
		for i := 0; i < numKeysPerTx; i++ {
			keyNumber := rand.Intn(maxKeyNumber)
			key := constructKey(keyNumber)
			value, err := simulator.GetState(chaincodeName, key)
			common.PanicOnError(err)
			if !verifyValue(keyNumber, value) {
				panic(fmt.Errorf("Value %s is not expected for key number %d", value, keyNumber))
			}
			common.PanicOnError(simulator.SetState(chaincodeName, key, value))
		}
		simulator.Done()
		sr, err := simulator.GetTxSimulationResults()
		common.PanicOnError(err)
		chain.SubmitTx(sr)
	}
	wg.Done()
}
