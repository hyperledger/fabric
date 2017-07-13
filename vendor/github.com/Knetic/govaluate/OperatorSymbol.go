package govaluate

/*
	Represents the valid symbols for operators.

*/
type OperatorSymbol int

const (
	VALUE OperatorSymbol = iota
	LITERAL
	NOOP
	EQ
	NEQ
	GT
	LT
	GTE
	LTE
	REQ
	NREQ
	IN

	AND
	OR

	PLUS
	MINUS
	BITWISE_AND
	BITWISE_OR
	BITWISE_XOR
	BITWISE_LSHIFT
	BITWISE_RSHIFT
	MULTIPLY
	DIVIDE
	MODULUS
	EXPONENT

	NEGATE
	INVERT
	BITWISE_NOT

	TERNARY_TRUE
	TERNARY_FALSE
	COALESCE

	FUNCTIONAL
	SEPARATE
)

type OperatorPrecedence int

const (
	NOOP_PRECEDENCE OperatorPrecedence = iota
	VALUE_PRECEDENCE
	FUNCTIONAL_PRECEDENCE
	PREFIX_PRECEDENCE
	EXPONENTIAL_PRECEDENCE
	ADDITIVE_PRECEDENCE
	BITWISE_PRECEDENCE
	BITWISE_SHIFTPRECEDENCE
	MULTIPLICATIVE_PRECEDENCE
	COMPARATOR_PRECEDENCE
	TERNARY_PRECEDENCE
	LOGICAL_PRECEDENCE
	SEPARATE_PRECEDENCE
)

func findOperatorPrecedenceForSymbol(symbol OperatorSymbol) OperatorPrecedence {

	switch symbol {
	case NOOP:
		return NOOP_PRECEDENCE
	case VALUE:
		return VALUE_PRECEDENCE
	case EQ:
		fallthrough
	case NEQ:
		fallthrough
	case GT:
		fallthrough
	case LT:
		fallthrough
	case GTE:
		fallthrough
	case LTE:
		fallthrough
	case REQ:
		fallthrough
	case NREQ:
		fallthrough
	case IN:
		return COMPARATOR_PRECEDENCE
	case AND:
		fallthrough
	case OR:
		return LOGICAL_PRECEDENCE
	case BITWISE_AND:
		fallthrough
	case BITWISE_OR:
		fallthrough
	case BITWISE_XOR:
		return BITWISE_PRECEDENCE
	case BITWISE_LSHIFT:
		fallthrough
	case BITWISE_RSHIFT:
		return BITWISE_SHIFTPRECEDENCE
	case PLUS:
		fallthrough
	case MINUS:
		return ADDITIVE_PRECEDENCE
	case MULTIPLY:
		fallthrough
	case DIVIDE:
		fallthrough
	case MODULUS:
		return MULTIPLICATIVE_PRECEDENCE
	case EXPONENT:
		return EXPONENTIAL_PRECEDENCE
	case BITWISE_NOT:
		fallthrough
	case NEGATE:
		fallthrough
	case INVERT:
		return PREFIX_PRECEDENCE
	case COALESCE:
		fallthrough
	case TERNARY_TRUE:
		fallthrough
	case TERNARY_FALSE:
		return TERNARY_PRECEDENCE
	case FUNCTIONAL:
		return FUNCTIONAL_PRECEDENCE
	case SEPARATE:
		return SEPARATE_PRECEDENCE
	}

	return VALUE_PRECEDENCE
}

/*
	Map of all valid comparators, and their string equivalents.
	Used during parsing of expressions to determine if a symbol is, in fact, a comparator.
	Also used during evaluation to determine exactly which comparator is being used.
*/
var COMPARATOR_SYMBOLS = map[string]OperatorSymbol{
	"==": EQ,
	"!=": NEQ,
	">":  GT,
	">=": GTE,
	"<":  LT,
	"<=": LTE,
	"=~": REQ,
	"!~": NREQ,
	"in": IN,
}

var LOGICAL_SYMBOLS = map[string]OperatorSymbol{
	"&&": AND,
	"||": OR,
}

var BITWISE_SYMBOLS = map[string]OperatorSymbol{
	"^": BITWISE_XOR,
	"&": BITWISE_AND,
	"|": BITWISE_OR,
}

var BITWISE_SHIFT_SYMBOLS = map[string]OperatorSymbol{
	">>": BITWISE_RSHIFT,
	"<<": BITWISE_LSHIFT,
}

