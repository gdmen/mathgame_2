package mathcore

import (
	"strings"
	"testing"
)

// TestDetectProblemTypeBitmap_Mapping pins the per-bit detection table.
func TestDetectProblemTypeBitmap_Mapping(t *testing.T) {
	cases := []struct {
		expr string
		want uint64
	}{
		{"3 + 5", uint64(ADDITION)},
		{"12 - 5", uint64(SUBTRACTION)},
		{"9 * 12", uint64(MULTIPLICATION)},
		{"42 ÷ 6", uint64(DIVISION | MEDIUM_NUMBERS)},
		{"6 / 8", uint64(FRACTIONS)}, // spaced slash = fraction, not division
		{"47 + 28", uint64(ADDITION | MEDIUM_NUMBERS)},
		{"1 + 999", uint64(ADDITION | LARGE_NUMBERS)}, // bracket, not cumulative
		{"13 + 13 + 13", uint64(ADDITION | MEDIUM_NUMBERS | CHAINED_OPERATIONS)},
		{"? + 5 = 12", uint64(ADDITION | MISSING_NUMBER)},
		{"12 - x = 5", uint64(SUBTRACTION | MISSING_NUMBER)}, // lone letter rewritten
		{"1/2 + 1/2", uint64(ADDITION | FRACTIONS)},
		{"2/3 + 3/4", uint64(ADDITION | FRACTIONS | MISMATCHED_DENOMINATORS)},
		{"-12 - 5", uint64(SUBTRACTION | NEGATIVES)},
		{"0.5 + 0.5", uint64(ADDITION | DECIMALS)},
		{"0.75 + 0.25", uint64(ADDITION | DECIMALS | MEDIUM_NUMBERS)}, // digit-magnitude 75
		{"25% * 4", uint64(MULTIPLICATION | PERCENTAGES | MEDIUM_NUMBERS)},
		{"5 + 2 * 3", uint64(ADDITION | MULTIPLICATION | CHAINED_OPERATIONS | PEMDAS)},
		{"2 * 3 + 5", uint64(ADDITION | MULTIPLICATION | CHAINED_OPERATIONS)}, // no PEMDAS
		{"3x + 7 = 22", uint64(ADDITION | SINGLE_VARIABLE | MEDIUM_NUMBERS)},
		{"x + x = 10", uint64(ADDITION | SINGLE_VARIABLE)},
		// WORD: shape bits from the parser (magnitude reads prose numerals);
		// topic bits are validator territory and absent here.
		{`\text{Mia has 47 stickers. She gives away 28. How many are left?}`,
			uint64(WORD | MEDIUM_NUMBERS)},
		{`\text{Solve for x: }3x + 7 = 22`,
			uint64(WORD | ADDITION | SINGLE_VARIABLE | MEDIUM_NUMBERS)},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got := DetectProblemTypeBitmap(tc.expr)
			if got != tc.want {
				t.Errorf("DetectProblemTypeBitmap(%q) = %d (%v), want %d (%v)",
					tc.expr, got, ProblemTypeToFeatures(ProblemType(got)),
					tc.want, ProblemTypeToFeatures(ProblemType(tc.want)))
			}
		})
	}
}

// TestAdmitExpression_Rejects: pipeline stages reject with the right funnel
// stage names.
func TestAdmitExpression_Rejects(t *testing.T) {
	cases := []struct {
		expr      string
		wantStage string
	}{
		{`\sqrt{16}`, RejectLexer},
		{"2^3 + 1", RejectLexer},
		{"? + x = 10", RejectUnknownRules},   // two distinct unknowns
		{"3x + 2y = 12", RejectUnknownRules}, // two distinct letters
		{"? + ? = 10", RejectUnknownRules},   // multi-?
	}
	for _, tc := range cases {
		adm := AdmitExpression(tc.expr)
		if adm.RejectStage != tc.wantStage {
			t.Errorf("AdmitExpression(%q) stage = %q (%s), want %q",
				tc.expr, adm.RejectStage, adm.RejectWhy, tc.wantStage)
		}
	}
	// Survivors come back with canonical text and bits.
	adm := AdmitExpression("  12 - x = 5 ")
	if adm.RejectStage != "" {
		t.Fatalf("expected admit, got %s: %s", adm.RejectStage, adm.RejectWhy)
	}
	if adm.Expr != "12 - ? = 5" {
		t.Errorf("canonical expr = %q, want rewritten form", adm.Expr)
	}
	if adm.Bitmap != uint64(SUBTRACTION|MISSING_NUMBER) {
		t.Errorf("bitmap = %d", adm.Bitmap)
	}
}

