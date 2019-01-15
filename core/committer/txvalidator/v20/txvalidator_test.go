/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/configtx/test"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/mocks/config"
	"github.com/hyperledger/fabric/common/semaphore"
	util2 "github.com/hyperledger/fabric/common/util"
	ledger2 "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/ledgermgmt"
	"github.com/hyperledger/fabric/core/ledger/util"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	"github.com/hyperledger/fabric/core/mocks/validator"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protos/common"
	"github.com/hyperledger/fabric/protos/peer"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func testValidationWithNTXes(t *testing.T, ledger ledger2.PeerLedger, gbHash []byte, nBlocks int) {
	txid := util2.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()

	simRes, _ := simulator.GetTxSimulationResults()
	pubSimulationResBytes, _ := simRes.GetPubSimulationBytes()
	_, err := testutil.ConstructBytesProposalResponsePayload("v1", pubSimulationResBytes)
	if err != nil {
		t.Fatalf("Could not construct ProposalResponsePayload bytes, err: %s", err)
	}

	mockVsccValidator := &validator.MockVsccValidator{}
	tValidator := &TxValidator{
		ChainID:          "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{LedgerVal: ledger, ACVal: &config.MockApplicationCapabilities{}},
		Vscc:             mockVsccValidator,
	}

	bcInfo, _ := ledger.GetBlockchainInfo()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	sr := [][]byte{}
	for i := 0; i < nBlocks; i++ {
		sr = append(sr, pubSimulationResBytes)
	}
	block := testutil.ConstructBlock(t, 1, gbHash, sr, true)

	tValidator.Validate(block)

	txsfltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	for i := 0; i < nBlocks; i++ {
		assert.True(t, txsfltr.IsSetTo(i, peer.TxValidationCode_VALID))
	}
}

func TestDetectTXIdDuplicates(t *testing.T) {
	txids := []string{"", "1", "2", "3", "", "2", ""}
	txsfltr := ledgerUtil.NewTxValidationFlags(len(txids))
	markTXIdDuplicates(txids, txsfltr)
	assert.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(2, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(3, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(4, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(5, peer.TxValidationCode_DUPLICATE_TXID))
	assert.True(t, txsfltr.IsSetTo(6, peer.TxValidationCode_NOT_VALIDATED))

	txids = []string{"", "1", "2", "3", "", "21", ""}
	txsfltr = ledgerUtil.NewTxValidationFlags(len(txids))
	markTXIdDuplicates(txids, txsfltr)
	assert.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(2, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(3, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(4, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(5, peer.TxValidationCode_NOT_VALIDATED))
	assert.True(t, txsfltr.IsSetTo(6, peer.TxValidationCode_NOT_VALIDATED))
}

func TestBlockValidationDuplicateTXId(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest-2.0")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	gb, _ := test.MakeGenesisBlock("TestLedger-2.0")
	gbHash := gb.Header.Hash()
	ledger, _ := ledgermgmt.CreateLedger(gb)
	defer ledger.Close()

	txid := util2.GenerateUUID()
	simulator, _ := ledger.NewTxSimulator(txid)
	simulator.SetState("ns1", "key1", []byte("value1"))
	simulator.SetState("ns1", "key2", []byte("value2"))
	simulator.SetState("ns1", "key3", []byte("value3"))
	simulator.Done()

	simRes, _ := simulator.GetTxSimulationResults()
	pubSimulationResBytes, _ := simRes.GetPubSimulationBytes()
	_, err := testutil.ConstructBytesProposalResponsePayload("v1", pubSimulationResBytes)
	if err != nil {
		t.Fatalf("Could not construct ProposalResponsePayload bytes, err: %s", err)
	}

	mockVsccValidator := &validator.MockVsccValidator{}
	acv := &config.MockApplicationCapabilities{
		ForbidDuplicateTXIdInBlockRv: true,
	}
	tValidator := &TxValidator{
		ChainID:          "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{LedgerVal: ledger, ACVal: acv},
		Vscc:             mockVsccValidator,
	}

	bcInfo, _ := ledger.GetBlockchainInfo()
	assert.Equal(t, &common.BlockchainInfo{
		Height: 1, CurrentBlockHash: gbHash, PreviousBlockHash: nil,
	}, bcInfo)

	envs := []*common.Envelope{}
	env, _, err := testutil.ConstructTransaction(t, pubSimulationResBytes, "", true)
	envs = append(envs, env)
	envs = append(envs, env)
	block := testutil.NewBlock(envs, 1, gbHash)

	tValidator.Validate(block)

	txsfltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	assert.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_VALID))
	assert.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_DUPLICATE_TXID))
}