var ADDITIVE_SYMBOLS = map[string]OperatorSymbol{
	"+": PLUS,
	"-": MINUS,
}

var MULTIPLICATIVE_SYMBOLS = map[string]OperatorSymbol{
	"*": MULTIPLY,
	"/": DIVIDE,
	"%": MODULUS,
}

var EXPONENTIAL_SYMBOLS = map[string]OperatorSymbol{
	"**": EXPONENT,
}

var PREFIX_SYMBOLS = map[string]OperatorSymbol{
	"-": NEGATE,
	"!": INVERT,
	"~": BITWISE_NOT,
}

var TERNARY_SYMBOLS = map[string]OperatorSymbol{
	"?":  TERNARY_TRUE,
	":":  TERNARY_FALSE,
	"??": COALESCE,
}

// this is defined separately from ADDITIVE_SYMBOLS et al because it's needed for parsing, not stage planning.
var MODIFIER_SYMBOLS = map[string]OperatorSymbol{
	"+":  PLUS,
	"-":  MINUS,
	"*":  MULTIPLY,
	"/":  DIVIDE,
	"%":  MODULUS,
	"**": EXPONENT,
	"&":  BITWISE_AND,
	"|":  BITWISE_OR,
	"^":  BITWISE_XOR,
	">>": BITWISE_RSHIFT,
	"<<": BITWISE_LSHIFT,
}

var SEPARATOR_SYMBOLS = map[string]OperatorSymbol{
	",": SEPARATE,
}

var ADDITIVE_MODIFIERS = []OperatorSymbol{
	PLUS, MINUS,
}

var BITWISE_MODIFIERS = []OperatorSymbol{
	BITWISE_AND, BITWISE_OR, BITWISE_XOR,
}

var BITWISE_SHIFT_MODIFIERS = []OperatorSymbol{
	BITWISE_LSHIFT, BITWISE_RSHIFT,
}

var MULTIPLICATIVE_MODIFIERS = []OperatorSymbol{
	MULTIPLY, DIVIDE, MODULUS,
}

var EXPONENTIAL_MODIFIERS = []OperatorSymbol{
	EXPONENT,
}

var PREFIX_MODIFIERS = []OperatorSymbol{
	NEGATE, INVERT, BITWISE_NOT,
}

var NUMERIC_COMPARATORS = []OperatorSymbol{
	GT, GTE, LT, LTE,
}

var STRING_COMPARATORS = []OperatorSymbol{
	REQ, NREQ,
}

/*
	Returns true if this operator is contained by the given array of candidate symbols.
	False otherwise.
*/
func (this OperatorSymbol) IsModifierType(candidate []OperatorSymbol) bool {

	for _, symbolType := range candidate {
		if this == symbolType {
			return true
		}
	}

	return false
}

/*
	Generally used when formatting type check errors.
	We could store the stringified symbol somewhere else and not require a duplicated codeblock to translate
	OperatorSymbol to string, but that would require more memory, and another field somewhere.
	Adding operators is rare enough that we just stringify it here instead.
*/
func (this OperatorSymbol) String() string {

	switch this {
	case NOOP:
		return "NOOP"
	case VALUE:
		return "VALUE"
	case EQ:
		return "="
	case NEQ:
		return "!="
	case GT:
		return ">"
	case LT:
		return "<"
	case GTE:
		return ">="
	case LTE:
		return "<="
	case REQ:
		return "=~"
	case NREQ:
		return "!~"
	case AND:
		return "&&"
	case OR:
		return "||"
	case IN:
		return "in"
	case BITWISE_AND:
		return "&"
	case BITWISE_OR:
		return "|"
	case BITWISE_XOR:
		return "^"
	case BITWISE_LSHIFT:
		return "<<"
	case BITWISE_RSHIFT:
		return ">>"
	case PLUS:
		return "+"
	case MINUS:
		return "-"
	case MULTIPLY:
		return "*"
	case DIVIDE:
		return "/"
	case MODULUS:
		return "%"
	case EXPONENT:
		return "**"
	case NEGATE:
		return "-"
	case INVERT:
		return "!"
	case BITWISE_NOT:
		return "~"
	case TERNARY_TRUE:
		return "?"
	case TERNARY_FALSE:
		return ":"
	case COALESCE:
		return "??"
	}
	return ""
}
