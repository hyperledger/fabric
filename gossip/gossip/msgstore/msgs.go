/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package msgstore

import (
	"sync"

	"github.com/hyperledger/fabric/gossip/common"
)

// invalidationTrigger is invoked on each message that was invalidated because of a message addition
// i.e: if add(0), add(1) was called one after the other, and the store has only {1} after the sequence of invocations
// then the invalidation trigger on 0 was called when 1 was added.
type invalidationTrigger func(message interface{})

// NewMessageStore returns a new MessageStore with the message replacing
// policy and invalidation trigger passed.
func NewMessageStore(pol common.MessageReplacingPolicy, trigger invalidationTrigger) MessageStore {
	return &messageStoreImpl{pol: pol, lock: &sync.RWMutex{}, messages: make([]*msg, 0), invTrigger: trigger}
}

// MessageStore adds messages to an internal buffer.
// When a message is received, it might:
// 	- Be added to the buffer
// 	- Discarded because of some message already in the buffer (invalidated)
// 	- Make a message already in the buffer to be discarded (invalidates)
// When a message is invalidated, the invalidationTrigger is invoked on that message.
type MessageStore interface {
	// add adds a message to the store
	// returns true or false whether the message was added to the store
	Add(msg interface{}) bool

	// size returns the amount of messages in the store
	Size() int

	// get returns all messages in the store
	Get() []interface{}
}

type messageStoreImpl struct {
	pol        common.MessageReplacingPolicy
	lock       *sync.RWMutex
	messages   []*msg
	invTrigger invalidationTrigger
}

type msg struct {
	data interface{}
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
			break
		default:
			break
		}
	}

	s.messages = append(s.messages, &msg{data: message})
	return true
}

// size returns the amount of messages in the store
func (s *messageStoreImpl) Size() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.messages)
}

// get returns all messages in the store
func (s *messageStoreImpl) Get() []interface{} {
	s.lock.RLock()
	defer s.lock.RUnlock()

	n := len(s.messages)
	res := make([]interface{}, n)
	for i := 0; i < n; i++ {
		res[i] = s.messages[i].data
	}
	return res
}