func TestBlockValidation(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest-2.0")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	gb, _ := test.MakeGenesisBlock("TestLedger-2.0")
	gbHash := gb.Header.Hash()
	ledger, _ := ledgermgmt.CreateLedger(gb)
	defer ledger.Close()

	// here we test validation of a block with a single tx
	testValidationWithNTXes(t, ledger, gbHash, 1)
}

func TestParallelBlockValidation(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest-2.0")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	gb, _ := test.MakeGenesisBlock("TestLedger-2.0")
	gbHash := gb.Header.Hash()
	ledger, _ := ledgermgmt.CreateLedger(gb)
	defer ledger.Close()

	// here we test validation of a block with 128 txes
	testValidationWithNTXes(t, ledger, gbHash, 128)
}

func TestVeryLargeParallelBlockValidation(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest-2.0")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	gb, _ := test.MakeGenesisBlock("TestLedger-2.0")
	gbHash := gb.Header.Hash()
	ledger, _ := ledgermgmt.CreateLedger(gb)
	defer ledger.Close()

	// here we test validation of a block with 4096 txes,
	// which is larger than both the number of workers in
	// the pool and the buffer in the channels
	testValidationWithNTXes(t, ledger, gbHash, 4096)
}

func TestTxValidationFailure_InvalidTxid(t *testing.T) {
	viper.Set("peer.fileSystemPath", "/tmp/fabric/txvalidatortest-2.0")
	ledgermgmt.InitializeTestEnv()
	defer ledgermgmt.CleanupTestEnv()

	gb, _ := test.MakeGenesisBlock("TestLedger-2.0")
	ledger, _ := ledgermgmt.CreateLedger(gb)

	defer ledger.Close()

	tValidator := &TxValidator{
		ChainID:          "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{LedgerVal: ledger, ACVal: &config.MockApplicationCapabilities{}},
		Vscc:             &validator.MockVsccValidator{},
	}

	mockSigner, err := mspmgmt.GetLocalMSP().GetDefaultSigningIdentity()
	assert.NoError(t, err)
	mockSignerSerialized, err := mockSigner.Serialize()
	assert.NoError(t, err)

	// Create simple endorsement transaction
	payload := &common.Payload{
		Header: &common.Header{
			ChannelHeader: utils.MarshalOrPanic(&common.ChannelHeader{
				TxId:      "INVALID TXID!!!",
				Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
				ChannelId: util2.GetTestChainID(),
			}),
			SignatureHeader: utils.MarshalOrPanic(&common.SignatureHeader{
				Nonce:   []byte("nonce"),
				Creator: mockSignerSerialized,
			}),
		},
		Data: []byte("test"),
	}

	payloadBytes, err := proto.Marshal(payload)

	// Check marshaling didn't fail
	assert.NoError(t, err)

	sig, err := mockSigner.Sign(payloadBytes)
	assert.NoError(t, err)

	// Envelope the payload
	envelope := &common.Envelope{
		Payload:   payloadBytes,
		Signature: sig,
	}

	envelopeBytes, err := proto.Marshal(envelope)

	// Check marshaling didn't fail
	assert.NoError(t, err)

	block := &common.Block{
		Data: &common.BlockData{
			// Enconde transactions
			Data: [][]byte{envelopeBytes},
		},
	}

	block.Header = &common.BlockHeader{
		Number:   0,
		DataHash: block.Data.Hash(),
	}

	// Initialize metadata
	utils.InitBlockMetadata(block)
	txsFilter := util.NewTxValidationFlagsSetValue(len(block.Data.Data), peer.TxValidationCode_VALID)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	// Commit block to the ledger
	ledger.CommitWithPvtData(&ledger2.BlockAndPvtData{
		Block: block,
	})

	// Validation should invalidate transaction,
	// because it's already committed
	tValidator.Validate(block)

	txsfltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assert.True(t, txsfltr.IsInvalid(0))

	// We expect the tx to be invalid because of a bad txid
	assert.True(t, txsfltr.Flag(0) == peer.TxValidationCode_BAD_PROPOSAL_TXID)
}
