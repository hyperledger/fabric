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
	ch   chan<- any
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
func (m *ChannelDeMultiplexer) AddChannel(predicate common.MessageAcceptor) <-chan any {
	m.lock.Lock()
	if m.closed { // closed once, can't put anything more in.
		m.lock.Unlock()
		ch := make(chan any)
		close(ch)
		return ch
	}
	bidirectionalCh := make(chan any, 10)
	// Assignment to channel converts bidirectionalCh to send-only.
	// Return converts bidirectionalCh to a receive-only.
	ch := &channel{ch: bidirectionalCh, pred: predicate}
	// Copy-on-write: deMultiplex iterates a snapshot captured under the lock
	// after releasing it, so the captured backing array must never be
	// mutated by a concurrent registration.
	channels := make([]*channel, len(m.channels)+1)
	copy(channels, m.channels)
	channels[len(m.channels)] = ch
	m.channels = channels
	m.lock.Unlock()
	return bidirectionalCh
}

// consumptionTracker is implemented by messages that hold accounted resources
// until their deliveries are consumed. DeMultiplexAbortable reports the number
// of deliveries before the first one and disposes the deliveries it aborts.
type consumptionTracker interface {
	// trackDeliveries records the delivery count before the first delivery.
	trackDeliveries(n int)
	// Dispose marks a single delivery as consumed (or as never made).
	Dispose()
}

// DeMultiplex broadcasts the message to all channels that were returned
// by AddChannel calls and that hold the respected predicates.
//
// Blocks if any one channel that would receive msg has a full buffer.
func (m *ChannelDeMultiplexer) DeMultiplex(msg any) {
	m.deMultiplex(msg, nil, false)
}

// DeMultiplexAbortable is DeMultiplex with delivery abortion: it stops
// delivering when abort fires, and tracks deliveries of messages implementing
// consumptionTracker so their resources stay accounted until consumed.
func (m *ChannelDeMultiplexer) DeMultiplexAbortable(msg any, abort <-chan struct{}) {
	m.deMultiplex(msg, abort, true)
}

func (m *ChannelDeMultiplexer) deMultiplex(msg any, abort <-chan struct{}, track bool) {
	m.lock.Lock()
	if m.closed {
		m.lock.Unlock()
		if t, ok := msg.(consumptionTracker); ok && track {
			t.trackDeliveries(0)
		}
		return
	}
	channels := m.channels
	m.deMuxInProgress.Add(1)
	m.lock.Unlock()
	defer m.deMuxInProgress.Done()

	if !track {
		// Single pass on the untracked hot path; no delivery count is needed.
		for _, ch := range channels {
			if ch.pred(msg) {
				select {
				case <-m.stopCh:
					return // stopping
				case <-abort:
					return // aborted
				case ch.ch <- msg:
				}
			}
		}
		return
	}

	// The delivery count must be known before the first delivery, as consumers
	// may release the message while later deliveries are still in progress.
	// The stack buffer keeps the common case allocation-free.
	var matchedBuf [8]*channel
	matched := matchedBuf[:0]
	for _, ch := range channels {
		if ch.pred(msg) {
			matched = append(matched, ch)
		}
	}
	tracker, tracked := msg.(consumptionTracker)
	if tracked {
		tracker.trackDeliveries(len(matched))
	}

	for i, ch := range matched {
		select {
		case <-m.stopCh:
			m.disposeUndelivered(tracker, tracked, len(matched)-i)
			return // stopping
		case <-abort:
			m.disposeUndelivered(tracker, tracked, len(matched)-i)
			return // aborted
		case ch.ch <- msg:
		}
	}
}

// disposeUndelivered consumes the deliveries that were tracked but not made.
func (m *ChannelDeMultiplexer) disposeUndelivered(tracker consumptionTracker, tracked bool, n int) {
	if !tracked {
		return
	}
	for range n {
		tracker.Dispose()
	}
}
