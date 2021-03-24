/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit/mock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/queryprovider.go --fake-name QueryProvider . queryProvider
type queryProvider interface { // Mimic QueryProvider to avoid circular import with generated mock
	QueryProvider
}

func TestFinder(t *testing.T) {
	sendUntilDone := func(commitSend chan<- *ledger.CommitNotification, msg *ledger.CommitNotification) chan struct{} {
		done := make(chan struct{})

		go func() {
			for ; ; time.Sleep(10 * time.Millisecond) {
				select {
				case commitSend <- msg:
				case <-done:
					return
				}
			}
		}()

		return done
	}

	t.Run("passes channel name to query provider", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(peer.TxValidationCode_MVCC_READ_CONFLICT, nil)
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(newNotificationSupplier(nil)),
		}

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, provider.TransactionStatusCallCount(), "unexpected call count")
		actual, _ := provider.TransactionStatusArgsForCall(0)
		require.Equal(t, "CHANNEL", actual)
	})

	t.Run("passes transaction ID to query provider", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(peer.TxValidationCode_MVCC_READ_CONFLICT, nil)
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(newNotificationSupplier(nil)),
		}

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, provider.TransactionStatusCallCount(), "unexpected call count")
		_, actual := provider.TransactionStatusArgsForCall(0)
		require.Equal(t, "TX_ID", actual)
	})

	t.Run("returns previously committed transaction status", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(peer.TxValidationCode_MVCC_READ_CONFLICT, nil)
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(newNotificationSupplier(nil)),
		}

		actual, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.NoError(t, err)
		require.Equal(t, peer.TxValidationCode_MVCC_READ_CONFLICT, actual)
	})

	t.Run("returns notified transaction status when no previous commit", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(0, errors.New("NOT_FOUND"))
		commitSend := make(chan *ledger.CommitNotification)
		msg := &ledger.CommitNotification{
			BlockNumber: 1,
			TxIDValidationCodes: map[string]peer.TxValidationCode{
				"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
			},
		}
		defer close(sendUntilDone(commitSend, msg))
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(newNotificationSupplier(commitSend)),
		}

		actual, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.NoError(t, err)
		require.Equal(t, peer.TxValidationCode_MVCC_READ_CONFLICT, actual)
	})

	t.Run("returns error from notifier", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(0, errors.New("NOT_FOUND"))
		supplier := &mock.NotificationSupplier{}
		supplier.CommitNotificationsReturns(nil, errors.New("MY_ERROR"))
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(supplier),
		}

		_, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")
		require.ErrorContains(t, err, "MY_ERROR")
	})

	t.Run("passes channel name to supplier", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(0, errors.New("NOT_FOUND"))
		supplier := &mock.NotificationSupplier{}
		supplier.CommitNotificationsReturns(nil, errors.New("ERROR"))
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(supplier),
		}

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, supplier.CommitNotificationsCallCount(), "unexpected call count")
		_, actual := supplier.CommitNotificationsArgsForCall(0)
		require.Equal(t, "CHANNEL", actual)
	})

	t.Run("returns context error when context cancelled", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(0, errors.New("NOT_FOUND"))
		supplier := &mock.NotificationSupplier{}
		supplier.CommitNotificationsReturns(nil, nil)
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(supplier),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := finder.TransactionStatus(ctx, "CHANNEL", "TX_ID")

		require.Equal(t, context.Canceled, err)
	})

	t.Run("returns error when notification supplier fails", func(t *testing.T) {
		provider := &mock.QueryProvider{}
		provider.TransactionStatusReturns(0, errors.New("NOT_FOUND"))
		supplier := &mock.NotificationSupplier{}
		commitSend := make(chan *ledger.CommitNotification)
		close(commitSend)
		supplier.CommitNotificationsReturns(commitSend, nil)
		finder := &Finder{
			Query:    provider,
			Notifier: NewNotifier(supplier),
		}

		_, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.ErrorContains(t, err, "unexpected close of commit notification channel")
	})
}
