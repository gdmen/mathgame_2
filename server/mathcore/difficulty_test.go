package mathcore

import (
	"math"
	"testing"
)

// TestComputeProblemDifficulty_ReferenceValues pins the formula v0.2
// reference values. These are THE canonical numbers: the illustrative table
// in docs/problem-generation.md points here as the owning test. Any formula
// change that moves these requires a DifficultyVersion bump, updated values
// here, and a recompute on deploy.
func TestComputeProblemDifficulty_ReferenceValues(t *testing.T) {
	const tol = 0.1
	cases := []struct {
		expr string
		want float64
	}{
		{"3 + 5", 3.62},
		{"13 + 13", 4.93},
		{"12 - ? = 5", 6.20},
		{"12 - x = 5", 6.20}, // lone bare letter rewritten to '?' (stage 1.5)
		{"47 + 28", 6.51},
		{"99 - 87", 7.87},
		{"9 * 12", 9.09},
		{"1/2 + 1/2", 5.27},                   // same-denom fractions x2.0
		{"11/12 - 5/12", 9.09},                // same-denom at 2-digit magnitude
		{"2/3 + 3/4", 8.87},                   // mismatched: 2.0 x 1.5 = net 3.0
		{"96 ÷ 8", 13.81},                     // division
		{"0.75 + 0.25", 11.22},                // decimals: digit-magnitude 75, x2.0
		{"1.5 + 2.5", 9.69},                   // decimals: digit-magnitude 25
		{"x + x = 10", 14.14},                 // multi-occurrence letter = SINGLE_VARIABLE
		{"3x + 7 = 22", 15.65},                // coefficient algebra
		{"13 + 13 + 13", 5.61},                // 2-op chain
		{"13 + 13 + 13 + 13 + 13 + 13", 7.36}, // 5-op chain (MaxChainLen)
		// PEMDAS dual-evaluation rule.
		{"2 * 3 + 5", 8.31},    // no fire
		{"5 + 2 * 3", 10.81},   // fires
		{"(3 + 5) * 2", 8.31},  // no fire (parens spell out natural order)
		{"2 * (3 + 5)", 10.81}, // fires
		// Word problems (prose rule: difficulty reads prose numerals; ops don't fire).
		{`\text{Mia has 12 stickers. She gives away 5. How many are left?}`, 6.12},
		{`\text{What is }25%\text{ of }80\text{?}`, 18.71}, // percent-of: \text{ of } folds to the symbolic connective, so the mul fires
		{`\text{Solve for x: }3x + 7 = 22`, 17.56},         // word-framed algebra stacks
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got := ComputeProblemDifficulty(tc.expr, "")
			if math.Abs(got-tc.want) > tol {
				t.Errorf("ComputeProblemDifficulty(%q) = %.2f, want %.2f +/- %.1f",
					tc.expr, got, tc.want, tol)
			}
		})
	}
}

