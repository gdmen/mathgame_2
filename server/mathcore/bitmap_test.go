package mathcore

import (
	"sort"
	"testing"
)

// TestValidBitmap exercises each settings-level dependency rule the enumerator
// enforces: one pass and one violation per rule, plus the WORD exclusion.
func TestValidBitmap(t *testing.T) {
	cases := []struct {
		name string
		pt   ProblemType
		want bool
	}{
		{"addition only", ADDITION, true},
		{"no core op", MEDIUM_NUMBERS, false},
		{"word excluded", ADDITION | WORD, false},
		{"large requires medium: violated", ADDITION | LARGE_NUMBERS, false},
		{"large requires medium: satisfied", ADDITION | LARGE_NUMBERS | MEDIUM_NUMBERS, true},
		{"medium alone is fine", ADDITION | MEDIUM_NUMBERS, true},
		{"mismatched requires fractions: violated", ADDITION | MISMATCHED_DENOMINATORS, false},
		{"mismatched requires fractions: satisfied", ADDITION | FRACTIONS | MISMATCHED_DENOMINATORS, true},
		{"fractions alone is fine", ADDITION | FRACTIONS, true},
		{"pemdas requires chained: violated", ADDITION | PEMDAS, false},
		{"pemdas requires chained: satisfied", ADDITION | CHAINED_OPERATIONS | PEMDAS, true},
		{"chained alone is fine", ADDITION | CHAINED_OPERATIONS, true},
		{"percentages require multiplication: violated", ADDITION | MEDIUM_NUMBERS | PERCENTAGES, false},
		{"percentages require medium: violated", MULTIPLICATION | PERCENTAGES, false},
		{"percentages fully satisfied", MULTIPLICATION | MEDIUM_NUMBERS | PERCENTAGES, true},
		{"multiplication alone is fine", MULTIPLICATION, true},
		{"all non-word feature bits", ALL_PROBLEM_TYPES &^ WORD, true},
	}
	for _, tc := range cases {
		if got := ValidBitmap(tc.pt); got != tc.want {
			t.Errorf("ValidBitmap(%v) = %v, want %v (%s)",
				ProblemTypeToFeatures(tc.pt), got, tc.want, tc.name)
		}
	}
}

// TestEnumerateValidBitmaps pins the exact size of the valid non-WORD bitmap
// space (load-bearing: the admin matrix universe and the doc-sync anchor both
// depend on it), that the result is ascending and unique, and that every entry
// passes ValidBitmap while nothing outside the enumeration does.
func TestEnumerateValidBitmaps(t *testing.T) {
	got := EnumerateValidBitmaps()

	// Per core-op combo: 3 (MISMATCHED/FRACTIONS) x 3 (PEMDAS/CHAINED)
	// x 2^4 other free bits x the bracket/percent states = 144 x brackets.
	// Brackets: none, MEDIUM, MEDIUM+LARGE (3); PERCENTAGES is free only
	// with MULTIPLICATION and a MEDIUM bracket, so mul combos get 5
	// bracket-percent states (1 + 2x2) vs 3. 8 mul combos x 720 + 7 non-mul
	// combos x 432 = 8,784.
	const want = 8784
	if len(got) != want {
		t.Fatalf("EnumerateValidBitmaps() returned %d bitmaps, want %d", len(got), want)
	}

	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Errorf("EnumerateValidBitmaps() is not ascending")
	}

	seen := map[ProblemType]bool{}
	for _, pt := range got {
		if seen[pt] {
			t.Errorf("EnumerateValidBitmaps() contains duplicate %v", ProblemTypeToFeatures(pt))
		}
		seen[pt] = true
		if !ValidBitmap(pt) {
			t.Errorf("EnumerateValidBitmaps() emitted invalid bitmap %v", ProblemTypeToFeatures(pt))
		}
	}

	// The enumeration is exactly the ValidBitmap-passing subset of the space:
	// a brute-force count over the whole value range must agree.
	brute := 0
	for pt := ProblemType(0); pt <= ALL_PROBLEM_TYPES; pt++ {
		if ValidBitmap(pt) {
			brute++
		}
	}
	if brute != want {
		t.Errorf("brute-force ValidBitmap count = %d, want %d", brute, want)
	}
}
