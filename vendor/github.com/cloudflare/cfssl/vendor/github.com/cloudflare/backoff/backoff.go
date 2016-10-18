// Package backoff contains an implementation of an intelligent backoff
// strategy. It is based on the approach in the AWS architecture blog
// article titled "Exponential Backoff And Jitter", which is found at
// http://www.awsarchitectureblog.com/2015/03/backoff.html.
//
// Essentially, the backoff has an interval `time.Duration`; the nth
// call to backoff will return a `time.Duration` that is 2^n *
// interval. If jitter is enabled (which is the default behaviour),
// the duration is a random value between 0 and 2^n * interval.  The
// backoff is configured with a maximum duration that will not be
// exceeded.
//
// The `New` function will attempt to use the system's cryptographic
// random number generator to seed a Go math/rand random number
// source. If this fails, the package will panic on startup.
package backoff

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"math"
	mrand "math/rand"
	"sync"
	"time"
)

var prngMu sync.Mutex
var prng *mrand.Rand

// DefaultInterval is used when a Backoff is initialised with a
// zero-value Interval.
var DefaultInterval = 5 * time.Minute

// DefaultMaxDuration is maximum amount of time that the backoff will
// delay for.
var DefaultMaxDuration = 6 * time.Hour

// A Backoff contains the information needed to intelligently backoff
// and retry operations using an exponential backoff algorithm. It should
// be initialised with a call to `New`.
//
// Only use a Backoff from a single goroutine, it is not safe for concurrent
// access.
type Backoff struct {
	// maxDuration is the largest possible duration that can be
	// returned from a call to Duration.
	maxDuration time.Duration

	// interval controls the time step for backing off.
	interval time.Duration

	// noJitter controls whether to use the "Full Jitter"
	// improvement to attempt to smooth out spikes in a high
	// contention scenario. If noJitter is set to true, no
	// jitter will be introduced.
	noJitter bool

	// decay controls the decay of n. If it is non-zero, n is
	// reset if more than the last backoff + decay has elapsed since
	// the last try.
	decay time.Duration

	n       uint64
	lastTry time.Time
}

// New creates a new backoff with the specified max duration and
// interval. Zero values may be used to use the default values.
//
// Panics if either max or interval is negative.
func New(max time.Duration, interval time.Duration) *Backoff {
	if max < 0 || interval < 0 {
		panic("backoff: max or interval is negative")
	}

	b := &Backoff{
		maxDuration: max,
		interval:    interval,
	}
	b.setup()
	return b
}

// NewWithoutJitter works similarly to New, except that the created
// Backoff will not use jitter.
func NewWithoutJitter(max time.Duration, interval time.Duration) *Backoff {
	b := New(max, interval)
	b.noJitter = true
	return b
}

func init() {
	var buf [8]byte
	var n int64

	_, err := io.ReadFull(rand.Reader, buf[:])
	if err != nil {
		panic(err.Error())
	}

	n = int64(binary.LittleEndian.Uint64(buf[:]))

	src := mrand.NewSource(n)
	prng = mrand.New(src)
}

func (b *Backoff) setup() {
	if b.interval == 0 {
		b.interval = DefaultInterval
	}

	if b.maxDuration == 0 {
		b.maxDuration = DefaultMaxDuration
	}
}

// Duration returns a time.Duration appropriate for the backoff,
// incrementing the attempt counter.
func (b *Backoff) Duration() time.Duration {
	b.setup()

	b.decayN()

	t := b.duration(b.n)

	if b.n < math.MaxUint64 {
		b.n++
	}

	if !b.noJitter {
		prngMu.Lock()
		t = time.Duration(prng.Int63n(int64(t)))
		prngMu.Unlock()
	}

	return t
}

// requires b to be locked.
func (b *Backoff) duration(n uint64) (t time.Duration) {
	// Saturate pow
	pow := time.Duration(math.MaxInt64)
	if n < 63 {
		pow = 1 << n
	}

	t = b.interval * pow
	if t/pow != b.interval || t > b.maxDuration {
		t = b.maxDuration
	}

	return
}

// Reset resets the attempt counter of a backoff.
//
// It should be called when the rate-limited action succeeds.
func (b *Backoff) Reset() {
	b.lastTry = time.Time{}
	b.n = 0
}

// SetDecay sets the duration after which the try counter will be reset.
// Panics if decay is smaller than 0.
//
// The decay only kicks in if at least the last backoff + decay has elapsed
// since the last try.
func (b *Backoff) SetDecay(decay time.Duration) {
	if decay < 0 {
		panic("backoff: decay < 0")
	}

	b.decay = decay
}

// requires b to be locked
func (b *Backoff) decayN() {
	if b.decay == 0 {
		return
	}

	if b.lastTry.IsZero() {
		b.lastTry = time.Now()
		return
	}

	lastDuration := b.duration(b.n - 1)
	decayed := time.Since(b.lastTry) > lastDuration+b.decay
	b.lastTry = time.Now()

	if !decayed {
		return
	}

	b.n = 0
}
