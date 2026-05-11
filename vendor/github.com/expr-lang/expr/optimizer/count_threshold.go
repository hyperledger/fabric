package optimizer

import (
	. "github.com/expr-lang/expr/ast"
)

// countThreshold optimizes count comparisons by setting a threshold for early termination.
// The threshold allows the count loop to exit early once enough matches are found.
// Patterns:
//   - count(arr, pred) > N  → threshold = N + 1 (exit proves > N is true)
//   - count(arr, pred) >= N → threshold = N (exit proves >= N is true)
//   - count(arr, pred) < N  → threshold = N (exit proves < N is false)
//   - count(arr, pred) <= N → threshold = N + 1 (exit proves <= N is false)
type countThreshold struct{}

func (*countThreshold) Visit(node *Node) {
	binary, ok := (*node).(*BinaryNode)
	if !ok {
		return
	}

	count, ok := binary.Left.(*BuiltinNode)
	if !ok || count.Name != "count" || len(count.Arguments) != 2 {
		return
	}

	integer, ok := binary.Right.(*IntegerNode)
	if !ok || integer.Value < 0 {
		return
	}

	var threshold int
	switch binary.Operator {
	case ">":
		threshold = integer.Value + 1
	case ">=":
		threshold = integer.Value
	case "<":
		threshold = integer.Value
	case "<=":
		threshold = integer.Value + 1
	default:
		return
	}

	// Skip if threshold is 0 or 1 (handled by count_any optimizer)
	if threshold <= 1 {
		return
	}

	// Set threshold on the count node for early termination
	// The original comparison remains unchanged
	count.Threshold = &threshold
}
