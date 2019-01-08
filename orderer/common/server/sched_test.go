/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExponentialDuration(t *testing.T) {
	t.Parallel()
	exp := exponentialDurationSeries(time.Millisecond*100, time.Second)
	prev := exp()
	for i := 0; i < 3; i++ {
		n := exp()
		assert.Equal(t, prev*2, n)
		prev = n
		assert.True(t, n < time.Second)
	}

	for i := 0; i < 10; i++ {
		assert.Equal(t, time.Second, exp())
	}
}

func TestTicker(t *testing.T) {
	t.Parallel()
	everyMillis := func() time.Duration {
		return time.Millisecond
	}

	t.Run("Stop ticker serially", func(t *testing.T) {
		ticker := newTicker(everyMillis)
		for i := 0; i < 10; i++ {
			<-ticker.C
		}

		ticker.stop()
		// Ensure no more ticks are received
		_, ok := <-ticker.C
		assert.False(t, ok)
	})

	t.Run("Stop ticker concurrently", func(t *testing.T) {
		ticker := newTicker(func() time.Duration {
			return time.Millisecond
		})

		var tickerStopped sync.WaitGroup
		tickerStopped.Add(1)

		go func() {
			defer tickerStopped.Done()
			time.Sleep(time.Millisecond * 50)
			ticker.stop()
			<-ticker.C
		}()

		tickerStopped.Wait()
		_, ok := <-ticker.C
		assert.False(t, ok)

	})

}
