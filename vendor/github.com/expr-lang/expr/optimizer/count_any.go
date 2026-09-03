package optimizer

import (
	. "github.com/expr-lang/expr/ast"
)

// countAny optimizes count comparisons to use any for early termination.
// Patterns:
//   - count(arr, pred) > 0  → any(arr, pred)
//   - count(arr, pred) >= 1 → any(arr, pred)
type countAny struct{}

func (*countAny) Visit(node *Node) {
	binary, ok := (*node).(*BinaryNode)
	if !ok {
		return
	}

	count, ok := binary.Left.(*BuiltinNode)
	if !ok || count.Name != "count" || len(count.Arguments) != 2 {
		return
	}

	integer, ok := binary.Right.(*IntegerNode)
	if !ok {
		return
	}

	if (binary.Operator == ">" && integer.Value == 0) ||
		(binary.Operator == ">=" && integer.Value == 1) {
		patchCopyType(node, &BuiltinNode{
			Name:      "any",
			Arguments: count.Arguments,
		})
	}
}
