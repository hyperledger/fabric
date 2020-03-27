/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chainmgmt

import (
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	"github.com/hyperledger/fabric/protoutil"
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
		panicOnError(err)
		txEnvBytes, err := proto.Marshal(txEnv)
		panicOnError(err)
		bg.txQueue <- txEnvBytes
	}
	bg.wg.Done()
}

func (bg *blkGenerator) addTx(sr SimulationResult) {
	bg.srQueue <- sr
}

func (bg *blkGenerator) nextBlock() *common.Block {
	block := protoutil.NewBlock(bg.blockNum, bg.previousBlockHash)
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
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
	block.Header.Number = bg.blockNum
	block.Header.PreviousHash = bg.previousBlockHash
	txsfltr := txflags.NewWithValues(len(block.Data.Data), peer.TxValidationCode_VALID)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsfltr

	bg.blockNum++
	bg.previousBlockHash = protoutil.BlockHeaderHash(block.Header)
	return block
}

func (bg *blkGenerator) close() {
	close(bg.srQueue)
	bg.wg.Wait()
	close(bg.txQueue)
}
