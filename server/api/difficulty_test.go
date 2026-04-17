package api

import (
	"math"
	"testing"
)

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
		// Grade 1-2 anchors
		{"simple add", "3 + 5", 2.5, 5, "grade 1 basic"},
		{"simple add within 20", "7 + 9", 3, 6, "grade 1 top"},
		{"two-digit add", "23 + 47", 5, 8, "grade 2"},
		{"missing addend", "? + 5 = 12", 4, 7, "grade 2 missing"},
		// Grade 3 anchors
		{"single-digit mul", "7 * 8", 6, 10, "grade 3 multiplication"},
		{"simple division", "42 / 6", 10, 14, "grade 3 division (bigger dividend)"},
		{"same-denom fraction", "1/2 + 1/2", 3, 6, "grade 3 fraction (same denom is simpler)"},
		// Grade 4 anchors
		{"multi-digit mul", "342 * 7", 11, 16, "grade 4 mul"},
		{"same-denom mixed", "3/8 + 2/8", 4, 8, "grade 4 fraction"},
		// Grade 5 anchors
		{"unlike-denom fraction", "2/3 + 3/4", 7, 12, "grade 5 unlike denom"},
		{"order of ops", "2 + 3 * 4", 6, 10, "grade 5 precedence"},
		// Grade 6-7 anchors
		{"negatives mul", "-4 * -3 + 2", 8, 13, "grade 7 negatives with mul"},
		// Grade 8 anchors
		{"simple algebra", "\\text{Solve for x: 3x + 7 = 22}", 13, 19, "grade 8 algebra"},
		{"exponent", "2^3 * 2^4", 8, 14, "grade 8 exponent"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := ComputeProblemDifficulty(tc.expr)
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
			a := ComputeProblemDifficulty(tc.easier)
			b := ComputeProblemDifficulty(tc.harder)
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
	simple := ComputeProblemDifficulty("8 + 9")
	mul := ComputeProblemDifficulty("8 * 9")
	frac := ComputeProblemDifficulty("2/3 + 3/4")
	algebra := ComputeProblemDifficulty("\\text{Solve for x: 3x + 7 = 22}")

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

// TestComputeProblemDifficulty_Bounds verifies output is always [1, 20].
func TestComputeProblemDifficulty_Bounds(t *testing.T) {
	cases := []string{
		"",                         // empty
		"1",                        // just a number
		"0 + 0",                    // trivial
		"999999 * 999999 * 999999", // huge
		"\\text{This is just word content with no math}",
		"some garbage string with no pattern",
	}
	for _, expr := range cases {
		d := ComputeProblemDifficulty(expr)
		if d < 1.0 || d > 20.0 {
			t.Errorf("expr=%q: difficulty %.2f out of [1, 20]", expr, d)
		}
		if math.IsNaN(d) || math.IsInf(d, 0) {
			t.Errorf("expr=%q: difficulty is NaN or Inf: %v", expr, d)
		}
	}
}

// TestComputeProblemDifficulty_MissingNumber verifies missing-number templates
// get a structure bump but stay sensible.
func TestComputeProblemDifficulty_MissingNumber(t *testing.T) {
	plain := ComputeProblemDifficulty("5 + 7")
	missing := ComputeProblemDifficulty("? + 7 = 12")
	if missing <= plain {
		t.Errorf("missing template (%.2f) should be >= plain (%.2f)", missing, plain)
	}
}

// TestComputeProblemDifficulty_Fractions distinguishes same vs unlike denom.
func TestComputeProblemDifficulty_Fractions(t *testing.T) {
	same := ComputeProblemDifficulty("1/4 + 2/4")
	unlike := ComputeProblemDifficulty("1/4 + 2/3")
	if unlike <= same {
		t.Errorf("unlike-denom (%.2f) should be > same-denom (%.2f)", unlike, same)
	}
}
