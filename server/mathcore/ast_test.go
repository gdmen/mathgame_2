package mathcore

import (
	"math/big"
	"testing"
)

func ratI(n int64) *big.Rat    { return big.NewRat(n, 1) }
func ratF(n, d int64) *big.Rat { return big.NewRat(n, d) }

// TestRender pins the rendered form of hand-built ASTs to the normalized ASCII
// the canonical lexer accepts. (No Parse exists yet to round-trip against, so
// the AST is exercised by asserting Render output directly.)
func TestRender(t *testing.T) {
	coeff := func(n int64, letter byte) BinaryExpr {
		return BinaryExpr{Op: '*', L: Num{Value: ratI(n)}, R: Var{Letter: letter, HasCoefficient: true}}
	}
	cases := []struct {
		name string
		node Node
		want string
	}{
		{"add", BinaryExpr{Op: '+', L: Num{Value: ratI(3)}, R: Num{Value: ratI(5)}}, "3 + 5"},
		{"parens force order", BinaryExpr{Op: '*',
			L: Paren{X: BinaryExpr{Op: '+', L: Num{Value: ratI(3)}, R: Num{Value: ratI(5)}}},
			R: Num{Value: ratI(2)}}, "(3 + 5) * 2"},
		{"coefficient equation", Equation{
			LHS: BinaryExpr{Op: '+', L: coeff(3, 'x'), R: Num{Value: ratI(7)}},
			RHS: Num{Value: ratI(22)}}, "3x + 7 = 22"},
		{"missing equation", Equation{
			LHS: BinaryExpr{Op: '-', L: Num{Value: ratI(12)}, R: Missing{}},
			RHS: Num{Value: ratI(5)}}, "12 - ? = 5"},
		{"multi-occurrence variable", Equation{
			LHS: BinaryExpr{Op: '+', L: Var{Letter: 'x'}, R: Var{Letter: 'x'}},
			RHS: Num{Value: ratI(10)}}, "x + x = 10"},
		{"same-denom fractions", BinaryExpr{Op: '+',
			L: Num{Value: ratF(3, 8), IsFraction: true},
			R: Num{Value: ratF(1, 8), IsFraction: true}}, "3/8 + 1/8"},
		{"division is spaced (not a fraction)", BinaryExpr{Op: '/',
			L: Num{Value: ratI(6)}, R: Num{Value: ratI(2)}}, "6 / 2"},
		{"decimals", BinaryExpr{Op: '+',
			L: Num{Value: ratF(3, 4), IsDecimal: true},
			R: Num{Value: ratF(1, 4), IsDecimal: true}}, "0.75 + 0.25"},
		{"percent", BinaryExpr{Op: '*',
			L: Num{Value: ratF(1, 4), IsPercent: true},
			R: Num{Value: ratI(4)}}, "25% * 4"},
		{"unreduced fraction via Raw", BinaryExpr{Op: '+',
			L: Num{Value: ratF(3, 4), Raw: "6/8", IsFraction: true},
			R: Num{Value: ratF(1, 8), IsFraction: true}}, "6/8 + 1/8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Render(tc.node); got != tc.want {
				t.Errorf("Render = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRenderFlowsThroughPipeline confirms rendered output lexes and evaluates
// through the canonical token pipeline to the value the AST was built around —
// the property the builder relies on (its candidates are scored/admitted via
// the canonical path, not the AST).
func TestRenderFlowsThroughPipeline(t *testing.T) {
	cases := []struct {
		node Node
		want *big.Rat // value of the single arithmetic side
	}{
		{BinaryExpr{Op: '+', L: Num{Value: ratI(3)}, R: Num{Value: ratI(5)}}, ratI(8)},
		{BinaryExpr{Op: '*',
			L: Paren{X: BinaryExpr{Op: '+', L: Num{Value: ratI(3)}, R: Num{Value: ratI(5)}}},
			R: Num{Value: ratI(2)}}, ratI(16)},
		{BinaryExpr{Op: '/', L: Num{Value: ratI(6)}, R: Num{Value: ratI(2)}}, ratI(3)},
		{BinaryExpr{Op: '+',
			L: Num{Value: ratF(3, 8), IsFraction: true},
			R: Num{Value: ratF(1, 8), IsFraction: true}}, ratF(1, 2)},
		{BinaryExpr{Op: '+',
			L: Num{Value: ratF(3, 4), IsDecimal: true},
			R: Num{Value: ratF(1, 4), IsDecimal: true}}, ratI(1)},
		// Fraction/decimal operands UNDER '*' and '/': the slash convention (an
		// unspaced slash is a fraction, a spaced slash is division, and Render
		// emits operators spaced) keeps these unambiguous — the property the
		// multiplicative fraction/decimal splits rely on.
		{BinaryExpr{Op: '*',
			L: Num{Value: ratF(3, 4), IsFraction: true},
			R: Num{Value: ratF(5, 6), IsFraction: true}}, ratF(5, 8)},
		{BinaryExpr{Op: '/',
			L: Num{Value: ratF(3, 4), IsFraction: true},
			R: Num{Value: ratF(2, 3), IsFraction: true}}, ratF(9, 8)},
		{BinaryExpr{Op: '/',
			L: Num{Value: ratI(6)},
			R: Num{Value: ratF(3, 4), IsFraction: true}}, ratI(8)},
		{BinaryExpr{Op: '*',
			L: Num{Value: ratF(1, 2), IsDecimal: true},
			R: Num{Value: ratI(3)}}, ratF(3, 2)},
	}
	for _, tc := range cases {
		expr := Render(tc.node)
		toks, lexErr := LexExpression(NormalizeExpression(expr))
		if lexErr != nil {
			t.Errorf("rendered %q does not lex: %v", expr, lexErr)
			continue
		}
		got, err := EvalTokens(toks, Binding{})
		if err != nil {
			t.Errorf("rendered %q does not evaluate: %v", expr, err)
			continue
		}
		if got.Cmp(tc.want) != 0 {
			t.Errorf("rendered %q evaluates to %s, want %s", expr, got, tc.want)
		}
	}
}
