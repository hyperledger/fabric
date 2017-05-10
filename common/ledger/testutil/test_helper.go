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
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/util"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	ptestutils "github.com/hyperledger/fabric/protos/testutils"
	"github.com/hyperledger/fabric/protos/utils"
)

//BlockGenerator generates a series of blocks for testing
type BlockGenerator struct {
	blockNum     uint64
	previousHash []byte
	signTxs      bool
	t            *testing.T
}

// NewBlockGenerator instantiates new BlockGenerator for testing
func NewBlockGenerator(t *testing.T, ledgerID string, signTxs bool) (*BlockGenerator, *common.Block) {
	gb, err := test.MakeGenesisBlock(ledgerID)
	AssertNoError(t, err, "")
	gb.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = lutils.NewTxValidationFlags(len(gb.Data.Data))
	return &BlockGenerator{1, gb.GetHeader().Hash(), signTxs, t}, gb
}

// NextBlock constructs next block in sequence that includes a number of transactions - one per simulationResults
func (bg *BlockGenerator) NextBlock(simulationResults [][]byte) *common.Block {
	block := ConstructBlock(bg.t, bg.blockNum, bg.previousHash, simulationResults, bg.signTxs)
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
	return bg.NextBlock(simulationResults)
}

// NextTestBlocks constructs 'numBlocks' number of blocks for testing
func (bg *BlockGenerator) NextTestBlocks(numBlocks int) []*common.Block {
	blocks := []*common.Block{}
	for i := 0; i < numBlocks; i++ {
		blocks = append(blocks, bg.NextTestBlock(10, 100))
	}
	return blocks
}

// ConstructTransaction constructs a transaction for testing
func ConstructTransaction(_ *testing.T, simulationResults []byte, sign bool) (*common.Envelope, string, error) {
	ccid := &pb.ChaincodeID{
		Name:    "foo",
		Version: "v1",
	}
	//response := &pb.Response{Status: 200}
	var txID string
	var txEnv *common.Envelope
	var err error
	if sign {
		txEnv, txID, err = ptestutils.ConstructSingedTxEnvWithDefaultSigner(util.GetTestChainID(), ccid, nil, simulationResults, nil, nil)
	} else {
		txEnv, txID, err = ptestutils.ConstructUnsingedTxEnv(util.GetTestChainID(), ccid, nil, simulationResults, nil, nil)
	}
	return txEnv, txID, err
}

// ConstructBlock constructs a single block
func ConstructBlock(t *testing.T, blockNum uint64, previousHash []byte, simulationResults [][]byte, sign bool) *common.Block {
	envs := []*common.Envelope{}
	for i := 0; i < len(simulationResults); i++ {
		env, _, err := ConstructTransaction(t, simulationResults[i], sign)
		if err != nil {
			t.Fatalf("ConstructTestTransaction failed, err %s", err)
		}
		envs = append(envs, env)
	}
	return newBlock(envs, blockNum, previousHash)
}

//ConstructTestBlock constructs a single block with random contents
func ConstructTestBlock(t *testing.T, blockNum uint64, numTx int, txSize int) *common.Block {
	simulationResults := [][]byte{}
	for i := 0; i < numTx; i++ {
		simulationResults = append(simulationResults, ConstructRandomBytes(t, txSize))
	}
	return ConstructBlock(t, blockNum, ConstructRandomBytes(t, 32), simulationResults, false)
}

// ConstructTestBlocks returns a series of blocks starting with blockNum=0.
// The first block in the returned array is a config tx block that represents a genesis block
func ConstructTestBlocks(t *testing.T, numBlocks int) []*common.Block {
	bg, gb := NewBlockGenerator(t, util.GetTestChainID(), false)
	blocks := []*common.Block{}
	if numBlocks != 0 {
		blocks = append(blocks, gb)
	}
	return append(blocks, bg.NextTestBlocks(numBlocks-1)...)
}

// ConstructBytesProposalResponsePayload constructs a ProposalResponse byte with given chaincode version and simulationResults for testing
func ConstructBytesProposalResponsePayload(version string, simulationResults []byte) ([]byte, error) {
	ccid := &pb.ChaincodeID{
		Name:    "foo",
		Version: version,
	}
	return ptestutils.ConstractBytesProposalResponsePayload(util.GetTestChainID(), ccid, nil, simulationResults)
}

func newBlock(env []*common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	block := common.NewBlock(blockNum, previousHash)
	for i := 0; i < len(env); i++ {
		txEnvBytes, _ := proto.Marshal(env[i])
		block.Data.Data = append(block.Data.Data, txEnvBytes)
	}
	block.Header.DataHash = block.Data.Hash()
	utils.InitBlockMetadata(block)

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = lutils.NewTxValidationFlags(len(env))

	return block
}
