/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package telemetry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The first argument is attacker-controlled and arrives straight off the wire,
// so anything that is not plausibly a function name has to be rejected rather
// than forwarded to a telemetry backend as a span attribute.
func TestChaincodeFunctionName(t *testing.T) {
	for name, tc := range map[string]struct {
		args     [][]byte
		expected string
	}{
		"typical invocation": {
			args:     [][]byte{[]byte("CreateAsset"), []byte("asset1"), []byte("blue")},
			expected: "CreateAsset",
		},
		"function only": {
			args:     [][]byte{[]byte("GetAllAssets")},
			expected: "GetAllAssets",
		},
		"no arguments": {
			args:     nil,
			expected: "",
		},
		"empty first argument": {
			args:     [][]byte{{}},
			expected: "",
		},
		"invalid utf8": {
			args:     [][]byte{{0xff, 0xfe, 0xfd}},
			expected: "",
		},
		// A newline would let a caller forge extra lines in anything that
		// renders span attributes as text.
		"embedded newline": {
			args:     [][]byte{[]byte("Create\nAsset")},
			expected: "",
		},
		"embedded nul": {
			args:     [][]byte{[]byte("Create\x00Asset")},
			expected: "",
		},
		"tab": {
			args:     [][]byte{[]byte("Create\tAsset")},
			expected: "",
		},
		// Data passed in the first argument instead of a function name must not
		// become an unbounded span attribute.
		"over length limit": {
			args:     [][]byte{[]byte(strings.Repeat("a", maxFunctionNameLength+1))},
			expected: "",
		},
		"at length limit": {
			args:     [][]byte{[]byte(strings.Repeat("a", maxFunctionNameLength))},
			expected: strings.Repeat("a", maxFunctionNameLength),
		},
		"unicode function name": {
			args:     [][]byte{[]byte("créerActif")},
			expected: "créerActif",
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, ChaincodeFunctionName(tc.args))
		})
	}
}

// Shim spans are pointless when nothing is exporting, and the check has to be
// cheap because it runs on every callback a chaincode makes.
func TestChaincodeShimSpansDisabledWhenTelemetryIsOff(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv(EnvChaincodeShimSpans, "true")

	require.False(t, Enabled())
	require.False(t, ChaincodeShimSpansEnabled())
}

func TestChaincodeShimSpansOptOut(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")

	shutdown, err := Initialize(t.Context(), Config{ServiceName: "fabric-peer"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(t.Context()) })

	// The switch is resolved at startup rather than per callback, so tests have
	// to re-resolve it the way Initialize would.
	setShimSpans := func(value string) {
		t.Setenv(EnvChaincodeShimSpans, value)
		resolveChaincodeShimSpans()
	}

	// Enabled by default, because a chaincode's state access is usually what
	// explains a slow transaction.
	setShimSpans("")
	require.True(t, ChaincodeShimSpansEnabled())

	setShimSpans("false")
	require.False(t, ChaincodeShimSpansEnabled())

	setShimSpans("true")
	require.True(t, ChaincodeShimSpansEnabled())

	// A typo must not silently switch off the spans someone was relying on.
	setShimSpans("yes-please")
	require.True(t, ChaincodeShimSpansEnabled())
}

// The gRPC stats handler is consulted on every inbound RPC, and the peer and
// orderer multiplex gossip and Raft traffic onto the same servers as
// transactions. With tracing off it must not be installed at all.
func TestServerHandlerNotInstalledWhenDisabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	require.False(t, Enabled())
	require.Nil(t, ServerHandler())
}

func TestServerHandlerInstalledWhenEnabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")

	shutdown, err := Initialize(t.Context(), Config{ServiceName: "fabric-peer"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(t.Context()) })

	require.NotNil(t, ServerHandler())
}
