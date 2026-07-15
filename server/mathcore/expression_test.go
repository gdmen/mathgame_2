package mathcore

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
		{`8 \div 2`, `8 ÷ 2`},
		{`\left(3 + 5\right) * 2`, `(3 + 5) * 2`},
		{`\frac{1}{2} + \frac{3}{4}`, `1/2 + 3/4`},
		{`\dfrac{1}{2}`, `1/2`},
		{`12 − 5`, `12 - 5`},    // unicode minus
		{`9 × 12`, `9 * 12`},    // unicode multiplication sign
		{`96 ÷ 8`, `96 ÷ 8`},    // obelus is the canonical division form
		{`70\% + 5`, `70% + 5`}, // escaped percent unescapes to literal %
		{`3 + 5`, `3 + 5`},      // untouched
		// Census-driven entries (2026-06 backfill dry run):
		{`$15 + $5`, `15 + 5`},      // money prefix stripped
		{`\$200 - 50`, `200 - 50`},  // escaped money prefix
		{`15,000 + 5`, `15000 + 5`}, // thousands separator joined
		{`1,234,567`, `1234567`},    // multi-group number
		{`12,3456`, `12,3456`},      // not a thousands pattern: untouched
		// Parenthesized negative literals (the display convention) fold to the
		// bare grammar form; parens around a SUBEXPRESSION are load-bearing
		// (PEMDAS) and stay.
		{`9 + (-4)`, `9 + -4`},
		{`5 - (-3)`, `5 - -3`},
		{`3 \times (-5)`, `3 * -5`},
		{`9 + (-3/4)`, `9 + -3/4`},
		{`5 + (-0.5)`, `5 + -0.5`},
		{`(-4 + 9) * 2`, `(-4 + 9) * 2`},
	}
	for _, tc := range cases {
		if got := NormalizeExpression(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDisplayExpression: unspaced a/b fraction literals fold to \frac{a}{b} and
// the division obelus folds to \div; non-fraction text is left alone. The
// display form normalizes to the same canonical grammar as its input (the
// storage invariant: NormalizeExpression bridges display and grammar).
func TestDisplayExpression(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`58/3 ÷ 8`, `\frac{58}{3} \div 8`},           // fraction dividend under division
		{`5/4 ÷ 2/3`, `\frac{5}{4} \div \frac{2}{3}`}, // fraction ÷ fraction
		{`3/4 * 5`, `\frac{3}{4} \times 5`},           // multiplication → \times
		{`6 ÷ 3`, `6 \div 3`},                         // pure division: no fraction literal
		{`70% ÷ 7`, `70\% \div 7`},                    // percent escaped (KaTeX comment otherwise)
		{`25% + 10%`, `25\% + 10\%`},                  // every percent escaped
		{`47 + 28`, `47 + 28`},                        // no fractions: passthrough
		{`1/2`, `\frac{1}{2}`},
		// A negative literal after an operator is parenthesized (textbook
		// convention: "9 + (-4)", never the bare "9 + -4"); a LEADING negative
		// stays bare, and an equation RHS ("= -7") stays bare.
		{`9 + -4`, `9 + (-4)`},
		{`5 - -3`, `5 - (-3)`},
		{`3 * -5`, `3 \times (-5)`},
		{`10 ÷ -2`, `10 \div (-2)`},
		{`-15 ÷ 3`, `-15 \div 3`},
		{`-2 - 3`, `-2 - 3`},
		{`9 + -3/4`, `9 + (-\frac{3}{4})`},
		{`? - 3 = -7`, `? - 3 = -7`},
	}
	for _, tc := range cases {
		got := DisplayExpression(tc.in)
		if got != tc.want {
			t.Errorf("DisplayExpression(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if a, b := NormalizeExpression(got), NormalizeExpression(tc.in); a != b {
			t.Errorf("invariant broken: NormalizeExpression(display)=%q != NormalizeExpression(grammar)=%q", a, b)
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
		{"(-3) + 5", 3},    // NORMALIZE folds (-3) to -3: number, op, number
		{"1/2 + 3/4", 3},   // fraction op fraction
		{"42 ÷ 6", 3},      // obelus = division
		{"42 / 6", 1},      // spaced slash = fraction (spacing-agnostic)
		{"0.75 + 0.25", 3}, // decimals
		{"25% of 80", 3},   // percent-of (strict grammar)
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

// TestLexExpression_SpacedFractionCanonical: a spaced slash lexes as a fraction
// whose Raw is the canonical unspaced form, so `2 / 3` and `2/3` render and hash
// alike (and DisplayExpression's \frac fold still fires).
func TestLexExpression_SpacedFractionCanonical(t *testing.T) {
	cases := []struct {
		in, wantRaw string
	}{
		{"2/3", "2/3"},
		{"2 / 3", "2/3"},
		{"-2 / 3", "-2/3"},
	}
	for _, tc := range cases {
		toks, err := LexExpression(NormalizeExpression(tc.in))
		if err != nil {
			t.Fatalf("lex(%q): %v", tc.in, err)
		}
		if len(toks) != 1 || toks[0].Kind != TokFraction {
			t.Fatalf("lex(%q): want one fraction token, got %v", tc.in, toks)
		}
		if toks[0].Raw != tc.wantRaw {
			t.Errorf("lex(%q) fraction Raw = %q, want %q", tc.in, toks[0].Raw, tc.wantRaw)
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

// TestLexExpression_PercentOfGrammar pins the strict percent grammar: a
// percent literal may appear ONLY as "n% of X". `of` lexes as the
// multiplication operator; any other percent placement is a lex reject
// (blocked by default - conversion equations, complements, and what-percent
// unknowns are deliberate later extensions).
func TestLexExpression_PercentOfGrammar(t *testing.T) {
	accepts := []struct {
		expr    string
		numToks int
	}{
		{"25% of 80", 3},
		{"25% of (40 + 28)", 7},
		{"25% of ? = 5", 5},  // find-the-whole: percent stays left of `of`
		{"5 + 25% of 20", 5}, // percent-of as an additive-chain operand
		{"25% of (40 + 28) ÷ 2", 9},
	}
	for _, tc := range accepts {
		toks, err := LexExpression(NormalizeExpression(tc.expr))
		if err != nil {
			t.Errorf("Lex(%q) rejected: %v", tc.expr, err)
			continue
		}
		if len(toks) != tc.numToks {
			t.Errorf("Lex(%q) = %d tokens, want %d", tc.expr, len(toks), tc.numToks)
		}
	}
	// `of` is the mul operator past the lexer.
	toks, err := LexExpression("25% of 80")
	if err != nil {
		t.Fatalf("Lex(25%% of 80): %v", err)
	}
	if toks[1].Kind != TokOperator || toks[1].Op != '*' {
		t.Errorf("`of` token = %+v, want TokOperator '*'", toks[1])
	}

	rejects := []string{
		"25% * 80",         // percent must be followed by `of`
		"80 * 25%",         // percent as a right factor
		"50% + 25%",        // percent under addition
		"25%",              // bare trailing percent
		"80 of 3",          // `of` must follow a percent literal
		"of 80",            // `of` with no percent before it
		"2/4 of 80",        // fraction is not a percent literal
		"3 * 25% of 4",     // percent preceded by `*`: would left-associate as a right factor
		"6 ÷ 25% of 2",     // percent preceded by the obelus
		"25% of 50% of 80", // percent chain: the second percent is preceded by `of`
	}
	for _, expr := range rejects {
		if _, err := LexExpression(NormalizeExpression(expr)); err == nil {
			t.Errorf("Lex(%q) accepted, want strict-grammar reject", expr)
		}
	}

	// Rule-3 guarantee: every admitted percent string round-trips through
	// Parse -> Render unchanged (the percent always binds as the left factor
	// of its `of` multiplication, the only placement Render emits).
	for _, tc := range accepts {
		norm := NormalizeExpression(tc.expr)
		node, err := Parse(norm)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.expr, err)
			continue
		}
		if got := Render(node); got != norm {
			t.Errorf("Render(Parse(%q)) = %q, want unchanged", tc.expr, got)
		}
	}

	// Single letters o and f are still variables; `off` is still rejected.
	if _, err := LexExpression("o + 5 = 12"); err != nil {
		t.Errorf("lone 'o' variable rejected: %v", err)
	}
	if _, err := LexExpression("2 + off"); err == nil {
		t.Error("'off' accepted, want reject")
	}
}

// TestNormalizeDisplayOf: the display skin \text{ of } folds back to the
// grammar's ` of ` (NormalizeExpression), and DisplayExpression emits it, so
// the two columns round-trip exactly.
func TestNormalizeDisplayOf(t *testing.T) {
	if got := NormalizeExpression(`25\%\text{ of }80`); got != "25% of 80" {
		t.Errorf("normalize display form = %q, want %q", got, "25% of 80")
	}
	if got := DisplayExpression("25% of 80"); got != `25\%\text{ of }80` {
		t.Errorf("DisplayExpression = %q, want %q", got, `25\%\text{ of }80`)
	}
	if got := NormalizeExpression(DisplayExpression("25% of (40 + 28)")); got != "25% of (40 + 28)" {
		t.Errorf("display round-trip = %q", got)
	}
}
