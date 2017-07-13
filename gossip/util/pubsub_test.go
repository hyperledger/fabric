/*
Copyright IBM Corp. 2017 All Rights Reserved.

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

package util

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewPubsub(t *testing.T) {
	ps := NewPubSub()
	// Check a publishing to a topic with a subscription succeeds
	sub1 := ps.Subscribe("test", time.Second)
	sub2 := ps.Subscribe("test2", time.Second)
	assert.NotNil(t, sub1)
	go func() {
		err := ps.Publish("test", 5)
		assert.NoError(t, err)
	}()
	item, err := sub1.Listen()
	assert.NoError(t, err)
	assert.Equal(t, 5, item)
	// Check that a publishing to a topic with no subscribers fails
	err = ps.Publish("test3", 5)
	assert.Error(t, err)
	assert.Contains(t, "no subscribers", err.Error())
	// Check that a listen on a topic that its publish is too late, times out
	// and returns an error
	go func() {
		time.Sleep(time.Second * 2)
		ps.Publish("test2", 10)
	}()
	item, err = sub2.Listen()
	assert.Error(t, err)
	assert.Contains(t, "timed out", err.Error())
	assert.Nil(t, item)
	// Have multiple subscribers subscribe to the same topic
	subscriptions := []Subscription{}
	n := 100
	for i := 0; i < n; i++ {
		subscriptions = append(subscriptions, ps.Subscribe("test4", time.Second))
	}
	go func() {
		// Send items and fill the buffer and overflow
		// it by 1 item
		for i := 0; i <= subscriptionBuffSize; i++ {
			err := ps.Publish("test4", 100+i)
			assert.NoError(t, err)
		}
	}()
	wg := sync.WaitGroup{}
	wg.Add(n)
	for _, s := range subscriptions {
		go func(s Subscription) {
			time.Sleep(time.Second)
			defer wg.Done()
			for i := 0; i < subscriptionBuffSize; i++ {
				item, err := s.Listen()
				assert.NoError(t, err)
				assert.Equal(t, 100+i, item)
			}
			// The last item that we published was dropped
			// due to the buffer being full
			item, err := s.Listen()
			assert.Nil(t, item)
			assert.Error(t, err)
		}(s)
	}
	wg.Wait()

	// Ensure subscriptions are cleaned after use
	for i := 0; i < 10; i++ {
		time.Sleep(time.Second)
		ps.Lock()
		empty := len(ps.subscriptions) == 0
		ps.Unlock()
		if empty {
			break
		}
	}
	ps.Lock()
	defer ps.Unlock()
	assert.Empty(t, ps.subscriptions)
}
