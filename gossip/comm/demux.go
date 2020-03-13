/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"sync"

	"github.com/hyperledger/fabric/gossip/common"
)

// ChannelDeMultiplexer is a struct that can receive channel registrations (AddChannel)
// and publications (DeMultiplex) and it broadcasts the publications to registrations
// according to their predicate. Can only be closed once and never open after a close.
type ChannelDeMultiplexer struct {
	// lock protects everything below it.
	lock   sync.Mutex
	closed bool // one way boolean from false -> true
	stopCh chan struct{}
	// deMuxInProgress keeps track of any calls to DeMultiplex
	// that are still being handled. This is used to determine
	// when it is safe to close all of the tracked channels
	deMuxInProgress sync.WaitGroup
	channels        []*channel
}

// NewChannelDemultiplexer creates a new ChannelDeMultiplexer
func NewChannelDemultiplexer() *ChannelDeMultiplexer {
	return &ChannelDeMultiplexer{stopCh: make(chan struct{})}
}

type channel struct {
	pred common.MessageAcceptor
	ch   chan<- interface{}
}

// Close closes this channel, which makes all channels registered before
// to close as well.
func (m *ChannelDeMultiplexer) Close() {
	m.lock.Lock()
	if m.closed {
		m.lock.Unlock()
		return
	}
	m.closed = true
	close(m.stopCh)
	m.deMuxInProgress.Wait()
	for _, ch := range m.channels {
		close(ch.ch)
	}
	m.channels = nil
	m.lock.Unlock()
}

// AddChannel registers a channel with a certain predicate. AddChannel
// returns a read-only channel that will produce values that are
// matched by the predicate function.
//
// If the DeMultiplexer is closed, the channel returned will be closed
// to prevent users of the channel from waiting on the channel.
func (m *ChannelDeMultiplexer) AddChannel(predicate common.MessageAcceptor) <-chan interface{} {
	m.lock.Lock()
	if m.closed { // closed once, can't put anything more in.
		m.lock.Unlock()
		ch := make(chan interface{})
		close(ch)
		return ch
	}
	bidirectionalCh := make(chan interface{}, 10)
	// Assignment to channel converts bidirectionalCh to send-only.
	// Return converts bidirectionalCh to a receive-only.
	ch := &channel{ch: bidirectionalCh, pred: predicate}
	m.channels = append(m.channels, ch)
	m.lock.Unlock()
	return bidirectionalCh
}

// DeMultiplex broadcasts the message to all channels that were returned
// by AddChannel calls and that hold the respected predicates.
//
// Blocks if any one channel that would receive msg has a full buffer.
func (m *ChannelDeMultiplexer) DeMultiplex(msg interface{}) {
	m.lock.Lock()
	if m.closed {
		m.lock.Unlock()
		return
	}
	channels := m.channels
	m.deMuxInProgress.Add(1)
	m.lock.Unlock()

	for _, ch := range channels {
		if ch.pred(msg) {
			select {
			case <-m.stopCh:
				m.deMuxInProgress.Done()
				return // stopping
			case ch.ch <- msg:
			}
		}
	}
	m.deMuxInProgress.Done()
}
