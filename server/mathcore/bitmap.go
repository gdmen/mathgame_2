// bitmap.go: enumeration of the valid non-WORD settings-bitmap space.
//
// The settings-level dependency rules (canonical spec in
// docs/problem-generation.md) constrain which bit combinations describe a
// servable envelope. This file materializes that space for tools that need to
// walk every valid combination — the admin bitmap × difficulty coverage matrix
// live-generates one example per (bitmap, difficulty) cell across all of it.
//
// These rules are also expressed in web/src/bitmap_validation.js (the settings
// UI) and constrained co-set-wise in stamping.go (NormalizeProblemBitmap). The
// canonical source of truth is docs/problem-generation.md; keep the three in
// sync.

package mathcore

// ValidBitmap reports whether pt describes a servable non-WORD envelope: it
// satisfies every settings-level dependency rule and carries no WORD bit. The
// rules mirror web/src/bitmap_validation.js (WORD excluded here — the heuristic
// generators emit no WORD problems, so the matrix universe is symbolic only):
//
//	(1) at least one core operation
//	(2) WORD clear
//	(3) LARGE_NUMBERS requires MEDIUM_NUMBERS
//	(4) MISMATCHED_DENOMINATORS requires FRACTIONS
//	(5) PEMDAS requires CHAINED_OPERATIONS
//	(6) PERCENTAGES requires MULTIPLICATION and MEDIUM_NUMBERS
func ValidBitmap(pt ProblemType) bool {
	coreOps := ADDITION | SUBTRACTION | MULTIPLICATION | DIVISION
	if pt&coreOps == 0 {
		return false
	}
	if pt&WORD != 0 {
		return false
	}
	if pt&LARGE_NUMBERS != 0 && pt&MEDIUM_NUMBERS == 0 {
		return false
	}
	if pt&MISMATCHED_DENOMINATORS != 0 && pt&FRACTIONS == 0 {
		return false
	}
	if pt&PEMDAS != 0 && pt&CHAINED_OPERATIONS == 0 {
		return false
	}
	if pt&PERCENTAGES != 0 && (pt&MULTIPLICATION == 0 || pt&MEDIUM_NUMBERS == 0) {
		return false
	}
	return true
}

// EnumerateValidBitmaps returns every valid non-WORD bitmap in ascending order.
// The result has exactly 8,784 entries (see docs/problem-generation.md
// valid_bitmap_count). Filtering the whole 0..ALL_PROBLEM_TYPES range is ~65k
// pure bit-tests (microseconds) — explicit beats clever here.
func EnumerateValidBitmaps() []ProblemType {
	out := make([]ProblemType, 0, 8784)
	for pt := ProblemType(0); pt <= ALL_PROBLEM_TYPES; pt++ {
		if ValidBitmap(pt) {
			out = append(out, pt)
		}
	}
	return out
}