// TestComputeProblemDifficulty_AnchorTable verifies computed difficulty values
// against a curated anchor table. Values are ranges because the formula should
// roughly match pedagogical intent but exact numbers will shift with tuning.
func TestComputeProblemDifficulty_AnchorTable(t *testing.T) {
	// Anchor ranges are intentionally wide. The formula prioritizes ordering
	// and sensible absolute bands over precise pedagogical match. Recalibrate
	// weights in difficulty.go if you want tighter anchors.
	cases := []struct {
		name    string
		expr    string
		minD    float64
		maxD    float64
		comment string
	}{
		{"simple add", "3 + 5", 2.5, 5, "basic add"},
		{"simple add within 20", "7 + 9", 3, 6, "add within 20"},
		{"two-digit add", "23 + 47", 5, 8, "two-digit add"},
		{"missing addend", "? + 5 = 12", 4, 7, "missing addend"},
		{"single-digit mul", "7 * 8", 6, 10, "multiplication facts"},
		{"simple division", "42 ÷ 6", 10, 14, "division (bigger dividend)"},
		{"same-denom fraction", "1/2 + 1/2", 4, 7, "same-denom fraction (v0.2: x2.0)"},
		{"multi-digit mul", "342 * 7", 11, 16, "multi-digit mul"},
		{"same-denom mixed", "3/8 + 2/8", 5, 9, "same-denom fraction (v0.2: x2.0)"},
		{"unlike-denom fraction", "2/3 + 3/4", 7, 12, "mismatched denominators"},
		{"order of ops", "2 + 3 * 4", 8, 12, "precedence (v0.2: PEMDAS x1.5 fires - LTR 20 != correct 14)"},
		{"negatives mul", "-4 * -3 + 2", 8, 13, "negatives with mul"},
		// Algebra anchors. NOTE (v0.2 prose rule): algebra written entirely
		// inside \text{} is invisible to the parser (its bits and quality come
		// from the LLM validator); the formula anchor uses the mixed
		// symbolic-in-text form, which is the generation convention.
		{"simple algebra", "\\text{Solve for x: }3x + 7 = 22", 13, 19, "algebra (mixed form)"},
		{"prose-only algebra", "\\text{Solve for x: 3x + 7 = 22}", 5, 9, "pure prose: parser sees a word problem with numerals (v0.2 prose rule)"},
		// Exponents are outside the allowlist alphabet: legacy fallback
		// path, no exponent weight.
		{"exponent excluded", "2^3 * 2^4", 5, 9, "lexer-rejected, fallback path, no exp weight"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := ComputeProblemDifficulty(tc.expr, "")
			if d < tc.minD || d > tc.maxD {
				t.Errorf("expr=%q: computed=%.2f, expected range [%.1f, %.1f] (%s)",
					tc.expr, d, tc.minD, tc.maxD, tc.comment)
			}
		})
	}
}

// TestComputeProblemDifficulty_OrderingWithinTopic verifies that within a
// single topic, larger numbers mean higher difficulty.
func TestComputeProblemDifficulty_OrderingWithinTopic(t *testing.T) {
	tests := []struct {
		name   string
		easier string
		harder string
	}{
		{"add magnitude", "3 + 5", "347 + 289"},
		{"mul magnitude", "3 * 4", "23 * 47"},
		{"fraction denom", "1/3 + 1/3", "2/3 + 3/4"},
		{"chain length", "3 + 5", "3 + 5 + 7 + 2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := ComputeProblemDifficulty(tc.easier, "")
			b := ComputeProblemDifficulty(tc.harder, "")
			if a >= b {
				t.Errorf("expected %q (%.2f) < %q (%.2f)", tc.easier, a, tc.harder, b)
			}
		})
	}
}

// TestComputeProblemDifficulty_OrderingAcrossTopics verifies that harder ops
// produce higher difficulty than easier ops at similar magnitudes.
func TestComputeProblemDifficulty_OrderingAcrossTopics(t *testing.T) {
	// At similar operand sizes, mul should outrank add, fractions should outrank
	// plain arithmetic, algebra should outrank fractions.
	simple := ComputeProblemDifficulty("8 + 9", "")
	mul := ComputeProblemDifficulty("8 * 9", "")
	frac := ComputeProblemDifficulty("2/3 + 3/4", "")
	// Mixed symbolic-in-text form: the v0.2 prose rule means pure-prose
	// algebra is invisible to the parser (validator territory).
	algebra := ComputeProblemDifficulty("\\text{Solve for x: }3x + 7 = 22", "")

	if mul <= simple {
		t.Errorf("expected mul (%.2f) > add (%.2f)", mul, simple)
	}
	if frac <= simple {
		t.Errorf("expected unlike-denom frac (%.2f) > simple add (%.2f)", frac, simple)
	}
	if algebra <= frac {
		t.Errorf("expected algebra (%.2f) > frac (%.2f)", algebra, frac)
	}
}

