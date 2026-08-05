/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"os"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// EnvChaincodeShimSpans selects how much detail is recorded about the callbacks
// a chaincode makes back into the peer.
//
//	aggregate  counts and durations per callback type, attached to the
//	           enclosing execution span (default)
//	spans      one span per callback
//	off        nothing
//
// The default is aggregate because it answers the question that actually gets
// asked — is this contract doing too much I/O, and how much of the transaction
// did it account for — at one span per transaction rather than one per state
// access. A contract that reads five hundred keys is the difference between a
// handful of attributes and five hundred spans to create, batch and export.
//
// Detailed spans remain available for when the totals are not enough and the
// individual timeline is needed, which is a deliberate investigation rather
// than something worth paying for continuously.
const EnvChaincodeShimSpans = "FABRIC_TRACE_CHAINCODE_SHIM"

// ShimMode is how much is recorded about chaincode callbacks.
type ShimMode int

const (
	// ShimAggregate records per-type counts and durations on the execution span.
	ShimAggregate ShimMode = iota
	// ShimSpans additionally emits a span per callback.
	ShimSpans
	// ShimOff records nothing.
	ShimOff
)

// maxFunctionNameLength bounds what is accepted as a chaincode function name.
// Real function names are short; anything longer is a caller passing data in the
// first argument rather than a function name, and does not belong in a span.
const maxFunctionNameLength = 128

// shimMode caches the resolved value of EnvChaincodeShimSpans.
//
// This is read once per callback a chaincode makes into the peer, which for a
// query-heavy contract is the busiest path in the process. os.Getenv is a linear
// scan of the environment, so reading it there would make every state access pay
// for a lookup whose answer cannot change while the node is running.
var shimMode atomic.Int32

// resolveChaincodeShimSpans reads the switch from the environment. Called from
// Initialize, and from tests that need to change it.
//
// Boolean values are still accepted, since that is what the switch originally
// took: true selects the detailed spans it used to mean, false selects off.
func resolveChaincodeShimSpans() {
	switch raw := strings.ToLower(strings.TrimSpace(os.Getenv(EnvChaincodeShimSpans))); raw {
	case "", "aggregate":
		shimMode.Store(int32(ShimAggregate))
	case "spans", "true", "1":
		shimMode.Store(int32(ShimSpans))
	case "off", "false", "0":
		shimMode.Store(int32(ShimOff))
	default:
		logger.Warnw("Unrecognized "+EnvChaincodeShimSpans+", defaulting to aggregate", "value", raw)
		shimMode.Store(int32(ShimAggregate))
	}
}

// ChaincodeShimMode reports how much detail to record about chaincode callbacks.
// It is ShimOff whenever telemetry is not exporting.
func ChaincodeShimMode() ShimMode {
	if !Enabled() {
		return ShimOff
	}
	return ShimMode(shimMode.Load())
}

// ChaincodeFunctionName extracts the invoked function from a chaincode's
// arguments.
//
// Every Fabric contract API puts the function name in the first argument, but
// nothing in the protocol enforces that, so the value here is attacker-supplied
// and arrives straight off the wire. It is only accepted when it looks like a
// function name: valid UTF-8, short, and free of control characters that would
// corrupt a backend's display. Anything else yields an empty string and the
// attribute is simply omitted.
//
// Only the first argument is ever considered. The remaining arguments are the
// transaction's business data and are deliberately never recorded.
func ChaincodeFunctionName(args [][]byte) string {
	if len(args) == 0 {
		return ""
	}

	name := string(args[0])
	if name == "" || len(name) > maxFunctionNameLength || !utf8.ValidString(name) {
		return ""
	}
	for _, r := range name {
		// Control characters, including newlines and NUL, are not part of any
		// real function name.
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return name
}
