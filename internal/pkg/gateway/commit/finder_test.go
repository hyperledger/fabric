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
	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

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

	newMocks := func(code peer.TxValidationCode, blockNumber uint64, err error) (*mocks.Provider, *mocks.Ledger) {
		provider, ledger := newLedgerMocks()
		ledger.GetTxValidationCodeByTxIDReturns(code, blockNumber, err)

		return provider, ledger
	}

	t.Run("passes channel name to provider", func(t *testing.T) {
		provider, _ := newMocks(peer.TxValidationCode_MVCC_READ_CONFLICT, 101, nil)
		finder := NewFinder(provider, newTestNotifier(nil))

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, provider.LedgerCallCount())

		actual := provider.LedgerArgsForCall(0)
		require.Equal(t, "CHANNEL", actual)
	})

	t.Run("passes transaction ID to ledger", func(t *testing.T) {
		provider, ledger := newMocks(peer.TxValidationCode_MVCC_READ_CONFLICT, 101, nil)
		finder := NewFinder(provider, newTestNotifier(nil))

		finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.Equal(t, 1, ledger.GetTxValidationCodeByTxIDCallCount())

		actual := ledger.GetTxValidationCodeByTxIDArgsForCall(0)
		require.Equal(t, "TX_ID", actual)
	})

	t.Run("returns previously committed transaction status", func(t *testing.T) {
		provider, _ := newMocks(peer.TxValidationCode_MVCC_READ_CONFLICT, 101, nil)
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
		provider, _ := newMocks(peer.TxValidationCode_MVCC_READ_CONFLICT, 101, nil)
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
		provider, ledger := newMocks(0, 0, errors.New("NOT_FOUND"))
		ledger.CommitNotificationsChannelReturns(nil, errors.New("MY_ERROR"))
		finder := NewFinder(provider, NewNotifier(provider))

		_, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")
		require.ErrorContains(t, err, "MY_ERROR")
	})

	t.Run("returns context error when context cancelled", func(t *testing.T) {
		provider, _ := newMocks(0, 0, errors.New("NOT_FOUND"))
		finder := NewFinder(provider, newTestNotifier(nil))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := finder.TransactionStatus(ctx, "CHANNEL", "TX_ID")

		require.Equal(t, context.Canceled, err)
	})

	t.Run("returns error when notification supplier fails", func(t *testing.T) {
		provider, _ := newMocks(0, 0, errors.New("NOT_FOUND"))
		commitSend := make(chan *ledger.CommitNotification)
		close(commitSend)
		finder := NewFinder(provider, newTestNotifier(commitSend))

		_, err := finder.TransactionStatus(context.Background(), "CHANNEL", "TX_ID")

		require.ErrorContains(t, err, "unexpected close of commit notification channel")
	})
}
