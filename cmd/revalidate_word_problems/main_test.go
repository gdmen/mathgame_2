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
