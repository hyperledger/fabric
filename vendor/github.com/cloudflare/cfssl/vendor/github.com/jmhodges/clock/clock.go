// Package clock provides an abstraction for system time that enables
// testing of time-sensitive code.
//
// Where you'd use time.Now, instead use clk.Now where clk is an
// instance of Clock.
//
// When running your code in production, pass it a Clock given by
// Default() and when you're running it in your tests, pass it an
// instance of Clock from NewFake().
//
// When you do that, you can use FakeClock's Add and Set methods to
// control how time behaves in your code making them more reliable
// while also expanding the space of problems you can test.
//
// This code intentionally does not attempt to provide an abstraction
// over time.Ticker and time.Timer because Go does not have the
// runtime or API hooks available to do reliably. See
// https://github.com/golang/go/issues/8869
//
// Be sure to test Time equality with time.Time#Equal, not ==.
package clock

import (
	"sync"
	"time"
)

var systemClock Clock = sysClock{}

// Default returns a Clock that matches the actual system time.
func Default() Clock {
	// This is a method instead of a public var to prevent folks from
	// "making things work" by writing to the var instead of passing
	// in a Clock.
	return systemClock
}

// Clock is an abstraction over system time. New instances of it can
// be made with Default and NewFake.
type Clock interface {
	// Now returns the Clock's current view of the time. Mutating the
	// returned Time will not mutate the clock's time.
	Now() time.Time
	Sleep(time.Duration)
}

type sysClock struct{}

func (s sysClock) Now() time.Time {
	return time.Now()
}

func (s sysClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// NewFake returns a FakeClock to be used in tests that need to
// manipulate time. Its initial value is always the unix epoch in the
// UTC timezone. The FakeClock returned is thread-safe.
func NewFake() FakeClock {
	// We're explicit about this time construction to avoid early user
	// questions about why the time object doesn't have a Location by
	// default.
	return &fake{t: time.Unix(0, 0).UTC()}
}

// FakeClock is a Clock with additional controls. The return value of
// Now return can be modified with Add. Use NewFake to get a
// thread-safe FakeClock implementation.
type FakeClock interface {
	Clock
	// Adjust the time that will be returned by Now.
	Add(d time.Duration)

	// Set the Clock's time to exactly the time given.
	Set(t time.Time)
}

// To prevent mistakes with the API, we hide this behind NewFake. It's
// easy forget to create a pointer to a fake since time.Time (and
// sync.Mutex) are also simple values. The code will appear to work
// but the clock's time will never be adjusted.
type fake struct {
	sync.RWMutex
	t time.Time
}

func (f *fake) Now() time.Time {
	f.RLock()
	defer f.RUnlock()
	return f.t
}

func (f *fake) Sleep(d time.Duration) {
	if d < 0 {
		// time.Sleep just returns immediately. Do the same.
		return
	}
	f.Add(d)
}

func (f *fake) Add(d time.Duration) {
	f.Lock()
	defer f.Unlock()
	f.t = f.t.Add(d)
}

func (f *fake) Set(t time.Time) {
	f.Lock()
	defer f.Unlock()
	f.t = t
}
