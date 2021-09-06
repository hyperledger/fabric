/*
Copyright IBM Corp. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package event_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/gateway"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/ledger"
	"github.com/hyperledger/fabric/internal/pkg/gateway/event"
	"github.com/hyperledger/fabric/internal/pkg/gateway/event/mocks"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/resultsiterator.go --fake-name ResultsIterator . mockResultsIterator
type mockResultsIterator interface {
	ledger.ResultsIterator
}

func TestIterators(t *testing.T) {
	now := time.Now()
	transactionId := "TRANSACTION_ID"

	chaincodeEvent := &peer.ChaincodeEvent{
		ChaincodeId: "CHAINCODE_ID",
		TxId:        transactionId,
		EventName:   "EVENT_NAME",
		Payload:     []byte("PAYLOAD"),
	}

	txEnvelope := &common.Envelope{
		Payload: protoutil.MarshalOrPanic(&common.Payload{
			Header: &common.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
					Type: int32(common.HeaderType_ENDORSER_TRANSACTION),
					Timestamp: &timestamp.Timestamp{
						Seconds: now.Unix(),
						Nanos:   int32(now.Nanosecond()),
					},
					TxId: transactionId,
				}),
			},
			Data: protoutil.MarshalOrPanic(&peer.Transaction{
				Actions: []*peer.TransactionAction{
					{
						Payload: protoutil.MarshalOrPanic(&peer.ChaincodeActionPayload{
							Action: &peer.ChaincodeEndorsedAction{
								ProposalResponsePayload: protoutil.MarshalOrPanic(&peer.ProposalResponsePayload{
									Extension: protoutil.MarshalOrPanic(&peer.ChaincodeAction{
										Events: protoutil.MarshalOrPanic(chaincodeEvent),
									}),
								}),
							},
						}),
					},
				},
			}),
		}),
	}

	blockProto := &common.Block{
		Header: &common.BlockHeader{
			Number: 1337,
		},
		Metadata: &common.BlockMetadata{
			Metadata: [][]byte{
				nil,
				nil,
				{
					byte(peer.TxValidationCode_MVCC_READ_CONFLICT),
					byte(peer.TxValidationCode_VALID),
					byte(peer.TxValidationCode_VALID),
				},
				nil,
				nil,
			},
		},
		Data: &common.BlockData{
			Data: [][]byte{
				protoutil.MarshalOrPanic(txEnvelope),
				protoutil.MarshalOrPanic(txEnvelope),
				protoutil.MarshalOrPanic(&common.Envelope{
					Payload: protoutil.MarshalOrPanic(&common.Payload{
						Header: &common.Header{
							ChannelHeader: protoutil.MarshalOrPanic(&common.ChannelHeader{
								Type: int32(common.HeaderType_CONFIG_UPDATE),
							}),
						},
					}),
				}),
			},
		},
	}

	const invalidTxIndex = 0
	const validTxIndex = 1

	assertExpectedBlock := func(t *testing.T, block *event.Block) {
		require.NotNil(t, block, "block")
		require.EqualValues(t, blockProto.GetHeader().GetNumber(), block.Number(), "block.Number()")

		transactions, err := block.Transactions()
		require.NoError(t, err, "Transactions()")
		require.Len(t, transactions, 2, "transactions")

		for txIndex, transaction := range transactions {
			require.Equal(t, block, transaction.Block(), "transaction[%d].Block()", txIndex)
			require.Equal(t, transactionId, transaction.ID(), "transaction[%d].ID()", txIndex)
			require.EqualValues(t, now.Unix(), transaction.Timestamp().Seconds, "transaction[%d].Timestamp.Seconds", txIndex)
			require.EqualValues(t, now.Nanosecond(), transaction.Timestamp().Nanos, "transaction[%d].Tomestamp.Nanos", txIndex)

			events, err := transaction.ChaincodeEvents()
			require.NoError(t, err, "ChaincodeEvents()")
			require.Len(t, events, 1, "chaincodeEvents")

			for eventIndex, event := range events {
				require.Equal(t, transaction, event.Transaction(), "transaction[%d].ChaincodeEvents()[%d].Transaction()", txIndex, eventIndex)
				require.Equal(t, chaincodeEvent.GetChaincodeId(), event.ChaincodeID(), "transaction[%d].ChaincodeEvents()[%d].ChaincodeID()", txIndex, eventIndex)
				require.Equal(t, chaincodeEvent.GetEventName(), event.EventName(), "transaction[%d].ChaincodeEvents()[%d].EventName()", txIndex, eventIndex)
				require.EqualValues(t, chaincodeEvent.GetPayload(), event.Payload(), "transaction[%d].ChaincodeEvents()[%d].Payload()", txIndex, eventIndex)
				require.True(t, proto.Equal(chaincodeEvent, event.ProtoMessage()), "transaction[%d].ChaincodeEvents()[%d].ProtoMessage(): %v", txIndex, eventIndex, event.ProtoMessage())
			}
		}
	}

	t.Run("BlockIterator", func(t *testing.T) {
		t.Run("Next", func(t *testing.T) {
			t.Run("returns error from wrapped iterator", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}
				resultIter.NextReturns(nil, errors.New("MY_ERROR"))

				blockIter := event.NewBlockIterator(resultIter)
				_, err := blockIter.Next()

				require.ErrorContains(t, err, "MY_ERROR")
			})

			t.Run("returns error if wrapped iterator returns unexpected type", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}
				result := &common.Envelope{}
				resultIter.NextReturns(result, nil)

				blockIter := event.NewBlockIterator(resultIter)
				_, err := blockIter.Next()

				require.ErrorContains(t, err, fmt.Sprintf("%T", result))
			})

			t.Run("returns a block with no transactions", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}
				result := &common.Block{
					Header: &common.BlockHeader{
						Number: 418,
					},
				}
				resultIter.NextReturns(result, nil)

				blockIter := event.NewBlockIterator(resultIter)
				block, err := blockIter.Next()

				require.NoError(t, err, "Next()")
				require.NotNil(t, block, "block")
				require.EqualValues(t, result.GetHeader().GetNumber(), block.Number(), "Number()")

				transactions, err := block.Transactions()
				require.NoError(t, err, "Transactions()")
				require.Len(t, transactions, 0, "transactions")
			})

			t.Run("returns a block with invalid transaction", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}
				resultIter.NextReturns(blockProto, nil)

				blockIter := event.NewBlockIterator(resultIter)
				block, err := blockIter.Next()

				require.NoError(t, err, "Next()")
				assertExpectedBlock(t, block)

				transactions, _ := block.Transactions()
				transaction := transactions[invalidTxIndex]
				require.Equal(t, peer.TxValidationCode_MVCC_READ_CONFLICT, transaction.Status())
				require.False(t, transaction.Valid(), "Valid()")
			})

			t.Run("returns a block with valid transaction", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}
				resultIter.NextReturns(blockProto, nil)

				blockIter := event.NewBlockIterator(resultIter)
				block, err := blockIter.Next()

				require.NoError(t, err, "Next()")
				assertExpectedBlock(t, block)

				transactions, _ := block.Transactions()
				transaction := transactions[validTxIndex]
				require.Equal(t, peer.TxValidationCode_VALID, transaction.Status())
				require.True(t, transaction.Valid(), "Valid()")
			})
		})

		t.Run("Close", func(t *testing.T) {
			t.Run("closes wrapped iterator", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}

				blockIter := event.NewBlockIterator(resultIter)
				blockIter.Close()

				require.Equal(t, 1, resultIter.CloseCallCount())
			})
		})
	})

	t.Run("ChaincodeEventsIterator", func(t *testing.T) {
		t.Run("Next", func(t *testing.T) {
			t.Run("returns error from wrapped iterator", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}
				resultIter.NextReturns(nil, errors.New("MY_ERROR"))

				eventsIter := event.NewChaincodeEventsIterator(resultIter)
				_, err := eventsIter.Next()

				require.ErrorContains(t, err, "MY_ERROR")
			})
		})

		t.Run("only returns events for valid transactions", func(t *testing.T) {
			resultIter := &mocks.ResultsIterator{}
			resultIter.NextReturns(blockProto, nil)

			eventsIter := event.NewChaincodeEventsIterator(resultIter)
			actual, err := eventsIter.Next()

			require.NoError(t, err, "Next()")
			require.NotNil(t, actual, "events")

			expected := &gateway.ChaincodeEventsResponse{
				BlockNumber: blockProto.GetHeader().GetNumber(),
				Events: []*peer.ChaincodeEvent{
					chaincodeEvent,
				},
			}
			require.True(t, proto.Equal(expected, actual), "ChaincodeEventsResponse: %v", actual)
		})

		t.Run("skips blocks with no valid chaincode events", func(t *testing.T) {
			emptyBlock := &common.Block{
				Header: &common.BlockHeader{
					Number: 418,
				},
			}
			resultIter := &mocks.ResultsIterator{}
			resultIter.NextReturnsOnCall(0, emptyBlock, nil)
			resultIter.NextReturnsOnCall(1, blockProto, nil)

			eventsIter := event.NewChaincodeEventsIterator(resultIter)
			actual, err := eventsIter.Next()

			require.NoError(t, err, "Next()")
			require.NotNil(t, actual, "events")

			expected := &gateway.ChaincodeEventsResponse{
				BlockNumber: blockProto.GetHeader().GetNumber(),
				Events: []*peer.ChaincodeEvent{
					chaincodeEvent,
				},
			}
			require.True(t, proto.Equal(expected, actual), "ChaincodeEventsResponse: %v", actual)
		})

		t.Run("Close", func(t *testing.T) {
			t.Run("closes wrapped iterator", func(t *testing.T) {
				resultIter := &mocks.ResultsIterator{}

				eventsIter := event.NewChaincodeEventsIterator(resultIter)
				eventsIter.Close()

				require.Equal(t, 1, resultIter.CloseCallCount())
			})
		})
	})
}
