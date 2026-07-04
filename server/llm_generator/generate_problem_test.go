package llm_generator

import "testing"

// TestPairNarrations pins the content-based matching: narrations pair to
// skeletons by echoed symbolic_expression, so a dropped or reordered narration
// costs only its own item rather than shifting the whole tail out of alignment
// (the positional-zip bug that collapsed word-generation yield).
func TestPairNarrations(t *testing.T) {
	skeletons := []Skeleton{
		{SymbolicExpression: "20% * 125", Answer: "25"},
		{SymbolicExpression: "1/4 * 252", Answer: "63"},
		{SymbolicExpression: "0.71 * 7", Answer: "4.97"},
	}

	// A narration that echoes symbolic s carries prose we can assert on.
	narr := func(sym string) narratedItem {
		return narratedItem{SymbolicExpression: sym, Expression: `\text{story for ` + sym + `}`, Explanation: "expl"}
	}
	// wantPaired asserts each result copies its skeleton's math verbatim and the
	// prose that echoed that skeleton — i.e. no cross-wiring.
	wantPaired := func(t *testing.T, got []Problem, syms ...string) {
		t.Helper()
		if len(got) != len(syms) {
			t.Fatalf("paired %d, want %d: %+v", len(got), len(syms), got)
		}
		for i, sym := range syms {
			if got[i].SymbolicExpression != sym {
				t.Errorf("[%d] symbolic %q, want %q", i, got[i].SymbolicExpression, sym)
			}
			if got[i].Expression != `\text{story for `+sym+`}` {
				t.Errorf("[%d] prose %q not the one that echoed %q", i, got[i].Expression, sym)
			}
		}
	}

	t.Run("dropped middle item does not shift the tail", func(t *testing.T) {
		// The model omitted the narration for skeleton[1]; positional zipping
		// would mispair skeleton[2] with skeleton[1]'s slot and reject it.
		got := pairNarrations(skeletons, []narratedItem{narr("20% * 125"), narr("0.71 * 7")})
		wantPaired(t, got, "20% * 125", "0.71 * 7")
		if got[1].Answer != "4.97" {
			t.Errorf("answer %q, want 4.97 (skeleton's, not a neighbor's)", got[1].Answer)
		}
	})

	t.Run("reordered narrations pair to the right skeletons", func(t *testing.T) {
		got := pairNarrations(skeletons, []narratedItem{narr("0.71 * 7"), narr("20% * 125"), narr("1/4 * 252")})
		wantPaired(t, got, "0.71 * 7", "20% * 125", "1/4 * 252")
	})

	t.Run("duplicate symbolic: first unused skeleton wins, no double-use", func(t *testing.T) {
		dup := []Skeleton{{SymbolicExpression: "2 + 2", Answer: "4"}, {SymbolicExpression: "2 + 2", Answer: "4"}}
		got := pairNarrations(dup, []narratedItem{narr("2 + 2"), narr("2 + 2"), narr("2 + 2")})
		// Only two skeletons exist; the third echo has no unused skeleton left.
		if len(got) != 2 {
			t.Fatalf("paired %d, want 2 (skeletons are consumed once each)", len(got))
		}
	})

	t.Run("echoed symbolic matching no skeleton is dropped", func(t *testing.T) {
		got := pairNarrations(skeletons, []narratedItem{narr("9 * 9"), narr("1/4 * 252")})
		wantPaired(t, got, "1/4 * 252")
	})

	t.Run("empty prose is skipped", func(t *testing.T) {
		got := pairNarrations(skeletons, []narratedItem{{SymbolicExpression: "20% * 125", Expression: "  "}, narr("1/4 * 252")})
		wantPaired(t, got, "1/4 * 252")
	})
}
