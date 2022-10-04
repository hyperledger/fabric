/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgstore

import (
	"sync"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
)

var noopLock = func() {}

// Noop is a function that doesn't do anything
func Noop(_ interface{}) {
}

// invalidationTrigger is invoked on each message that was invalidated because of a message addition
// i.e: if add(0), add(1) was called one after the other, and the store has only {1} after the sequence of invocations
// then the invalidation trigger on 0 was called when 1 was added.
type invalidationTrigger func(message interface{})

// NewMessageStore returns a new MessageStore with the message replacing
// policy and invalidation trigger passed.
func NewMessageStore(pol common.MessageReplacingPolicy, trigger invalidationTrigger) MessageStore {
	return newMsgStore(pol, trigger)
}

// NewMessageStoreExpirable returns a new MessageStore with the message replacing
// policy and invalidation trigger passed. It supports old message expiration after msgTTL, during expiration first external
// lock taken, expiration callback invoked and external lock released. Callback and external lock can be nil.
func NewMessageStoreExpirable(pol common.MessageReplacingPolicy, trigger invalidationTrigger, msgTTL time.Duration, externalLock func(), externalUnlock func(), externalExpire func(interface{})) MessageStore {
	store := newMsgStore(pol, trigger)
	store.msgTTL = msgTTL

	if externalLock != nil {
		store.externalLock = externalLock
	}

	if externalUnlock != nil {
		store.externalUnlock = externalUnlock
	}

	if externalExpire != nil {
		store.expireMsgCallback = externalExpire
	}

	go store.expirationRoutine()
	return store
}

func newMsgStore(pol common.MessageReplacingPolicy, trigger invalidationTrigger) *messageStoreImpl {
	return &messageStoreImpl{
		pol:        pol,
		messages:   make([]*msg, 0),
		invTrigger: trigger,

		externalLock:      noopLock,
		externalUnlock:    noopLock,
		expireMsgCallback: func(m interface{}) {},
		expiredCount:      0,

		doneCh: make(chan struct{}),
	}
}

// MessageStore adds messages to an internal buffer.
// When a message is received, it might:
//   - Be added to the buffer
//   - Discarded because of some message already in the buffer (invalidated)
//   - Make a message already in the buffer to be discarded (invalidates)
//
// When a message is invalidated, the invalidationTrigger is invoked on that message.
type MessageStore interface {
	// add adds a message to the store
	// returns true or false whether the message was added to the store
	Add(msg interface{}) bool

	// Checks if message is valid for insertion to store
	// returns true or false whether the message can be added to the store
	CheckValid(msg interface{}) bool

	// size returns the amount of messages in the store
	Size() int

	// get returns all messages in the store
	Get() []interface{}

	// Stop all associated go routines
	Stop()

	// Purge purges all messages that are accepted by
	// the given predicate
	Purge(func(interface{}) bool)
}

type messageStoreImpl struct {
	pol               common.MessageReplacingPolicy
	lock              sync.RWMutex
	messages          []*msg
	invTrigger        invalidationTrigger
	msgTTL            time.Duration
	expiredCount      int
	externalLock      func()
	externalUnlock    func()
	expireMsgCallback func(msg interface{})
	doneCh            chan struct{}
	stopOnce          sync.Once
}

type msg struct {
	data    interface{}
	created time.Time
	expired bool
}

// add adds a message to the store
func (s *messageStoreImpl) Add(message interface{}) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	n := len(s.messages)
	for i := 0; i < n; i++ {
		m := s.messages[i]
		switch s.pol(message, m.data) {
		case common.MessageInvalidated:
			return false
		case common.MessageInvalidates:
			s.invTrigger(m.data)
			s.messages = append(s.messages[:i], s.messages[i+1:]...)
			n--
			i--
		}
	}

	s.messages = append(s.messages, &msg{data: message, created: time.Now()})
	return true
}

func (s *messageStoreImpl) Purge(shouldBePurged func(interface{}) bool) {
	shouldMsgBePurged := func(m *msg) bool {
		return shouldBePurged(m.data)
	}
	if !s.isPurgeNeeded(shouldMsgBePurged) {
		return
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	n := len(s.messages)
	for i := 0; i < n; i++ {
		if !shouldMsgBePurged(s.messages[i]) {
			continue
		}
		s.invTrigger(s.messages[i].data)
		s.messages = append(s.messages[:i], s.messages[i+1:]...)
		n--
		i--
	}
}

// Checks if message is valid for insertion to store
func (s *messageStoreImpl) CheckValid(message interface{}) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for _, m := range s.messages {
		if s.pol(message, m.data) == common.MessageInvalidated {
			return false
		}
	}
	return true
}

// size returns the amount of messages in the store
func (s *messageStoreImpl) Size() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.messages) - s.expiredCount
}

// get returns all messages in the store
func (s *messageStoreImpl) Get() []interface{} {
	res := make([]interface{}, 0)

	s.lock.RLock()
	defer s.lock.RUnlock()

	for _, msg := range s.messages {
		if !msg.expired {
			res = append(res, msg.data)
		}
	}
	return res
}

func (s *messageStoreImpl) expireMessages() {
	s.externalLock()
	s.lock.Lock()
	defer s.lock.Unlock()
	defer s.externalUnlock()

	n := len(s.messages)
	for i := 0; i < n; i++ {
		m := s.messages[i]
		if !m.expired {
			if time.Since(m.created) > s.msgTTL {
				m.expired = true
				s.expireMsgCallback(m.data)
				s.expiredCount++
			}
		} else {
			if time.Since(m.created) > (s.msgTTL * 2) {
				s.messages = append(s.messages[:i], s.messages[i+1:]...)
				n--
				i--
				s.expiredCount--
			}
		}
	}
}

func (s *messageStoreImpl) isPurgeNeeded(shouldBePurged func(*msg) bool) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()
	for _, m := range s.messages {
		if shouldBePurged(m) {
			return true
		}
	}
	return false
}

func (s *messageStoreImpl) expirationRoutine() {
	for {
		select {
		case <-s.doneCh:
			return
		case <-time.After(s.expirationCheckInterval()):
			hasMessageExpired := func(m *msg) bool {
				if !m.expired && time.Since(m.created) > s.msgTTL {
					return true
				} else if time.Since(m.created) > (s.msgTTL * 2) {
					return true
				}
				return false
			}
			if s.isPurgeNeeded(hasMessageExpired) {
				s.expireMessages()
			}
		}
	}
}

func (s *messageStoreImpl) Stop() {
	stopFunc := func() {
		close(s.doneCh)
	}
	s.stopOnce.Do(stopFunc)
}

func (s *messageStoreImpl) expirationCheckInterval() time.Duration {
	return s.msgTTL / 100
}
