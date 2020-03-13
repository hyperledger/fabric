/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txvalidator

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/ledger/testutil"
	"github.com/hyperledger/fabric/common/semaphore"
	tmocks "github.com/hyperledger/fabric/core/committer/txvalidator/mocks"
	"github.com/hyperledger/fabric/core/committer/txvalidator/v20/mocks"
	ledger2 "github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/core/ledger/util"
	ledgerUtil "github.com/hyperledger/fabric/core/ledger/util"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockDispatcher is still useful for the parallel test. Auto-generated mocks
// serialize invocations with locks but we don't need that and the test is
// much faster with this mock
type mockDispatcher struct {
	DispatchRv  peer.TxValidationCode
	DispatchErr error
}

func (v *mockDispatcher) Dispatch(seq int, payload *common.Payload, envBytes []byte, block *common.Block) (error, peer.TxValidationCode) {
	return v.DispatchErr, v.DispatchRv
}

func testValidationWithNTXes(t *testing.T, nBlocks int) {
	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet("ns1", "key1", []byte("value1"))
	rwsb.AddToWriteSet("ns1", "key2", []byte("value2"))
	rwsb.AddToWriteSet("ns1", "key3", []byte("value3"))

	simRes, _ := rwsb.GetTxSimulationResults()
	pubSimulationResBytes, _ := simRes.GetPubSimulationBytes()
	_, err := testutil.ConstructBytesProposalResponsePayload("v1", pubSimulationResBytes)
	if err != nil {
		t.Fatalf("Could not construct ProposalResponsePayload bytes, err: %s", err)
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	mockDispatcher := &mockDispatcher{}
	mockLedger := &mocks.LedgerResources{}
	mockCapabilities := &tmocks.ApplicationCapabilities{}
	mockLedger.On("GetTransactionByID", mock.Anything).Return(nil, ledger2.NotFoundInIndexErr("Day after day, day after day"))
	tValidator := &TxValidator{
		ChannelID:        "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{ACVal: mockCapabilities},
		Dispatcher:       mockDispatcher,
		LedgerResources:  mockLedger,
		CryptoProvider:   cryptoProvider,
	}

	sr := [][]byte{}
	for i := 0; i < nBlocks; i++ {
		sr = append(sr, pubSimulationResBytes)
	}
	block := testutil.ConstructBlock(t, 1, []byte("we stuck nor breath nor motion"), sr, true)

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
	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet("ns1", "key1", []byte("value1"))
	rwsb.AddToWriteSet("ns1", "key2", []byte("value2"))
	rwsb.AddToWriteSet("ns1", "key3", []byte("value3"))

	simRes, _ := rwsb.GetTxSimulationResults()
	pubSimulationResBytes, _ := simRes.GetPubSimulationBytes()
	_, err := testutil.ConstructBytesProposalResponsePayload("v1", pubSimulationResBytes)
	if err != nil {
		t.Fatalf("Could not construct ProposalResponsePayload bytes, err: %s", err)
	}

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	mockDispatcher := &mockDispatcher{}
	mockCapabilities := &tmocks.ApplicationCapabilities{}
	mockCapabilities.On("ForbidDuplicateTXIdInBlock").Return(true)
	mockLedger := &mocks.LedgerResources{}
	mockLedger.On("GetTransactionByID", mock.Anything).Return(nil, ledger2.NotFoundInIndexErr("As idle as a painted ship upon a painted ocean"))
	tValidator := &TxValidator{
		ChannelID:        "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{ACVal: mockCapabilities},
		Dispatcher:       mockDispatcher,
		LedgerResources:  mockLedger,
		CryptoProvider:   cryptoProvider,
	}

	envs := []*common.Envelope{}
	env, _, err := testutil.ConstructTransaction(t, pubSimulationResBytes, "", true)
	envs = append(envs, env)
	envs = append(envs, env)
	block := testutil.NewBlock(envs, 1, []byte("Water, water everywhere and all the boards did shrink"))

	tValidator.Validate(block)

	txsfltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	assert.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_VALID))
	assert.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_DUPLICATE_TXID))
}

func TestBlockValidation(t *testing.T) {
	// here we test validation of a block with a single tx
	testValidationWithNTXes(t, 1)
}

func TestParallelBlockValidation(t *testing.T) {
	// here we test validation of a block with 128 txes
	testValidationWithNTXes(t, 128)
}

func TestVeryLargeParallelBlockValidation(t *testing.T) {
	// here we test validation of a block with 4096 txes,
	// which is larger than both the number of workers in
	// the pool and the buffer in the channels
	testValidationWithNTXes(t, 4096)
}

func TestTxValidationFailure_InvalidTxid(t *testing.T) {
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	assert.NoError(t, err)

	mockLedger := &mocks.LedgerResources{}
	mockLedger.On("GetTransactionByID", mock.Anything).Return(nil, ledger2.NotFoundInIndexErr("Water, water, everywhere, nor any drop to drink"))
	mockCapabilities := &tmocks.ApplicationCapabilities{}
	tValidator := &TxValidator{
		ChannelID:        "",
		Semaphore:        semaphore.New(10),
		ChannelResources: &mocktxvalidator.Support{ACVal: mockCapabilities},
		Dispatcher:       &mockDispatcher{},
		LedgerResources:  mockLedger,
		CryptoProvider:   cryptoProvider,
	}

	mockSigner, err := mspmgmt.GetLocalMSP(cryptoProvider).GetDefaultSigningIdentity()
	assert.NoError(t, err)
	mockSignerSerialized, err := mockSigner.Serialize()
	assert.NoError(t, err)

	// Create simple endorsement transaction
	payload := &common.Payload{
		Header: &common.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
				TxId:      "INVALID TXID!!!",
				Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
				ChannelId: "testchannelid",
			}),
			SignatureHeader: protoutil.MarshalOrPanic(&common.SignatureHeader{
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
		DataHash: protoutil.BlockDataHash(block.Data),
	}

	// Initialize metadata
	protoutil.InitBlockMetadata(block)
	txsFilter := util.NewTxValidationFlagsSetValue(len(block.Data.Data), peer.TxValidationCode_VALID)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	// Validation should invalidate transaction,
	// because it's already committed
	tValidator.Validate(block)

	txsfltr := util.TxValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	assert.True(t, txsfltr.IsInvalid(0))

	// We expect the tx to be invalid because of a bad txid
	assert.True(t, txsfltr.Flag(0) == peer.TxValidationCode_BAD_PROPOSAL_TXID)
}
