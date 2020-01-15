/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package experiments

import (
	"fmt"
	"sync"
	"testing"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger/benchmark/chainmgmt"
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
	numWritesPerTx := conf.txConf.numWritesPerTx
	kvSize := conf.dataConf.kvSize
	useJSON := conf.dataConf.useJSON

	currentKey := startKey
	for currentKey <= endKey {
		simulator, err := chain.NewTxSimulator(util.GenerateUUID())
		panicOnError(err)
		for i := 0; i < numWritesPerTx; i++ {
			if useJSON {
				panicOnError(simulator.SetState(
					chaincodeName, constructKey(currentKey), constructJSONValue(currentKey, kvSize)))
			} else {
				panicOnError(simulator.SetState(
					chaincodeName, constructKey(currentKey), constructValue(currentKey, kvSize)))
			}
			currentKey++
			if currentKey > endKey {
				break
			}
		}
		simulator.Done()
		sr, err := simulator.GetTxSimulationResults()
		panicOnError(err)
		srBytes, err := sr.GetPubSimulationBytes()
		panicOnError(err)
		chain.SubmitTx(srBytes)
	}
	wg.Done()
}