// TestComputeProblemDifficulty_Bounds verifies the floor at 1.0 holds and
// output is always finite. v0.2 removed the upper clamp - the scale is
// open-ended, bounded ~62 by construction (see TestComputeProblemDifficulty_OpenScale).
func TestComputeProblemDifficulty_Bounds(t *testing.T) {
	cases := []string{
		"",                         // empty
		"1",                        // just a number
		"0 + 0",                    // trivial
		"999999 * 999999 * 999999", // huge magnitude (legitimately scores >20 now)
		"\\text{This is just word content with no math}",
		"some garbage string with no pattern",
	}
	for _, expr := range cases {
		d := ComputeProblemDifficulty(expr, "")
		if d < 1.0 {
			t.Errorf("expr=%q: difficulty %.2f below the 1.0 floor", expr, d)
		}
		if math.IsNaN(d) || math.IsInf(d, 0) {
			t.Errorf("expr=%q: difficulty is NaN or Inf: %v", expr, d)
		}
	}
}

// TestComputeProblemDifficulty_OpenScale: v0.2 removed the upper clamp. The
// scale is open-ended (bounded ~62 by construction); the floor at 1.0 stays.
func TestComputeProblemDifficulty_OpenScale(t *testing.T) {
	// A multi-concept stack exceeds 20 - the truth the old clamp hid.
	monster := "(25% of x - 3/4 + 5/8) ÷ 0.8 - 99.99 = -1234"
	d := ComputeProblemDifficulty(monster, "")
	if d <= 20 {
		t.Errorf("six-concept monster scored %.1f, want > 20 (clamp should be gone)", d)
	}
	if d > 70 {
		t.Errorf("monster scored %.1f, exceeding the by-construction bound", d)
	}
}

// TestComputeProblemDifficulty_MissingNumber verifies missing-number templates
// get a structure bump but stay sensible.
func TestComputeProblemDifficulty_MissingNumber(t *testing.T) {
	plain := ComputeProblemDifficulty("5 + 7", "")
	missing := ComputeProblemDifficulty("? + 7 = 12", "")
	if missing <= plain {
		t.Errorf("missing template (%.2f) should be >= plain (%.2f)", missing, plain)
	}
}

// TestComputeProblemDifficulty_Fractions distinguishes same vs unlike denom.
func TestComputeProblemDifficulty_Fractions(t *testing.T) {
	same := ComputeProblemDifficulty("1/4 + 2/4", "")
	unlike := ComputeProblemDifficulty("1/4 + 2/3", "")
	if unlike <= same {
		t.Errorf("unlike-denom (%.2f) should be > same-denom (%.2f)", unlike, same)
	}
}

// TestComputeProblemDifficulty_ExponentsExcluded: '^' is outside the lexer
// alphabet; such expressions take the legacy fallback path and carry no
// exponent weight.
func TestComputeProblemDifficulty_ExponentsExcluded(t *testing.T) {
	d := ComputeProblemDifficulty("2^3 * 2^4", "")
	if d > 10 {
		t.Errorf("exponent expression scored %.1f - exponent weight should be gone", d)
	}
	f := parseProblemFeatures("2^3 * 2^4")
	if !f.lexFailed {
		t.Error("'^' should be rejected by the lexer (fallback path)")
	}
}

// TestComputeProblemDifficulty_WordScoredFromSymbolic: a word problem is scored
// from its symbolic_expression (the operators the prose hides), with the word
// concept applied - so a division word problem scores like its symbolic twin
// plus the word bonus, not as addition.
func TestComputeProblemDifficulty_WordScoredFromSymbolic(t *testing.T) {
	prose := `\text{There are 9999 beads shared equally among 11 jars; how many per jar?}`

	proseOnly := ComputeProblemDifficulty(prose, "") // division invisible -> opWeight 1.0
	symbolic := ComputeProblemDifficulty("9999 ÷ 11", "")
	withForm := ComputeProblemDifficulty(prose, "9999 ÷ 11")

	if withForm <= proseOnly+3 {
		t.Errorf("scored from symbolic_expression (%.2f) should far exceed scored-from-prose (%.2f)", withForm, proseOnly)
	}
	if withForm <= symbolic {
		t.Errorf("word problem (%.2f) should exceed its bare symbolic twin (%.2f) by the word concept", withForm, symbolic)
	}
	if got := ComputeProblemDifficulty(prose, ""); got != proseOnly {
		t.Errorf("empty symbolic_expression should score the expression itself, got %.2f", got)
	}
}

