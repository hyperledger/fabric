/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"sync"
	"testing"

	"github.com/hyperledger/fabric/core/ledger"
	"github.com/hyperledger/fabric/internal/pkg/gateway/commit/mocks"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/blocklistener.go --fake-name BlockListener . blockListener

func TestBlockNotifier(t *testing.T) {
	t.Run("delivers events to listeners", func(t *testing.T) {
		commitSend := make(chan *ledger.CommitNotification, 1)
		listener := &mocks.BlockListener{}
		var wait sync.WaitGroup
		wait.Add(1)
		listener.ReceiveBlockCalls(func(event *ledger.CommitNotification) {
			wait.Done()
		})
		notifier := newBlockNotifier(nil, commitSend, listener)
		defer notifier.close()

		commitSend <- &ledger.CommitNotification{
			BlockNumber: 1,
		}

		wait.Wait()

		require.Equal(t, listener.ReceiveBlockArgsForCall(0).BlockNumber, uint64(1))
	})

	t.Run("closes listeners on failure to read event", func(t *testing.T) {
		commitSend := make(chan *ledger.CommitNotification)
		close(commitSend)
		listener := &mocks.BlockListener{}
		var wait sync.WaitGroup
		wait.Add(1)
		listener.CloseCalls(func() {
			wait.Done()
		})
		newBlockNotifier(nil, commitSend, listener)

		wait.Wait()

		require.Equal(t, listener.CloseCallCount(), 1)
	})

	t.Run("close is idempotent", func(t *testing.T) {
		listener := &mocks.BlockListener{}
		notifier := newBlockNotifier(nil, nil, listener)

		notifier.close()
		notifier.close()

		require.Equal(t, listener.CloseCallCount(), 1)
	})
}
