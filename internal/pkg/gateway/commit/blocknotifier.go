/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"sync"

	"github.com/hyperledger/fabric/core/ledger"
)

type blockListener interface {
	ReceiveBlock(*ledger.CommitNotification)
	Close()
}

type blockNotifier struct {
	commitChannel <-chan *ledger.CommitNotification
	done          <-chan struct{}
	listeners     []blockListener
	lock          sync.Mutex
	closed        bool
}

func newBlockNotifier(done <-chan struct{}, commitChannel <-chan *ledger.CommitNotification, listeners ...blockListener) *blockNotifier {
	notifier := &blockNotifier{
		commitChannel: commitChannel,
		listeners:     listeners,
		done:          done,
	}
	go notifier.run()
	return notifier
}

func (notifier *blockNotifier) run() {
	for !notifier.isClosed() {
		select {
		case blockCommit, ok := <-notifier.commitChannel:
			if !ok {
				notifier.close()
				return
			}
			notifier.notify(blockCommit)
		case <-notifier.done:
			notifier.close()
			return
		}
	}
}

func (notifier *blockNotifier) notify(event *ledger.CommitNotification) {
	for _, listener := range notifier.listeners {
		listener.ReceiveBlock(event)
	}
}

func (notifier *blockNotifier) close() {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	if notifier.closed {
		return
	}

	for _, listener := range notifier.listeners {
		listener.Close()
	}

	notifier.closed = true
}

func (notifier *blockNotifier) isClosed() bool {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	return notifier.closed
}
