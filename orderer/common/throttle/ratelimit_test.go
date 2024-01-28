/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package throttle

import (
	"fmt"
	"sync"
	"testing"
	"time"

	clock "code.cloudfoundry.org/clock/fakeclock"
	"github.com/stretchr/testify/assert"
)

func TestRateLimitSingleClient(t *testing.T) {
	t.Parallel()

	clock := clock.NewFakeClock(time.Now())

	stop := make(chan struct{})
	defer close(stop)

	go advanceTime(stop, clock)

	rl := SharedRateLimiter{
		Time:              clock,
		Limit:             500,
		InactivityTimeout: time.Minute,
	}

	defer rl.Stop()

	transactions := 10000

	start := clock.Now()

	for i := 0; i < transactions; i++ {
		rl.LimitRate("alice")
	}
	elapsed := clock.Since(start)

	TPS := float64(transactions / int(elapsed.Seconds()))

	max := float64(500) * 1.3
	min := float64(500) * 0.7

	assert.Less(t, TPS, max)
	assert.Less(t, min, TPS)
}

func TestRateLimitMultipleClients(t *testing.T) {
	t.Parallel()

	clock := clock.NewFakeClock(time.Now())

	stop := make(chan struct{})
	defer close(stop)

	go advanceTime(stop, clock)

	rl := SharedRateLimiter{
		Time:              clock,
		Limit:             500,
		InactivityTimeout: time.Minute,
	}
	defer rl.Stop()

	start := clock.Now()

	var wg sync.WaitGroup
	wg.Add(10)

	transactions := 10000

	for clientID := 0; clientID < 10; clientID++ {
		client := fmt.Sprintf("client%d", clientID)
		go func(client string) {
			defer wg.Done()
			for i := 0; i < transactions/10; i++ {
				rl.LimitRate(client)
			}
		}(client)

	}

	wg.Wait()

	elapsed := clock.Since(start)

	TPS := float64(transactions / int(elapsed.Seconds()))

	max := float64(500) * 1.3
	min := float64(500) * 0.7

	assert.Less(t, TPS, max)
	assert.Less(t, min, TPS)
}

func TestRateLimitSameClient(t *testing.T) {
	t.Parallel()

	clock := clock.NewFakeClock(time.Now())

	stop := make(chan struct{})
	defer close(stop)

	go advanceTime(stop, clock)

	rl := SharedRateLimiter{
		Time:              clock,
		Limit:             500,
		InactivityTimeout: time.Minute,
	}
	defer rl.Stop()

	start := clock.Now()

	var wg sync.WaitGroup
	wg.Add(10)

	transactions := 10000

	for clientID := 0; clientID < 10; clientID++ {
		go func() {
			defer wg.Done()
			for i := 0; i < transactions/10; i++ {
				rl.LimitRate("alice")
			}
		}()
	}

	wg.Wait()

	elapsed := clock.Since(start)

	TPS := float64(transactions / int(elapsed.Seconds()))

	max := float64(500) * 1.3
	min := float64(500) * 0.7

	assert.Less(t, TPS, max)
	assert.Less(t, min, TPS)
}

func TestRateLimitClientNumChange(t *testing.T) {
	t.Parallel()

	clock := clock.NewFakeClock(time.Now())

	stop := make(chan struct{})
	defer close(stop)

	go advanceTime(stop, clock)

	rl := SharedRateLimiter{
		Time:              clock,
		Limit:             500,
		InactivityTimeout: time.Second * 10,
	}
	defer rl.Stop()

	t.Run("", func(t *testing.T) {
		start := time.Now()

		var wg sync.WaitGroup
		wg.Add(10)

		transactions := 5000

		for clientID := 0; clientID < 10; clientID++ {
			client := fmt.Sprintf("client%d", clientID)
			go func(client string) {
				defer wg.Done()
				for i := 0; i < transactions/10; i++ {
					rl.LimitRate(client)
				}
			}(client)

		}

		wg.Wait()

		elapsed := clock.Since(start)

		TPS := float64(transactions / int(elapsed.Seconds()))

		max := float64(500) * 1.3
		min := float64(500) * 0.7

		assert.Less(t, TPS, max)
		assert.Less(t, min, TPS)
	})

	// Next, wait two seconds and then run only a single client.
	// should have all the quota available to itself.
	clock.Sleep(time.Second * 2)

	t.Run("", func(t *testing.T) {
		transactions := 10000

		start := clock.Now()

		for i := 0; i < transactions; i++ {
			rl.LimitRate("alice")
		}
		elapsed := clock.Since(start)

		TPS := float64(transactions / int(elapsed.Seconds()))

		max := float64(500) * 1.3
		min := float64(500) * 0.7

		assert.Less(t, TPS, max)
		assert.Less(t, min, TPS)
	})
}

func advanceTime(stop chan struct{}, clock *clock.FakeClock) {
	func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond * 100):
				clock.Increment(time.Second)
			}
		}
	}()
}
