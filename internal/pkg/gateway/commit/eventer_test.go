/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestEventer(t *testing.T) {
	t.Run("realtime events", func(t *testing.T) {
		t.Run("returns error from notification supplier", func(t *testing.T) {
			supplier := &mocks.NotificationSupplier{}
			supplier.CommitNotificationsReturns(nil, errors.New("MY_ERROR"))
			notifier := NewNotifier(supplier)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			_, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")

			require.ErrorContains(t, err, "MY_ERROR")
		})

		t.Run("delivers events for matching chaincode", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			eventReceive, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)

			chaincodeEvent := newTestChaincodeEvent("CHAINCODE_NAME")
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
				},
			}
			actual := <-eventReceive

			expectedEvents := []*peer.ChaincodeEvent{
				chaincodeEvent,
			}
			require.Equal(t, uint64(1), actual.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual.Events)
		})

		t.Run("ignores events for non-matching chaincode", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			eventReceive, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)

			chaincodeEvent := newTestChaincodeEvent("CHAINCODE_NAME")
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
					{
						ChaincodeEventData: assertMarshallProto(t, newTestChaincodeEvent("WRONG")),
					},
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
				},
			}
			actual := <-eventReceive

			expectedEvents := []*peer.ChaincodeEvent{
				chaincodeEvent,
				chaincodeEvent,
			}
			require.Equal(t, uint64(1), actual.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual.Events)
		})

		t.Run("delivers events for multiple blocks", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			eventReceive, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)

			chaincodeEvent := newTestChaincodeEvent("CHAINCODE_NAME")
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
				},
			}

			expectedEvents := []*peer.ChaincodeEvent{
				chaincodeEvent,
			}

			actual1 := <-eventReceive
			require.Equal(t, uint64(1), actual1.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual1.Events)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
				},
			}
			actual2 := <-eventReceive

			require.Equal(t, uint64(2), actual2.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual2.Events)
		})

		t.Run("ignores blocks with no events", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			eventReceive, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)

			chaincodeEvent := newTestChaincodeEvent("CHAINCODE_NAME")
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
				},
			}

			actual := <-eventReceive

			expectedEvents := []*peer.ChaincodeEvent{
				chaincodeEvent,
			}
			require.Equal(t, uint64(2), actual.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual.Events)
		})

		t.Run("ignores blocks with no events matching chaincode name", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			eventReceive, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)

			chaincodeEvent := newTestChaincodeEvent("CHAINCODE_NAME")
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, newTestChaincodeEvent("WRONG")),
					},
				},
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
				},
			}

			actual := <-eventReceive

			expectedEvents := []*peer.ChaincodeEvent{
				chaincodeEvent,
			}
			require.Equal(t, uint64(2), actual.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual.Events)
		})

		t.Run("delivers events to multiple listeners", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			eventReceive1, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)
			eventReceive2, err := eventer.ChaincodeEvents(context.Background(), "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)

			chaincodeEvent := newTestChaincodeEvent("CHAINCODE_NAME")
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, chaincodeEvent),
					},
				},
			}
			actual1 := <-eventReceive1
			actual2 := <-eventReceive2

			expectedEvents := []*peer.ChaincodeEvent{
				chaincodeEvent,
			}
			require.Equal(t, uint64(1), actual1.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual1.Events)
			require.Equal(t, uint64(1), actual2.BlockNumber, "block number")
			assertEqualChaincodeEvents(t, expectedEvents, actual2.Events)
		})

		t.Run("stops listening when done channel is closed", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()
			eventer := &Eventer{
				Notifier: notifier,
			}

			ctx, cancel := context.WithCancel(context.Background())
			eventReceive, err := eventer.ChaincodeEvents(ctx, "CHANNEL_NAME", "CHAINCODE_NAME")
			require.NoError(t, err)

			cancel()
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						ChaincodeEventData: assertMarshallProto(t, newTestChaincodeEvent("CHAINCODE_NAME")),
					},
				},
			}
			_, ok := <-eventReceive

			require.False(t, ok, "Expected notification channel to be closed but receive was successful")
		})
	})
}
