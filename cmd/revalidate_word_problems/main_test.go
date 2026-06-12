package main

import (
	"testing"

	"garydmenezes.com/mathgame/server/api"
)

// TestNewBitmapFor: parser shape bits merge with validator topic features;
// legacy self-report is absent (SET semantics for topics).
func TestNewBitmapFor(t *testing.T) {
	expr := `\text{A garden has }5\text{ rows of plants, with each row containing }12\text{ plants. If }3\text{ plants die, how many plants are left?}`
	got := newBitmapFor(expr, []string{"multiplication", "subtraction", "chained_operations", "word"})
	want := uint64(api.WORD | api.MULTIPLICATION | api.SUBTRACTION | api.CHAINED_OPERATIONS)
	if got != want {
		t.Errorf("newBitmapFor = %d (%v), want %d",
			got, api.ProblemTypeToFeatures(api.ProblemType(got)), want)
	}

	// Unknown validator names are ignored, never stamped.
	got = newBitmapFor(expr, []string{"word", "nonsense_feature"})
	if got != uint64(api.WORD) {
		t.Errorf("unknown feature stamped bits: %d", got)
	}
}

// TestNeedsValidation: the prefilter against the measured cases - the 10
// real chained rows the numeral count alone missed, plus skip-safe rows.
func TestNeedsValidation(t *testing.T) {
	send := []struct {
		expr   string
		bitmap uint64
	}{
		// >=3 digit numerals (the 99% class)
		{`\text{You have }50\text{ candies. You give away }10\text{ and then receive }20\text{ more.}`, 65},
		// implicit third quantity: spelled-out number
		{`\text{A farmer has }20\text{ sheep. He buys }5\text{ more and one sheep runs away.}`, 67},
		{`\text{How many meals were served in total over the two days? }200\text{ then }50\text{ more}`, 65},
		// multiplicative comparison cues
		{`\text{If a teacher has } 3 \text{ textbooks and buys } 4 \text{ times as many, how many in total?}`, 65},
		{`\text{A painter painted }15\text{ houses in summer and }5\text{ fewer during fall. Total?}`, 65},
		// prose suggests an op the stamp lacks (the missing-op leak class)
		{`\text{If a gardener planted }5\text{ flowers in each of the }4\text{ sections, how many in total?}`, 65},
	}
	for _, tc := range send {
		if !needsValidation(tc.expr, tc.bitmap) {
			t.Errorf("should send: %q", tc.expr)
		}
	}

	skip := []struct {
		expr   string
		bitmap uint64
	}{
		// simple two-quantity rows with consistent stamps
		{`\text{In a garden, there are }25\text{ flowers. If }15\text{ more flowers bloom, how many now?}`, 193},
		{`\text{Tom had }10\text{ marbles and lost }4\text{ of them. How many left?}`, 66},
		// division cue but the stamp already claims DIV: nothing to leak
		{`\text{If you have }20\text{ sticks and split them equally among }5\text{ friends?}`, 200},
	}
	for _, tc := range skip {
		if needsValidation(tc.expr, tc.bitmap) {
			t.Errorf("should skip: %q", tc.expr)
		}
	}
}
