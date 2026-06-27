package mathcore

import (
	"errors"
	"math/big"
	"math/rand"
	"testing"
)

// ---- round-trip contract (B) ----
//
// For a canonical (Render-produced) expression the parser guarantees:
//   - idempotent:  Render(Parse(s)) == s
//   - value-equal: astEval(Parse(s)) == EvalTokens(s)         (Eval(Parse) agrees)
//   - structural:  canon(Parse(Render(ast))) == canon(ast)    (tree recovered up
//                  to associativity; canon re-associates a same-precedence chain
//                  left, the only freedom Render does not encode)

// astEval folds the AST directly (no re-render), so it independently checks that
// Parse built the right tree rather than a merely render-equivalent one — the
// shape an AST-backed evaluator would take.
func astEval(n Node, bind map[byte]*big.Rat) (*big.Rat, error) {
	switch t := n.(type) {
	case Num:
		return new(big.Rat).Set(t.Value), nil
	case Missing:
		if v, ok := bind['?']; ok {
			return v, nil
		}
		return nil, errors.New("unbound ?")
	case Var:
		if v, ok := bind[t.Letter]; ok {
			return v, nil
		}
		return nil, errors.New("unbound variable")
	case Paren:
		return astEval(t.X, bind)
	case BinaryExpr:
		l, err := astEval(t.L, bind)
		if err != nil {
			return nil, err
		}
		r, err := astEval(t.R, bind)
		if err != nil {
			return nil, err
		}
		out := new(big.Rat)
		switch t.Op {
		case '+':
			return out.Add(l, r), nil
		case '-':
			return out.Sub(l, r), nil
		case '*':
			return out.Mul(l, r), nil
		case '/':
			if r.Sign() == 0 {
				return nil, errors.New("division by zero")
			}
			return out.Quo(l, r), nil
		}
		return nil, errors.New("bad operator")
	}
	return nil, errors.New("unevaluable node")
}

// canon re-associates same-precedence chains to the left and drops Num.Raw, the
// normal form for structural comparison. Render encodes precedence and parens
// but not the grouping of an associative run ("3 + 4 + 3" could be either
// nesting), so two trees that render identically must be compared up to it.
func canon(n Node) Node {
	switch t := n.(type) {
	case Num:
		return Num{Value: new(big.Rat).Set(t.Value), IsDecimal: t.IsDecimal, IsPercent: t.IsPercent, IsFraction: t.IsFraction}
	case Missing, Var:
		return t
	case Paren:
		return Paren{X: canon(t.X)}
	case Equation:
		return Equation{LHS: canon(t.LHS), RHS: canon(t.RHS)}
	case BinaryExpr:
		if isCoeffTerm(t) {
			return BinaryExpr{Op: '*', L: canon(t.L), R: t.R}
		}
		mul := t.Op == '*' || t.Op == '/'
		type term struct {
			op   byte
			node Node
		}
		var seq []term
		var walk func(n Node, op byte)
		walk = func(n Node, op byte) {
			be, ok := n.(BinaryExpr)
			if ok && !isCoeffTerm(be) && (be.Op == '*' || be.Op == '/') == mul {
				walk(be.L, op)
				// Only '+' and '*' are associative, so only their right child may
				// be merged into the chain; '-' and '/' keep it atomic (Render
				// would have parenthesized a non-atomic right child of '-' or '/').
				if be.Op == '+' || be.Op == '*' {
					walk(be.R, be.Op)
				} else {
					seq = append(seq, term{be.Op, canon(be.R)})
				}
				return
			}
			seq = append(seq, term{op, canon(n)})
		}
		walk(t, 0)
		out := seq[0].node
		for _, s := range seq[1:] {
			out = BinaryExpr{Op: s.op, L: out, R: s.node}
		}
		return out
	}
	return n
}

func isCoeffTerm(b BinaryExpr) bool {
	if b.Op != '*' {
		return false
	}
	_, lok := b.L.(Num)
	r, rok := b.R.(Var)
	return lok && rok && r.HasCoefficient
}

// nodeEqual compares two nodes structurally, ignoring Num.Raw (a render hint).
func nodeEqual(a, b Node) bool {
	switch x := a.(type) {
	case Num:
		y, ok := b.(Num)
		return ok && x.Value.Cmp(y.Value) == 0 && x.IsDecimal == y.IsDecimal &&
			x.IsPercent == y.IsPercent && x.IsFraction == y.IsFraction
	case Missing:
		_, ok := b.(Missing)
		return ok
	case Var:
		y, ok := b.(Var)
		return ok && x.Letter == y.Letter && x.HasCoefficient == y.HasCoefficient
	case Paren:
		y, ok := b.(Paren)
		return ok && nodeEqual(x.X, y.X)
	case Equation:
		y, ok := b.(Equation)
		return ok && nodeEqual(x.LHS, y.LHS) && nodeEqual(x.RHS, y.RHS)
	case BinaryExpr:
		y, ok := b.(BinaryExpr)
		return ok && x.Op == y.Op && nodeEqual(x.L, y.L) && nodeEqual(x.R, y.R)
	}
	return false
}

