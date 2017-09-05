/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"sync"
	"time"

	"github.com/pkg/errors"
)

const (
	subscriptionBuffSize = 50
)

// PubSub defines a struct that one can use to:
// - publish items to a topic to multiple subscribers
// - and subscribe to items from a topic
// The subscriptions have a TTL and are cleaned when it passes.
type PubSub struct {
	sync.RWMutex

	// a map from topic to Set of subscriptions
	subscriptions map[string]*Set
}

// Subscription defines a subscription to a topic
// that can be used to receive publishes on
type Subscription interface {
	// Listen blocks until a publish was made
	// to the subscription, or an error if the
	// subscription's TTL passed
	Listen() (interface{}, error)
}

type subscription struct {
	top string
	ttl time.Duration
	c   chan interface{}
}

// Listen blocks until a publish was made
// to the subscription, or an error if the
// subscription's TTL passed
func (s *subscription) Listen() (interface{}, error) {
	select {
	case <-time.After(s.ttl):
		return nil, errors.New("timed out")
	case item := <-s.c:
		return item, nil
	}
}

// NewPubSub creates a new PubSub with an empty
// set of subscriptions
func NewPubSub() *PubSub {
	return &PubSub{
		subscriptions: make(map[string]*Set),
	}
}

// Publish publishes an item to all subscribers on the topic
func (ps *PubSub) Publish(topic string, item interface{}) error {
	ps.RLock()
	defer ps.RUnlock()
	s, subscribed := ps.subscriptions[topic]
	if !subscribed {
		return errors.New("no subscribers")
	}
	for _, sub := range s.ToArray() {
		c := sub.(*subscription).c
		// Not enough room in buffer, continue in order to not block publisher
		if len(c) == subscriptionBuffSize {
			continue
		}
		c <- item
	}
	return nil
}

// Subscribe returns a subscription to a topic that expires when given TTL passes
func (ps *PubSub) Subscribe(topic string, ttl time.Duration) Subscription {
	sub := &subscription{
		top: topic,
		ttl: ttl,
		c:   make(chan interface{}, subscriptionBuffSize),
	}

	ps.Lock()
	// Add subscription to subscriptions map
	s, exists := ps.subscriptions[topic]
	// If no subscription set for the topic exists, create one
	if !exists {
		s = NewSet()
		ps.subscriptions[topic] = s
	}
	ps.Unlock()

	// Add the subscription
	s.Add(sub)

	// When the timeout expires, remove the subscription
	time.AfterFunc(ttl, func() {
		ps.unSubscribe(sub)
	})
	return sub
}

func (ps *PubSub) unSubscribe(sub *subscription) {
	ps.Lock()
	defer ps.Unlock()
	ps.subscriptions[sub.top].Remove(sub)
	if ps.subscriptions[sub.top].Size() != 0 {
		return
	}
	// Else, this is the last subscription for the topic.
	// Remove the set from the subscriptions map
	delete(ps.subscriptions, sub.top)

}
