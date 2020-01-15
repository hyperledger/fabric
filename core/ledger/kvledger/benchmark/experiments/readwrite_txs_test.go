/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package experiments

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/ledger/kvledger/benchmark/chainmgmt"
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
	runReadWriteTest()
}

func runReadWriteTest() {
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
	numWritesPerTx := conf.txConf.numWritesPerTx
	numReadsPerTx := conf.txConf.numReadsPerTx
	maxKeyNumber := calculateShare(conf.dataConf.numKVs, conf.chainMgrConf.NumChains, int(chain.ID))
	kvSize := conf.dataConf.kvSize
	useJSON := conf.dataConf.useJSON
	var value []byte

	for i := 0; i < numTx; i++ {
		simulator, err := chain.NewTxSimulator(util.GenerateUUID())
		panicOnError(err)
		maxKeys := max(numReadsPerTx, numWritesPerTx)
		keysToOperateOn := []int{}
		for i := 0; i < maxKeys; i++ {
			keysToOperateOn = append(keysToOperateOn, rand.Intn(maxKeyNumber))
		}

		for i := 0; i < numReadsPerTx; i++ {
			keyNumber := keysToOperateOn[i]
			value, err = simulator.GetState(chaincodeName, constructKey(keyNumber))
			panicOnError(err)
			if useJSON {
				if !verifyJSONValue(keyNumber, value) {
					panic(fmt.Errorf("Value %s is not expected for key number %d", value, keyNumber))
				}
			} else {
				if !verifyValue(keyNumber, value) {
					panic(fmt.Errorf("Value %s is not expected for key number %d", value, keyNumber))
				}
			}
		}

		for i := 0; i < numWritesPerTx; i++ {
			keyNumber := keysToOperateOn[i]
			key := constructKey(keyNumber)
			if useJSON {
				value = []byte(constructJSONValue(keyNumber, kvSize))
			} else {
				value = []byte(constructValue(keyNumber, kvSize))
			}
			panicOnError(simulator.SetState(chaincodeName, key, value))
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
