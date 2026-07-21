/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package policydsl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Regression tests for a stack-overflow DoS in the policy DSL parser
// (deeply nested AND/OR/OutOf expressions). The parser previously used
// github.com/Knetic/govaluate, whose recursive-descent planner had no
// depth or size limit and could exhaust the goroutine stack on
// maliciously nested input. FromString now compiles via
// github.com/expr-lang/expr, which enforces conf.DefaultMaxNodes (1e4)
// during parsing, rejecting oversized expressions before recursion
// depth can approach the stack limit. These tests pin that behavior.

// nestedGate builds `GATE('A.member', GATE('A.member', ... 'A.member'))`
// nested `depth` times.
func nestedGate(gate string, depth int) string {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString(gate)
		b.WriteString("('A.member', ")
	}
	b.WriteString("'A.member'")
	for i := 0; i < depth; i++ {
		b.WriteString(")")
	}
	return b.String()
}

func TestFromStringDeepNestingDoesNotOverflowStack(t *testing.T) {
	for _, gate := range []string{"AND", "OR"} {
		for _, depth := range []int{1000, 5000, 20000, 200000} {
			gate, depth := gate, depth
			t.Run(fmt.Sprintf("%s_depth_%d", gate, depth), func(t *testing.T) {
				// A stack overflow is an unrecoverable fatal error that kills
				// the test binary; simply returning (err or not) is proof
				// the input did not blow the stack.
				env, err := FromString(nestedGate(gate, depth))
				if err != nil {
					require.Contains(t, err.Error(), "exceeds maximum allowed nodes")
					return
				}
				require.Len(t, env.Identities, depth+1)
			})
		}
	}
}

func TestFromStringNodeLimitIsStructuralNotLengthBased(t *testing.T) {
	// A long but shallow (wide) policy well beyond naive byte-length
	// thresholds must still parse: the cap is on AST node count, not
	// input length.
	const n = 2000
	var b strings.Builder
	b.WriteString("OutOf(1")
	for i := 0; i < n; i++ {
		b.WriteString(", 'A.member'")
	}
	b.WriteString(")")

	env, err := FromString(b.String())
	require.NoError(t, err)
	require.Len(t, env.Identities, n)
	require.Len(t, env.Rule.GetNOutOf().GetRules(), n)
}

func TestFromStringMalformedInputNeverPanics(t *testing.T) {
	cases := []string{
		"",
		"AND(",
		"AND('A.member'",
		"NOPE('A.member')",
		"AND('A.member',)",
		"AND('A.member', 'B.member'))))))",
		strings.Repeat("(", 100000),
		strings.Repeat("AND(", 100000),
		"'" + strings.Repeat("A", 1000000) + ".member'",
	}
	for i, c := range cases {
		i, c := i, c
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			require.NotPanics(t, func() {
				_, _ = FromString(c)
			})
		})
	}
}