// TestVerifyAnswerSymbolic: the local-first answer check.
func TestVerifyAnswerSymbolic(t *testing.T) {
	cases := []struct {
		expr, answer string
		wantOK       bool
	}{
		{"3 + 5", "8", true},
		{"3 + 5", "9", false},
		{"12 - ? = 5", "7", true},
		{"12 - ? = 5", "8", false},
		{"3x + 7 = 22", "5", true},
		{"3x + 7 = 22", "6", false},
		{"x + x = 10", "5", true},
		{"1/2 + 1/4", "3/4", true},
		{"1/2 + 1/4", "6/8", true}, // equivalent rational accepted
		{"0.75 + 0.25", "1", true},
		{"25% * 80", "20", true},
		{"12 - 5 = 7", "7", true},  // equation, no unknown: sides + answer agree
		{"12 - 5 = 8", "8", false}, // sides disagree
		{"12 - ?", "5", false},     // unknown without an equation
		{"6 ÷ 0", "0", false},      // division by zero
	}
	for _, tc := range cases {
		toks, lexErr := LexExpression(NormalizeExpression(tc.expr))
		if lexErr != nil {
			t.Fatalf("lex(%q): %v", tc.expr, lexErr)
		}
		toks, _, _ = RewriteLoneVariable(toks, tc.expr)
		err := VerifyAnswerSymbolic(toks, tc.answer)
		if (err == nil) != tc.wantOK {
			t.Errorf("VerifyAnswerSymbolic(%q, %q) err=%v, wantOK=%v", tc.expr, tc.answer, err, tc.wantOK)
		}
	}
}

// TestEnvelopeViolation: subset check and violation naming.
func TestEnvelopeViolation(t *testing.T) {
	user := uint64(ADDITION | SUBTRACTION)
	if v := EnvelopeViolation(uint64(ADDITION), user); v != "" {
		t.Errorf("subset flagged: %s", v)
	}
	if v := EnvelopeViolation(uint64(ADDITION|MEDIUM_NUMBERS), user); !strings.Contains(v, "medium_numbers") {
		t.Errorf("violation = %q, want medium_numbers", v)
	}
}

// TestNormalizeProblemBitmap: structural invariants that the WORD validator
// (independent feature checkboxes) can violate. Each ORs in an implied bit;
// the operation only narrows a problem's audience, never widens it.
func TestNormalizeProblemBitmap(t *testing.T) {
	cases := []struct {
		name string
		in   ProblemType
		want ProblemType
	}{
		{"two core ops imply chained",
			MULTIPLICATION | SUBTRACTION | WORD,
			MULTIPLICATION | SUBTRACTION | WORD | CHAINED_OPERATIONS},
		{"three core ops imply chained",
			ADDITION | SUBTRACTION | MULTIPLICATION | WORD,
			ADDITION | SUBTRACTION | MULTIPLICATION | WORD | CHAINED_OPERATIONS},
		{"pemdas implies chained",
			ADDITION | PEMDAS | WORD,
			ADDITION | PEMDAS | WORD | CHAINED_OPERATIONS},
		{"mismatched implies fractions",
			MISMATCHED_DENOMINATORS | WORD,
			MISMATCHED_DENOMINATORS | WORD | FRACTIONS},
		{"single core op untouched",
			SUBTRACTION | WORD | MEDIUM_NUMBERS,
			SUBTRACTION | WORD | MEDIUM_NUMBERS},
		{"already-chained untouched",
			ADDITION | SUBTRACTION | CHAINED_OPERATIONS,
			ADDITION | SUBTRACTION | CHAINED_OPERATIONS},
		{"zero stays zero",
			0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeProblemBitmap(uint64(tc.in))
			if got != uint64(tc.want) {
				t.Errorf("NormalizeProblemBitmap(%d) = %d (%v), want %d (%v)",
					tc.in, got, ProblemTypeToFeatures(ProblemType(got)),
					tc.want, ProblemTypeToFeatures(tc.want))
			}
		})
	}
}

