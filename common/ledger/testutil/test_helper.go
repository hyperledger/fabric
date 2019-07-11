/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
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
	"github.com/stretchr/testify/assert"
)

//BlockGenerator generates a series of blocks for testing
type BlockGenerator struct {
	blockNum     uint64
	previousHash []byte
	signTxs      bool
	t            *testing.T
}

type TxDetails struct {
	TxID                            string
	ChaincodeName, ChaincodeVersion string
	SimulationResults               []byte
}

type BlockDetails struct {
	BlockNum     uint64
	PreviousHash []byte
	Txs          []*TxDetails
}

// NewBlockGenerator instantiates new BlockGenerator for testing
func NewBlockGenerator(t *testing.T, ledgerID string, signTxs bool) (*BlockGenerator, *common.Block) {
	gb, err := test.MakeGenesisBlock(ledgerID)
	assert.NoError(t, err)
	gb.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = lutils.NewTxValidationFlagsSetValue(len(gb.Data.Data), pb.TxValidationCode_VALID)
	return &BlockGenerator{1, gb.GetHeader().Hash(), signTxs, t}, gb
}

// NextBlock constructs next block in sequence that includes a number of transactions - one per simulationResults
func (bg *BlockGenerator) NextBlock(simulationResults [][]byte) *common.Block {
	block := ConstructBlock(bg.t, bg.blockNum, bg.previousHash, simulationResults, bg.signTxs)
	bg.blockNum++
	bg.previousHash = block.Header.Hash()
	return block
}

// NextBlockWithTxid constructs next block in sequence that includes a number of transactions - one per simulationResults
func (bg *BlockGenerator) NextBlockWithTxid(simulationResults [][]byte, txids []string) *common.Block {
	// Length of simulationResults should be same as the length of txids.
	if len(simulationResults) != len(txids) {
		return nil
	}
	block := ConstructBlockWithTxid(bg.t, bg.blockNum, bg.previousHash, simulationResults, txids, bg.signTxs)
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
	numTx := 10
	for i := 0; i < numBlocks; i++ {
		block := bg.NextTestBlock(numTx, 100)
		block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = lutils.NewTxValidationFlagsSetValue(numTx, pb.TxValidationCode_VALID)
		blocks = append(blocks, block)
	}
	return blocks
}

// ConstructTransaction constructs a transaction for testing
func ConstructTransaction(_ *testing.T, simulationResults []byte, txid string, sign bool) (*common.Envelope, string, error) {
	return ConstructTransactionFromTxDetails(
		&TxDetails{
			ChaincodeName:     "foo",
			ChaincodeVersion:  "v1",
			TxID:              txid,
			SimulationResults: simulationResults,
		},
		sign,
	)
}

func ConstructTransactionFromTxDetails(txDetails *TxDetails, sign bool) (*common.Envelope, string, error) {
	ccid := &pb.ChaincodeID{
		Name:    txDetails.ChaincodeName,
		Version: txDetails.ChaincodeVersion,
	}
	var txEnv *common.Envelope
	var err error
	var txID string
	if sign {
		txEnv, txID, err = ptestutils.ConstructSignedTxEnvWithDefaultSigner(util.GetTestChainID(), ccid, nil, txDetails.SimulationResults, txDetails.TxID, nil, nil)
	} else {
		txEnv, txID, err = ptestutils.ConstructUnsignedTxEnv(util.GetTestChainID(), ccid, nil, txDetails.SimulationResults, txDetails.TxID, nil, nil)
	}
	return txEnv, txID, err
}

func ConstructBlockFromBlockDetails(t *testing.T, blockDetails *BlockDetails, sign bool) *common.Block {
	var envs []*common.Envelope
	for _, txDetails := range blockDetails.Txs {
		env, _, err := ConstructTransactionFromTxDetails(txDetails, sign)
		if err != nil {
			t.Fatalf("ConstructTestTransaction failed, err %s", err)
		}
		envs = append(envs, env)
	}
	return NewBlock(envs, blockDetails.BlockNum, blockDetails.PreviousHash)
}

func ConstructBlockWithTxid(t *testing.T, blockNum uint64, previousHash []byte, simulationResults [][]byte, txids []string, sign bool) *common.Block {
	envs := []*common.Envelope{}
	for i := 0; i < len(simulationResults); i++ {
		env, _, err := ConstructTransaction(t, simulationResults[i], txids[i], sign)
		if err != nil {
			t.Fatalf("ConstructTestTransaction failed, err %s", err)
		}
		envs = append(envs, env)
	}
	return NewBlock(envs, blockNum, previousHash)
}

// ConstructBlock constructs a single block
func ConstructBlock(t *testing.T, blockNum uint64, previousHash []byte, simulationResults [][]byte, sign bool) *common.Block {
	envs := []*common.Envelope{}
	for i := 0; i < len(simulationResults); i++ {
		env, _, err := ConstructTransaction(t, simulationResults[i], "", sign)
		if err != nil {
			t.Fatalf("ConstructTestTransaction failed, err %s", err)
		}
		envs = append(envs, env)
	}
	return NewBlock(envs, blockNum, previousHash)
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
// Except the genesis block, the size of each of the block would be the same.
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
	return ptestutils.ConstructBytesProposalResponsePayload(util.GetTestChainID(), ccid, nil, simulationResults)
}

func NewBlock(env []*common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	block := common.NewBlock(blockNum, previousHash)
	for i := 0; i < len(env); i++ {
		txEnvBytes, _ := proto.Marshal(env[i])
		block.Data.Data = append(block.Data.Data, txEnvBytes)
	}
	block.Header.DataHash = block.Data.Hash()
	utils.InitBlockMetadata(block)

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = lutils.NewTxValidationFlagsSetValue(len(env), pb.TxValidationCode_VALID)

	return block
}

func SetTxID(t *testing.T, block *common.Block, txNum int, txID string) {
	envelopeBytes := block.Data.Data[txNum]
	envelope, err := utils.UnmarshalEnvelope(envelopeBytes)
	if err != nil {
		t.Fatalf("error unmarshaling envelope: %s", err)
	}

	payload, err := utils.GetPayload(envelope)
	if err != nil {
		t.Fatalf("error getting payload from envelope: %s", err)
	}

	channelHeader, err := utils.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		t.Fatalf("error unmarshaling channel header: %s", err)
	}
	channelHeader.TxId = txID
	channelHeaderBytes, err := proto.Marshal(channelHeader)
	if err != nil {
		t.Fatalf("error marshaling channel header: %s", err)
	}
	payload.Header.ChannelHeader = channelHeaderBytes

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		t.Fatalf("error marshaling payload: %s", err)
	}

	envelope.Payload = payloadBytes
	envelopeBytes, err = proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("error marshaling envelope: %s", err)
	}
	block.Data.Data[txNum] = envelopeBytes
}
