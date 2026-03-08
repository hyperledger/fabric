/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package throttle

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Clock observes time
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type SharedRateLimiter struct {
	// Time is how this rate limiter observes time
	Time Clock
	// Limit is the total limit of throughput for all clients
	Limit int
	// InactivityTimeout is the inactivity time after which a client is pruned from memory
	InactivityTimeout time.Duration
	// state
	once    sync.Once
	stop    chan struct{}
	lock    sync.RWMutex
	clients map[string]*clientState
}

type clientState struct {
	lock       sync.Mutex
	lastSeen   atomic.Value
	budget     chan struct{}
	budgetUsed uint32
}

func (srl *SharedRateLimiter) LimitRate(client string) {
	srl.once.Do(srl.init)

	cs := srl.getOrCreateState(client)

	numClients := srl.clientNum()

	if numClients == 0 {
		numClients = 1
	}

	budget := srl.Limit / numClients

	budgetWeUsedSoFar := atomic.AddUint32(&cs.budgetUsed, 1)

	if budgetWeUsedSoFar <= uint32(budget) {
		// We still haven't used up all our budget for the time unit
		return
	}

	// Else, budgetWeUsedSoFar > uint32(budget).
	// We need to:
	// (1) Lock in order to stall all other goroutines of this client
	// (2) Wait for a signal that allocates us a new budget portion

	cs.lock.Lock()
	defer cs.lock.Unlock()

	// Check again, maybe we visited this section from another goroutine?
	if atomic.LoadUint32(&cs.budgetUsed) <= uint32(budget) {
		// Some other goroutine already visited this section,
		// nothing left for us to do.
		return
	}

	// Wait for budget to be allocated to us
	select {
	case <-cs.budget:
	case <-srl.stop:
		return
	}
	// Mark budget as allocated and exit
	cs.lastSeen.Store(time.Now())
	atomic.StoreUint32(&cs.budgetUsed, 0)
}

func (srl *SharedRateLimiter) clientNum() int {
	srl.lock.RLock()
	defer srl.lock.RUnlock()

	return len(srl.clients)
}

func (srl *SharedRateLimiter) getOrCreateState(client string) *clientState {
	srl.lock.RLock()
	cs, exists := srl.clients[client]
	if exists {
		srl.lock.RUnlock()
		return cs
	}
	srl.lock.RUnlock()

	srl.lock.Lock()
	defer srl.lock.Unlock()

	cs, exists = srl.clients[client]
	if exists {
		return cs
	}

	cs = &clientState{
		budget: make(chan struct{}, 1),
	}

	cs.lastSeen.Store(time.Now())

	srl.clients[client] = cs

	return cs
}

// Stop stops the shared rate limiter.
// Not thread safe.
func (srl *SharedRateLimiter) Stop() error {
	select {
	case <-srl.stop:
		return fmt.Errorf("stop must be called only once")
	default:
		close(srl.stop)
		return nil
	}
}

func (srl *SharedRateLimiter) init() {
	if srl.Limit == 0 {
		panic("Limit is un-initialized")
	}

	srl.clients = make(map[string]*clientState)
	srl.stop = make(chan struct{})

	go func() {
		for {
			select {
			case <-srl.stop:
				return
			case <-srl.Time.After(time.Second):
				srl.distributeBudget()
			}
		}
	}()
}

func (srl *SharedRateLimiter) distributeBudget() {
	srl.lock.RLock()
	for _, cs := range srl.clients {
		fillBudgetForClient(cs)
	}
	clientsToBePruned := srl.clientsInactiveForTooLong()
	srl.lock.RUnlock()

	// Next, prune the clients that were lastly seen.
	// It matters not if that client had activity since unlocking the mutex and now.
	srl.lock.Lock()
	defer srl.lock.Unlock()

	for _, client := range clientsToBePruned {
		delete(srl.clients, client)
	}
}

func (srl *SharedRateLimiter) clientsInactiveForTooLong() []string {
	now := srl.Time.Now()
	clientsToBePruned := make([]string, 0, len(srl.clients))
	for client, cs := range srl.clients {
		if shouldPrune(cs, now, srl.InactivityTimeout) {
			clientsToBePruned = append(clientsToBePruned, client)
		}
	}
	return clientsToBePruned
}

func shouldPrune(cs *clientState, now time.Time, timeout time.Duration) bool {
	since := now.Sub(cs.lastSeen.Load().(time.Time))
	return since > timeout
}

func fillBudgetForClient(cs *clientState) {
	select {
	case cs.budget <- struct{}{}:
	default:
	}
}