// assertRoundTrip checks the three guarantees for one AST.
func assertRoundTrip(t *testing.T, ast Node) {
	t.Helper()
	s := Render(ast)
	got, err := Parse(s)
	if err != nil {
		t.Errorf("Parse(%q) error: %v", s, err)
		return
	}
	if rr := Render(got); rr != s {
		t.Errorf("not idempotent: Render(Parse(%q)) = %q", s, rr)
	}
	if !nodeEqual(canon(got), canon(ast)) {
		t.Errorf("structural mismatch for %q:\n  canon(parsed)   = %s\n  canon(original) = %s", s, Render(canon(got)), Render(canon(ast)))
	}
	if _, isEq := ast.(Equation); !isEq {
		want, werr := astEval(ast, nil)
		have, herr := astEval(got, nil)
		toks, _ := LexExpression(NormalizeExpression(s))
		tokVal, terr := EvalTokens(toks, Binding{})
		if werr == nil && herr == nil && terr == nil {
			if want.Cmp(have) != 0 || have.Cmp(tokVal) != 0 {
				t.Errorf("value mismatch for %q: ast=%s parsed=%s tokens=%s", s, want, have, tokVal)
			}
		}
	}
}

func TestParseRoundTripHandCases(t *testing.T) {
	n := func(i int64) Num { return Num{Value: big.NewRat(i, 1)} }
	cases := []Node{
		n(7),
		BinaryExpr{Op: '+', L: n(3), R: n(5)},
		// left- and right-nested associative chains must canonicalize equal
		BinaryExpr{Op: '+', L: BinaryExpr{Op: '+', L: n(3), R: n(4)}, R: n(3)},
		BinaryExpr{Op: '+', L: n(3), R: BinaryExpr{Op: '+', L: n(4), R: n(3)}},
		// mixed precedence; '*' binds tighter, stays a single additive term
		BinaryExpr{Op: '+', L: n(3), R: BinaryExpr{Op: '*', L: n(4), R: n(5)}},
		BinaryExpr{Op: '-', L: BinaryExpr{Op: '-', L: n(10), R: n(4)}, R: n(2)},
		// load-bearing paren (a - (b - c))
		BinaryExpr{Op: '-', L: n(10), R: Paren{X: BinaryExpr{Op: '-', L: n(5), R: n(2)}}},
		// fractions under * and /, mismatched denominators
		BinaryExpr{Op: '*', L: Num{Value: ratF(3, 4), IsFraction: true}, R: Num{Value: ratF(5, 6), IsFraction: true}},
		BinaryExpr{Op: '/', L: Num{Value: ratF(3, 4), IsFraction: true}, R: Num{Value: ratF(2, 3), IsFraction: true}},
		BinaryExpr{Op: '/', L: n(6), R: Num{Value: ratF(3, 4), IsFraction: true}},
		// decimal and percent
		BinaryExpr{Op: '*', L: Num{Value: ratF(1, 2), IsDecimal: true}, R: n(3)},
		BinaryExpr{Op: '+', L: Num{Value: ratF(1, 4), IsPercent: true}, R: Num{Value: ratF(1, 2), IsPercent: true}},
		// negative operand
		BinaryExpr{Op: '+', L: Num{Value: big.NewRat(-3, 1)}, R: n(5)},
		// equation, missing, coefficient
		Equation{LHS: BinaryExpr{Op: '+', L: n(3), R: Missing{}}, RHS: n(12)},
		Equation{LHS: BinaryExpr{Op: '+', L: BinaryExpr{Op: '*', L: n(3), R: Var{Letter: 'x', HasCoefficient: true}}, R: n(5)}, RHS: n(17)},
	}
	for _, c := range cases {
		assertRoundTrip(t, c)
	}
}

// randomArith builds a faithful arithmetic AST: a child is parenthesized exactly
// when infix precedence/associativity would otherwise reparse it differently, so
// Render reproduces its value. No free variables, so astEval is total.
func randomArith(rng *rand.Rand, depth int) Node {
	if depth <= 0 || rng.Intn(3) == 0 {
		switch rng.Intn(4) {
		case 0:
			return Num{Value: big.NewRat(int64(1+rng.Intn(12)), 1)}
		case 1:
			return Num{Value: big.NewRat(int64(1+rng.Intn(11)), int64(2+rng.Intn(10))), IsFraction: true}
		case 2:
			return Num{Value: big.NewRat(int64(1+rng.Intn(99)), 100), IsDecimal: true}
		default:
			return Num{Value: big.NewRat(int64(-1-rng.Intn(12)), 1)}
		}
	}
	ops := []byte{'+', '-', '*', '/'}
	op := ops[rng.Intn(len(ops))]
	l := parenIfNeeded(randomArith(rng, depth-1), op, false)
	r := parenIfNeeded(randomArith(rng, depth-1), op, true)
	return BinaryExpr{Op: op, L: l, R: r}
}

func opLevel(op byte) int {
	if op == '*' || op == '/' {
		return 1
	}
	return 0
}

// parenIfNeeded wraps child in a Paren exactly when infix precedence would
// otherwise reparse it: a lower-precedence child under a higher one, or the
// right operand of the non-associative '-' / '/'.
func parenIfNeeded(child Node, parentOp byte, isRight bool) Node {
	be, ok := child.(BinaryExpr)
	if !ok || isCoeffTerm(be) {
		return child
	}
	cl, pl := opLevel(be.Op), opLevel(parentOp)
	if cl < pl || (cl == pl && isRight && (parentOp == '-' || parentOp == '/')) {
		return Paren{X: child}
	}
	return child
}

func TestParseRoundTripRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(20260626))
	for i := 0; i < 20000; i++ {
		assertRoundTrip(t, randomArith(rng, 1+rng.Intn(5)))
	}
}

func TestParseRejectsText(t *testing.T) {
	if _, err := Parse(`5 + \text{apples}`); err == nil {
		t.Error("Parse accepted a \\text token; WORD problems are not in the AST")
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	for _, s := range []string{"3 +", "* 4", "(3 + 4", "3 4", "3 + + 4"} {
		if node, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) = %s, want error", s, Render(node))
		}
	}
}
