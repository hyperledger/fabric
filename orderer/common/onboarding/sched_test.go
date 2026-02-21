/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package onboarding

import (
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
)

func TestExponentialDuration(t *testing.T) {
	t.Parallel()
	exp := exponentialDurationSeries(time.Millisecond*100, time.Second)
	prev := exp()
	for range 3 {
		n := exp()
		require.Equal(t, prev*2, n)
		prev = n
		require.True(t, n < time.Second)
	}

	for range 10 {
		require.Equal(t, time.Second, exp())
	}
}

func TestTicker(t *testing.T) {
	t.Parallel()
	everyMillis := func() time.Duration {
		return time.Millisecond
	}

	t.Run("Stop ticker serially", func(t *testing.T) {
		ticker := newTicker(everyMillis)
		for range 10 {
			<-ticker.C
		}

		ticker.stop()
		// Ensure the ticker channel is closed once stop() is called.
		gt := gomega.NewGomegaWithT(t)
		gt.Eventually(func() bool {
			_, ok := <-ticker.C
			return ok
		}).Should(gomega.BeFalse())
	})

	t.Run("Stop ticker concurrently", func(t *testing.T) {
		ticker := newTicker(func() time.Duration {
			return time.Millisecond
		})

		var tickerStopped sync.WaitGroup

		tickerStopped.Go(func() {
			time.Sleep(time.Millisecond * 50)
			ticker.stop()
			<-ticker.C
		})

		tickerStopped.Wait()
		_, ok := <-ticker.C
		require.False(t, ok)
	})
}
