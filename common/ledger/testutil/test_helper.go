/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testutil

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/crypto"
	"github.com/hyperledger/fabric/internal/pkg/txflags"

	"github.com/hyperledger/fabric/common/ledger/testutil/fakes"
	"github.com/hyperledger/fabric/msp"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/require"
)

var signer msp.SigningIdentity

func init() {
	// setup the MSP manager so that we can sign/verify
	err := msptesttools.LoadMSPSetupForTesting()
	if err != nil {
		panic(fmt.Errorf("Could not load msp config, err %s", err))
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		panic(fmt.Errorf("Initialize cryptoProvider failed: %s", err))
	}
	signer, err = mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	if err != nil {
		panic(fmt.Errorf("Could not initialize msp/signer"))
	}
}

// BlockGenerator generates a series of blocks for testing
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
	ChaincodeEvents                 []byte
	Type                            common.HeaderType
}

type BlockDetails struct {
	BlockNum     uint64
	PreviousHash []byte
	Txs          []*TxDetails
}

//go:generate counterfeiter -o fakes/signing_identity.go --fake-name SigningIdentity . signingIdentity

type signingIdentity interface {
	msp.SigningIdentity
}

// NewBlockGenerator instantiates new BlockGenerator for testing
func NewBlockGenerator(t *testing.T, ledgerID string, signTxs bool) (*BlockGenerator, *common.Block) {
	gb, err := test.MakeGenesisBlock(ledgerID)
	require.NoError(t, err)
	gb.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txflags.NewWithValues(len(gb.Data.Data), pb.TxValidationCode_VALID)
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
		block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txflags.NewWithValues(numTx, pb.TxValidationCode_VALID)
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
			"testchannelid",
			ccid,
			nil,
			txDetails.SimulationResults,
			txDetails.TxID,
			txDetails.ChaincodeEvents,
			nil,
			txDetails.Type,
		)
	} else {
		txEnv, txID, err = ConstructUnsignedTxEnv(
			"testchannelid",
			ccid,
			nil,
			txDetails.SimulationResults,
			txDetails.TxID,
			txDetails.ChaincodeEvents,
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

// ConstructTestBlock constructs a single block with random contents
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
	bg, gb := NewBlockGenerator(t, "testchannelid", false)
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
	return constructBytesProposalResponsePayload("testchannelid", ccid, nil, simulationResults)
}

func NewBlock(env []*common.Envelope, blockNum uint64, previousHash []byte) *common.Block {
	block := protoutil.NewBlock(blockNum, previousHash)
	for i := 0; i < len(env); i++ {
		txEnvBytes, _ := proto.Marshal(env[i])
		block.Data.Data = append(block.Data.Data, txEnvBytes)
	}
	block.Header.DataHash = protoutil.BlockDataHash(block.Data)
	protoutil.InitBlockMetadata(block)

	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txflags.NewWithValues(len(env), pb.TxValidationCode_VALID)

	return block
}

// constructBytesProposalResponsePayload constructs a ProposalResponsePayload byte for tests with a default signer.
func constructBytesProposalResponsePayload(channelID string, ccid *pb.ChaincodeID, pResponse *pb.Response, simulationResults []byte) ([]byte, error) {
	ss, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	prop, _, err := protoutil.CreateChaincodeProposal(common.HeaderType_ENDORSER_TRANSACTION, channelID, &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{ChaincodeId: ccid}}, ss)
	if err != nil {
		return nil, err
	}

	presp, err := protoutil.CreateProposalResponse(prop.Header, prop.Payload, pResponse, simulationResults, nil, ccid, signer)
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
	sigId := &fakes.SigningIdentity{}

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
	channelID string,
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
			channelID,
			&pb.ChaincodeInvocationSpec{
				ChaincodeSpec: &pb.ChaincodeSpec{
					ChaincodeId: ccid,
				},
			},
			ss,
		)
	} else {
		// if txid is set, we should not generate a txid instead reuse the given txid
		var nonce []byte
		nonce, err = crypto.GetRandomNonce()
		if err != nil {
			return nil, "", err
		}
		prop, txid, err = protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(
			txid,
			headerType,
			channelID,
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
		events,
		ccid,
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

func SetTxID(t *testing.T, block *common.Block, txNum int, txID string) {
	envelopeBytes := block.Data.Data[txNum]
	envelope, err := protoutil.UnmarshalEnvelope(envelopeBytes)
	if err != nil {
		t.Fatalf("error unmarshalling envelope: %s", err)
	}

	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	if err != nil {
		t.Fatalf("error getting payload from envelope: %s", err)
	}

	channelHeader, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
	if err != nil {
		t.Fatalf("error unmarshalling channel header: %s", err)
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
