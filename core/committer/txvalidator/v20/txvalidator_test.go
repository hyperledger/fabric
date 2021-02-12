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
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	mocktxvalidator "github.com/hyperledger/fabric/core/mocks/txvalidator"
	"github.com/hyperledger/fabric/internal/pkg/txflags"
	mspmgmt "github.com/hyperledger/fabric/msp/mgmt"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockDispatcher is still useful for the parallel test. Auto-generated mocks
// serialize invocations with locks but we don't need that and the test is
// much faster with this mock
type mockDispatcher struct {
	DispatchRv  peer.TxValidationCode
	DispatchErr error
}

func (v *mockDispatcher) Dispatch(seq int, payload *common.Payload, envBytes []byte, block *common.Block) (peer.TxValidationCode, error) {
	return v.DispatchRv, v.DispatchErr
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
	require.NoError(t, err)

	mockDispatcher := &mockDispatcher{}
	mockLedger := &mocks.LedgerResources{}
	mockCapabilities := &tmocks.ApplicationCapabilities{}
	mockLedger.On("TxIDExists", mock.Anything).Return(false, nil)
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

	txsfltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	for i := 0; i < nBlocks; i++ {
		require.True(t, txsfltr.IsSetTo(i, peer.TxValidationCode_VALID))
	}
}

func TestDetectTXIdDuplicates(t *testing.T) {
	txids := []string{"", "1", "2", "3", "", "2", ""}
	txsfltr := txflags.New(len(txids))
	markTXIdDuplicates(txids, txsfltr)
	require.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(2, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(3, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(4, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(5, peer.TxValidationCode_DUPLICATE_TXID))
	require.True(t, txsfltr.IsSetTo(6, peer.TxValidationCode_NOT_VALIDATED))

	txids = []string{"", "1", "2", "3", "", "21", ""}
	txsfltr = txflags.New(len(txids))
	markTXIdDuplicates(txids, txsfltr)
	require.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(2, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(3, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(4, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(5, peer.TxValidationCode_NOT_VALIDATED))
	require.True(t, txsfltr.IsSetTo(6, peer.TxValidationCode_NOT_VALIDATED))
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
	require.NoError(t, err)

	mockDispatcher := &mockDispatcher{}
	mockCapabilities := &tmocks.ApplicationCapabilities{}
	mockCapabilities.On("ForbidDuplicateTXIdInBlock").Return(true)
	mockLedger := &mocks.LedgerResources{}
	mockLedger.On("TxIDExists", mock.Anything).Return(false, nil)
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
	require.NoError(t, err)
	envs = append(envs, env)
	envs = append(envs, env)
	block := testutil.NewBlock(envs, 1, []byte("Water, water everywhere and all the boards did shrink"))

	tValidator.Validate(block)

	txsfltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])

	require.True(t, txsfltr.IsSetTo(0, peer.TxValidationCode_VALID))
	require.True(t, txsfltr.IsSetTo(1, peer.TxValidationCode_DUPLICATE_TXID))
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
	require.NoError(t, err)

	mockLedger := &mocks.LedgerResources{}
	mockLedger.On("TxIDExists", mock.Anything).Return(false, nil)
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
	require.NoError(t, err)
	mockSignerSerialized, err := mockSigner.Serialize()
	require.NoError(t, err)

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
	require.NoError(t, err)

	sig, err := mockSigner.Sign(payloadBytes)
	require.NoError(t, err)

	// Envelope the payload
	envelope := &common.Envelope{
		Payload:   payloadBytes,
		Signature: sig,
	}

	envelopeBytes, err := proto.Marshal(envelope)

	// Check marshaling didn't fail
	require.NoError(t, err)

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
	txsFilter := txflags.NewWithValues(len(block.Data.Data), peer.TxValidationCode_VALID)
	block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = txsFilter

	// Validation should invalidate transaction,
	// because it's already committed
	tValidator.Validate(block)

	txsfltr := txflags.ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	require.True(t, txsfltr.IsInvalid(0))

	// We expect the tx to be invalid because of a bad txid
	require.True(t, txsfltr.Flag(0) == peer.TxValidationCode_BAD_PROPOSAL_TXID)
}
