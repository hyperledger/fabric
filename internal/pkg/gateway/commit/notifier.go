/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"sync"

	"github.com/hyperledger/fabric/internal/pkg/gateway/ledger"
)

type notifiers struct {
	block  *blockNotifier
	status *statusNotifier
}

// Notifier provides notification of transaction commits.
type Notifier struct {
	provider           ledger.Provider
	lock               sync.Mutex
	notifiersByChannel map[string]*notifiers
	cancel             chan struct{}
	once               sync.Once
}

func NewNotifier(provider ledger.Provider) *Notifier {
	return &Notifier{
		provider:           provider,
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

	ledger, err := n.provider.Ledger(channelName)
	if err != nil {
		return nil, err
	}

	commitChannel, err := ledger.CommitNotificationsChannel(n.cancel)
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
