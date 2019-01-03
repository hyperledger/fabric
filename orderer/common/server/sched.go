/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import "time"

type durationSeries func() time.Duration

// ticker has a channel that will send the
// time with intervals computed by the durationSeries.
type ticker struct {
	stopped      bool
	C            <-chan time.Time
	nextInterval durationSeries
	stopChan     chan struct{}
}

// newTicker returns a channel that sends the time at periods
// specified by the given durationSeries.
func newTicker(nextInterval durationSeries) *ticker {
	c := make(chan time.Time)
	ticker := &ticker{
		stopChan:     make(chan struct{}),
		C:            c,
		nextInterval: nextInterval,
	}

	go func() {
		defer close(c)
		ticker.run(c)
	}()

	return ticker
}

func (t *ticker) run(c chan<- time.Time) {
	for {
		if t.stopped {
			return
		}
		select {
		case <-time.After(t.nextInterval()):
			t.tick(c)
		case <-t.stopChan:
			return
		}
	}
}

func (t *ticker) tick(c chan<- time.Time) {
	select {
	case c <- time.Now():
	case <-t.stopChan:
		t.stopped = true
	}
}

func (t *ticker) stop() {
	close(t.stopChan)
}

func exponentialDurationSeries(initialDuration, maxDuration time.Duration) func() time.Duration {
	exp := &exponentialDuration{
		n:   initialDuration,
		max: maxDuration,
	}
	return exp.next
}

type exponentialDuration struct {
	n   time.Duration
	max time.Duration
}

func (exp *exponentialDuration) next() time.Duration {
	n := exp.n
	exp.n *= 2
	if exp.n > exp.max {
		exp.n = exp.max
	}
	return n
}
