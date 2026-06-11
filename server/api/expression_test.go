package api

import (
	"testing"
)

// TestNormalizeExpression: notation synonyms convert to one standard form.
func TestNormalizeExpression(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`3 \times 4`, `3 * 4`},
		{`3 \cdot 4`, `3 * 4`},
		{`8 \div 2`, `8 / 2`},
		{`\left(3 + 5\right) * 2`, `(3 + 5) * 2`},
		{`\frac{1}{2} + \frac{3}{4}`, `1/2 + 3/4`},
		{`\dfrac{1}{2}`, `1/2`},
		{`12 − 5`, `12 - 5`}, // unicode minus
		{`9 × 12`, `9 * 12`}, // unicode multiplication sign
		{`96 ÷ 8`, `96 / 8`}, // unicode division sign
		{`3 + 5`, `3 + 5`},   // untouched
		// Census-driven entries (2026-06 backfill dry run):
		{`$15 + $5`, `15 + 5`},      // money prefix stripped
		{`\$200 - 50`, `200 - 50`},  // escaped money prefix
		{`15,000 + 5`, `15000 + 5`}, // thousands separator joined
		{`1,234,567`, `1234567`},    // multi-group number
		{`12,3456`, `12,3456`},      // not a thousands pattern: untouched
	}
	for _, tc := range cases {
		if got := NormalizeExpression(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestLexExpression_Accepts: the alphabet lexes cleanly.
func TestLexExpression_Accepts(t *testing.T) {
	cases := []struct {
		expr string
		want int // expected token count
	}{
		{"3 + 5", 3},
		{"47 + 28", 3},
		{"3+5", 3}, // unspaced ops lex too
		{"12 - 5", 3},
		{"-12 - 5", 3},     // unary minus number, op, number
		{"(-3) + 5", 5},    // ( -3 ) + 5
		{"1/2 + 3/4", 3},   // fraction op fraction
		{"42 / 6", 3},      // spaced slash = division
		{"0.75 + 0.25", 3}, // decimals
		{"25% * 80", 3},    // percent number
		{"? + 5 = 12", 5},  // missing, op, number, equals, number
		{"3x + 7 = 22", 6}, // number, variable, op, number, equals, number
		{"x + x = 10", 5},  // variable, op, variable, equals, number
		{`\text{Mia has 12 stickers. She gives away 5. How many are left?}`, 1},
		{`\text{There are }12\text{ red balls}`, 3},
		{`\text{What is }x\text{ if x doubled is 10?}`, 3}, // letter before text WITH space content: legit variable
		{"(3 + 5) * 2", 7},
	}
	for _, tc := range cases {
		toks, err := LexExpression(NormalizeExpression(tc.expr))
		if err != nil {
			t.Errorf("Lex(%q) rejected: %v", tc.expr, err)
			continue
		}
		if len(toks) != tc.want {
			t.Errorf("Lex(%q) = %d tokens, want %d (%v)", tc.expr, len(toks), tc.want, toks)
		}
	}
}

// TestLexExpression_Rejects: out-of-alphabet notation is blocked by default.
func TestLexExpression_Rejects(t *testing.T) {
	cases := []string{
		`\sqrt{16}`,
		`2^3`,
		`5!`,
		`|x| + 2`,
		`ab + 2`,           // multi-letter identifier outside text
		`\text{unbalanced`, // unbalanced text braces (the llm_0.1 garbage class)
		`5 # 3`,
		`some garbage string`,
		// Prose-splice: a letter glued to a \text block continuing a word is
		// broken prose, NOT a variable - without this reject, the stage-1.5
		// rewrite would corrupt the word ("14 ?ooks"). Found by the backfill
		// census (row 57509807).
		`\text{Lucas had }14 b\text{ooks, but he gave away }5\text{ of them.}`,
		`\text{half}14b\text{ooks}`,
	}
	for _, expr := range cases {
		if _, err := LexExpression(NormalizeExpression(expr)); err == nil {
			t.Errorf("Lex(%q) accepted, want reject", expr)
		}
	}
}

// TestLexExpression_ProseRule: letters and '?' inside \text{} never become
// structural tokens. "John has a dog." must not produce a variable for 'a'.
func TestLexExpression_ProseRule(t *testing.T) {
	toks, err := LexExpression(`\text{John has a dog. A cat naps.} 3 + 4`)
	if err != nil {
		t.Fatalf("lex rejected: %v", err)
	}
	for _, tok := range toks {
		if tok.Kind == TokVariable || tok.Kind == TokMissing {
			t.Errorf("prose produced structural token %+v", tok)
		}
	}
	f := parseProblemFeatures(`\text{John has a dog. A cat naps.} 3 + 4`)
	if f.hasVariables || f.hasMissing || f.distinctUnknowns != 0 {
		t.Errorf("prose fired unknown detection: %+v", f)
	}
	if f.numOps != 1 || !f.hasAdd {
		t.Errorf("symbolic '3 + 4' not detected: %+v", f)
	}
	// Prose '?' must not fire MISSING.
	f2 := parseProblemFeatures(`\text{How many are left?}`)
	if f2.hasMissing || f2.questionMarks != 0 {
		t.Errorf("prose '?' fired MISSING: %+v", f2)
	}
}

// TestLexExpression_VariableEdgeCases: coefficient adjacency with the
// negative lookahead - "2nd" / "5km" shapes never produce variables.
func TestLexExpression_VariableEdgeCases(t *testing.T) {
	// Outside text, "5km" is an unknown token (rejected), never a variable.
	if _, err := LexExpression("5km + 2"); err == nil {
		t.Error("'5km' accepted, want reject (two letters after digit)")
	}
	// Coefficient form.
	toks, err := LexExpression("3x + 7")
	if err != nil {
		t.Fatalf("lex 3x: %v", err)
	}
	if toks[1].Kind != TokVariable || !toks[1].HasCoefficient {
		t.Errorf("3x: want coefficient variable, got %+v", toks[1])
	}
	// Standalone form.
	toks, err = LexExpression("12 - x")
	if err != nil {
		t.Fatalf("lex bare x: %v", err)
	}
	if toks[2].Kind != TokVariable || toks[2].HasCoefficient {
		t.Errorf("bare x: want standalone variable, got %+v", toks[2])
	}
}

// TestRewriteLoneVariable: the stage-1.5 rewrite table.
func TestRewriteLoneVariable(t *testing.T) {
	cases := []struct {
		expr        string
		wantRewrite bool
		wantExpr    string
	}{
		{"12 - x = 5", true, "12 - ? = 5"},    // lone bare letter -> ?
		{"x + x = 10", false, "x + x = 10"},   // multi-occurrence stays (variable identity)
		{"3x + 7 = 22", false, "3x + 7 = 22"}, // coefficient stays
		{"? + 5 = 12", false, "? + 5 = 12"},   // already a blank
		{"3x + 2x = 10", false, "3x + 2x = 10"},
		// Degenerate adjacency: "1/2x" lexes as FRACTION + bare VARIABLE, so
		// the rewrite produces "1/2?". Harmless by construction - the
		// operand-operand adjacency is unevaluable, so the insert pipeline's
		// answer check rejects the problem either way. Pinned deliberately.
		{"1/2x", true, "1/2?"},
	}
	for _, tc := range cases {
		norm := NormalizeExpression(tc.expr)
		toks, err := LexExpression(norm)
		if err != nil {
			t.Fatalf("lex(%q): %v", tc.expr, err)
		}
		toks, got, rewrote := RewriteLoneVariable(toks, norm)
		if rewrote != tc.wantRewrite {
			t.Errorf("rewrite(%q) = %v, want %v", tc.expr, rewrote, tc.wantRewrite)
		}
		if got != tc.wantExpr {
			t.Errorf("rewrite(%q) expr = %q, want %q", tc.expr, got, tc.wantExpr)
		}
		if tc.wantRewrite {
			for _, tok := range toks {
				if tok.Kind == TokVariable {
					t.Errorf("rewrite(%q) left a variable token", tc.expr)
				}
			}
		}
	}
}

// TestCountDistinctUnknowns: the per-problem unknown rules' inputs.
func TestCountDistinctUnknowns(t *testing.T) {
	cases := []struct {
		expr          string
		wantDistinct  int
		wantQuestions int
	}{
		{"? + 5 = 12", 1, 1},
		{"3x + 7 = 22", 1, 0},
		{"3x + 2x = 10", 1, 0},
		{"? + x = 10", 2, 1},   // two unknowns -> rejected at insert (PR2)
		{"3x + 2y = 12", 2, 0}, // two distinct letters -> rejected at insert
		{"? + ? = 10", 1, 2},   // multi-? -> rejected at insert
		{"12 + 5", 0, 0},
	}
	for _, tc := range cases {
		toks, err := LexExpression(NormalizeExpression(tc.expr))
		if err != nil {
			t.Fatalf("lex(%q): %v", tc.expr, err)
		}
		d, q := CountDistinctUnknowns(toks)
		if d != tc.wantDistinct || q != tc.wantQuestions {
			t.Errorf("CountDistinctUnknowns(%q) = (%d, %d), want (%d, %d)",
				tc.expr, d, q, tc.wantDistinct, tc.wantQuestions)
		}
	}
}