// TestComputeProblemDifficulty_SymbolicWithoutProseIsNotWord: the word bonus
// keys on \text{} in the expression, not on symbolic_expression being present.
// A heuristic_2.0 problem stores its grammar in symbolic_expression and a \frac
// display (no \text{}) in expression, so it is scored from the grammar with no
// word bonus - identical to scoring the grammar alone.
func TestComputeProblemDifficulty_SymbolicWithoutProseIsNotWord(t *testing.T) {
	grammar := "58/3 ÷ 8"
	display := DisplayExpression(grammar) // \frac{58}{3} \div 8 - carries no \text{}

	grammarOnly := ComputeProblemDifficulty(grammar, "")
	dual := ComputeProblemDifficulty(display, grammar)
	if math.Abs(dual-grammarOnly) > 1e-9 {
		t.Errorf("display+symbolic (%.4f) must match grammar-only (%.4f): a non-prose expression earns no word bonus", dual, grammarOnly)
	}

	// Contrast: the SAME symbolic under a \text{} expression IS a word problem.
	word := ComputeProblemDifficulty(`\text{share `+grammar+`}`, grammar)
	if word <= grammarOnly {
		t.Errorf("prose expression (%.4f) should exceed the non-word score (%.4f) by the word bonus", word, grammarOnly)
	}
}

// TestComputeProblemDifficulty_WordSuppressesPEMDAS (v0.4): operator-precedence
// parsing is a written-notation skill; a story solver takes the operation order
// from the narrative. A word problem scored from a PEMDAS-firing skeleton gets
// the word concept but NOT the PEMDAS multiplier; the bare symbolic form keeps
// it.
func TestComputeProblemDifficulty_WordSuppressesPEMDAS(t *testing.T) {
	prose := `\text{Tickets cost 2 dollars plus 3 packs of 5; what is the total?}`
	skeleton := "2 + 3 * 5" // dual-eval fires on the bare form

	word := ComputeDifficultyBreakdownFor(prose, skeleton)
	for _, c := range word.Concepts {
		if c.Name == "pemdas" {
			t.Fatalf("word scoring applied the PEMDAS multiplier: %+v", word.Concepts)
		}
	}

	sym := ComputeDifficultyBreakdownFor(skeleton, "")
	hasPEMDAS := false
	for _, c := range sym.Concepts {
		if c.Name == "pemdas" {
			hasPEMDAS = true
		}
	}
	if !hasPEMDAS {
		t.Fatal("bare symbolic form should keep the PEMDAS multiplier")
	}

	// Exact relation: word raw = (symbolic raw without PEMDAS) x word concept.
	wantRaw := sym.Raw / ConceptPEMDAS * ConceptWord
	if math.Abs(word.Raw-wantRaw) > 1e-9 {
		t.Errorf("word raw = %.6f, want %.6f (skeleton sans PEMDAS x word)", word.Raw, wantRaw)
	}
}

