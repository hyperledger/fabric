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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyperledger/fabric/gossip/util"
	"github.com/stretchr/testify/assert"
)

func init() {
	util.SetupTestLogging()
}

func TestBatchingEmitterAddAndSize(t *testing.T) {
	emitter := newBatchingEmitter(1, 10, time.Second, func(a []interface{}) {})
	defer emitter.Stop()
	emitter.Add(1)
	emitter.Add(2)
	emitter.Add(3)
	assert.Equal(t, 3, emitter.Size())
}

func TestBatchingEmitterStop(t *testing.T) {
	// In this test we make sure the emitter doesn't do anything after it's stopped
	disseminationAttempts := int32(0)
	cb := func(a []interface{}) {
		atomic.AddInt32(&disseminationAttempts, int32(1))
	}

	emitter := newBatchingEmitter(10, 1, time.Duration(100)*time.Millisecond, cb)
	emitter.Add(1)
	time.Sleep(time.Duration(100) * time.Millisecond)
	emitter.Stop()
	time.Sleep(time.Duration(1000) * time.Millisecond)
	assert.True(t, atomic.LoadInt32(&disseminationAttempts) < int32(5))
}

func TestBatchingEmitterExpiration(t *testing.T) {
	// In this test we make sure that a message is expired and is discarded after enough time
	// and that it was forwarded an adequate amount of times
	disseminationAttempts := int32(0)
	cb := func(a []interface{}) {
		atomic.AddInt32(&disseminationAttempts, int32(1))
	}

	emitter := newBatchingEmitter(10, 1, time.Duration(10)*time.Millisecond, cb)
	defer emitter.Stop()

	emitter.Add(1)
	time.Sleep(time.Duration(500) * time.Millisecond)
	assert.Equal(t, int32(10), atomic.LoadInt32(&disseminationAttempts), "Inadequate amount of dissemination attempts detected")
	assert.Equal(t, 0, emitter.Size())
}

func TestBatchingEmitterCounter(t *testing.T) {
	// In this test we count the number of times each message is forwarded, with relation to the time passed
	counters := make(map[int]int)
	lock := &sync.Mutex{}
	cb := func(a []interface{}) {
		lock.Lock()
		defer lock.Unlock()
		for _, e := range a {
			n := e.(int)
			if _, exists := counters[n]; !exists {
				counters[n] = 0
			} else {
				counters[n]++
			}
		}
	}

	emitter := newBatchingEmitter(5, 100, time.Duration(500)*time.Millisecond, cb)
	defer emitter.Stop()

	for i := 1; i <= 5; i++ {
		emitter.Add(i)
		if i == 5 {
			break
		}
		time.Sleep(time.Duration(600) * time.Millisecond)
	}
	emitter.Stop()

	lock.Lock()
	assert.Equal(t, 0, counters[4])
	assert.Equal(t, 1, counters[3])
	assert.Equal(t, 2, counters[2])
	assert.Equal(t, 3, counters[1])
	lock.Unlock()
}

// TestBatchingEmitterBurstSizeCap tests that the emitter
func TestBatchingEmitterBurstSizeCap(t *testing.T) {
	disseminationAttempts := int32(0)
	cb := func(a []interface{}) {
		atomic.AddInt32(&disseminationAttempts, int32(1))
	}
	emitter := newBatchingEmitter(1, 10, time.Duration(800)*time.Millisecond, cb)
	defer emitter.Stop()

	for i := 0; i < 50; i++ {
		emitter.Add(i)
	}
	assert.Equal(t, int32(5), atomic.LoadInt32(&disseminationAttempts))
}
