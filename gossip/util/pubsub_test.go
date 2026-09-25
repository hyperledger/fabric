/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewPubsub(t *testing.T) {
	ps := NewPubSub()
	// Check a publishing to a topic with a subscription succeeds
	sub1 := ps.Subscribe("test", time.Second)
	sub2 := ps.Subscribe("test2", time.Second)
	require.NotNil(t, sub1)
	publishErr := make(chan error, 1)
	go func() {
		publishErr <- ps.Publish("test", 5)
	}()
	item, err := sub1.Listen()
	require.NoError(t, err)
	require.NoError(t, <-publishErr)
	require.Equal(t, 5, item)
	// Check that a publishing to a topic with no subscribers fails
	err = ps.Publish("test3", 5)
	require.Error(t, err)
	require.Contains(t, "no subscribers", err.Error())
	// Check that a listen on a topic that its publish is too late, times out
	// and returns an error
	publishErr = make(chan error, 1)
	go func() {
		time.Sleep(time.Second * 2)
		publishErr <- ps.Publish("test2", 10)
	}()
	item, err = sub2.Listen()
	require.Error(t, err)
	require.Contains(t, "timed out", err.Error())
	require.Nil(t, item)
	err = <-publishErr
	require.Error(t, err)
	require.Contains(t, "no subscribers", err.Error())
	// Have multiple subscribers subscribe to the same topic
	subscriptions := []Subscription{}
	n := 100
	for range n {
		subscriptions = append(subscriptions, ps.Subscribe("test4", time.Second))
	}
	publishErr = make(chan error, 1)
	go func() {
		// Send items and fill the buffer and overflow
		// it by 1 item
		for i := 0; i <= subscriptionBuffSize; i++ {
			if err := ps.Publish("test4", 100+i); err != nil {
				publishErr <- err
				return
			}
		}
		publishErr <- nil
	}()
	wg := sync.WaitGroup{}
	listenErrs := make(chan error, n*2)
	wg.Add(n)
	for _, s := range subscriptions {
		go func(s Subscription) {
			time.Sleep(time.Second)
			defer wg.Done()
			for i := range subscriptionBuffSize {
				item, err := s.Listen()
				if err != nil {
					listenErrs <- fmt.Errorf("subscription %d: item %d: %w", i, 100+i, err)
					return
				}
				if item != 100+i {
					listenErrs <- fmt.Errorf("subscription %d: item %d: got %d", i, 100+i, item)
					return
				}
			}
			// The last item that we published was dropped
			// due to the buffer being full
			item, err := s.Listen()
			if item != nil {
				listenErrs <- fmt.Errorf("expected nil item, got %d", item)
			}
			if err == nil {
				listenErrs <- errors.New("expected timeout error")
			}
		}(s)
	}
	wg.Wait()
	close(listenErrs)
	for err := range listenErrs {
		require.NoError(t, err)
	}
	require.NoError(t, <-publishErr)

	// Ensure subscriptions are cleaned after use
	for range 10 {
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
	require.Empty(t, ps.subscriptions)
}