// TestMaxDiffForBitmap_WordBranch (v0.4): the ceiling computes the word and
// non-word bands as either/or branches. The word branch earns ConceptWord but
// caps chain length at MaxWordChainLen (deep chains have no faithful story) and
// never earns ConceptPEMDAS (suppressed for word problems). The field case
// behind this: envelope ADD|SUB|MUL|DIV|NEGATIVES|WORD|MEDIUM|CHAINED
// advertised ~21.1 while the narratable word ceiling was ~19.6 - the target
// ratcheted into a band empty by construction.
func TestMaxDiffForBitmap_WordBranch(t *testing.T) {
	const tol = 0.1
	env := uint64(ADDITION | SUBTRACTION | MULTIPLICATION | DIVISION |
		NEGATIVES | WORD | MEDIUM_NUMBERS | CHAINED_OPERATIONS)
	if got := MaxDiffForBitmap(env); math.Abs(got-19.56) > tol {
		t.Errorf("word-enabled chained envelope ceiling = %.2f, want 19.56 (word branch, chain capped)", got)
	}

	// A PEMDAS-heavy envelope: the non-word branch (full chain, PEMDAS, no word
	// concept) wins, so adding WORD leaves the ceiling unchanged.
	pem := uint64(ADDITION | SUBTRACTION | CHAINED_OPERATIONS | PEMDAS)
	if MaxDiffForBitmap(pem|uint64(WORD)) != MaxDiffForBitmap(pem) {
		t.Errorf("WORD lifted a ceiling the non-word branch owns: %.2f != %.2f",
			MaxDiffForBitmap(pem|uint64(WORD)), MaxDiffForBitmap(pem))
	}

	// Enabling WORD never lowers a ceiling (the non-word branch remains).
	for _, b := range EnumerateValidBitmaps() {
		if MaxDiffForBitmap(uint64(b|WORD)) < MaxDiffForBitmap(uint64(b)) {
			t.Fatalf("bitmap %d: adding WORD lowered the ceiling", b)
		}
	}
}

// TestMinDiffForBitmap pins the per-bitmap difficulty floor: the score of the
// EASIEST problem the envelope can construct. Concepts and structure are all
// MAY bits (the easiest problem uses none), so the floor is the minimum
// enabled op-weight at the smallest constructible magnitude.
func TestMinDiffForBitmap(t *testing.T) {
	const tol = 0.1
	cases := []struct {
		name string
		bits uint64
		want float64
	}{
		{"ADD only", uint64(ADDITION), 2.36},
		{"SUB only", uint64(SUBTRACTION), 2.70},
		{"MUL only", uint64(MULTIPLICATION), 5.75},
		{"DIV only", uint64(DIVISION), 7.02},
		{"DIV+MUL", uint64(DIVISION | MULTIPLICATION), 5.75},
		{"DIV+ADD (min over ops)", uint64(DIVISION | ADDITION), 2.36},
		// Every non-op bit is permissive: the floor ignores them.
		{"DIV with concepts", uint64(DIVISION | FRACTIONS | DECIMALS | CHAINED_OPERATIONS |
			MEDIUM_NUMBERS | LARGE_NUMBERS | WORD | NEGATIVES), 7.02},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MinDiffForBitmap(tc.bits); math.Abs(got-tc.want) > tol {
				t.Errorf("MinDiffForBitmap(%s) = %.2f, want %.2f", tc.name, got, tc.want)
			}
		})
	}
}

// TestTargetDifficultyRange: the one source of truth for the band
// target_difficulty may occupy - floor = max(global floor, envelope floor),
// ceiling = envelope ceiling - and the clamp into it.
func TestTargetDifficultyRange(t *testing.T) {
	// Low-floor envelope: the global MinTargetDifficulty dominates.
	lo, hi := TargetDifficultyRange(uint64(ADDITION))
	if lo != MinTargetDifficulty {
		t.Errorf("ADD floor = %.2f, want the global floor %.2f", lo, MinTargetDifficulty)
	}
	if hi != MaxDiffForBitmap(uint64(ADDITION)) {
		t.Errorf("ADD ceiling = %.2f, want MaxDiffForBitmap", hi)
	}

	// High-floor envelope: the per-bitmap floor dominates.
	lo, _ = TargetDifficultyRange(uint64(DIVISION))
	if math.Abs(lo-7.02) > 0.1 {
		t.Errorf("DIV floor = %.2f, want ~7.02 (envelope floor above global)", lo)
	}

	// The band is non-empty for every valid envelope.
	for _, b := range EnumerateValidBitmaps() {
		lo, hi := TargetDifficultyRange(uint64(b))
		if lo >= hi {
			t.Fatalf("bitmap %d: empty band [%.2f, %.2f]", b, lo, hi)
		}
	}

	// Clamp: up to the floor, down to the ceiling, identity inside.
	div := uint64(DIVISION)
	lo, hi = TargetDifficultyRange(div)
	if got := ClampTargetDifficulty(div, 3.0); got != lo {
		t.Errorf("clamp below floor = %.2f, want %.2f", got, lo)
	}
	if got := ClampTargetDifficulty(div, 999); got != hi {
		t.Errorf("clamp above ceiling = %.2f, want %.2f", got, hi)
	}
	mid := (lo + hi) / 2
	if got := ClampTargetDifficulty(div, mid); got != mid {
		t.Errorf("in-band clamp = %.2f, want identity %.2f", got, mid)
	}
}

