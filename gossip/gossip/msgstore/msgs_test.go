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

	"sync"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/assert"
)

func init() {
	util.SetupTestLogging()
	rand.Seed(time.Now().UnixNano())
}

func alwaysNoAction(_ interface{}, _ interface{}) common.InvalidationResult {
	return common.MessageNoAction
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

func nonReplaceInts(this interface{}, that interface{}) common.InvalidationResult {
	a := this.(int)
	b := that.(int)
	if a == b {
		return common.MessageInvalidated
	}

	return common.MessageNoAction
}

func TestSize(t *testing.T) {
	msgStore := NewMessageStore(alwaysNoAction, Noop)
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

	msgStore := NewMessageStore(alwaysNoAction, Noop)
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
	msgStore := NewMessageStore(compareInts, Noop)
	assert.True(t, msgStore.Add(10))
	for i := 9; i >= 0; i-- {
		assert.False(t, msgStore.Add(i))
		assert.Equal(t, 1, msgStore.Size())
		assert.Equal(t, 10, msgStore.Get()[0].(int))
	}
}

func TestConcurrency(t *testing.T) {
	t.Parallel()
	stopFlag := int32(0)
	msgStore := NewMessageStore(compareInts, Noop)
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

func TestExpiration(t *testing.T) {
	t.Parallel()
	expired := make([]int, 0)
	msgTTL := time.Second * 3

	msgStore := NewMessageStoreExpirable(nonReplaceInts, Noop, msgTTL, nil, nil, func(m interface{}) {
		expired = append(expired, m.(int))
	})

	for i := 0; i < 10; i++ {
		assert.True(t, msgStore.Add(i), "Adding", i)
	}

	assert.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - first batch")

	time.Sleep(time.Second * 2)

	for i := 0; i < 10; i++ {
		assert.False(t, msgStore.CheckValid(i))
		assert.False(t, msgStore.Add(i))
	}

	for i := 10; i < 20; i++ {
		assert.True(t, msgStore.CheckValid(i))
		assert.True(t, msgStore.Add(i))
		assert.False(t, msgStore.CheckValid(i))
	}
	assert.Equal(t, 20, msgStore.Size(), "Wrong number of items in store - second batch")

	time.Sleep(time.Second * 2)

	for i := 0; i < 20; i++ {
		assert.False(t, msgStore.Add(i))
	}

	assert.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after first batch expiration")
	assert.Equal(t, 10, len(expired), "Wrong number of expired msgs - after first batch expiration")

	time.Sleep(time.Second * 4)

	assert.Equal(t, 0, msgStore.Size(), "Wrong number of items in store - after second batch expiration")
	assert.Equal(t, 20, len(expired), "Wrong number of expired msgs - after second batch expiration")

	for i := 0; i < 10; i++ {
		assert.True(t, msgStore.CheckValid(i))
		assert.True(t, msgStore.Add(i))
		assert.False(t, msgStore.CheckValid(i))
	}

	assert.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after second batch expiration and first banch re-added")

}

func TestExpirationConcurrency(t *testing.T) {
	t.Parallel()
	expired := make([]int, 0)
	msgTTL := time.Second * 3
	lock := &sync.RWMutex{}

	msgStore := NewMessageStoreExpirable(nonReplaceInts, Noop, msgTTL,
		func() {
			lock.Lock()
		},
		func() {
			lock.Unlock()
		},
		func(m interface{}) {
			expired = append(expired, m.(int))
		})

	lock.Lock()
	for i := 0; i < 10; i++ {
		assert.True(t, msgStore.Add(i), "Adding", i)
	}
	assert.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - first batch")
	lock.Unlock()

	time.Sleep(time.Second * 2)

	lock.Lock()
	time.Sleep(time.Second * 2)

	for i := 0; i < 10; i++ {
		assert.False(t, msgStore.Add(i))
	}

	assert.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after first batch expiration, external lock taken")
	assert.Equal(t, 0, len(expired), "Wrong number of expired msgs - after first batch expiration, external lock taken")
	lock.Unlock()

	time.Sleep(time.Second * 1)

	lock.Lock()
	for i := 0; i < 10; i++ {
		assert.False(t, msgStore.Add(i))
	}

	assert.Equal(t, 0, msgStore.Size(), "Wrong number of items in store - after first batch expiration, expiration should run")
	assert.Equal(t, 10, len(expired), "Wrong number of expired msgs - after first batch expiration, expiration should run")

	lock.Unlock()
}

func TestStop(t *testing.T) {
	t.Parallel()
	expired := make([]int, 0)
	msgTTL := time.Second * 3

	msgStore := NewMessageStoreExpirable(nonReplaceInts, Noop, msgTTL, nil, nil, func(m interface{}) {
		expired = append(expired, m.(int))
	})

	for i := 0; i < 10; i++ {
		assert.True(t, msgStore.Add(i), "Adding", i)
	}

	assert.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - first batch")

	msgStore.Stop()

	time.Sleep(time.Second * 4)

	assert.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after first batch expiration, but store was stopped, so no expiration")
	assert.Equal(t, 0, len(expired), "Wrong number of expired msgs - after first batch expiration, but store was stopped, so no expiration")

	msgStore.Stop()
}

func TestPurge(t *testing.T) {
	t.Parallel()
	purged := make(chan int, 5)
	msgStore := NewMessageStore(alwaysNoAction, func(o interface{}) {
		purged <- o.(int)
	})
	for i := 0; i < 10; i++ {
		assert.True(t, msgStore.Add(i))
	}
	// Purge all numbers greater than 9 - shouldn't do anything
	msgStore.Purge(func(o interface{}) bool {
		return o.(int) > 9
	})
	assert.Len(t, msgStore.Get(), 10)
	// Purge all even numbers
	msgStore.Purge(func(o interface{}) bool {
		return o.(int)%2 == 0
	})
	// Ensure only odd numbers are left
	assert.Len(t, msgStore.Get(), 5)
	for _, o := range msgStore.Get() {
		assert.Equal(t, 1, o.(int)%2)
	}
	close(purged)
	i := 0
	for n := range purged {
		assert.Equal(t, i, n)
		i += 2
	}
}
