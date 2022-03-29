/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newLedgerMocks() (*mocks.Provider, *mocks.Ledger) {
	ledger := &mocks.Ledger{}

	provider := &mocks.Provider{}
	provider.LedgerReturns(ledger, nil)

	return provider, ledger
}

func newNotifierProvider(commitSends ...<-chan *ledger.CommitNotification) *mocks.Provider {
	provider, ledger := newLedgerMocks()
	for i, commitSend := range commitSends {
		ledger.CommitNotificationsChannelReturnsOnCall(i, commitSend, nil)
	}

	return provider
}

func newTestNotifier(commitSends ...<-chan *ledger.CommitNotification) *Notifier {
	supplier := newNotifierProvider(commitSends...)
	return NewNotifier(supplier)
}

func assertMarshallProto(t *testing.T, message proto.Message) []byte {
	result, err := proto.Marshal(message)
	require.NoError(t, err)
	return result
}

func assertEqualChaincodeEvents(t *testing.T, expected []*peer.ChaincodeEvent, actual []*peer.ChaincodeEvent) {
	require.Equal(t, len(expected), len(actual), "number of events")
	for i, event := range actual {
		require.Truef(t, proto.Equal(expected[i], event), "expected %v, got %v", expected, actual)
	}
}

func newTestChaincodeEvent(chaincodeName string) *peer.ChaincodeEvent {
	return &peer.ChaincodeEvent{
		ChaincodeId: chaincodeName,
		EventName:   "EVENT_NAME",
		TxId:        "TX_ID",
		Payload:     []byte("PAYLOAD"),
	}
}

func TestNotifier(t *testing.T) {
	t.Run("notifyStatus", func(t *testing.T) {
		t.Run("returns error from notification supplier", func(t *testing.T) {
			provider, ledger := newLedgerMocks()
			ledger.CommitNotificationsChannelReturns(nil, errors.New("MY_ERROR"))
			notifier := NewNotifier(provider)
			defer notifier.close()

			_, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")

			require.ErrorContains(t, err, "MY_ERROR")
		})

		t.Run("delivers notification for matching transaction", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			actual := <-commitReceive

			expected := &Status{
				BlockNumber:   1,
				TransactionID: "TX_ID",
				Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("ignores non-matching transaction in same block", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "WRONG_TX_ID",
						ValidationCode: peer.TxValidationCode_VALID,
					},
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			actual := <-commitReceive

			expected := &Status{
				BlockNumber:   1,
				TransactionID: "TX_ID",
				Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("ignores blocks without matching transaction", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "WRONG_TX_ID",
						ValidationCode: peer.TxValidationCode_VALID,
					},
				},
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			actual := <-commitReceive

			expected := &Status{
				BlockNumber:   2,
				TransactionID: "TX_ID",
				Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("processes blocks in order", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			actual := <-commitReceive

			expected := &Status{
				BlockNumber:   1,
				TransactionID: "TX_ID",
				Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("closes channel after notification", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_VALID,
					},
				},
			}
			<-commitReceive
			_, ok := <-commitReceive

			require.False(t, ok, "Expected notification channel to be closed but receive was successful")
		})

		t.Run("stops notification when done channel closed", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			done := make(chan struct{})
			commitReceive, err := notifier.notifyStatus(done, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(done)
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			_, ok := <-commitReceive

			require.False(t, ok, "Expected notification channel to be closed but receive was successful")
		})

		t.Run("multiple listeners receive notifications", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive1, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitReceive2, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			actual1 := <-commitReceive1
			actual2 := <-commitReceive2

			expected := &Status{
				BlockNumber:   1,
				TransactionID: "TX_ID",
				Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual1)
			require.EqualValues(t, expected, actual2)
		})

		t.Run("multiple listeners can stop listening independently", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			done := make(chan struct{})
			commitReceive1, err := notifier.notifyStatus(done, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitReceive2, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(done)
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}
			_, ok1 := <-commitReceive1
			_, ok2 := <-commitReceive2

			require.False(t, ok1, "Expected notification channel to be closed but receive was successful")
			require.True(t, ok2, "Expected notification channel to deliver a result but was closed")
		})

		t.Run("passes open done channel to notification supplier", func(t *testing.T) {
			provider, ledger := newLedgerMocks()
			ledger.CommitNotificationsChannelReturns(nil, nil)

			notifier := NewNotifier(provider)
			defer notifier.close()

			_, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			require.Equal(t, 1, ledger.CommitNotificationsChannelCallCount())

			done := ledger.CommitNotificationsChannelArgsForCall(0)
			select {
			case <-done:
				require.FailNow(t, "Expected done channel to be open but was closed")
			default:
			}
		})

		t.Run("passes channel name to notification supplier", func(t *testing.T) {
			provider, _ := newLedgerMocks()

			notifier := NewNotifier(provider)
			defer notifier.close()

			_, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			require.Equal(t, 1, provider.LedgerCallCount())

			actual := provider.LedgerArgsForCall(0)
			require.Equal(t, "CHANNEL_NAME", actual)
		})

		t.Run("stops notification if supplier stops", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(commitSend)
			_, ok := <-commitReceive

			require.False(t, ok, "Expected notification channel to be closed but receive was successful")
		})

		t.Run("can attach new listener after supplier stops", func(t *testing.T) {
			commitSend1 := make(chan *ledger.CommitNotification, 1)
			commitSend2 := make(chan *ledger.CommitNotification, 1)

			notifier := newTestNotifier(commitSend1, commitSend2)
			defer notifier.close()

			commitReceive1, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(commitSend1)
			_, ok := <-commitReceive1
			require.False(t, ok, "Expected notification channel to be closed but receive was successful")

			commitReceive2, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend2 <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxsInfo: []*ledger.CommitNotificationTxInfo{
					{
						TxID:           "TX_ID",
						ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
					},
				},
			}

			actual, ok := <-commitReceive2
			require.True(t, ok, "Expected notification channel to deliver a result but was closed")

			expected := &Status{
				BlockNumber:   1,
				TransactionID: "TX_ID",
				Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})
	})

	t.Run("Close", func(t *testing.T) {
		t.Run("stops all listeners", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification)
			notifier := newTestNotifier(commitSend)

			commitReceive, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)
			notifier.close()

			_, ok := <-commitReceive

			require.False(t, ok, "Expected notification channel to be closed but receive was successful")
		})

		t.Run("idempotent", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification)
			notifier := newTestNotifier(commitSend)
			notifier.close()

			require.NotPanics(t, func() {
				notifier.close()
			})
		})

		t.Run("stops notification supplier", func(t *testing.T) {
			provider, ledger := newLedgerMocks()

			notifier := NewNotifier(provider)

			_, err := notifier.notifyStatus(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)
			notifier.close()

			require.Equal(t, 1, ledger.CommitNotificationsChannelCallCount())

			done := ledger.CommitNotificationsChannelArgsForCall(0)
			_, ok := <-done
			require.False(t, ok, "Expected notification supplier done channel to be closed but receive was successful")
		})
	})
}
