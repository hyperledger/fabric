/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"sync"

	"github.com/hyperledger/fabric/core/ledger"
)

type statusListenerSet map[*statusListener]struct{}

type statusNotifier struct {
	lock            sync.Mutex
	listenersByTxID map[string]statusListenerSet
	closed          bool
}

func newStatusNotifier() *statusNotifier {
	return &statusNotifier{
		listenersByTxID: make(map[string]statusListenerSet),
	}
}

func (notifier *statusNotifier) ReceiveBlock(blockEvent *ledger.CommitNotification) {
	notifier.removeCompletedListeners()

	for _, txInfo := range blockEvent.TxsInfo {
		statusEvent := &Status{
			BlockNumber:   blockEvent.BlockNumber,
			TransactionID: txInfo.TxID,
			Code:          txInfo.ValidationCode,
		}
		notifier.notify(statusEvent)
	}
}

func (notifier *statusNotifier) removeCompletedListeners() {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	for transactionID, listeners := range notifier.listenersByTxID {
		for listener := range listeners {
			if listener.isDone() {
				notifier.removeListener(transactionID, listener)
			}
		}
	}
}

func (notifier *statusNotifier) notify(event *Status) {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	for listener := range notifier.listenersByTxID[event.TransactionID] {
		listener.receive(event)
		notifier.removeListener(event.TransactionID, listener)
	}
}

func (notifier *statusNotifier) registerListener(done <-chan struct{}, transactionID string) <-chan *Status {
	notifyChannel := make(chan *Status, 1) // Avoid blocking and only expect one notification per channel

	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	if notifier.closed {
		close(notifyChannel)
	} else {
		listener := &statusListener{
			done:          done,
			notifyChannel: notifyChannel,
		}
		notifier.listenersForTxID(transactionID)[listener] = struct{}{}
	}

	return notifyChannel
}

func (notifier *statusNotifier) listenersForTxID(transactionID string) statusListenerSet {
	listeners, exists := notifier.listenersByTxID[transactionID]
	if !exists {
		listeners = make(statusListenerSet)
		notifier.listenersByTxID[transactionID] = listeners
	}

	return listeners
}

func (notifier *statusNotifier) removeListener(transactionID string, listener *statusListener) {
	listener.close()

	listeners := notifier.listenersByTxID[transactionID]
	delete(listeners, listener)

	if len(listeners) == 0 {
		delete(notifier.listenersByTxID, transactionID)
	}
}

func (notifier *statusNotifier) Close() {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	for _, listeners := range notifier.listenersByTxID {
		for listener := range listeners {
			listener.close()
		}
	}

	notifier.listenersByTxID = nil
	notifier.closed = true
}

type statusListener struct {
	done          <-chan struct{}
	notifyChannel chan<- *Status
}

func (listener *statusListener) isDone() bool {
	select {
	case <-listener.done:
		return true
	default:
		return false
	}
}

func (listener *statusListener) close() {
	close(listener.notifyChannel)
}

func (listener *statusListener) receive(event *Status) {
	listener.notifyChannel <- event
}
