/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// endorsements is the process-wide record of which trace each transaction was
// endorsed under.
//
// This is deliberately global, in the same way the OpenTelemetry TracerProvider
// is. The alternative is threading a registry from peer startup through the
// endorser and, separately, all the way down the gossip and committer path to
// where blocks are finally written, which would mean changing a long chain of
// constructors that upstream owns. Every one of those changes is a conflict to
// resolve on each rebase, for a value that is genuinely process-scoped and is
// only ever read by instrumentation. Keeping it here confines the fork's
// footprint to the handful of call sites that actually create spans.
var endorsements = NewSpanContextRegistry(DefaultRegistryTTL, DefaultRegistryMaxEntries)

// RememberEndorsement records the trace a transaction was endorsed under, so
// that the commit spans emitted for it later can link back to the client
// request that produced it.
//
// It is a no-op when tracing is disabled, when the transaction is not sampled,
// or when the context carries no span, so call sites do not need to guard it.
func RememberEndorsement(ctx context.Context, txID string) {
	if !Enabled() {
		return
	}
	endorsements.Remember(txID, trace.SpanContextFromContext(ctx))
}

// LookupEndorsement returns the trace a transaction was endorsed under, if this
// peer endorsed it recently enough to still remember.
//
// A miss is ordinary rather than exceptional: peers commit many transactions
// they never endorsed. Callers should fall back to the trace context carried in
// the transaction itself, and settle for the fabric.tx_id attribute if that is
// absent too.
func LookupEndorsement(txID string) (trace.SpanContext, bool) {
	if !Enabled() {
		return trace.SpanContext{}, false
	}
	return endorsements.Lookup(txID)
}

// ForgetEndorsement drops a transaction's entry once it has been committed and
// the link has been made, so that a busy peer reclaims the memory promptly
// instead of waiting for it to age out.
func ForgetEndorsement(txID string) {
	endorsements.Forget(txID)
}