// TestMaxDiffForBitmap_PerBitCeilings pins the per-bit ceiling-lift table
// (each row = ADD|SUB baseline plus the named bits).
func TestMaxDiffForBitmap_PerBitCeilings(t *testing.T) {
	const tol = 0.1
	base := uint64(ADDITION | SUBTRACTION)
	cases := []struct {
		name string
		bits uint64
		want float64
	}{
		{"ADD only", uint64(ADDITION), 4.82},
		{"ADD|SUB baseline", base, 5.28},
		{"+MISSING", base | uint64(MISSING_NUMBER), 6.20},
		{"+NEGATIVES", base | uint64(NEGATIVES), 6.62},
		{"+WORD", base | uint64(WORD), 6.62},
		{"+CHAINED", base | uint64(CHAINED_OPERATIONS), 7.77},
		{"+MEDIUM", base | uint64(MEDIUM_NUMBERS), 7.87},
		{"+MUL", base | uint64(MULTIPLICATION), 9.09},
		{"+FRACTIONS", base | uint64(FRACTIONS), 9.09},
		{"+DECIMALS", base | uint64(DECIMALS), 9.09},
		// Percent exists only as "n% of X" with two-digit operands: without
		// MULTIPLICATION and a >=MEDIUM bracket the bit lifts nothing.
		{"+PERCENTAGES (no mul: inert)", base | uint64(PERCENTAGES), 5.28},
		{"+PERCENTAGES+MUL (no medium: inert)", base | uint64(PERCENTAGES|MULTIPLICATION), 9.09},
		{"+PERCENTAGES+MUL+MEDIUM", base | uint64(PERCENTAGES|MULTIPLICATION|MEDIUM_NUMBERS), 17.08},
		{"+CHAINED+PEMDAS", base | uint64(CHAINED_OPERATIONS|PEMDAS), 10.22},
		{"+DIV", base | uint64(DIVISION), 10.60},
		{"+FRAC+MISMATCHED", base | uint64(FRACTIONS|MISMATCHED_DENOMINATORS), 11.67},
		{"+MEDIUM+LARGE", base | uint64(MEDIUM_NUMBERS|LARGE_NUMBERS), 11.76},
		{"+SINGLE_VARIABLE", base | uint64(SINGLE_VARIABLE), 15.18},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaxDiffForBitmap(tc.bits)
			if math.Abs(got-tc.want) > tol {
				t.Errorf("MaxDiffForBitmap(%s) = %.2f, want %.2f +/- %.1f",
					tc.name, got, tc.want, tol)
			}
		})
	}
}

