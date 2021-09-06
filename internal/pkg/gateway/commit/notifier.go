/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"sync"

	"github.com/hyperledger/fabric/core/ledger"
)

// NotificationSupplier obtains a commit notification channel for a specific ledger. It provides an abstraction of the use of
// Peer, Channel and Ledger to obtain this result, and allows mocking in unit tests.
type NotificationSupplier interface {
	CommitNotifications(done <-chan struct{}, channelName string) (<-chan *ledger.CommitNotification, error)
}

type notifiers struct {
	block  *blockNotifier
	status *statusNotifier
}

// Notifier provides notification of transaction commits.
type Notifier struct {
	supplier           NotificationSupplier
	lock               sync.Mutex
	notifiersByChannel map[string]*notifiers
	cancel             chan struct{}
	once               sync.Once
}

func NewNotifier(supplier NotificationSupplier) *Notifier {
	return &Notifier{
		supplier:           supplier,
		notifiersByChannel: make(map[string]*notifiers),
		cancel:             make(chan struct{}),
	}
}

// notifyStatus notifies the caller when the named transaction commits on the named channel. The caller is only notified
// of commits occurring after registering for notifications.
func (n *Notifier) notifyStatus(done <-chan struct{}, channelName string, transactionID string) (<-chan *Status, error) {
	notifiers, err := n.notifiersForChannel(channelName)
	if err != nil {
		return nil, err
	}

	notifyChannel := notifiers.status.registerListener(done, transactionID)
	return notifyChannel, nil
}

// close the notifier. This closes all notification channels obtained from this notifier. Behavior is undefined after
// closing and the notifier should not be used.
func (n *Notifier) close() {
	n.once.Do(func() {
		close(n.cancel)
	})
}

func (n *Notifier) notifiersForChannel(channelName string) (*notifiers, error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	result := n.notifiersByChannel[channelName]
	if result != nil && !result.block.isClosed() {
		return result, nil
	}

	commitChannel, err := n.supplier.CommitNotifications(n.cancel, channelName)
	if err != nil {
		return nil, err
	}

	statusNotifier := newStatusNotifier()
	blockNotifier := newBlockNotifier(n.cancel, commitChannel, statusNotifier)
	result = &notifiers{
		block:  blockNotifier,
		status: statusNotifier,
	}
	n.notifiersByChannel[channelName] = result

	return result, nil
}
