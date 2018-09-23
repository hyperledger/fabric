/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"sync"
	"sync/atomic"

	"github.com/hyperledger/fabric/gossip/util"
	proto "github.com/hyperledger/fabric/protos/gossip"
)

// PayloadsBuffer is used to store payloads into which used to
// support payloads with blocks reordering according to the
// sequence numbers. It also will provide the capability
// to signal whenever expected block has arrived.
type PayloadsBuffer interface {
	// Adds new block into the buffer
	Push(payload *proto.Payload)

	// Returns next expected sequence number
	Next() uint64

	// Remove and return payload with given sequence number
	Pop() *proto.Payload

	// Get current buffer size
	Size() int

	// Channel to indicate event when new payload pushed with sequence
	// number equal to the next expected value.
	Ready() chan struct{}

	Close()
}

// PayloadsBufferImpl structure to implement PayloadsBuffer interface
// store inner state of available payloads and sequence numbers
type PayloadsBufferImpl struct {
	next uint64

	buf map[uint64]*proto.Payload

	readyChan chan struct{}

	mutex sync.RWMutex

	logger util.Logger
}

// NewPayloadsBuffer is factory function to create new payloads buffer
func NewPayloadsBuffer(next uint64) PayloadsBuffer {
	return &PayloadsBufferImpl{
		buf:       make(map[uint64]*proto.Payload),
		readyChan: make(chan struct{}, 1),
		next:      next,
		logger:    util.GetLogger(util.LoggingStateModule, ""),
	}
}

// Ready function returns the channel which indicates whenever expected
// next block has arrived and one could safely pop out
// next sequence of blocks
func (b *PayloadsBufferImpl) Ready() chan struct{} {
	return b.readyChan
}

// Push new payload into the buffer structure in case new arrived payload
// sequence number is below the expected next block number payload will be
// thrown away.
// TODO return bool to indicate if payload was added or not, so that caller can log result.
func (b *PayloadsBufferImpl) Push(payload *proto.Payload) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	seqNum := payload.SeqNum

	if seqNum < b.next || b.buf[seqNum] != nil {
		logger.Debugf("Payload with sequence number = %d has been already processed", payload.SeqNum)
		return
	}

	b.buf[seqNum] = payload

	// Send notification that next sequence has arrived
	if seqNum == b.next && len(b.readyChan) == 0 {
		b.readyChan <- struct{}{}
	}
}

// Next function provides the number of the next expected block
func (b *PayloadsBufferImpl) Next() uint64 {
	// Atomically read the value of the top sequence number
	return atomic.LoadUint64(&b.next)
}

// Pop function extracts the payload according to the next expected block
// number, if no next block arrived yet, function returns nil.
func (b *PayloadsBufferImpl) Pop() *proto.Payload {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	result := b.buf[b.Next()]

	if result != nil {
		// If there is such sequence in the buffer need to delete it
		delete(b.buf, b.Next())
		// Increment next expect block index
		atomic.AddUint64(&b.next, 1)

		b.drainReadChannel()

	}

	return result
}

// drainReadChannel empties ready channel in case last
// payload has been poped up and there are still awaiting
// notifications in the channel
func (b *PayloadsBufferImpl) drainReadChannel() {
	if len(b.buf) == 0 {
		for {
			if len(b.readyChan) > 0 {
				<-b.readyChan
			} else {
				break
			}
		}
	}
}

// Size returns current number of payloads stored within buffer
func (b *PayloadsBufferImpl) Size() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return len(b.buf)
}

// Close cleanups resources and channels in maintained
func (b *PayloadsBufferImpl) Close() {
	close(b.readyChan)
}
