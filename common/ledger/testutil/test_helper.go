/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutil

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto"
	mmsp "github.com/hyperledger/fabric/common/mocks/msp"
	"github.com/hyperledger/fabric/common/util"
	lutils "github.com/hyperledger/fabric/core/ledger/util"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protos/common"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
)

var signer msp.SigningIdentity

func init() {
	var err error
	// setup the MSP manager so that we can sign/verify
	err = msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		panic(fmt.Errorf("Could not load msp config, err %s", err))
	}
	signer, err = mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	if err != nil {
		panic(fmt.Errorf("Could not initialize msp/signer"))
	}
}

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
	Type                            common.HeaderType
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
	return &BlockGenerator{1, protoutil.BlockHeaderHash(gb.GetHeader()), signTxs, t}, gb
}

// NextBlock constructs next block in sequence that includes a number of transactions - one per simulationResults
func (bg *BlockGenerator) NextBlock(simulationResults [][]byte) *common.Block {
	block := ConstructBlock(bg.t, bg.blockNum, bg.previousHash, simulationResults, bg.signTxs)
	bg.blockNum++
	bg.previousHash = protoutil.BlockHeaderHash(block.Header)
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
	bg.previousHash = protoutil.BlockHeaderHash(block.Header)
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
func ConstructTransaction(
	t *testing.T,
	simulationResults []byte,
	txid string,
	sign bool,
) (*common.Envelope, string, error) {
	return ConstructTransactionWithHeaderType(
		t,
		simulationResults,
		txid,
		sign,
		common.HeaderType_ENDORSER_TRANSACTION,
	)
}

// ConstructTransaction constructs a transaction for testing with header type
func ConstructTransactionWithHeaderType(
	t *testing.T,
	simulationResults []byte,
	txid string,
	sign bool,
	headerType common.HeaderType,
) (*common.Envelope, string, error) {
	return ConstructTransactionFromTxDetails(
		&TxDetails{
			ChaincodeName:     "foo",
			ChaincodeVersion:  "v1",
			TxID:              txid,
			SimulationResults: simulationResults,
			Type:              headerType,
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
		txEnv, txID, err = ConstructSignedTxEnvWithDefaultSigner(
			util.GetTestChainID(),
			ccid,
			nil,
			txDetails.SimulationResults,
			txDetails.TxID,
			nil,
			nil,
			txDetails.Type,
		)
	} else {
		txEnv, txID, err = ConstructUnsignedTxEnv(
			util.GetTestChainID(),
			ccid,
			nil,
			txDetails.SimulationResults,
			txDetails.TxID,
			nil,
			nil,
			txDetails.Type,
		)
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

func ConstructBlockWithTxid(
	t *testing.T,
	blockNum uint64,
	previousHash []byte,
	simulationResults [][]byte,
	txids []string,
	sign bool,
) *common.Block {
	return ConstructBlockWithTxidHeaderType(
		t,
		blockNum,
		previousHash,
		simulationResults,
		txids,
		sign,
		common.HeaderType_ENDORSER_TRANSACTION,
	)
}

func ConstructBlockWithTxidHeaderType(
	t *testing.T,
	blockNum uint64,
	previousHash []byte,
	simulationResults [][]byte,
	txids []string,
	sign bool,
	headerType common.HeaderType,
) *common.Block {
	envs := []*common.Envelope{}
	for i := 0; i < len(simulationResults); i++ {
		env, _, err := ConstructTransactionWithHeaderType(
			t,
			simulationResults[i],
			txids[i],
			sign,
			headerType,
		)
		if err != nil {
			t.Fatalf("ConstructTestTransaction failed, err %s", err)
		}
		envs = append(envs, env)
	}
	return NewBlock(envs, blockNum, previousHash)
}

// ConstructBlock constructs a single block
func ConstructBlock(
	t *testing.T,
	blockNum uint64,
	previousHash []byte,
	simulationResults [][]byte,
	sign bool,
) *common.Block {
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
	return constructBytesProposalResponsePayload(util.GetTestChainID(), ccid, nil, simulationResults)
}

func NewBlock(env []*common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	block := protoutil.NewBlock(blockNum, previousHash)
	for i := 0; i < len(env); i++ {
		txEnvBytes, _ := proto.Marshal(env[i])
		block.Data.Data = append(block.Data.Data, txEnvBytes)
	}
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
	protoutil.InitBlockMetadata(block)

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = lutils.NewTxValidationFlagsSetValue(len(env), pb.TxValidationCode_VALID)

	return block
}

// constructBytesProposalResponsePayload constructs a ProposalResponsePayload byte for tests with a default signer.
func constructBytesProposalResponsePayload(chainID string, ccid *pb.ChaincodeID, pResponse *pb.Response, simulationResults []byte) ([]byte, error) {
	ss, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	prop, _, err := protoutil.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, chainID, &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: ccid}}, ss)
	if err != nil {
		return nil, err
	}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, pResponse, simulationResults, nil, ccid, nil, signer)
	if err != nil {
		return nil, err
	}

	return presp.Payload, nil
}

