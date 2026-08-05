/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

// EnvChaincodeShimSpans turns off the span emitted for each callback a chaincode
// makes back into the peer.
//
// These are on by default because they are usually the answer to why a
// transaction is slow: a contract that issues several hundred GetState calls
// looks, from the endorsement span alone, simply like slow chaincode. The
// sampler is the volume control — shim spans inherit the sampling decision of
// the transaction that caused them, so a ratio-based sampler bounds them the
// same way it bounds everything else.
//
// The escape hatch exists for the case that a single sampled transaction fans
// out to thousands of state reads, where the spans stop being readable and start
// being a bill. The aggregate view survives either way, since Fabric already
// meters shim requests by type, channel and chaincode.
const EnvChaincodeShimSpans = "FABRIC_TRACE_CHAINCODE_SHIM"

// maxFunctionNameLength bounds what is accepted as a chaincode function name.
// Real function names are short; anything longer is a caller passing data in the
// first argument rather than a function name, and does not belong in a span.
const maxFunctionNameLength = 128

// shimSpans caches the resolved value of EnvChaincodeShimSpans.
//
// This is read once per callback a chaincode makes into the peer, which for a
// query-heavy contract is the busiest path in the process. os.Getenv is a linear
// scan of the environment, so reading it there would make every state access pay
// for a lookup whose answer cannot change while the node is running.
var shimSpans atomic.Bool

// resolveChaincodeShimSpans reads the switch from the environment. Called from
// Initialize, and from tests that need to change it.
func resolveChaincodeShimSpans() {
	raw := strings.TrimSpace(os.Getenv(EnvChaincodeShimSpans))
	if raw == "" {
		shimSpans.Store(true)
		return
	}

	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warnw("Invalid "+EnvChaincodeShimSpans+", defaulting to enabled", "value", raw)
		shimSpans.Store(true)
		return
	}
	shimSpans.Store(enabled)
}

// ChaincodeShimSpansEnabled reports whether per-callback spans should be emitted.
func ChaincodeShimSpansEnabled() bool {
	return Enabled() && shimSpans.Load()
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
