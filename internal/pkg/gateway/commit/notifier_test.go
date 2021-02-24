/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit_test

import (
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	commonProto "github.com/hyperledger/fabric-protos-go/common"
	peerProto "github.com/hyperledger/fabric-protos-go/peer"
	deliverMock "github.com/hyperledger/fabric/common/deliver/mock"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	commitMocks "github.com/hyperledger/fabric/internal/pkg/gateway/commit/mocks"
	"github.com/stretchr/testify/assert"
)

func TestNotifier(t *testing.T) {
	type Transaction struct {
		ID     string
		Status peerProto.TxValidationCode
	}

	NewPayload := func(transactionID string) *commonProto.Payload {
		channelHeader := &commonProto.ChannelHeader{
			Type: int32(commonProto.HeaderType_ENDORSER_TRANSACTION),
			TxId: transactionID,
		}
		channelHeaderBytes, err := proto.Marshal(channelHeader)
		assert.NoError(t, err)

		return &commonProto.Payload{
			Header: &commonProto.Header{
				ChannelHeader: channelHeaderBytes,
			},
		}
	}

	NewTransactionEnvelope := func(transaction *Transaction) *commonProto.Envelope {
		payload := NewPayload(transaction.ID)
		payloadBytes, err := proto.Marshal(payload)
		assert.NoError(t, err)

		return &commonProto.Envelope{
			Payload: payloadBytes,
		}
	}

	NewBlock := func(t *testing.T, transactions ...*Transaction) *commonProto.Block {
		var validationCodes []byte
		var envelopes [][]byte

		for _, transaction := range transactions {
			validationCodes = append(validationCodes, byte(transaction.Status))

			envelope := NewTransactionEnvelope(transaction)
			envelopeBytes, err := proto.Marshal(envelope)
			assert.NoError(t, err)
			envelopes = append(envelopes, envelopeBytes)
		}

		metadata := make([][]byte, len(commonProto.BlockMetadataIndex_name))
		metadata[int(commonProto.BlockMetadataIndex_TRANSACTIONS_FILTER)] = validationCodes

		return &commonProto.Block{
			Header: &commonProto.BlockHeader{
				Number: 1,
			},
			Metadata: &commonProto.BlockMetadata{
				Metadata: metadata,
			},
			Data: &commonProto.BlockData{
				Data: envelopes,
			},
		}
	}

	NewMockBlockReader := func(blocks ...*commonProto.Block) *commitMocks.BlockReader {
		blockReader := &commitMocks.BlockReader{}

		iterator := &deliverMock.BlockIterator{}
		blockReader.IteratorReturns(iterator, nil)

		for i, block := range blocks {
			iterator.NextReturnsOnCall(i, block, commonProto.Status_SUCCESS)
		}

		return blockReader
	}

	NewTestNotifier := func(blocks ...*commonProto.Block) *commit.Notifier {
		blockReader := NewMockBlockReader(blocks...)
		return commit.NewNotifier(blockReader)
	}

	t.Run("Notify returns error if block iterator cannot be obtained", func(t *testing.T) {
		expected := errors.New("MY_ERROR_MESSAGE")
		blockReader := &commitMocks.BlockReader{}
		blockReader.IteratorReturns(nil, expected)
		notifier := commit.NewNotifier(blockReader)

		_, err := notifier.Notify("channel", "transactionID")
		assert.ErrorContains(t, err, expected.Error())
	})

	t.Run("Closes notification channel if unable to read from block iterator", func(t *testing.T) {
		blockReader := &commitMocks.BlockReader{}
		blockIterator := &deliverMock.BlockIterator{}
		blockReader.IteratorReturns(blockIterator, nil)
		blockIterator.NextReturns(nil, commonProto.Status_INTERNAL_SERVER_ERROR)
		notifier := commit.NewNotifier(blockReader)

		commitChannel, err := notifier.Notify("channel", "transactionID")
		assert.NoError(t, err)

		_, ok := <-commitChannel

		assert.False(t, ok, "Commit channel not closed")
	})

	t.Run("Notifies valid transaction status", func(t *testing.T) {
		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_VALID,
		}
		block := NewBlock(t, transaction)
		notifier := NewTestNotifier(block)

		commitChannel, err := notifier.Notify("channel", transaction.ID)
		assert.NoError(t, err)

		result, ok := <-commitChannel

		assert.True(t, ok, "Commit channel was closed")
		assert.Equal(t, transaction.Status, result)
	})

	t.Run("Notifies invalid transaction status", func(t *testing.T) {
		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_MVCC_READ_CONFLICT,
		}
		block := NewBlock(t, transaction)
		notifier := NewTestNotifier(block)

		commitChannel, _ := notifier.Notify("channel", transaction.ID)

		result, ok := <-commitChannel

		assert.True(t, ok, "Commit channel was closed")
		assert.Equal(t, transaction.Status, result)
	})

	t.Run("Closes notification channel after delivering transaction status", func(t *testing.T) {
		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_MVCC_READ_CONFLICT, // Don't use VALID since it matches default channel value
		}
		block := NewBlock(t, transaction)
		notifier := NewTestNotifier(block)

		commitChannel, err := notifier.Notify("channel", transaction.ID)
		assert.NoError(t, err)

		<-commitChannel
		result, ok := <-commitChannel

		assert.False(t, ok, "Commit channel not closed")
		assert.Equal(t, peerProto.TxValidationCode(0), result)
	})

	t.Run("Ignores envelopes for other transactions", func(t *testing.T) {
		dummyTransaction := &Transaction{
			ID:     "DUMMY",
			Status: peerProto.TxValidationCode_MVCC_READ_CONFLICT,
		}
		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_VALID,
		}
		block := NewBlock(t, dummyTransaction, transaction)
		notifier := NewTestNotifier(block)

		commitChannel, err := notifier.Notify("channel", transaction.ID)
		assert.NoError(t, err)

		result, ok := <-commitChannel

		assert.True(t, ok, "Commit channel was closed")
		assert.Equal(t, transaction.Status, result)
	})

	t.Run("Ignores blocks not containing specified transaction", func(t *testing.T) {
		dummyTransaction := &Transaction{
			ID:     "DUMMY",
			Status: peerProto.TxValidationCode_MVCC_READ_CONFLICT,
		}
		dummyBlock := NewBlock(t, dummyTransaction)

		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_VALID,
		}
		block := NewBlock(t, transaction)

		notifier := NewTestNotifier(dummyBlock, block)

		commitChannel, err := notifier.Notify("channel", transaction.ID)
		assert.NoError(t, err)

		result, ok := <-commitChannel

		assert.True(t, ok, "Commit channel was closed")
		assert.Equal(t, transaction.Status, result)
	})

	t.Run("Closes notification channel on missing payload header", func(t *testing.T) {
		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_VALID,
		}
		block := NewBlock(t, transaction)

		payload := NewPayload(transaction.ID)
		payload.Header = nil
		payloadBytes, err := proto.Marshal(payload)
		assert.NoError(t, err)

		envelope := &commonProto.Envelope{
			Payload: payloadBytes,
		}
		envelopeBytes, err := proto.Marshal(envelope)
		assert.NoError(t, err)

		block.Data.Data[0] = envelopeBytes

		// Dummy block required to prevent channel close if first block does not cause close correctly
		dummyTransaction := &Transaction{
			ID:     transaction.ID,
			Status: peerProto.TxValidationCode_VALID,
		}
		dummyBlock := NewBlock(t, dummyTransaction)

		notifier := NewTestNotifier(block, dummyBlock)

		commitChannel, err := notifier.Notify("channel", transaction.ID)
		assert.NoError(t, err)

		_, ok := <-commitChannel

		assert.False(t, ok, "Commit channel not closed")
	})

	t.Run("Ignores transactions with TxValidationCode_BAD_PROPOSAL_TXID status since may have faked the ID we want", func(t *testing.T) {
		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_VALID,
		}
		badTransaction := &Transaction{
			ID:     transaction.ID,
			Status: peerProto.TxValidationCode_BAD_PROPOSAL_TXID,
		}
		block := NewBlock(t, badTransaction, transaction, badTransaction)
		notifier := NewTestNotifier(block)

		commitChannel, err := notifier.Notify("channel", transaction.ID)
		assert.NoError(t, err)

		result, ok := <-commitChannel

		assert.True(t, ok, "Commit channel was closed")
		assert.Equal(t, transaction.Status, result)
	})

	t.Run("Notifies status of first transaction with matching ID in block", func(t *testing.T) {
		transaction1 := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_VALID,
		}
		transaction2 := &Transaction{
			ID:     transaction1.ID,
			Status: peerProto.TxValidationCode_MVCC_READ_CONFLICT,
		}
		block := NewBlock(t, transaction1, transaction2)
		notifier := NewTestNotifier(block)

		commitChannel, err := notifier.Notify("channel", transaction1.ID)
		assert.NoError(t, err)

		result, ok := <-commitChannel

		assert.True(t, ok, "Commit channel was closed")
		assert.Equal(t, transaction1.Status, result)
	})
}
