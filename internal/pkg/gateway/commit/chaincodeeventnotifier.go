/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commit

import (
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger"
)

type BlockChaincodeEvents struct {
	BlockNumber uint64
	Events      []*peer.ChaincodeEvent
}

type chaincodeEventListenerSet map[*chaincodeEventListener]struct{}

type chaincodeEventNotifier struct {
	lock                     sync.Mutex
	listenersByChaincodeName map[string]chaincodeEventListenerSet
	closed                   bool
}

func newChaincodeEventNotifier() *chaincodeEventNotifier {
	return &chaincodeEventNotifier{
		listenersByChaincodeName: make(map[string]chaincodeEventListenerSet),
	}
}

func (notifier *chaincodeEventNotifier) ReceiveBlock(blockEvent *ledger.CommitNotification) {
	notifier.removeCompletedListeners()

	chaincodeEvents := getEventsByChaincodeName(blockEvent)
	notifier.notify(chaincodeEvents)
}

func (notifier *chaincodeEventNotifier) removeCompletedListeners() {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	for chaincodeName, listeners := range notifier.listenersByChaincodeName {
		for listener := range listeners {
			if listener.isDone() {
				notifier.removeListener(chaincodeName, listener)
			}
		}
	}
}

func (notifier *chaincodeEventNotifier) removeListener(chaincodeName string, listener *chaincodeEventListener) {
	listener.close()

	listeners := notifier.listenersByChaincodeName[chaincodeName]
	delete(listeners, listener)

	if len(listeners) == 0 {
		delete(notifier.listenersByChaincodeName, chaincodeName)
	}
}

func getEventsByChaincodeName(blockEvent *ledger.CommitNotification) map[string]*BlockChaincodeEvents {
	results := make(map[string]*BlockChaincodeEvents)

	for _, txInfo := range blockEvent.TxsInfo {
		if txInfo.ChaincodeEventData != nil {
			event := &peer.ChaincodeEvent{}
			if err := proto.Unmarshal(txInfo.ChaincodeEventData, event); err != nil {
				continue
			}

			events := results[event.ChaincodeId]
			if events == nil {
				events = &BlockChaincodeEvents{
					BlockNumber: blockEvent.BlockNumber,
				}
				results[event.ChaincodeId] = events
			}

			events.Events = append(events.Events, event)
		}
	}

	return results
}

func (notifier *chaincodeEventNotifier) notify(eventsByChaincodeName map[string]*BlockChaincodeEvents) {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	for chaincodeName, events := range eventsByChaincodeName {
		for listener := range notifier.listenersByChaincodeName[chaincodeName] {
			listener.receive(events)
		}
	}
}

func (notifier *chaincodeEventNotifier) registerListener(done <-chan struct{}, chaincodeName string) <-chan *BlockChaincodeEvents {
	notifyChannel := make(chan *BlockChaincodeEvents, 100) // Avoid blocking by buffering a number of blocks

	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	if notifier.closed {
		close(notifyChannel)
	} else {
		listener := &chaincodeEventListener{
			done:          done,
			notifyChannel: notifyChannel,
		}
		notifier.listenersForChaincodeName(chaincodeName)[listener] = struct{}{}
	}

	return notifyChannel
}

func (notifier *chaincodeEventNotifier) listenersForChaincodeName(chaincodeName string) chaincodeEventListenerSet {
	listeners, exists := notifier.listenersByChaincodeName[chaincodeName]
	if !exists {
		listeners = make(chaincodeEventListenerSet)
		notifier.listenersByChaincodeName[chaincodeName] = listeners
	}

	return listeners
}

func (notifier *chaincodeEventNotifier) Close() {
	notifier.lock.Lock()
	defer notifier.lock.Unlock()

	for _, listeners := range notifier.listenersByChaincodeName {
		for listener := range listeners {
			listener.close()
		}
	}

	notifier.listenersByChaincodeName = nil
	notifier.closed = true
}

type chaincodeEventListener struct {
	done          <-chan struct{}
	notifyChannel chan<- *BlockChaincodeEvents
}

func (listener *chaincodeEventListener) isDone() bool {
	select {
	case <-listener.done:
		return true
	default:
		return false
	}
}

func (listener *chaincodeEventListener) close() {
	close(listener.notifyChannel)
}

func (listener *chaincodeEventListener) receive(events *BlockChaincodeEvents) {
	listener.notifyChannel <- events
}
