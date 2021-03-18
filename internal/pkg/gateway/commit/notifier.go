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

// notification of a specific transaction commit.
type notification struct {
	BlockNumber    uint64
	TransactionID  string
	ValidationCode peer.TxValidationCode
}

func NewNotifier(supplier NotificationSupplier) *Notifier {
	return &Notifier{
		supplier:         supplier,
		channelNotifiers: make(map[string]*channelLevelNotifier),
		cancel:           make(chan struct{}),
	}
}

// notify the caller when the named transaction commits on the named channel. The caller is only notified of commits
// occurring after registering for notifications.
func (n *Notifier) notify(done <-chan struct{}, channelName string, transactionID string) (<-chan notification, error) {
	channelNotifier, err := n.channelNotifier(channelName)
	if err != nil {
		return nil, err
	}

	notifyChannel := channelNotifier.registerListener(done, transactionID)
	return notifyChannel, nil
}

// close the notifier. This closes all notification channels obtained from this notifier. Behavior is undefined after
// closing and the notifier should not be used.
func (n *Notifier) close() {
	n.once.Do(func() {
		close(n.cancel)
	})
}

func (n *Notifier) channelNotifier(channelName string) (*channelLevelNotifier, error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	result := n.channelNotifiers[channelName]
	if result != nil && !result.isClosed() {
		return result, nil
	}

	commitChannel, err := n.supplier.CommitNotifications(n.cancel, channelName)
	if err != nil {
		return nil, err
	}

	result = newChannelNotifier(n.cancel, commitChannel)
	n.channelNotifiers[channelName] = result

	return result, nil
}
