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
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/queryprovider.go --fake-name QueryProvider . queryProvider
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
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(peer.TxValidationCode_MVCC_READ_CONFLICT, 101, nil)
		finder := NewFinder(provider, newTestNotifier(nil))

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, provider.TransactionStatusCallCount())

		actual, _ := provider.TransactionStatusArgsForCall(0)
		require.Equal(t, "CHANNEL", actual)
	})

	t.Run("passes transaction ID to query provider", func(t *testing.T) {
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(peer.TxValidationCode_MVCC_READ_CONFLICT, 101, nil)
		finder := NewFinder(provider, newTestNotifier(nil))

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, provider.TransactionStatusCallCount())

		_, actual := provider.TransactionStatusArgsForCall(0)
		require.Equal(t, "TX_ID", actual)
	})

	t.Run("returns previously committed transaction status", func(t *testing.T) {
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(peer.TxValidationCode_MVCC_READ_CONFLICT, 101, nil)
		finder := NewFinder(provider, newTestNotifier(nil))

		actual, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")
		require.NoError(t, err)

		expected := &Status{
			Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			BlockNumber:   101,
			TransactionID: "TX_ID",
		}
		require.Equal(t, expected, actual)
	})

	t.Run("returns notified transaction status when no previous commit", func(t *testing.T) {
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(0, 0, errors.New("NOT_FOUND"))
		commitSend := make(chan *ledger.CommitNotification)
		finder := NewFinder(provider, newTestNotifier(commitSend))

		msg := &ledger.CommitNotification{
			BlockNumber: 101,
			TxsInfo: []*ledger.CommitNotificationTxInfo{
				{
					TxID:           "TX_ID",
					ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			},
		}
		defer close(sendUntilDone(commitSend, msg))

		actual, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")
		require.NoError(t, err)

		expected := &Status{
			Code:          peer.TxValidationCode_MVCC_READ_CONFLICT,
			BlockNumber:   101,
			TransactionID: "TX_ID",
		}
		require.Equal(t, expected, actual)
	})

	t.Run("returns error from notifier", func(t *testing.T) {
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(0, 0, errors.New("NOT_FOUND"))
		supplier := &mocks.NotificationSupplier{}
		supplier.CommitNotificationsReturns(nil, errors.New("MY_ERROR"))
		finder := NewFinder(provider, NewNotifier(supplier))

		_, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")
		require.ErrorContains(t, err, "MY_ERROR")
	})

	t.Run("passes channel name to supplier", func(t *testing.T) {
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(0, 0, errors.New("NOT_FOUND"))
		supplier := &mocks.NotificationSupplier{}
		supplier.CommitNotificationsReturns(nil, errors.New("MY_ERROR"))
		finder := NewFinder(provider, NewNotifier(supplier))

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, supplier.CommitNotificationsCallCount())

		_, actual := supplier.CommitNotificationsArgsForCall(0)
		require.Equal(t, "CHANNEL", actual)
	})

	t.Run("returns context error when context cancelled", func(t *testing.T) {
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(0, 0, errors.New("NOT_FOUND"))
		finder := NewFinder(provider, newTestNotifier(nil))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := finder.TransactionStatus(ctx, "CHANNEL", "TX_ID")

		require.Equal(t, context.Canceled, err)
	})

	t.Run("returns error when notification supplier fails", func(t *testing.T) {
		provider := &mocks.QueryProvider{}
		provider.TransactionStatusReturns(0, 0, errors.New("NOT_FOUND"))
		commitSend := make(chan *ledger.CommitNotification)
		close(commitSend)
		finder := NewFinder(provider, newTestNotifier(commitSend))

		_, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.ErrorContains(t, err, "unexpected close of commit notification channel")
	})
}