// TestMaxDiffForBitmap_CombinedProfiles pins the combined-profile ceilings:
// a cumulative ladder (each step adds bits) plus spiky profiles. Each bitmap
// gets a DISTINCT ceiling instead of saturating at a clamp.
func TestMaxDiffForBitmap_CombinedProfiles(t *testing.T) {
	const tol = 0.15
	p1 := uint64(ADDITION | SUBTRACTION)
	p2 := p1 | uint64(MEDIUM_NUMBERS|MISSING_NUMBER)
	p3 := p2 | uint64(MULTIPLICATION|DIVISION)
	p4 := p3 | uint64(LARGE_NUMBERS|CHAINED_OPERATIONS)
	p5 := p4 | uint64(FRACTIONS|WORD)
	p6 := p5 | uint64(MISMATCHED_DENOMINATORS|DECIMALS)
	p7 := p6 | uint64(NEGATIVES|PEMDAS|PERCENTAGES)
	p8 := p7 | uint64(SINGLE_VARIABLE)
	cases := []struct {
		name string
		bits uint64
		want float64
	}{
		// p5+ carry WORD (p7+ PEMDAS too), so v0.4's word/non-word either-or
		// prices them lower than v0.3's all-multiplied product.
		{"p1", p1, 5.28},
		{"p2", p2, 8.94},
		{"p3", p3, 15.14},
		{"p4", p4, 22.80},
		{"p5", p5, 28.81},
		{"p6", p6, 37.52},
		{"p7", p7, 47.76},
		{"p8 (everything)", p8, 59.72},
		{"spiky ADD|MEDIUM|CHAINED", uint64(ADDITION | MEDIUM_NUMBERS | CHAINED_OPERATIONS), 10.13},
		{"spiky 4 ops small single-step", uint64(ADDITION | SUBTRACTION | MULTIPLICATION | DIVISION), 10.60},
		{"spiky FRAC|MISMATCH|MEDIUM", uint64(ADDITION | SUBTRACTION | FRACTIONS | MISMATCHED_DENOMINATORS | MEDIUM_NUMBERS), 15.01},
		{"spiky ADD|MEDIUM|DECIMALS", uint64(ADDITION | MEDIUM_NUMBERS | DECIMALS), 11.57},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaxDiffForBitmap(tc.bits)
			if math.Abs(got-tc.want) > tol {
				t.Errorf("MaxDiffForBitmap(%s) = %.2f, want %.2f +/- %.2f",
					tc.name, got, tc.want, tol)
			}
		})
	}
	// Monotonicity across the cumulative ladder.
	prev := 0.0
	for i, b := range []uint64{p1, p2, p3, p4, p5, p6, p7, p8} {
		v := MaxDiffForBitmap(b)
		if v <= prev {
			t.Errorf("cumulative ladder not strictly increasing at step %d: %.2f <= %.2f", i+1, v, prev)
		}
		prev = v
	}
}

// TestParseProblemFeatures_BitInputs spot-checks the feature fields that
// DetectProblemTypeBitmap maps to bits.
func TestParseProblemFeatures_BitInputs(t *testing.T) {
	f := parseProblemFeatures("0.75 + 0.25")
	if !f.hasDecimalsSymbolic || f.hasPercentSymbolic {
		t.Errorf("0.75+0.25: %+v", f)
	}
	if f.maxMagnitude != 75 {
		t.Errorf("digit-magnitude of 0.75 = %v, want 75", f.maxMagnitude)
	}
	f = parseProblemFeatures(`\text{The sale price is 1.50 dollars off 25% today}`)
	if !f.hasDecimals || !f.hasPercent {
		t.Errorf("prose decimals/percent should feed the difficulty side: %+v", f)
	}
	if f.hasDecimalsSymbolic || f.hasPercentSymbolic {
		t.Errorf("prose decimals/percent must NOT feed the symbolic side: %+v", f)
	}
	f = parseProblemFeatures("2/3 + 3/4")
	if f.numFractions != 2 || f.sameDenom {
		t.Errorf("mismatched fractions: %+v", f)
	}
	f = parseProblemFeatures("1/2 + 1/2")
	if f.numFractions != 2 || !f.sameDenom {
		t.Errorf("same-denom fractions: %+v", f)
	}
	f = parseProblemFeatures("-12 - 5")
	if !f.hasNegatives || !f.hasSub {
		t.Errorf("negatives: %+v", f)
	}
	f = parseProblemFeatures("? + 5 = 12")
	if !f.hasMissing || f.numOps != 1 {
		t.Errorf("'=' must not count as an op; ?+5=12 should be numOps=1: %+v", f)
	}
}

