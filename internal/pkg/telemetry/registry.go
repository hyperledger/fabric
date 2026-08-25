/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Default bounds for the registry. A transaction normally reaches commit within
// a block cutting interval of being endorsed, so a couple of minutes is
// generous; the size cap is what actually protects the peer if an ordering
// service stalls and nothing commits for a while.
const (
	DefaultRegistryTTL        = 2 * time.Minute
	DefaultRegistryMaxEntries = 50000
)

// SpanContextRegistry remembers the trace context a transaction was endorsed
// under, so that the commit spans produced later can be linked back to it.
//
// This exists because of a structural mismatch in Fabric: endorsement is a
// per-transaction request/response and carries the client's trace context
// natively over gRPC, but commit is per *block*. A block aggregates
// transactions from many unrelated clients and arrives asynchronously, so there
// is no ambient context to continue and no single parent span a block could
// belong to. The registry lets a peer that endorsed a transaction recover its
// originating trace when the transaction comes back around in a block.
//
// It is deliberately lossy. An entry is dropped once it ages out, and a peer
// that did not endorse a transaction never had the context to begin with, which
// is the normal case on a large network. Missing entries cost a link, never
// correctness, and the fabric.tx_id attribute still allows the two traces to be
// joined at query time. For a link that survives on every peer, the trace
// context has to travel inside the signed transaction itself; see
// TraceContextFromHeaderExtension.
//
// The implementation is a two-generation map rotated on a timer rather than a
// per-entry expiry, which keeps both Remember and Lookup O(1) with no
// background goroutine and no per-entry timestamp.
type SpanContextRegistry struct {
	mu         sync.Mutex
	current    map[string]trace.SpanContext
	previous   map[string]trace.SpanContext
	rotateAt   time.Time
	ttl        time.Duration
	maxEntries int

	// now is overridable in tests.
	now func() time.Time
}

// NewSpanContextRegistry returns a registry retaining entries for at least ttl
// and at most 2*ttl, bounded to maxEntries per generation. Non-positive
// arguments fall back to the defaults.
func NewSpanContextRegistry(ttl time.Duration, maxEntries int) *SpanContextRegistry {
	if ttl <= 0 {
		ttl = DefaultRegistryTTL
	}
	if maxEntries <= 0 {
		maxEntries = DefaultRegistryMaxEntries
	}
	r := &SpanContextRegistry{
		current:    make(map[string]trace.SpanContext),
		previous:   make(map[string]trace.SpanContext),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}
	r.rotateAt = r.now().Add(ttl)
	return r
}

// Remember records the trace context a transaction was endorsed under. Unsampled
// and invalid contexts are ignored: they would produce links that no backend can
// resolve.
func (r *SpanContextRegistry) Remember(txID string, sc trace.SpanContext) {
	if r == nil || txID == "" || !sc.IsValid() || !sc.IsSampled() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rotateLocked()
	r.current[txID] = sc
}

// Lookup returns the trace context a transaction was endorsed under, if this
// peer endorsed it recently.
func (r *SpanContextRegistry) Lookup(txID string) (trace.SpanContext, bool) {
	if r == nil || txID == "" {
		return trace.SpanContext{}, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rotateLocked()
	if sc, ok := r.current[txID]; ok {
		return sc, true
	}
	sc, ok := r.previous[txID]
	return sc, ok
}

// Forget drops a transaction's entry. Commit calls this so that a busy peer
// reclaims memory as transactions finalize rather than waiting for rotation.
func (r *SpanContextRegistry) Forget(txID string) {
	if r == nil || txID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.current, txID)
	delete(r.previous, txID)
}

// rotateLocked ages out the older generation once the TTL has elapsed, or
// early if the current generation has hit its size cap. The caller must hold
// r.mu.
func (r *SpanContextRegistry) rotateLocked() {
	if r.now().Before(r.rotateAt) && len(r.current) < r.maxEntries {
		return
	}
	r.previous = r.current
	r.current = make(map[string]trace.SpanContext)
	r.rotateAt = r.now().Add(r.ttl)
}
