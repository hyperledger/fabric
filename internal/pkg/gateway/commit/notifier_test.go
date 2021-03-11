package commit_test

import (
	"context"
	"testing"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/ledgerfactory.go --fake-name LedgerFactory . LedgerFactory
//go:generate counterfeiter -o mocks/ledgernotifier.go --fake-name LedgerNotifier . LedgerNotifier

func TestNotifier(t *testing.T) {
	newTestNotifier := func(commitSend <-chan *ledger.CommitNotification) *commit.Notifier {
		ledgerNotifier := &mocks.LedgerNotifier{}
		ledgerNotifier.CommitNotificationsChannelReturnsOnCall(0, commitSend, nil)
		ledgerNotifier.CommitNotificationsChannelReturns(nil, errors.New("unexpected call of CommitNotificationChannel"))

		ledgerFactory := &mocks.LedgerFactory{}
		ledgerFactory.LedgerReturns(ledgerNotifier, nil)

		return commit.NewNotifier(ledgerFactory)
	}

	t.Run("NewNotifier with nil ledger panics", func(t *testing.T) {
		f := func() {
			commit.NewNotifier(nil)
		}
		require.Panics(t, f)
	})

	t.Run("Notify", func(t *testing.T) {
		t.Run("returns error if channel does not exist", func(t *testing.T) {
			ledgerFactory := &mocks.LedgerFactory{}
			ledgerFactory.LedgerReturns(nil, errors.New("ERROR"))
			notifier := commit.NewNotifier(ledgerFactory)

			_, err := notifier.Notify(context.Background().Done(), "CHANNEL_NAME", "TX_ID")

			require.ErrorContains(t, err, "CHANNEL_NAME")
		})

		t.Run("returns error if ledger notification channel not available", func(t *testing.T) {
			ledgerNotifier := &mocks.LedgerNotifier{}
			ledgerNotifier.CommitNotificationsChannelReturns(nil, errors.New("ERROR"))

			ledgerFactory := &mocks.LedgerFactory{}
			ledgerFactory.LedgerReturns(ledgerNotifier, nil)

			notifier := commit.NewNotifier(ledgerFactory)

			_, err := notifier.Notify(context.Background().Done(), "CHANNEL_NAME", "TX_ID")

			require.Error(t, err)
		})

		t.Run("returns notifier on successful registration", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification)
			notifier := newTestNotifier(commitSend)

			commitReceive, err := notifier.Notify(context.Background().Done(), "CHANNEL_NAME", "TX_ID")
			require.NoError(t, err)
			require.NotNil(t, commitReceive)
		})

		t.Run("delivers notification for matching transaction", func(t *testing.T) {
			commitSend := make(chan *ledger.CommitNotification, 1)
			notifier := newTestNotifier(commitSend)

			commitReceive, _ := notifier.Notify(context.Background().Done(), "CHANNEL_NAME", "TX_ID")
			commitSend <- &ledger.CommitNotification{
				BlockNumber: 1,
				TxIDValidationCodes: map[string]peer.TxValidationCode{
					"TX_ID": peer.TxValidationCode_MVCC_READ_CONFLICT,
				},
			}
			actual := <-commitReceive

			expected := commit.Notification{
				BlockNumber:    1,
				TransactionID:  "TX_ID",
				ValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			}
			require.EqualValues(t, expected, actual)
		})
	})
}