// TestRawForDifficultyRoundTrip: RawForDifficulty inverts CompressRaw across
// the working difficulty band, so the heuristic_2.0 aimer and the scorer agree.
func TestRawForDifficultyRoundTrip(t *testing.T) {
	for d := 1.0; d <= 40.0; d += 0.5 {
		raw := RawForDifficulty(d)
		if raw <= 0 {
			continue // at/below the scale floor the inverse is non-physical
		}
		got := CompressRaw(raw)
		if math.Abs(got-d) > 1e-9 {
			t.Errorf("CompressRaw(RawForDifficulty(%.1f)) = %.6f, want %.1f", d, got, d)
		}
	}
}

// TestTargetForBucket: the cell target is the bucket until it exceeds the
// bitmap's serving ceiling, then it clamps to the ceiling.
func TestTargetForBucket(t *testing.T) {
	bm := uint64(ADDITION | MEDIUM_NUMBERS)
	ceil := MaxDiffForBitmap(bm)
	// A bucket within the envelope is returned as-is.
	if got := TargetForBucket(bm, 5); got != 5.0 {
		t.Errorf("TargetForBucket(bucket=5) = %v, want 5", got)
	}
	// A bucket above the ceiling clamps down to the ceiling.
	if got := TargetForBucket(bm, int(ceil)+50); got != ceil {
		t.Errorf("TargetForBucket(above ceiling) = %v, want %v", got, ceil)
	}
}

// TestWordStampScoreCoherence: the word-PEMDAS policy lives in two layers -
// scoring (computeBreakdown suppresses the multiplier) and stamping
// (WordFormBitmap drops the bit). This pins them TOGETHER: for a
// PEMDAS-firing skeleton, the stamped word bitmap and the scored word
// concepts must agree that PEMDAS is absent, and both must retain it on the
// bare symbolic side.
func TestWordStampScoreCoherence(t *testing.T) {
	skeleton := "5 + 2 * 3"
	adm := AdmitExpression(skeleton)
	if adm.RejectStage != "" {
		t.Fatalf("skeleton rejected: %s", adm.RejectWhy)
	}
	if adm.Bitmap&uint64(PEMDAS) == 0 {
		t.Fatal("skeleton should fire PEMDAS symbolically")
	}

	stamp := WordFormBitmap(adm.Bitmap)
	if stamp&uint64(PEMDAS) != 0 {
		t.Error("word stamp kept the PEMDAS bit")
	}

	bd := ComputeDifficultyBreakdownFor(`\text{a story posing the skeleton}`, skeleton)
	for _, c := range bd.Concepts {
		if c.Name == "pemdas" {
			t.Error("word score kept the PEMDAS multiplier")
		}
	}
}

// TestMinDiffForBitmap_Reachable pins the floor to CONCRETE constructible
// problems: MinConstructibleOperand is a calibrated observation, and this is
// the calibration check - the easiest real problem per op scores exactly the
// floor, so the floor never advertises a band below what generation builds.
func TestMinDiffForBitmap_Reachable(t *testing.T) {
	const tol = 1e-9
	cases := []struct {
		bits uint64
		expr string
	}{
		{uint64(ADDITION), "2 + 1"},
		{uint64(SUBTRACTION), "2 - 1"},
		{uint64(MULTIPLICATION), "2 * 2"},
		{uint64(DIVISION), "2 ÷ 1"},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			floor := MinDiffForBitmap(tc.bits)
			got := ComputeProblemDifficulty(tc.expr, "")
			if math.Abs(got-floor) > tol {
				t.Errorf("easiest problem %q scores %.4f, floor says %.4f - recalibrate MinConstructibleOperand",
					tc.expr, got, floor)
			}
		})
	}
}
