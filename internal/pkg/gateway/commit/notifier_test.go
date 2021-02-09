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

	NewTransactionEnvelope := func(transaction *Transaction) *commonProto.Envelope {
		channelHeader := &commonProto.ChannelHeader{
			Type: int32(commonProto.HeaderType_ENDORSER_TRANSACTION),
			TxId: transaction.ID,
		}
		channelHeaderBytes, err := proto.Marshal(channelHeader)
		assert.NoError(t, err)

		payload := &commonProto.Payload{
			Header: &commonProto.Header{
				ChannelHeader: channelHeaderBytes,
			},
		}
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
			Metadata: &commonProto.BlockMetadata{
				Metadata: metadata,
			},
			Data: &commonProto.BlockData{
				Data: envelopes,
			},
		}
	}

	NewMockChannelFactory := func(blocks ...*commonProto.Block) *commitMocks.ChannelFactory {
		channelFactory := &commitMocks.ChannelFactory{}

		channel := &deliverMock.Chain{}
		channelFactory.ChannelReturns(channel, nil)

		reader := &deliverMock.BlockReader{}
		channel.ReaderReturns(reader)

		iterator := &deliverMock.BlockIterator{}
		reader.IteratorReturns(iterator, 0)

		for i, block := range blocks {
			iterator.NextReturnsOnCall(i, block, commonProto.Status_SUCCESS)
		}

		return channelFactory
	}

	NewTestNotifier := func(blocks ...*commonProto.Block) *commit.Notifier {
		channelFactory := NewMockChannelFactory(blocks...)
		return commit.NewNotifier(channelFactory)
	}

	t.Run("Notify returns error if channel does not exist", func(t *testing.T) {
		expected := errors.New("NO_CHANNEL_ERROR")
		channelFactory := &commitMocks.ChannelFactory{}
		channelFactory.ChannelReturns(nil, expected)
		notifier := commit.NewNotifier(channelFactory)

		commitChannel := make(chan peerProto.TxValidationCode)
		err := notifier.Notify(commitChannel, "channel", "transactionID")
		assert.ErrorContains(t, err, expected.Error())
	})

	t.Run("Notifies valid transaction status", func(t *testing.T) {
		transaction := &Transaction{
			ID:     "TRANSACTION_ID",
			Status: peerProto.TxValidationCode_VALID,
		}
		block := NewBlock(t, transaction)
		notifier := NewTestNotifier(block)

		commitChannel := make(chan peerProto.TxValidationCode)
		assert.NoError(t, notifier.Notify(commitChannel, "channel", transaction.ID))

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

		commitChannel := make(chan peerProto.TxValidationCode)
		assert.NoError(t, notifier.Notify(commitChannel, "channel", transaction.ID))

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

		commitChannel := make(chan peerProto.TxValidationCode)
		assert.NoError(t, notifier.Notify(commitChannel, "channel", transaction.ID))

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

		commitChannel := make(chan peerProto.TxValidationCode)
		assert.NoError(t, notifier.Notify(commitChannel, "channel", transaction.ID))

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

		commitChannel := make(chan peerProto.TxValidationCode)
		assert.NoError(t, notifier.Notify(commitChannel, "channel", transaction.ID))

		result, ok := <-commitChannel

		assert.True(t, ok, "Commit channel was closed")
		assert.Equal(t, transaction.Status, result)
	})
}
