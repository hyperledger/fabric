/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package blocksprovider

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func SetSleeper(d *Deliverer, sleeper customSleeper) {
	d.sleeper.sleep = sleeper.Sleep
}

type testSleeper struct {
	c int32
}

func (s *testSleeper) Sleep(duration time.Duration) {
	atomic.AddInt32(&s.c, 1)
}

func TestSleeper(t *testing.T) {
	t.Run("it sleeps", func(t *testing.T) {
		s := sleeper{}
		wg := sync.WaitGroup{}
		wg.Add(1)
		doneC := make(chan struct{})

		go func() {
			s.Sleep(time.Millisecond, doneC)
			wg.Done()
		}()

		wg.Wait()
	})

	t.Run("it is stoppable", func(t *testing.T) {
		s := sleeper{}
		wg := sync.WaitGroup{}
		wg.Add(1)
		doneC := make(chan struct{})

		go func() {
			s.Sleep(24*time.Hour, doneC)
			wg.Done()
		}()

		close(doneC)
		wg.Wait()
	})

	t.Run("it can use a custom sleeper", func(t *testing.T) {
		d := &Deliverer{}
		s := &testSleeper{}
		SetSleeper(d, s)
		doneC := make(chan struct{})

		go func() {
			for i := 0; i < 10; i++ {
				d.sleeper.Sleep(time.Millisecond, doneC)
			}
		}()

		require.Eventually(t,
			func() bool {
				return atomic.LoadInt32(&s.c) == 10
			},
			10*time.Second, time.Millisecond)
	})
}