// ConstructSignedTxEnvWithDefaultSigner constructs a transaction envelop for tests with a default signer.
// This method helps other modules to construct a transaction with supplied parameters
func ConstructSignedTxEnvWithDefaultSigner(
	chainID string,
	ccid *pb.ChaincodeID,
	response *pb.Response,
	simulationResults []byte,
	txid string,
	events []byte,
	visibility []byte,
	headerType common.HeaderType,
) (*common.Envelope, string, error) {

	return ConstructSignedTxEnv(
		chainID,
		ccid,
		response,
		simulationResults,
		txid,
		events,
		visibility,
		signer,
		headerType,
	)
}

// ConstructUnsignedTxEnv creates a Transaction envelope from given inputs
func ConstructUnsignedTxEnv(
	chainID string,
	ccid *pb.ChaincodeID,
	response *pb.Response,
	simulationResults []byte,
	txid string,
	events []byte,
	visibility []byte,
	headerType common.HeaderType,
) (*common.Envelope, string, error) {
	mspLcl := mmsp.NewNoopMsp()
	sigId, _ := mspLcl.GetDefaultSigningIdentity()

	return ConstructSignedTxEnv(
		chainID,
		ccid,
		response,
		simulationResults,
		txid,
		events,
		visibility,
		sigId,
		headerType,
	)
}

// ConstructSignedTxEnv constructs a transaction envelop for tests
func ConstructSignedTxEnv(
	chainID string,
	ccid *pb.ChaincodeID,
	pResponse *pb.Response,
	simulationResults []byte,
	txid string,
	events []byte,
	visibility []byte,
	signer msp.SigningIdentity,
	headerType common.HeaderType,
) (*common.Envelope, string, error) {
	ss, err := signer.Serialize()
	if err != nil {
		return nil, "", err
	}

	var prop *pb.Proposal
	if txid == "" {
		// if txid is not set, then we need to generate one while creating the proposal message
		prop, txid, err = protoutil.CreateChaincodeProposal(
			common.HeaderType_ENDORSER_TRANSACTION,
			chainID,
			&pb.ChaincodeInvocationSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					ChaincodeId: ccid,
				},
			},
			ss,
		)

	} else {
		// if txid is set, we should not generate a txid instead reuse the given txid
		nonce, err := crypto.GetRandomNonce()
		if err != nil {
			return nil, "", err
		}
		prop, txid, err = protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(
			txid,
			headerType,
			chainID,
			&pb.ChaincodeInvocationSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					ChaincodeId: ccid,
				},
			},
			nonce,
			ss,
			nil,
		)
	}
	if err != nil {
		return nil, "", err
	}

	presp, err := protoutil.CreateProposalResponse(
		prop.Header,
		prop.Payload,
		pResponse,
		simulationResults,
		nil,
		ccid,
		nil,
		signer,
	)
	if err != nil {
		return nil, "", err
	}

	env, err := protoutil.CreateSignedTx(prop, signer, presp)
	if err != nil {
		return nil, "", err
	}
	return env, txid, nil
}
