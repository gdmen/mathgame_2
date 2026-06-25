package mathcore

import (
	"math/big"
	"testing"
)

func mustLex(t *testing.T, expr string) []Token {
	t.Helper()
	toks, err := LexExpression(NormalizeExpression(expr))
	if err != nil {
		t.Fatalf("lex(%q): %v", expr, err)
	}
	return toks
}

// TestEvalTokens_Correct: recursive descent honors precedence and parens.
func TestEvalTokens_Correct(t *testing.T) {
	cases := []struct {
		expr string
		want *big.Rat
	}{
		{"3 + 5", big.NewRat(8, 1)},
		{"2 * 3 + 5", big.NewRat(11, 1)},
		{"5 + 2 * 3", big.NewRat(11, 1)},
		{"(3 + 5) * 2", big.NewRat(16, 1)},
		{"2 * (3 + 5)", big.NewRat(16, 1)},
		{"12 - (5 - 3)", big.NewRat(10, 1)},
		{"1/2 + 1/4", big.NewRat(3, 4)},
		{"0.75 + 0.25", big.NewRat(1, 1)},
		{"25% * 80", big.NewRat(20, 1)},
		{"42 / 6", big.NewRat(7, 1)},
		{"-4 * -3 + 2", big.NewRat(14, 1)},
		{"3 - -5", big.NewRat(8, 1)}, // unary minus after a binary operator
		{"6 / 4", big.NewRat(3, 2)},  // exact rational, no float loss
	}
	for _, tc := range cases {
		got, err := EvalTokens(mustLex(t, tc.expr), nil)
		if err != nil {
			t.Errorf("eval(%q): %v", tc.expr, err)
			continue
		}
		if got.Cmp(tc.want) != 0 {
			t.Errorf("eval(%q) = %s, want %s", tc.expr, got, tc.want)
		}
	}
}

// TestEvalTokens_NaiveLTR: parens stripped, strict left fold.
func TestEvalTokens_NaiveLTR(t *testing.T) {
	cases := []struct {
		expr string
		want *big.Rat
	}{
		{"2 * 3 + 5", big.NewRat(11, 1)},
		{"5 + 2 * 3", big.NewRat(21, 1)},
		{"(3 + 5) * 2", big.NewRat(16, 1)},
		{"2 * (3 + 5)", big.NewRat(11, 1)},
		{"12 - (5 - 3)", big.NewRat(4, 1)},
	}
	for _, tc := range cases {
		got, err := EvalTokensNaiveLTR(mustLex(t, tc.expr), nil)
		if err != nil {
			t.Errorf("naive(%q): %v", tc.expr, err)
			continue
		}
		if got.Cmp(tc.want) != 0 {
			t.Errorf("naive(%q) = %s, want %s", tc.expr, got, tc.want)
		}
	}
}

// TestRequiresPEMDAS: the dual-evaluation rule table (docs/problem-generation.md).
func TestRequiresPEMDAS(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{"2 * 3 + 5", false},       // LTR 11 = correct 11
		{"5 + 2 * 3", true},        // LTR 21 != correct 11
		{"(3 + 5) * 2", false},     // LTR 16 = correct 16 (parens spell out natural order)
		{"2 * (3 + 5)", true},      // LTR 11 != correct 16
		{"12 - (5 - 3)", true},     // LTR 4 != correct 10 (no multiplication needed)
		{"(-3) + 5", false},        // parenthesized negative, not PEMDAS
		{"13 + 13 + 13", false},    // same-precedence chain
		{"3 + 5", false},           // single op cannot disagree with itself
		{"-4 * -3 + 2", false},     // mul first in both orders
		{"6 / (2 - 2) + 1", false}, // CORRECT errors (div0): rejected elsewhere, no fire
		// Unknowns are bound to probes; structural disagreement fires.
		{"5 + ? * 3", true},
		{"? * 3 + 5", false},
		{"5 + 2x * 3 = 23", true}, // coefficient pair is one operand; precedence still required
		{"? + 5 = 12", false},     // single op per side
		// Prose never fires.
		{`\text{What is five plus two times three?}`, false},
		// Mixed-form word problems are judged on their symbolic part: prose
		// labels filter out, so WORD x PEMDAS is constructible (the ceiling
		// stacks both - this must be reachable).
		{`\text{Solve: }5 + 2 * 3`, true},
		{`\text{Solve: }2 * 3 + 5`, false},
		// Prose-fragmented numerals are not a symbolic expression: no fire.
		{`\text{There are }12\text{ red, }7\text{ green, }9\text{ blue}`, false},
		{"3 - -5", false}, // unary minus after operator, single binary op
	}
	for _, tc := range cases {
		got := requiresPEMDAS(mustLex(t, tc.expr))
		if got != tc.want {
			t.Errorf("requiresPEMDAS(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestEvalTokens_UnknownBinding: substitution into '?' and variables.
func TestEvalTokens_UnknownBinding(t *testing.T) {
	toks := mustLex(t, "12 - ? ")
	got, err := EvalTokens(toks, Binding{bindingKeyMissing: big.NewRat(5, 1)})
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got.Cmp(big.NewRat(7, 1)) != 0 {
		t.Errorf("12 - ?{5} = %s, want 7", got)
	}
	// Unbound unknown errors.
	if _, err := EvalTokens(toks, nil); err == nil {
		t.Error("unbound '?' evaluated without error")
	}
	// Same letter binds to the same value everywhere.
	toks = mustLex(t, "x + x")
	got, err = EvalTokens(toks, Binding{'x': big.NewRat(5, 1)})
	if err != nil {
		t.Fatalf("eval x+x: %v", err)
	}
	if got.Cmp(big.NewRat(10, 1)) != 0 {
		t.Errorf("x{5} + x{5} = %s, want 10", got)
	}
}

// TestEvalTokens_Errors: malformed and division-by-zero shapes error cleanly.
func TestEvalTokens_Errors(t *testing.T) {
	for _, expr := range []string{"3 +", "+ 3", "3 5", "()", "(3 + 5", "6 / 0"} {
		toks, lexErr := LexExpression(expr)
		if lexErr != nil {
			continue // lexer-rejected is fine too
		}
		if _, err := EvalTokens(toks, nil); err == nil {
			t.Errorf("eval(%q) succeeded, want error", expr)
		}
	}
}
