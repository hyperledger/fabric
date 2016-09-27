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

package gossip

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	rand.Seed(42)
}

func alwaysNoAction(this interface{}, that interface{}) invalidationResult {
	return MESSAGE_NO_ACTION
}

func noopTrigger(m interface{}) {

}

func compareInts(this interface{}, that interface{}) invalidationResult {
	a := this.(int)
	b := that.(int)
	if a == b {
		return MESSAGE_NO_ACTION
	}
	if a > b {
		return MESSAGE_INVALIDATES
	}

	return MESSAGE_INVALIDATED
}

func TestSize(t *testing.T) {
	msgStore := newMessageStore(alwaysNoAction, noopTrigger)
	msgStore.add(0)
	msgStore.add(1)
	msgStore.add(2)
	assert.Equal(t, 3, msgStore.size())
}

func TestNewMessagesInvalidates(t *testing.T) {
	invalidated := make([]int, 9)
	msgStore := newMessageStore(compareInts, func(m interface{}) {
		invalidated = append(invalidated, m.(int))
	})
	assert.True(t, msgStore.add(0))
	for i := 1; i < 10; i++ {
		assert.True(t, msgStore.add(i))
		assert.Equal(t, i - 1, invalidated[len(invalidated) - 1])
		assert.Equal(t, 1, msgStore.size())
		assert.Equal(t, i, msgStore.get()[0].(int))
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

	msgStore := newMessageStore(alwaysNoAction, noopTrigger)
	expected := make([]int, 0)
	for i := 0; i < 2; i++ {
		n := rand.Int()
		expected = append(expected, n)
		msgStore.add(n)
	}

	for _, num2Search := range expected {
		assert.True(t, contains(msgStore.get(), num2Search), "Value %v not found in array", num2Search)
	}

}

func TestNewMessagesInvalidated(t *testing.T) {
	msgStore := newMessageStore(compareInts, noopTrigger)
	assert.True(t, msgStore.add(10))
	for i := 9; i >= 0; i-- {
		assert.False(t, msgStore.add(i))
		assert.Equal(t, 1, msgStore.size())
		assert.Equal(t, 10, msgStore.get()[0].(int))
	}
}

func TestConcurrency(t *testing.T) {
	stopFlag := int32(0)
	msgStore := newMessageStore(compareInts, noopTrigger)
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
		msgStore.add(rand.Int())
	})

	getProcess := looper(func() {
		msgStore.get()
	})

	sizeProcess := looper(func() {
		msgStore.size()
	})

	go addProcess()
	go getProcess()
	go sizeProcess()

	time.Sleep(time.Duration(3) * time.Second)

	atomic.CompareAndSwapInt32(&stopFlag, 0, 1)
}
