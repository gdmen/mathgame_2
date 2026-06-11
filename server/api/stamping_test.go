package api

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
		{"42 / 6", uint64(DIVISION | MEDIUM_NUMBERS)},
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
		{`\sqrt{16}`, rejectLexer},
		{"2^3 + 1", rejectLexer},
		{"? + x = 10", rejectUnknownRules},   // two distinct unknowns
		{"3x + 2y = 12", rejectUnknownRules}, // two distinct letters
		{"? + ? = 10", rejectUnknownRules},   // multi-?
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
		{"6 / 0", "0", false},      // division by zero
	}
	for _, tc := range cases {
		toks, lexErr := LexExpression(NormalizeExpression(tc.expr))
		if lexErr != nil {
			t.Fatalf("lex(%q): %v", tc.expr, lexErr)
		}
		toks, _, _ = RewriteLoneVariable(toks, tc.expr)
		err := verifyAnswerSymbolic(toks, tc.answer)
		if (err == nil) != tc.wantOK {
			t.Errorf("verifyAnswerSymbolic(%q, %q) err=%v, wantOK=%v", tc.expr, tc.answer, err, tc.wantOK)
		}
	}
}

// TestEnvelopeViolation: subset check and violation naming.
func TestEnvelopeViolation(t *testing.T) {
	user := uint64(ADDITION | SUBTRACTION)
	if v := envelopeViolation(uint64(ADDITION), user); v != "" {
		t.Errorf("subset flagged: %s", v)
	}
	if v := envelopeViolation(uint64(ADDITION|MEDIUM_NUMBERS), user); !strings.Contains(v, "medium_numbers") {
		t.Errorf("violation = %q, want medium_numbers", v)
	}
}

// TestBuildBitConstraints: the May/MustNot prompt block contains every
// required clause, and a minimal bitmap forbids everything else.
func TestBuildBitConstraints(t *testing.T) {
	full := BuildBitConstraints(ALL_PROBLEM_TYPES)
	for _, want := range []string{
		"MAY use addition", "MAY include fractions", "MAY pose word problems",
		"MAY use a single variable letter",
		"any size up to 9999",
		"MAY chain 2 or more operators (at most 5)",
		"at most ONE unknown",
		"Use ONLY the operations and concepts explicitly allowed",
		"simultaneous",
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full constraints missing %q\n%s", want, full)
		}
	}

	addOnly := BuildBitConstraints(ADDITION)
	for _, want := range []string{
		"MAY use addition",
		"MUST NOT use subtraction", "MUST NOT use multiplication", "MUST NOT use division",
		"MUST NOT include any fractions", "MUST NOT use decimal numbers",
		"MUST NOT use percentages", "MUST NOT use negative numbers",
		"MUST NOT include any prose", "MUST NOT use ? blanks",
		"MUST NOT use variable letters",
		"MUST be between 1 and 12",
		"MUST have exactly one operator",
	} {
		if !strings.Contains(addOnly, want) {
			t.Errorf("ADDITION-only constraints missing %q\n%s", want, addOnly)
		}
	}
	// No MAYs beyond addition; no unknown-rules clause when no unknown bits.
	if strings.Count(addOnly, "- MAY ") != 1 {
		t.Errorf("ADDITION-only should have exactly one MAY clause:\n%s", addOnly)
	}
	if strings.Contains(addOnly, "at most ONE unknown") {
		t.Errorf("unknown rules emitted without MISSING/SINGLE_VARIABLE:\n%s", addOnly)
	}
	// MISMATCHED MustNot only appears when FRACTIONS is enabled.
	fracSame := BuildBitConstraints(ADDITION | FRACTIONS)
	if !strings.Contains(fracSame, "MUST share one denominator") {
		t.Errorf("FRACTIONS without MISMATCHED should pin same-denominator:\n%s", fracSame)
	}
	if strings.Contains(addOnly, "denominator") {
		t.Errorf("no-FRACTIONS constraints should not mention denominators:\n%s", addOnly)
	}
	// 3-state magnitude: MEDIUM only.
	med := BuildBitConstraints(ADDITION | MEDIUM_NUMBERS)
	if !strings.Contains(med, "MUST NOT exceed 99") {
		t.Errorf("MEDIUM-only magnitude clause wrong:\n%s", med)
	}
}

// TestGenerationFunnel_NoSilentDrops: every reject lands in a named stage and
// the funnel line accounts for all of them.
func TestGenerationFunnel_NoSilentDrops(t *testing.T) {
	f := newGenerationFunnel(10)
	f.returned = 8
	f.reject(rejectLexer)
	f.reject(rejectAnswer)
	f.reject(rejectAnswer)
	f.inserted = 5
	line := f.String()
	for _, want := range []string{"requested=10", "returned=8", "lexer=1", "answer=2", "inserted=5",
		"unknown_rules=0", "collision=0", "envelope=0", "validator=0", "create=0"} {
		if !strings.Contains(line, want) {
			t.Errorf("funnel line missing %q: %s", want, line)
		}
	}
}
