package core

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// If given New with 0's and no jitter, ensure that certain invariants are met:
//
//   - the default max duration and interval should be used
//   - noJitter should be true
//   - the RNG should not be initialised
//   - the first duration should be equal to the default interval
func TestDefaults(t *testing.T) {
	b := NewWithoutJitter(0, 0)

	if b.maxDuration != DefaultMaxDuration {
		t.Fatalf("expected new backoff to use the default max duration (%s), but have %s", DefaultMaxDuration, b.maxDuration)
	}

	if b.interval != DefaultInterval {
		t.Fatalf("exepcted new backoff to use the default interval (%s), but have %s", DefaultInterval, b.interval)
	}

	if b.noJitter != true {
		t.Fatal("backoff should have been initialised without jitter")
	}

	dur := b.Duration()
	if dur != DefaultInterval {
		t.Fatalf("expected first duration to be %s, have %s", DefaultInterval, dur)
	}
}

// Given a zero-value initialised Backoff, it should be transparently
// setup.
func TestSetup(t *testing.T) {
	b := new(Backoff)
	dur := b.Duration()
	if dur < 0 || dur > (5*time.Minute) {
		t.Fatalf("want duration between 0 and 5 minutes, have %s", dur)
	}
}

// Ensure that tries incremenets as expected.
func TestTries(t *testing.T) {
	b := NewWithoutJitter(5, 1)

	for i := uint64(0); i < 3; i++ {
		if b.n != i {
			t.Fatalf("want tries=%d, have tries=%d", i, b.n)
		} else if b.n != i {
			t.Fatalf("want tries=%d, have tries=%d", i, b.n)
		}

		pow := 1 << i
		expected := time.Duration(pow)
		dur := b.Duration()
		if dur != expected {
			t.Fatalf("want duration=%d, have duration=%d at i=%d", expected, dur, i)
		}
	}

	for i := uint(3); i < 5; i++ {
		dur := b.Duration()
		if dur != 5 {
			t.Fatalf("want duration=5, have %d at i=%d", dur, i)
		}
	}
}

// Ensure that a call to Reset will actually reset the Backoff.
func TestReset(t *testing.T) {
	const iter = 10
	b := New(1000, 1)
	for i := 0; i < iter; i++ {
		_ = b.Duration()
	}

	if b.n != iter {
		t.Fatalf("expected tries=%d, have tries=%d", iter, b.n)
	}

	b.Reset()
	if b.n != 0 {
		t.Fatalf("expected tries=0 after reset, have tries=%d", b.n)
	}
}

const decay = 5 * time.Millisecond
const max = 10 * time.Millisecond
const interval = time.Millisecond

func TestDecay(t *testing.T) {
	const iter = 10

	b := NewWithoutJitter(max, 1)
	b.SetDecay(decay)

	var backoff time.Duration
	for i := 0; i < iter; i++ {
		backoff = b.Duration()
	}

	if b.n != iter {
		t.Fatalf("expected tries=%d, have tries=%d", iter, b.n)
	}

	// Don't decay below backoff
	b.lastTry = time.Now().Add(-backoff + 1)
	backoff = b.Duration()
	if b.n != iter+1 {
		t.Fatalf("expected tries=%d, have tries=%d", iter+1, b.n)
	}

	// Reset after backoff + decay
	b.lastTry = time.Now().Add(-backoff - decay)
	b.Duration()
	if b.n != 1 {
		t.Fatalf("expected tries=%d, have tries=%d", 1, b.n)
	}
}

// Ensure that decay works even if the retry counter is saturated.
func TestDecaySaturation(t *testing.T) {
	b := NewWithoutJitter(1<<2, 1)
	b.SetDecay(decay)

	var duration time.Duration
	for i := 0; i <= 2; i++ {
		duration = b.Duration()
	}

	if duration != 1<<2 {
		t.Fatalf("expected duration=%v, have duration=%v", 1<<2, duration)
	}

	b.lastTry = time.Now().Add(-duration - decay)
	b.n = math.MaxUint64

	duration = b.Duration()
	if duration != 1 {
		t.Errorf("expected duration=%v, have duration=%v", 1, duration)
	}
}

func ExampleBackoff_SetDecay() {
	b := NewWithoutJitter(max, interval)
	b.SetDecay(decay)

	// try 0
	fmt.Println(b.Duration())

	// try 1
	fmt.Println(b.Duration())

	// try 2
	duration := b.Duration()
	fmt.Println(duration)

	// try 3, below decay
	time.Sleep(duration)
	duration = b.Duration()
	fmt.Println(duration)

	// try 4, resets
	time.Sleep(duration + decay)
	fmt.Println(b.Duration())

	// Output: 1ms
	// 2ms
	// 4ms
	// 8ms
	// 1ms
}
