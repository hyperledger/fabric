/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"sync"

	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
)

// NotificationSupplier obtains a commit notification channel for a specific ledger. It provides an abstraction of the use of
// Peer, Channel and Ledger to obtain this result, and allows mocking in unit tests.
type NotificationSupplier interface {
	CommitNotifications(done <-chan struct{}, channelName string) (<-chan *ledger.CommitNotification, error)
}

// Notifier provides notification of transaction commits.
type Notifier struct {
	supplier         NotificationSupplier
	lock             sync.Mutex
	channelNotifiers map[string]*channelLevelNotifier
	cancel           chan struct{}
	once             sync.Once
}

// Notification of a specific transaction commit.
type Notification struct {
	BlockNumber    uint64
	TransactionID  string
	ValidationCode peer.TxValidationCode
}

// NewNotifier constructor.
func NewNotifier(supplier NotificationSupplier) *Notifier {
	return &Notifier{
		supplier:         supplier,
		channelNotifiers: make(map[string]*channelLevelNotifier),
		cancel:           make(chan struct{}),
	}
}

// Notify the caller when the named transaction commits on the named channel. The caller is only notified of commits
// occurring after registering for notifications.
func (notifier *Notifier) Notify(done <-chan struct{}, channelName string, transactionID string) (<-chan Notification, error) {
	channelNotifier, err := notifier.channelNotifier(channelName)
	if err != nil {
		return nil, err
	}

	notifyChannel := channelNotifier.registerListener(done, transactionID)
	return notifyChannel, nil
}

// Close the notifier. This closes all notification channels obtained from this notifier. Behaviour is undefined after
// closing and the notifier should not be used.
func (notifier *Notifier) Close() {
	notifier.once.Do(func() {
		close(notifier.cancel)
	})
}

func (notifier *Notifier) channelNotifier(channelName string) (*channelLevelNotifier, error) {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	result := notifier.channelNotifiers[channelName]
	if result != nil && !result.isClosed() {
		return result, nil
	}

	commitChannel, err := notifier.supplier.CommitNotifications(notifier.cancel, channelName)
	if err != nil {
		return nil, err
	}

	result = newChannelNotifier(notifier.cancel, commitChannel)
	notifier.channelNotifiers[channelName] = result

	return result, nil
}
