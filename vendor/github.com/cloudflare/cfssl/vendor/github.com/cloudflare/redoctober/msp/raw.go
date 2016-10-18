package msp

import (
	"errors"
	"strings"
)

type NodeType int // Types of node in the binary expression tree.

const (
	NodeAnd NodeType = iota
	NodeOr
)

func (t NodeType) Type() NodeType {
	return t
}

type Layer struct {
	Conditions []Condition
	Operators  []NodeType
}

type Raw struct { // Represents one node in the tree.
	NodeType

	Left  Condition
	Right Condition
}

func StringToRaw(r string) (out Raw, err error) {
	// Automaton.  Modification of Dijkstra's Two-Stack Algorithm for parsing
	// infix notation.  Reads one long unbroken expression (several operators and
	// operands with no parentheses) at a time and parses it into a binary
	// expression tree (giving AND operators precedence).  Running time linear in
	// the size of the predicate?
	//
	// Steps to the next (un)parenthesis.
	//     (     -> Push new queue onto staging stack
	//     value -> Push onto back of queue at top of staging stack.
	//     )     -> Pop queue off top of staging stack, build BET, and push tree
	//              onto the back of the top queue.
	//
	// To build the binary expression tree, for each type of operation we iterate
	// through the (Condition, operator) lists compacting where that operation
	// occurs into tree nodes.
	//
	// Staging stack is empty on initialization and should have exactly 1 node
	// (the root node) at the end of the string.
	r = "(" + r + ")"

	min := func(a, b, c int) int { // Return smallest non-negative argument.
		if a > b { // Sort {a, b, c}
			a, b = b, a
		}
		if b > c {
			b, c = c, b
		}
		if a > b {
			a, b = b, a
		}

		if a != -1 {
			return a
		} else if b != -1 {
			return b
		} else {
			return c
		}
	}

	getNext := func(r string) (string, string) { // r -> (next, rest)
		r = strings.TrimSpace(r)

		if r[0] == '(' || r[0] == ')' || r[0] == '&' || r[0] == '|' {
			return r[0:1], r[1:]
		}

		nextOper := min(
			strings.Index(r, "&"),
			strings.Index(r, "|"),
			strings.Index(r, ")"),
		)

		if nextOper == -1 {
			return r, ""
		}
		return strings.TrimSpace(r[0:nextOper]), r[nextOper:]
	}

	staging := []Layer{} // Stack of (Condition list, operator list)
	indices := make(map[string]int, 0)

	var nxt string
	for len(r) > 0 {
		nxt, r = getNext(r)

		switch nxt {
		case "(":
			staging = append([]Layer{Layer{}}, staging...)
		case ")":
			if len(staging) < 1 { // Check 1
				return out, errors.New("Invalid string: Illegal close parenthesis.")
			}

			top := staging[0] // Legal because of check 1.
			staging = staging[1:]

			if len(top.Conditions) != (len(top.Operators) + 1) { // Check 2
				return out, errors.New("Invalid string: There needs to be an operator (& or |) for every pair of operands.")
			}

			for typ := NodeAnd; typ <= NodeOr; typ++ {
				i := 0
				for i < len(top.Operators) {
					oper := top.Operators[i] // Legal because for loop condition.

					// Copy left and right out of slice and THEN give a pointer for them!
					left, right := top.Conditions[i], top.Conditions[i+1] // Legal because of check 2.
					if oper == typ {
						built := Raw{typ, left, right}

						top.Conditions = append(
							top.Conditions[:i],
							append([]Condition{built}, top.Conditions[i+2:]...)...,
						)

						top.Operators = append(top.Operators[:i], top.Operators[i+1:]...) // Legal because for loop condition.
					} else {
						i++
					}
				}
			}

			if len(top.Conditions) != 1 || len(top.Operators) != 0 { // Check 3
				return out, errors.New("Invalid string: Couldn't evaluate all of the operators.")
			}

			if len(staging) == 0 { // Check 4
				if len(r) == 0 {
					res, ok := top.Conditions[0].(Raw) // Legal because of check 3.
					if !ok {
						return out, errors.New("Invalid string: Only one condition was found?")
					}
					return res, nil
				}
				return out, errors.New("Invalid string: Can't parse anymore, but there's still data. Too many closing parentheses or too few opening parentheses?")
			}
			staging[0].Conditions = append(staging[0].Conditions, top.Conditions[0]) // Legal because of checks 3 and 4.

		case "&":
			// Legal because first operation is to add an empty layer to the stack.
			// If the stack is ever empty again, the function tries to return or error.
			staging[0].Operators = append(staging[0].Operators, NodeAnd)
		case "|":
			staging[0].Operators = append(staging[0].Operators, NodeOr) // Legal for same reason as case &.
		default:
			if _, there := indices[nxt]; !there {
				indices[nxt] = 0
			}

			staging[0].Conditions = append(staging[0].Conditions, Name{nxt, indices[nxt]}) // Legal for same reason as case &.
			indices[nxt]++
		}
	}

	return out, errors.New("Invalid string: Not finished parsing, but out of data. Too many opening parentheses or too few closing parentheses?")
}

func (r Raw) String() string {
	out := ""

	switch left := r.Left.(type) {
	case Name:
		out += left.string
	case Raw:
		out += "(" + left.String() + ")"
	}

	if r.Type() == NodeAnd {
		out += " & "
	} else {
		out += " | "
	}

	switch right := r.Right.(type) {
	case Name:
		out += right.string
	case Raw:
		out += "(" + right.String() + ")"
	}

	return out
}

func (r Raw) Formatted() (out Formatted) {
	// Recursively maps a raw predicate to a formatted predicate by mapping AND
	// gates to (2, A, B) treshold gates and OR gates to (1, A, B) gates.
	if r.Type() == NodeAnd {
		out.Min = 2
	} else {
		out.Min = 1
	}

	switch left := r.Left.(type) {
	case Name:
		out.Conds = []Condition{left}
	case Raw:
		out.Conds = []Condition{left.Formatted()}
	}

	switch right := r.Right.(type) {
	case Name:
		out.Conds = append(out.Conds, right)
	case Raw:
		out.Conds = append(out.Conds, right.Formatted())
	}

	out.Compress() // Small amount of predicate compression.
	return
}

func (r Raw) Ok(db UserDatabase) bool {
	if r.Type() == NodeAnd {
		return r.Left.Ok(db) && r.Right.Ok(db)
	} else {
		return r.Left.Ok(db) || r.Right.Ok(db)
	}
}
