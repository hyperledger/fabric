package optimizer

import (
	. "github.com/expr-lang/expr/ast"
)

type sumRange struct{}

func (*sumRange) Visit(node *Node) {
	// Pattern 1: sum(m..n) or sum(m..n, predicate) where m and n are constant integers
	if sumBuiltin, ok := (*node).(*BuiltinNode); ok &&
		sumBuiltin.Name == "sum" &&
		(len(sumBuiltin.Arguments) == 1 || len(sumBuiltin.Arguments) == 2) {
		if rangeOp, ok := sumBuiltin.Arguments[0].(*BinaryNode); ok && rangeOp.Operator == ".." {
			if from, ok := rangeOp.Left.(*IntegerNode); ok {
				if to, ok := rangeOp.Right.(*IntegerNode); ok {
					m := from.Value
					n := to.Value
					if n >= m {
						count := n - m + 1
						// Use the arithmetic series formula: (n - m + 1) * (m + n) / 2
						sum := count * (m + n) / 2

						if len(sumBuiltin.Arguments) == 1 {
							// sum(m..n)
							patchWithType(node, &IntegerNode{Value: sum})
						} else if len(sumBuiltin.Arguments) == 2 {
							// sum(m..n, predicate)
							if result, ok := applySumPredicate(sum, count, sumBuiltin.Arguments[1]); ok {
								patchWithType(node, &IntegerNode{Value: result})
							}
						}
					}
				}
			}
		}
	}

	// Pattern 2: reduce(m..n, # + #acc) where m and n are constant integers
	if reduceBuiltin, ok := (*node).(*BuiltinNode); ok &&
		reduceBuiltin.Name == "reduce" &&
		(len(reduceBuiltin.Arguments) == 2 || len(reduceBuiltin.Arguments) == 3) {
		if rangeOp, ok := reduceBuiltin.Arguments[0].(*BinaryNode); ok && rangeOp.Operator == ".." {
			if from, ok := rangeOp.Left.(*IntegerNode); ok {
				if to, ok := rangeOp.Right.(*IntegerNode); ok {
					if isPointerPlusAcc(reduceBuiltin.Arguments[1]) {
						m := from.Value
						n := to.Value
						if n >= m {
							// Use the arithmetic series formula: (n - m + 1) * (m + n) / 2
							sum := (n - m + 1) * (m + n) / 2

							// Check for optional initialValue (3rd argument)
							if len(reduceBuiltin.Arguments) == 3 {
								if initialValue, ok := reduceBuiltin.Arguments[2].(*IntegerNode); ok {
									result := initialValue.Value + sum
									patchWithType(node, &IntegerNode{Value: result})
								}
							} else {
								patchWithType(node, &IntegerNode{Value: sum})
							}
						}
					}
				}
			}
		}
	}
}

// isPointerPlusAcc checks if the node represents `# + #acc` pattern
func isPointerPlusAcc(node Node) bool {
	predicate, ok := node.(*PredicateNode)
	if !ok {
		return false
	}

	binary, ok := predicate.Node.(*BinaryNode)
	if !ok {
		return false
	}

	if binary.Operator != "+" {
		return false
	}

	// Check for # + #acc (pointer + accumulator)
	leftPointer, leftIsPointer := binary.Left.(*PointerNode)
	rightPointer, rightIsPointer := binary.Right.(*PointerNode)

	if leftIsPointer && rightIsPointer {
		// # + #acc: Left is pointer (Name=""), Right is acc (Name="acc")
		if leftPointer.Name == "" && rightPointer.Name == "acc" {
			return true
		}
		// #acc + #: Left is acc (Name="acc"), Right is pointer (Name="")
		if leftPointer.Name == "acc" && rightPointer.Name == "" {
			return true
		}
	}

	return false
}

// applySumPredicate tries to compute the result of sum(m..n, predicate) at compile time.
// Returns (result, true) if optimization is possible, (0, false) otherwise.
// Supported predicates:
//   - # (identity): result = sum
//   - # * k (multiply by constant): result = k * sum
//   - k * # (multiply by constant): result = k * sum
//   - # + k (add constant): result = sum + count * k
//   - k + # (add constant): result = sum + count * k
//   - # - k (subtract constant): result = sum - count * k
func applySumPredicate(sum, count int, predicateArg Node) (int, bool) {
	predicate, ok := predicateArg.(*PredicateNode)
	if !ok {
		return 0, false
	}

	// Case 1: # (identity) - just return the sum
	if pointer, ok := predicate.Node.(*PointerNode); ok && pointer.Name == "" {
		return sum, true
	}

	// Case 2: Binary operations with pointer and constant
	binary, ok := predicate.Node.(*BinaryNode)
	if !ok {
		return 0, false
	}

	pointer, constant, pointerOnLeft := extractPointerAndConstantWithPosition(binary)
	if pointer == nil || constant == nil {
		return 0, false
	}

	switch binary.Operator {
	case "*":
		// # * k or k * # => k * sum
		return constant.Value * sum, true
	case "+":
		// # + k or k + # => sum + count * k
		return sum + count*constant.Value, true
	case "-":
		if pointerOnLeft {
			// # - k => sum - count * k
			return sum - count*constant.Value, true
		}
		// k - # => count * k - sum
		return count*constant.Value - sum, true
	}

	return 0, false
}

// extractPointerAndConstantWithPosition extracts pointer (#) and integer constant from a binary node.
// Returns (pointer, constant, pointerOnLeft) or (nil, nil, false) if not matching the expected pattern.
func extractPointerAndConstantWithPosition(binary *BinaryNode) (*PointerNode, *IntegerNode, bool) {
	// Try left=pointer, right=constant
	if pointer, ok := binary.Left.(*PointerNode); ok && pointer.Name == "" {
		if constant, ok := binary.Right.(*IntegerNode); ok {
			return pointer, constant, true
		}
	}

	// Try left=constant, right=pointer
	if constant, ok := binary.Left.(*IntegerNode); ok {
		if pointer, ok := binary.Right.(*PointerNode); ok && pointer.Name == "" {
			return pointer, constant, false
		}
	}

	return nil, nil, false
}
