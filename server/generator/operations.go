package generator // import "garydmenezes.com/mathgame/server/generator"

import (
	"fmt"
)

// Op represents an arithmetic operator.
type Op string

const (
	OpAdd Op = "+"
	OpSub Op = "-"
	OpMul Op = "*"
	OpDiv Op = "/"
)

// opsFromStrings converts legacy string-based operations list to Op values.
// Also accepts "mul" and "div" aliases.
func opsFromStrings(ss []string) []Op {
	var out []Op
	for _, s := range ss {
		switch s {
		case "+", "add", "addition":
			out = append(out, OpAdd)
		case "-", "sub", "subtraction":
			out = append(out, OpSub)
		case "*", "x", "mul", "multiplication":
			out = append(out, OpMul)
		case "/", "div", "division":
			out = append(out, OpDiv)
		}
	}
	return out
}

// pickOp picks a random operation from ops. Returns OpAdd if ops is empty.
func pickOp(ops []Op, rng randFunc) Op {
	if len(ops) == 0 {
		return OpAdd
	}
	return ops[rng(len(ops))]
}

// opSymbol returns the display symbol for the operator.
// Multiplication renders as × (multiplication sign) rather than * for readability.
// Division renders as ÷ rather than /.
// Addition and subtraction are themselves.
func opSymbol(op Op) string {
	switch op {
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	default:
		return string(op)
	}
}

// compute performs the operation on two ints and returns the result.
// Callers are responsible for ensuring division is clean (no remainder).
func compute(a int, op Op, b int) int {
	switch op {
	case OpAdd:
		return a + b
	case OpSub:
		return a - b
	case OpMul:
		return a * b
	case OpDiv:
		if b == 0 {
			return 0
		}
		return a / b
	}
	return 0
}

// formatBinary formats a binary expression with clean spacing: "a op b".
// No parens around operands unless needed for precedence (handled by caller).
func formatBinary(a int, op Op, b int) string {
	return fmt.Sprintf("%d %s %d", a, opSymbol(op), b)
}

// formatBinaryStrs is like formatBinary but accepts string operands for
// multi-term chains where operands may themselves be expressions or fractions.
func formatBinaryStrs(a string, op Op, b string) string {
	return fmt.Sprintf("%s %s %s", a, opSymbol(op), b)
}

// randFunc is the random integer source, abstracted for testability.
type randFunc func(n int) int
