/*
Copyright 2021 IBM All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gateway

import (
	"io"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	cp "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/gateway"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestChaincodeEvents(t *testing.T) {
	now := time.Now()
	lastTransactionID := "LAST_TX_ID"

	newChaincodeEvent := func(chaincodeName string, transactionID string) *peer.ChaincodeEvent {
		return &peer.ChaincodeEvent{
			ChaincodeId: chaincodeName,
			TxId:        transactionID,
			EventName:   "EVENT_NAME",
			Payload:     []byte("PAYLOAD"),
		}
	}

	newTransactionHeader := func(transactionID string) *cp.Header {
		return &cp.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&cp.ChannelHeader{
				Type: int32(cp.HeaderType_ENDORSER_TRANSACTION),
				Timestamp: &timestamp.Timestamp{
					Seconds: now.Unix(),
					Nanos:   int32(now.Nanosecond()),
				},
				TxId: transactionID,
			}),
		}
	}

	newTransactionEnvelope := func(event *peer.ChaincodeEvent) *cp.Envelope {
		return &cp.Envelope{
			Payload: protoutil.MarshalOrPanic(&cp.Payload{
				Header: newTransactionHeader(event.GetTxId()),
				Data: protoutil.MarshalOrPanic(&peer.Transaction{
					Actions: []*peer.TransactionAction{
						{
							Payload: protoutil.MarshalOrPanic(&peer.ChaincodeActionPayload{
								Action: &peer.ChaincodeEndorsedAction{
									ProposalResponsePayload: protoutil.MarshalOrPanic(&peer.ProposalResponsePayload{
										Extension: protoutil.MarshalOrPanic(&peer.ChaincodeAction{
											Events: protoutil.MarshalOrPanic(event),
										}),
									}),
								},
							}),
						},
					},
				}),
			}),
		}
	}

	newBlock := func(number uint64) *cp.Block {
		return &cp.Block{
			Header: &cp.BlockHeader{
				Number: number,
			},
			Metadata: &cp.BlockMetadata{
				Metadata: make([][]byte, 5),
			},
			Data: &cp.BlockData{
				Data: [][]byte{},
			},
		}
	}

	addTransaction := func(block *cp.Block, transaction *cp.Envelope, status peer.TxValidationCode) {
		metadata := block.GetMetadata().GetMetadata()
		metadata[cp.BlockMetadataIndex_TRANSACTIONS_FILTER] = append(metadata[cp.BlockMetadataIndex_TRANSACTIONS_FILTER], byte(status))

		blockData := block.GetData()
		blockData.Data = append(blockData.Data, protoutil.MarshalOrPanic(transaction))
	}

	matchEvent := newChaincodeEvent(testChaincode, "EXPECTED_TX_ID")
	wrongChaincodeEvent := newChaincodeEvent("WRONG_CHAINCODE", "WRONG__TX_ID")
	oldTransactionEvent := newChaincodeEvent(testChaincode, "OLD_TX_ID")
	lastTransactionEvent := newChaincodeEvent(testChaincode, lastTransactionID)
	lastTransactionWrongChaincodeEvent := newChaincodeEvent("WRONG_CHAINCODE", lastTransactionID)

	configTxEnvelope := &cp.Envelope{
		Payload: protoutil.MarshalOrPanic(&cp.Payload{
			Header: &cp.Header{
				ChannelHeader: protoutil.MarshalOrPanic(&cp.ChannelHeader{
					Type: int32(cp.HeaderType_CONFIG_UPDATE),
				}),
			},
		}),
	}

	noMatchingEventsBlock := newBlock(100)
	addTransaction(noMatchingEventsBlock, newTransactionEnvelope(wrongChaincodeEvent), peer.TxValidationCode_VALID)

	matchingEventBlock := newBlock(101)
	addTransaction(matchingEventBlock, configTxEnvelope, peer.TxValidationCode_VALID)
	addTransaction(matchingEventBlock, newTransactionEnvelope(wrongChaincodeEvent), peer.TxValidationCode_VALID)
	addTransaction(matchingEventBlock, newTransactionEnvelope(matchEvent), peer.TxValidationCode_VALID)

	partReadBlock := newBlock(200)
	addTransaction(partReadBlock, newTransactionEnvelope(oldTransactionEvent), peer.TxValidationCode_VALID)
	addTransaction(partReadBlock, newTransactionEnvelope(lastTransactionEvent), peer.TxValidationCode_VALID)
	addTransaction(partReadBlock, newTransactionEnvelope(matchEvent), peer.TxValidationCode_VALID)

	differentChaincodePartReadBlock := newBlock(300)
	addTransaction(differentChaincodePartReadBlock, newTransactionEnvelope(oldTransactionEvent), peer.TxValidationCode_VALID)
	addTransaction(differentChaincodePartReadBlock, newTransactionEnvelope(lastTransactionWrongChaincodeEvent), peer.TxValidationCode_VALID)
	addTransaction(differentChaincodePartReadBlock, newTransactionEnvelope(matchEvent), peer.TxValidationCode_VALID)

	tests := []testDef{
		{
			name:      "error reading events",
			eventErr:  errors.New("EVENT_ERROR"),
			errCode:   codes.Aborted,
			errString: "EVENT_ERROR",
		},
		{
			name: "returns chaincode events",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: matchingEventBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "skips blocks containing only non-matching chaincode events",
			blocks: []*cp.Block{
				noMatchingEventsBlock,
				matchingEventBlock,
			},
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: matchingEventBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "skips previously seen transactions",
			blocks: []*cp.Block{
				partReadBlock,
			},
			afterTxID: lastTransactionID,
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: partReadBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "identifies specified transaction if from different chaincode",
			blocks: []*cp.Block{
				differentChaincodePartReadBlock,
			},
			afterTxID: lastTransactionID,
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: differentChaincodePartReadBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "identifies specified transaction if not in first read block",
			blocks: []*cp.Block{
				noMatchingEventsBlock,
				partReadBlock,
			},
			afterTxID: lastTransactionID,
			expectedResponses: []proto.Message{
				&pb.ChaincodeEventsResponse{
					BlockNumber: partReadBlock.GetHeader().GetNumber(),
					Events: []*peer.ChaincodeEvent{
						{
							ChaincodeId: testChaincode,
							TxId:        matchEvent.GetTxId(),
							EventName:   matchEvent.GetEventName(),
							Payload:     matchEvent.GetPayload(),
						},
					},
				},
			},
		},
		{
			name: "passes channel name to ledger provider",
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledgerProvider.LedgerCallCount())
				require.Equal(t, testChannel, test.ledgerProvider.LedgerArgsForCall(0))
			},
		},
		{
			name: "returns error obtaining ledger",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.NotFound,
			errString: "LEDGER_PROVIDER_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.ledgerProvider.LedgerReturns(nil, errors.New("LEDGER_PROVIDER_ERROR"))
			},
		},
		{
			name: "returns error obtaining ledger height",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.Aborted,
			errString: "LEDGER_INFO_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.ledger.GetBlockchainInfoReturns(nil, errors.New("LEDGER_INFO_ERROR"))
			},
		},
		{
			name: "uses block height as start block if next commit is specified as start position",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			localLedgerHeight: 101,
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_NextCommit{
					NextCommit: &ab.SeekNextCommit{},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 101, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "uses specified start block",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			localLedgerHeight: 101,
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{
						Number: 99,
					},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 99, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "defaults to next commit if start position not specified",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			localLedgerHeight: 101,
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 101, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "uses block containing specified transaction instead of start block",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			localLedgerHeight: 101,
			postSetup: func(t *testing.T, test *preparedTest) {
				block := &cp.Block{
					Header: &cp.BlockHeader{
						Number: 99,
					},
				}
				test.ledger.GetBlockByTxIDReturns(block, nil)
			},
			afterTxID: "TX_ID",
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{
						Number: 1,
					},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 99, test.ledger.GetBlocksIteratorArgsForCall(0))
				require.Equal(t, 1, test.ledger.GetBlockByTxIDCallCount())
				require.Equal(t, "TX_ID", test.ledger.GetBlockByTxIDArgsForCall(0))
			},
		},
		{
			name: "uses start block if specified transaction not found",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			localLedgerHeight: 101,
			postSetup: func(t *testing.T, test *preparedTest) {
				test.ledger.GetBlockByTxIDReturns(nil, errors.New("NOT_FOUND"))
			},
			afterTxID: "TX_ID",
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Specified{
					Specified: &ab.SeekSpecified{
						Number: 1,
					},
				},
			},
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.ledger.GetBlocksIteratorCallCount())
				require.EqualValues(t, 1, test.ledger.GetBlocksIteratorArgsForCall(0))
			},
		},
		{
			name: "returns error for unsupported start position type",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			startPosition: &ab.SeekPosition{
				Type: &ab.SeekPosition_Oldest{
					Oldest: &ab.SeekOldest{},
				},
			},
			errCode:   codes.InvalidArgument,
			errString: "invalid start position type: *orderer.SeekPosition_Oldest",
		},
		{
			name: "returns error obtaining ledger iterator",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.Aborted,
			errString: "LEDGER_ITERATOR_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.ledger.GetBlocksIteratorReturns(nil, errors.New("LEDGER_ITERATOR_ERROR"))
			},
		},
		{
			name: "returns canceled status error when client closes stream",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode: codes.Canceled,
			postSetup: func(t *testing.T, test *preparedTest) {
				test.eventsServer.SendReturns(io.EOF)
			},
		},
		{
			name: "returns status error from send to client",
			blocks: []*cp.Block{
				matchingEventBlock,
			},
			errCode:   codes.Aborted,
			errString: "SEND_ERROR",
			postSetup: func(t *testing.T, test *preparedTest) {
				test.eventsServer.SendReturns(status.Error(codes.Aborted, "SEND_ERROR"))
			},
		},
		{
			name:      "failed policy or signature check",
			policyErr: errors.New("POLICY_ERROR"),
			errCode:   codes.PermissionDenied,
			errString: "POLICY_ERROR",
		},
		{
			name: "passes channel name to policy checker",
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.policy.CheckACLCallCount())
				_, channelName, _ := test.policy.CheckACLArgsForCall(0)
				require.Equal(t, testChannel, channelName)
			},
		},
		{
			name:     "passes identity to policy checker",
			identity: []byte("IDENTITY"),
			postTest: func(t *testing.T, test *preparedTest) {
				require.Equal(t, 1, test.policy.CheckACLCallCount())
				_, _, data := test.policy.CheckACLArgsForCall(0)
				require.IsType(t, &protoutil.SignedData{}, data)
				signedData := data.(*protoutil.SignedData)
				require.Equal(t, []byte("IDENTITY"), signedData.Identity)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := prepareTest(t, &tt)

			request := &pb.ChaincodeEventsRequest{
				ChannelId:   testChannel,
				Identity:    tt.identity,
				ChaincodeId: testChaincode,
			}
			if tt.startPosition != nil {
				request.StartPosition = tt.startPosition
			}
			if len(tt.afterTxID) > 0 {
				request.AfterTransactionId = tt.afterTxID
			}
			requestBytes, err := proto.Marshal(request)
			require.NoError(t, err)

			signedRequest := &pb.SignedChaincodeEventsRequest{
				Request:   requestBytes,
				Signature: []byte{},
			}

			err = test.server.ChaincodeEvents(signedRequest, test.eventsServer)

			if checkError(t, &tt, err) {
				return
			}

			for i, expectedResponse := range tt.expectedResponses {
				actualResponse := test.eventsServer.SendArgsForCall(i)
				require.True(t, proto.Equal(expectedResponse, actualResponse), "response[%d] mismatch: %v", i, actualResponse)
			}

			if tt.postTest != nil {
				tt.postTest(t, test)
			}
		})
	}
}
