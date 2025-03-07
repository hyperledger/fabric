/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msgstore

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/common"
	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/require"
)

var r *rand.Rand

func init() {
	util.SetupTestLogging()
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
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
	require.Equal(t, 3, msgStore.Size())
}

func TestNewMessagesInvalidates(t *testing.T) {
	invalidated := make([]int, 9)
	msgStore := NewMessageStore(compareInts, func(m interface{}) {
		invalidated = append(invalidated, m.(int))
	})
	require.True(t, msgStore.Add(0))
	for i := 1; i < 10; i++ {
		require.True(t, msgStore.Add(i))
		require.Equal(t, i-1, invalidated[len(invalidated)-1])
		require.Equal(t, 1, msgStore.Size())
		require.Equal(t, i, msgStore.Get()[0].(int))
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
		n := r.Int()
		expected = append(expected, n)
		msgStore.Add(n)
	}

	for _, num2Search := range expected {
		require.True(t, contains(msgStore.Get(), num2Search), "Value %v not found in array", num2Search)
	}
}

func TestNewMessagesInvalidated(t *testing.T) {
	msgStore := NewMessageStore(compareInts, Noop)
	require.True(t, msgStore.Add(10))
	for i := 9; i >= 0; i-- {
		require.False(t, msgStore.Add(i))
		require.Equal(t, 1, msgStore.Size())
		require.Equal(t, 10, msgStore.Get()[0].(int))
	}
}

func TestConcurrency(t *testing.T) {
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
		msgStore.Add(r.Int())
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
	expired := make(chan int, 50)
	msgTTL := time.Second * 3

	msgStore := NewMessageStoreExpirable(nonReplaceInts, Noop, msgTTL, nil, nil, func(m interface{}) {
		expired <- m.(int)
	})

	for i := 0; i < 10; i++ {
		require.True(t, msgStore.Add(i), "Adding", i)
	}

	require.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - first batch")

	time.Sleep(time.Second * 2)

	for i := 0; i < 10; i++ {
		require.False(t, msgStore.CheckValid(i))
		require.False(t, msgStore.Add(i))
	}

	for i := 10; i < 20; i++ {
		require.True(t, msgStore.CheckValid(i))
		require.True(t, msgStore.Add(i))
		require.False(t, msgStore.CheckValid(i))
	}
	require.Equal(t, 20, msgStore.Size(), "Wrong number of items in store - second batch")

	time.Sleep(time.Second * 2)

	for i := 0; i < 20; i++ {
		require.False(t, msgStore.Add(i))
	}

	require.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after first batch expiration")
	require.Equal(t, 10, len(expired), "Wrong number of expired msgs - after first batch expiration")

	time.Sleep(time.Second * 4)

	require.Equal(t, 0, msgStore.Size(), "Wrong number of items in store - after second batch expiration")
	require.Equal(t, 20, len(expired), "Wrong number of expired msgs - after second batch expiration")

	for i := 0; i < 10; i++ {
		require.True(t, msgStore.CheckValid(i))
		require.True(t, msgStore.Add(i))
		require.False(t, msgStore.CheckValid(i))
	}

	require.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after second batch expiration and first banch re-added")
}

func TestExpirationConcurrency(t *testing.T) {
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
		require.True(t, msgStore.Add(i), "Adding", i)
	}
	require.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - first batch")
	lock.Unlock()

	time.Sleep(time.Second * 2)

	lock.Lock()
	time.Sleep(time.Second * 2)

	for i := 0; i < 10; i++ {
		require.False(t, msgStore.Add(i))
	}

	require.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after first batch expiration, external lock taken")
	require.Equal(t, 0, len(expired), "Wrong number of expired msgs - after first batch expiration, external lock taken")
	lock.Unlock()

	time.Sleep(time.Second * 1)

	lock.Lock()
	for i := 0; i < 10; i++ {
		require.False(t, msgStore.Add(i))
	}

	require.Equal(t, 0, msgStore.Size(), "Wrong number of items in store - after first batch expiration, expiration should run")
	require.Equal(t, 10, len(expired), "Wrong number of expired msgs - after first batch expiration, expiration should run")

	lock.Unlock()
}

func TestStop(t *testing.T) {
	expired := make([]int, 0)
	msgTTL := time.Second * 3

	msgStore := NewMessageStoreExpirable(nonReplaceInts, Noop, msgTTL, nil, nil, func(m interface{}) {
		expired = append(expired, m.(int))
	})

	for i := 0; i < 10; i++ {
		require.True(t, msgStore.Add(i), "Adding", i)
	}

	require.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - first batch")

	msgStore.Stop()

	time.Sleep(time.Second * 4)

	require.Equal(t, 10, msgStore.Size(), "Wrong number of items in store - after first batch expiration, but store was stopped, so no expiration")
	require.Equal(t, 0, len(expired), "Wrong number of expired msgs - after first batch expiration, but store was stopped, so no expiration")

	msgStore.Stop()
}

func TestPurge(t *testing.T) {
	purged := make(chan int, 5)
	msgStore := NewMessageStore(alwaysNoAction, func(o interface{}) {
		purged <- o.(int)
	})
	for i := 0; i < 10; i++ {
		require.True(t, msgStore.Add(i))
	}
	// Purge all numbers greater than 9 - shouldn't do anything
	msgStore.Purge(func(o interface{}) bool {
		return o.(int) > 9
	})
	require.Len(t, msgStore.Get(), 10)
	// Purge all even numbers
	msgStore.Purge(func(o interface{}) bool {
		return o.(int)%2 == 0
	})
	// Ensure only odd numbers are left
	require.Len(t, msgStore.Get(), 5)
	for _, o := range msgStore.Get() {
		require.Equal(t, 1, o.(int)%2)
	}
	close(purged)
	i := 0
	for n := range purged {
		require.Equal(t, i, n)
		i += 2
	}
}