// TestWordFormBitmap: the final stamp for a WORD problem derived from its
// symbolic skeleton. PEMDAS is dropped (a story solver takes operation order
// from the narrative, so serving a word problem must not require the PEMDAS
// bit - matching the scoring path, which suppresses the multiplier), WORD is
// OR'd on, and the structural invariants are applied.
func TestWordFormBitmap(t *testing.T) {
	cases := []struct {
		name     string
		skeleton uint64
		want     uint64
	}{
		{"pemdas dropped, chained kept",
			uint64(ADDITION | MULTIPLICATION | CHAINED_OPERATIONS | PEMDAS),
			uint64(WORD | ADDITION | MULTIPLICATION | CHAINED_OPERATIONS)},
		{"plain skeleton gains WORD",
			uint64(DIVISION | MEDIUM_NUMBERS),
			uint64(WORD | DIVISION | MEDIUM_NUMBERS)},
		{"structural invariant still applied",
			uint64(ADDITION | SUBTRACTION),
			uint64(WORD | ADDITION | SUBTRACTION | CHAINED_OPERATIONS)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WordFormBitmap(tc.skeleton)
			if got != tc.want {
				t.Errorf("WordFormBitmap(%v) = %v, want %v",
					ProblemTypeToFeatures(ProblemType(tc.skeleton)),
					ProblemTypeToFeatures(ProblemType(got)),
					ProblemTypeToFeatures(ProblemType(tc.want)))
			}
		})
	}
}

// TestAdmitExpression_ReduceLabeledUnknown: a labeled DIRECT computation
// ("? = 100 - 25") is just "100 - 25" wearing an answer label - it carries no
// solve-for-the-blank load, so admission collapses it to the bare form rather
// than mis-stamping MISSING_NUMBER. A genuine operand unknown stays.
func TestAdmitExpression_ReduceLabeledUnknown(t *testing.T) {
	cases := []struct {
		expr       string
		wantExpr   string
		wantBitmap uint64
	}{
		{"? = 100 - 25", "100 - 25", uint64(SUBTRACTION | LARGE_NUMBERS)},
		{"100 - 25 = ?", "100 - 25", uint64(SUBTRACTION | LARGE_NUMBERS)},
		// Lone-letter labels reduce too (rewrite runs first: x -> ? -> collapsed).
		{"x = 100 - 25", "100 - 25", uint64(SUBTRACTION | LARGE_NUMBERS)},
		// Genuine operand unknowns are kept - the blank does real work.
		{"? + 10 = 30", "? + 10 = 30", uint64(ADDITION | MISSING_NUMBER | MEDIUM_NUMBERS)},
		{"30 = ? + 10", "30 = ? + 10", uint64(ADDITION | MISSING_NUMBER | MEDIUM_NUMBERS)},
		{"12 - ? = 5", "12 - ? = 5", uint64(SUBTRACTION | MISSING_NUMBER)},
		// Identify-the-value forms are kept: no computation on the other
		// side, so collapsing would leave a bare answerless literal.
		{"? = 3", "? = 3", uint64(MISSING_NUMBER)},
		{"3 = ?", "3 = ?", uint64(MISSING_NUMBER)},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			adm := AdmitExpression(tc.expr)
			if adm.RejectStage != "" {
				t.Fatalf("rejected [%s]: %s", adm.RejectStage, adm.RejectWhy)
			}
			if adm.Expr != tc.wantExpr {
				t.Errorf("Expr = %q, want %q", adm.Expr, tc.wantExpr)
			}
			if adm.Bitmap != tc.wantBitmap {
				t.Errorf("Bitmap = %v, want %v",
					ProblemTypeToFeatures(ProblemType(adm.Bitmap)),
					ProblemTypeToFeatures(ProblemType(tc.wantBitmap)))
			}
		})
	}

	// When the reduction removes a rewritten letter's '?' from the stored
	// text, RewroteLetter must clear: there is no '?' for explanation prose
	// to reference, so the letter stays in the prose untouched.
	adm := AdmitExpression("x = 100 - 25")
	if adm.RewroteLetter != 0 {
		t.Errorf("RewroteLetter = %q after the unknown was reduced away, want 0", adm.RewroteLetter)
	}
	adm = AdmitExpression("12 - x = 5")
	if adm.RewroteLetter != 'x' {
		t.Errorf("RewroteLetter = %q for a kept rewrite, want 'x'", adm.RewroteLetter)
	}
}
