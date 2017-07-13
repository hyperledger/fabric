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

package chainmgmt

import (
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/protos/common"
	benchcommon "github.com/hyperledger/fabric/test/tools/LTE/common"
)

const (
	numConcurrentTxEnvCreators = 30
)

type txEnvBytes []byte

// blkGenerator generates blocks in sequence. One instance of blkGenerator is maintained for each chain
type blkGenerator struct {
	batchConf         *BatchConf
	blockNum          uint64
	previousBlockHash []byte

	srQueue chan SimulationResult
	txQueue chan txEnvBytes
	wg      *sync.WaitGroup
}

func newBlkGenerator(batchConf *BatchConf, startingBlockNum uint64, previousBlockHash []byte) *blkGenerator {
	bg := &blkGenerator{
		batchConf,
		startingBlockNum,
		previousBlockHash,
		make(chan SimulationResult, batchConf.BatchSize),
		make(chan txEnvBytes, batchConf.BatchSize),
		&sync.WaitGroup{},
	}
	bg.startTxEnvCreators()
	return bg
}

func (bg *blkGenerator) startTxEnvCreators() {
	for i := 0; i < numConcurrentTxEnvCreators; i++ {
		go bg.startTxEnvCreator()
	}
}

func (bg *blkGenerator) startTxEnvCreator() {
	bg.wg.Add(1)
	for sr := range bg.srQueue {
		txEnv, err := createTxEnv(sr)
		benchcommon.PanicOnError(err)
		txEnvBytes, err := proto.Marshal(txEnv)
		benchcommon.PanicOnError(err)
		bg.txQueue <- txEnvBytes
	}
	bg.wg.Done()
}

func (bg *blkGenerator) addTx(sr SimulationResult) {
	bg.srQueue <- sr
}

func (bg *blkGenerator) nextBlock() *common.Block {
	block := common.NewBlock(bg.blockNum, bg.previousBlockHash)
	numTx := 0
	for txEnvBytes := range bg.txQueue {
		numTx++
		block.Data.Data = append(block.Data.Data, txEnvBytes)
		if numTx == bg.batchConf.BatchSize {
			break
		}
	}
	// close() has been called and no pending tx
	if len(block.Data.Data) == 0 {
		return nil
	}
	block.Header.DataHash = block.Data.Hash()
	block.Header.Number = bg.blockNum
	block.Header.PreviousHash = bg.previousBlockHash

	bg.blockNum++
	bg.previousBlockHash = block.Header.Hash()
	return block
}

func (bg *blkGenerator) close() {
	close(bg.srQueue)
	bg.wg.Wait()
	close(bg.txQueue)
}
