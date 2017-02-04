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

package testutil

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/protos/common"
	ptestutils "github.com/hyperledger/fabric/protos/testutils"
)

//BlockGenerator generates a series of blocks for testing
type BlockGenerator struct {
	blockNum     uint64
	previousHash []byte
	t            *testing.T
}

// NewBlockGenerator instantiates new BlockGenerator for testing
func NewBlockGenerator(t *testing.T) *BlockGenerator {
	return &BlockGenerator{1, []byte{}, t}
}

// NextBlock constructs next block in sequence that includes a number of transactions - one per simulationResults
func (bg *BlockGenerator) NextBlock(simulationResults [][]byte, sign bool) *common.Block {
	envs := []*common.Envelope{}
	for i := 0; i < len(simulationResults); i++ {
		env, _, err := ConstructTransaction(bg.t, simulationResults[i], sign)
		if err != nil {
			bg.t.Fatalf("ConstructTestTransaction failed, err %s", err)
		}
		envs = append(envs, env)
	}
	block := newBlock(envs, bg.blockNum, bg.previousHash)
	bg.blockNum++
	bg.previousHash = block.Header.Hash()
	return block
}

// NextTestBlock constructs next block in sequence block with 'numTx' number of transactions for testing
func (bg *BlockGenerator) NextTestBlock(numTx int, txSize int) *common.Block {
	simulationResults := [][]byte{}
	for i := 0; i < numTx; i++ {
		simulationResults = append(simulationResults, ConstructRandomBytes(bg.t, txSize))
	}
	return bg.NextBlock(simulationResults, false)
}

// NextTestBlocks constructs 'numBlocks' number of blocks for testing
func (bg *BlockGenerator) NextTestBlocks(numBlocks int) []*common.Block {
	blocks := []*common.Block{}
	for i := 0; i < numBlocks; i++ {
		blocks = append(blocks, bg.NextTestBlock(10, 100))
	}
	return blocks
}

// ConstructBlock constructs a single block with blockNum=1
func ConstructBlock(t *testing.T, simulationResults [][]byte, sign bool) *common.Block {
	bg := NewBlockGenerator(t)
	return bg.NextBlock(simulationResults, sign)
}

// ConstructTestBlock constructs a single block with blocknum=1
func ConstructTestBlock(t *testing.T, numTx int, txSize int) *common.Block {
	bg := NewBlockGenerator(t)
	return bg.NextTestBlock(numTx, txSize)
}

// ConstructTestBlocks returns a series of blocks starting with blockNum=1
func ConstructTestBlocks(t *testing.T, numBlocks int) []*common.Block {
	bg := NewBlockGenerator(t)
	return bg.NextTestBlocks(numBlocks)
}

// ConstructTransaction constructs a transaction for testing
func ConstructTransaction(t *testing.T, simulationResults []byte, sign bool) (*common.Envelope, string, error) {
	ccName := "foo"
	//response := &pb.Response{Status: 200}
	txID := util.GenerateUUID()
	var txEnv *common.Envelope
	var err error
	if sign {
		txEnv, err = ptestutils.ConstructSingedTxEnvWithDefaultSigner(txID, util.GetTestChainID(), ccName, nil, simulationResults, nil, nil)
	} else {
		txEnv, err = ptestutils.ConstructUnsingedTxEnv(txID, util.GetTestChainID(), ccName, nil, simulationResults, nil, nil)
	}
	return txEnv, txID, err
}

func newBlock(env []*common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	block := common.NewBlock(blockNum, previousHash)
	for i := 0; i < len(env); i++ {
		txEnvBytes, _ := proto.Marshal(env[i])
		block.Data.Data = append(block.Data.Data, txEnvBytes)
	}
	block.Header.DataHash = block.Data.Hash()
	return block
}
