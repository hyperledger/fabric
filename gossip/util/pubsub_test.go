/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
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
	publishErrs := make(chan error, 1)
	go func() {
		publishErrs <- ps.Publish("test", 5)
	}()
	item, err := sub1.Listen()
	require.NoError(t, err)
	require.Equal(t, 5, item)
	require.NoError(t, <-publishErrs)
	// Check that a publishing to a topic with no subscribers fails
	err = ps.Publish("test3", 5)
	require.Error(t, err)
	require.Contains(t, "no subscribers", err.Error())
	// Check that a listen on a topic that its publish is too late, times out
	// and returns an error
	go func() {
		time.Sleep(time.Second * 2)
		ps.Publish("test2", 10)
	}()
	item, err = sub2.Listen()
	require.Error(t, err)
	require.Contains(t, "timed out", err.Error())
	require.Nil(t, item)
	// Have multiple subscribers subscribe to the same topic
	subscriptions := []Subscription{}
	n := 100
	for range n {
		subscriptions = append(subscriptions, ps.Subscribe("test4", time.Second))
	}
	overflowPublishErrs := make(chan error, subscriptionBuffSize+1)
	go func() {
		// Send items and fill the buffer and overflow
		// it by 1 item
		for i := 0; i <= subscriptionBuffSize; i++ {
			overflowPublishErrs <- ps.Publish("test4", 100+i)
		}
		close(overflowPublishErrs)
	}()
	wg := sync.WaitGroup{}
	listenErrs := make([]error, n)
	for i, s := range subscriptions {
		wg.Go(func() {
			time.Sleep(time.Second)
			listenErrs[i] = checkSubscription(s)
		})
	}
	wg.Wait()

	for err = range overflowPublishErrs {
		require.NoError(t, err)
	}
	for _, err = range listenErrs {
		require.NoError(t, err)
	}

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

// checkSubscription drains s, verifying that the buffered items are received
// in order and that the item published after the buffer overflow is dropped.
// It is meant to be called from a goroutine other than the one running the
// test, so it reports failures through the returned error instead of asserting.
func checkSubscription(s Subscription) error {
	for i := range subscriptionBuffSize {
		item, err := s.Listen()
		if err != nil {
			return err
		}
		if item != 100+i {
			return fmt.Errorf("expected item %d, got %v", 100+i, item)
		}
	}
	// The last item that we published was dropped
	// due to the buffer being full
	item, err := s.Listen()
	if err == nil {
		return fmt.Errorf("expected an error for the dropped item, got %v", item)
	}
	return nil
}
