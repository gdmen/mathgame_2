// ast.go: the expression AST. Render serializes a node to normalized ASCII;
// Parse (parse.go) is its structural inverse.
//
// The heuristic_2.0 builder constructs nodes answer-first (every node's value
// is known as it is built, so no tree-walking evaluator is needed to know the
// answer) and renders them to normalized ASCII that flows through the canonical
// token pipeline (LexExpression / EvalTokens / DetectProblemTypeBitmap /
// ComputeProblemDifficulty). EvalTokens evaluates by parsing the stream into
// this AST (Parse) and folding it (Eval).
//
// Render output feeds construction and the pipeline; admission storage keeps the
// original notation in Admission.Expr.

package mathcore

import (
	"math/big"
	"strings"
)

// Node is a sealed expression-tree node. The unexported marker method keeps the
// set of concrete node types closed to this package (the go/ast pattern).
type Node interface {
	aNode()
}

// Num is a numeric literal. Value is the exact rational (percent already
// divided by 100). The provenance flags carry what rendering and detection need
// that the value alone cannot express (12 vs 12.0, the literal a/b before
// reduction, 25% vs 0.25). Raw, when non-empty, is the authoritative rendering —
// the escape hatch for literals Value cannot round-trip (an unreduced fraction
// like 6/8, or a specific decimal precision).
type Num struct {
	Value      *big.Rat
	Raw        string
	IsDecimal  bool
	IsPercent  bool
	IsFraction bool
}

func (Num) aNode() {}

// Missing is the '?' fill-in-the-blank.
type Missing struct{}

func (Missing) aNode() {}

// Var is a variable letter. HasCoefficient marks the coefficient form (3x),
// which renders glued to its preceding number by the BinaryExpr coefficient
// special case below.
type Var struct {
	Letter         byte
	HasCoefficient bool
}

func (Var) aNode() {}

// BinaryExpr is L Op R, with Op one of '+' '-' '*' '/'. A coefficient term
// (3x) is modeled as BinaryExpr{Op: '*', L: Num, R: Var{HasCoefficient: true}}
// and rendered glued.
type BinaryExpr struct {
	Op   byte
	L, R Node
}

func (BinaryExpr) aNode() {}

// Paren is an explicit parenthesized subexpression — load-bearing for the
// PEMDAS naive-vs-correct distinction and for rendering.
type Paren struct{ X Node }

func (Paren) aNode() {}

// Equation is a top-level LHS = RHS.
type Equation struct{ LHS, RHS Node }

func (Equation) aNode() {}

// Render renders a node to normalized ASCII in the form LexExpression accepts:
// binary operators spaced (`a + b`), fractions unspaced (`3/8`), the division
// operator spaced (`6 / 2`) so it is never mistaken for a fraction, coefficient
// variables glued (`3x`), and explicit parens preserved. It is faithful: an
// operand is parenthesized whenever infix precedence/associativity would
// otherwise reparse it (`(a + b) * c`, `a - (b - c)`), so the rendered string
// always evaluates to the node's value.
func Render(n Node) string {
	var b strings.Builder
	renderNode(&b, n)
	return b.String()
}

func renderNode(b *strings.Builder, n Node) {
	switch t := n.(type) {
	case Num:
		b.WriteString(renderNum(t))
	case Missing:
		b.WriteByte('?')
	case Var:
		b.WriteByte(t.Letter)
	case Paren:
		b.WriteByte('(')
		renderNode(b, t.X)
		b.WriteByte(')')
	case Equation:
		renderNode(b, t.LHS)
		b.WriteString(" = ")
		renderNode(b, t.RHS)
	case BinaryExpr:
		// Coefficient term: a number immediately followed by a coefficient
		// variable is one operand, rendered glued with no operator (3x).
		if t.Op == '*' {
			if l, ok := t.L.(Num); ok {
				if r, ok := t.R.(Var); ok && r.HasCoefficient {
					b.WriteString(renderNum(l))
					b.WriteByte(r.Letter)
					return
				}
			}
		}
		renderOperand(b, t.L, t.Op, false)
		if t.Op == '/' {
			// Division renders as the obelus so the operator is unambiguous
			// against the fraction slash (a bare "/" is a fraction literal); the
			// AST op stays '/'. NORMALIZE folds ÷ back for lex/eval/detection.
			b.WriteString(" ÷ ")
		} else {
			b.WriteByte(' ')
			b.WriteByte(t.Op)
			b.WriteByte(' ')
		}
		renderOperand(b, t.R, t.Op, true)
	default:
		// Unknown node type: render nothing rather than panic. The builder only
		// constructs the types above, so this is unreachable in practice.
	}
}

// renderOperand renders a binary operand, wrapping it in parens when infix
// precedence/associativity would otherwise reparse it — the rule that keeps
// Render faithful. An explicit Paren node is already parenthesized, so it is
// never double-wrapped (it is not a bare BinaryExpr).
func renderOperand(b *strings.Builder, child Node, parentOp byte, isRight bool) {
	if needParen(child, parentOp, isRight) {
		b.WriteByte('(')
		renderNode(b, child)
		b.WriteByte(')')
		return
	}
	renderNode(b, child)
}

func needParen(child Node, parentOp byte, isRight bool) bool {
	be, ok := child.(BinaryExpr)
	if !ok || isCoeffTerm(be) {
		return false
	}
	cl, pl := opLevel(be.Op), opLevel(parentOp)
	// Lower-precedence child under a higher-precedence op, or the right operand
	// of the non-associative '-' / '/', would bind wrong without parens.
	return cl < pl || (cl == pl && isRight && (parentOp == '-' || parentOp == '/'))
}

// opLevel ranks operator precedence: multiplicative binds tighter than additive.
func opLevel(op byte) int {
	if op == '*' || op == '/' {
		return 1
	}
	return 0
}

// isCoeffTerm reports the coefficient form BinaryExpr{'*', Num, Var{coefficient}}
// (3x), which renders glued and acts as a single operand.
func isCoeffTerm(b BinaryExpr) bool {
	if b.Op != '*' {
		return false
	}
	_, lok := b.L.(Num)
	r, rok := b.R.(Var)
	return lok && rok && r.HasCoefficient
}

func renderNum(n Num) string {
	if n.Raw != "" {
		return n.Raw
	}
	switch {
	case n.IsPercent:
		pct := new(big.Rat).Mul(n.Value, big.NewRat(100, 1))
		return RatDecimalOrInt(pct) + "%"
	case n.IsFraction:
		return n.Value.Num().String() + "/" + n.Value.Denom().String()
	case n.IsDecimal:
		return RatDecimalOrInt(n.Value)
	default:
		// No provenance flag: an integer renders as itself, a non-integer as its
		// exact reduced fraction so the value round-trips.
		if n.Value.IsInt() {
			return n.Value.Num().String()
		}
		return n.Value.Num().String() + "/" + n.Value.Denom().String()
	}
}

// RatDecimalOrInt renders an exact rational as a plain integer when integral,
// else as a trimmed terminating decimal. Builder-constructed decimals always
// terminate; a non-terminating value falls back to a 10-digit approximation
// (never produced by the builder).
func RatDecimalOrInt(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	s := r.FloatString(10)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}
