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
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(42)
}

func alwaysNoAction(this interface{}, that interface{}) common.InvalidationResult {
	return common.MessageNoAction
}

func noopTrigger(m interface{}) {

}

func compareInts(this interface{}, that interface{}) common.InvalidationResult {
	a := this.(int)
	b := that.(int)
	if a == b {
		return common.MessageNoAction
	}
	if a > b {
		return common.MessageInvalidates
	}

	return common.MessageInvalidated
}

func TestSize(t *testing.T) {
	msgStore := NewMessageStore(alwaysNoAction, noopTrigger)
	msgStore.Add(0)
	msgStore.Add(1)
	msgStore.Add(2)
	assert.Equal(t, 3, msgStore.Size())
}

func TestNewMessagesInvalidates(t *testing.T) {
	invalidated := make([]int, 9)
	msgStore := NewMessageStore(compareInts, func(m interface{}) {
		invalidated = append(invalidated, m.(int))
	})
	assert.True(t, msgStore.Add(0))
	for i := 1; i < 10; i++ {
		assert.True(t, msgStore.Add(i))
		assert.Equal(t, i-1, invalidated[len(invalidated)-1])
		assert.Equal(t, 1, msgStore.Size())
		assert.Equal(t, i, msgStore.Get()[0].(int))
	}
}

func TestMessagesGet(t *testing.T) {
	contains := func(a []interface{}, e interface{}) bool {
		for _, v := range a {
			if v == e {
				return true
			}
		}
		return false
	}

	msgStore := NewMessageStore(alwaysNoAction, noopTrigger)
	expected := []int{}
	for i := 0; i < 2; i++ {
		n := rand.Int()
		expected = append(expected, n)
		msgStore.Add(n)
	}

	for _, num2Search := range expected {
		assert.True(t, contains(msgStore.Get(), num2Search), "Value %v not found in array", num2Search)
	}

}

func TestNewMessagesInvalidated(t *testing.T) {
	msgStore := NewMessageStore(compareInts, noopTrigger)
	assert.True(t, msgStore.Add(10))
	for i := 9; i >= 0; i-- {
		assert.False(t, msgStore.Add(i))
		assert.Equal(t, 1, msgStore.Size())
		assert.Equal(t, 10, msgStore.Get()[0].(int))
	}
}

func TestConcurrency(t *testing.T) {
	stopFlag := int32(0)
	msgStore := NewMessageStore(compareInts, noopTrigger)
	looper := func(f func()) func() {
		return func() {
			for {
				if atomic.LoadInt32(&stopFlag) == int32(1) {
					return
				}
				f()
			}
		}
	}

	addProcess := looper(func() {
		msgStore.Add(rand.Int())
	})

	getProcess := looper(func() {
		msgStore.Get()
	})

	sizeProcess := looper(func() {
		msgStore.Size()
	})

	go addProcess()
	go getProcess()
	go sizeProcess()

	time.Sleep(time.Duration(3) * time.Second)

	atomic.CompareAndSwapInt32(&stopFlag, 0, 1)
}
