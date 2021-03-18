/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit/mock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/notificationsupplier.go --fake-name NotificationSupplier . notificationSupplier
type notificationSupplier interface { // Mimic NotificationSupplier to avoid circular import with generated mock
	NotificationSupplier
}

func newNotificationSupplier(commitSend <-chan *ledger.CommitNotification) *mock.NotificationSupplier {
	supplier := &mock.NotificationSupplier{}
	supplier.CommitNotificationsReturnsOnCall(0, commitSend, nil)
	supplier.CommitNotificationsReturns(nil, errors.New("unexpected call of CommitNotificationChannel"))
	return supplier
}

func TestNotifier(t *testing.T) {
	newTestNotifier := func(commitSend <-chan *ledger.CommitNotification) *Notifier {
		supplier := newNotificationSupplier(commitSend)
		return NewNotifier(supplier)
	}

	t.Run("Notify", func(t *testing.T) {
		t.Run("returns error from notification supplier", func(t *testing.T) {
			notificationSupplier := &mock.NotificationSupplier{}
			notificationSupplier.CommitNotificationsReturns(nil, errors.New("MY_ERROR"))
			notifier := NewNotifier(notificationSupplier)
			defer notifier.close()

			_, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")

			require.ErrorContains(t, err, "MY_ERROR")
		})

		t.Run("delivers notification for matching transaction", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			actual := <-commitReceive

			expected := notification{
				BlockNumber:    1,
				TransactionID:  "TX_ID",
				ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("ignores non-matching transaction in same block", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"WRONG_TX_ID": peer.TxValidationCode_VALID,
					"TX_ID":       peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			actual := <-commitReceive

			expected := notification{
				BlockNumber:    1,
				TransactionID:  "TX_ID",
				ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("ignores blocks without matching transaction", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"WRONG_TX_ID": peer.TxValidationCode_VALID,
				},
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			actual := <-commitReceive

			expected := notification{
				BlockNumber:    2,
				TransactionID:  "TX_ID",
				ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("processes blocks in order", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			actual := <-commitReceive

			expected := notification{
				BlockNumber:    1,
				TransactionID:  "TX_ID",
				ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})

		t.Run("closes channel after notification", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 2)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 2,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_VALID,
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
			commitReceive, err := notifier.notify(done, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(done)
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			_, ok := <-commitReceive

			require.False(t, ok, "Expected notification channel to be closed but receive was successful")
		})

		t.Run("multiple listeners receive notifications", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive1, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitReceive2, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			actual1 := <-commitReceive1
			actual2 := <-commitReceive2

			expected := notification{
				BlockNumber:    1,
				TransactionID:  "TX_ID",
				ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual1)
			require.EqualValues(t, expected, actual2)
		})

		t.Run("multiple listeners can stop listening independently", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			done := make(chan struct{})
			commitReceive1, err := notifier.notify(done, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitReceive2, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(done)
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			_, ok1 := <-commitReceive1
			_, ok2 := <-commitReceive2

			require.False(t, ok1, "Expected notification channel to be closed but receive was successful")
			require.True(t, ok2, "Expected notification channel to deliver a result but was closed")
		})

		t.Run("passes open done channel to notification supplier", func(t *testing.T) {
			notificationSupplier := &mock.NotificationSupplier{}
			notificationSupplier.CommitNotificationsReturns(nil, nil)
			notifier := NewNotifier(notificationSupplier)
			defer notifier.close()

			_, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			require.Equal(t, 1, notificationSupplier.CommitNotificationsCallCount(), "Unexpected call count")
			done, _ := notificationSupplier.CommitNotificationsArgsForCall(0)
			select {
			case <-done:
				require.FailNow(t, "Expected done channel to be open but was closed")
			default:
			}
		})

		t.Run("passes channel name to notification supplier", func(t *testing.T) {
			notificationSupplier := &mock.NotificationSupplier{}
			notificationSupplier.CommitNotificationsReturns(nil, nil)
			notifier := NewNotifier(notificationSupplier)
			defer notifier.close()

			_, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			require.Equal(t, 1, notificationSupplier.CommitNotificationsCallCount(), "Unexpected call count")
			_, actual := notificationSupplier.CommitNotificationsArgsForCall(0)
			require.Equal(t, "CHANNEL_NAME", actual)
		})

		t.Run("stops notification if supplier stops", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)
			defer notifier.close()

			commitReceive, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(commitSend)
			_, ok := <-commitReceive

			require.False(t, ok, "Expected notification channel to be closed but receive was successful")
		})

		t.Run("can attach new listener after supplier stops", func(t *testing.T) {
			commitSend1 := make(chan *ledger.CommitNotification, 1)
			commitSend2 := make(chan *ledger.CommitNotification, 1)
			notificationSupplier := &mock.NotificationSupplier{}
			notificationSupplier.CommitNotificationsReturnsOnCall(0, commitSend1, nil)
			notificationSupplier.CommitNotificationsReturnsOnCall(1, commitSend2, nil)
			notifier := NewNotifier(notificationSupplier)
			defer notifier.close()

			commitReceive1, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			close(commitSend1)
			_, ok := <-commitReceive1
			require.False(t, ok, "Expected notification channel to be closed but receive was successful")

			commitReceive2, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)

			commitSend2 <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}

			actual, ok := <-commitReceive2
			require.True(t, ok, "Expected notification channel to deliver a result but was closed")

			expected := notification{
				BlockNumber:    1,
				TransactionID:  "TX_ID",
				ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})
	})

	t.Run("Close", func(t *testing.T) {
		t.Run("stops all listeners", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification)
			notifier := newTestNotifier(commitSend)

			commitReceive, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
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
			notificationSupplier := &mock.NotificationSupplier{}
			notificationSupplier.CommitNotificationsReturns(nil, nil)
			notifier := NewNotifier(notificationSupplier)

			_, err := notifier.notify(nil, "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)
			notifier.close()

			require.Equal(t, 1, notificationSupplier.CommitNotificationsCallCount(), "Unexpected call count")
			done, _ := notificationSupplier.CommitNotificationsArgsForCall(0)
			_, ok := <-done
			require.False(t, ok, "Expected notification supplier done channel to be closed but receive was successful")
		})
	})
}
