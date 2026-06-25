package generator

import (
	"math"
	"math/rand"
	"testing"

	"garydmenezes.com/mathgame/server/mathcore"
)

// representativeBitmaps returns a curated set of valid (dependency-rule-
// satisfying, non-WORD) envelopes spanning the concept space: every core-op
// subset, each layered with the magnitude/chain/concept variants the heuristic
// is meant to cover.
func representativeBitmaps() []mathcore.ProblemType {
	core := []mathcore.ProblemType{
		mathcore.ADDITION, mathcore.SUBTRACTION, mathcore.MULTIPLICATION, mathcore.DIVISION,
		mathcore.ADDITION | mathcore.SUBTRACTION,
		mathcore.MULTIPLICATION | mathcore.DIVISION,
		mathcore.ADDITION | mathcore.MULTIPLICATION,
		mathcore.ADDITION | mathcore.SUBTRACTION | mathcore.MULTIPLICATION | mathcore.DIVISION,
	}
	variants := []mathcore.ProblemType{
		0,
		mathcore.MEDIUM_NUMBERS,
		mathcore.MEDIUM_NUMBERS | mathcore.LARGE_NUMBERS,
		mathcore.CHAINED_OPERATIONS,
		mathcore.CHAINED_OPERATIONS | mathcore.PEMDAS,
		mathcore.MISSING_NUMBER,
		mathcore.NEGATIVES,
		mathcore.DECIMALS,
		mathcore.PERCENTAGES,
		mathcore.FRACTIONS,
		mathcore.FRACTIONS | mathcore.MISMATCHED_DENOMINATORS,
		mathcore.SINGLE_VARIABLE,
		mathcore.MEDIUM_NUMBERS | mathcore.CHAINED_OPERATIONS,
		mathcore.MEDIUM_NUMBERS | mathcore.LARGE_NUMBERS | mathcore.CHAINED_OPERATIONS | mathcore.DECIMALS,
	}
	var out []mathcore.ProblemType
	for _, c := range core {
		for _, v := range variants {
			bm := c | v
			// PEMDAS needs at least two ops to fire; skip single-op + PEMDAS.
			out = append(out, bm)
		}
	}
	return out
}

// TestBuildProblem_PropertySweep is the Validation B-gate measurement: for every
// representative bitmap and every integer target in [MinTargetDifficulty,
// MaxDiffForBitmap], it builds a problem and checks the result is valid,
// in-envelope, correctly answered, and within the selection window of the
// target. It reports the hit rate, targeting MAE, and any uncovered cells.
func TestBuildProblem_PropertySweep(t *testing.T) {
	rng := rand.New(rand.NewSource(20260625))
	var cells, hits, valid int
	var absErrSum float64
	type gap struct {
		bm     mathcore.ProblemType
		target float64
		got    float64
	}
	var gaps []gap

	for _, bm := range representativeBitmaps() {
		ceil := mathcore.MaxDiffForBitmap(uint64(bm))
		lo := math.Ceil(mathcore.MinTargetDifficulty)
		for target := lo; target <= ceil; target += 1.0 {
			cells++
			expr, ans, err := BuildProblem(bm, target, rng)
			if err != nil {
				t.Errorf("BuildProblem(%d, %.0f) error: %v", bm, target, err)
				continue
			}
			// Validity: admits, answers, subset of envelope.
			adm := mathcore.AdmitExpression(expr)
			if adm.RejectStage != "" {
				t.Errorf("built expr rejected (%s): %q for bm=%d target=%.0f", adm.RejectStage, expr, bm, target)
				continue
			}
			if mathcore.VerifyAnswerSymbolic(adm.Tokens, ans) != nil {
				t.Errorf("built answer wrong: %q = %q (bm=%d target=%.0f)", expr, ans, bm, target)
				continue
			}
			stamped := mathcore.NormalizeProblemBitmap(adm.Bitmap)
			if v := mathcore.EnvelopeViolation(stamped, uint64(bm)); v != "" {
				t.Errorf("envelope violation [%s]: %q (bm=%d target=%.0f)", v, expr, bm, target)
				continue
			}
			valid++
			d := mathcore.ComputeProblemDifficulty(adm.Expr, "")
			absErrSum += math.Abs(d - target)
			if math.Abs(d-target) <= targetWindow {
				hits++
			} else {
				gaps = append(gaps, gap{bm, target, d})
			}
		}
	}

	mae := absErrSum / float64(max(1, valid))
	hitRate := float64(hits) / float64(max(1, cells))
	over, under, additiveEasyOver := 0, 0, 0
	for _, g := range gaps {
		if g.got > g.target {
			over++
			// An envelope with addition or subtraction can always build an easy
			// problem ("2 + 2" ~ 2.4, "9 - 1" ~ low), so overshooting an easy
			// target there is a real construction bug — unlike a mul/div/concept-
			// only envelope, whose inherent difficulty FLOOR sits above 3 (every
			// "a * b" scores >= ~5) and legitimately can't be dialed lower.
			if g.target <= 5 && g.bm&(mathcore.ADDITION|mathcore.SUBTRACTION) != 0 {
				additiveEasyOver++
			}
		} else {
			under++
		}
	}
	t.Logf("sweep: cells=%d valid=%d hits=%d hitRate=%.3f MAE=%.3f gaps=%d (overshoot=%d undershoot=%d additive-easy-overshoot=%d)",
		cells, valid, hits, hitRate, mae, len(gaps), over, under, additiveEasyOver)

	// Hard invariant: every cell yields a valid, in-envelope, correctly-answered
	// problem (asserted inline above via t.Errorf). Targeting gate: the closest
	// achievable difficulty tracks the target across the constructible space.
	// Residual gaps are the inherent floor/ceiling and coarse-concept cells (a
	// mul/div-only envelope can't go below ~5; a x5-SINGLE_VARIABLE-only envelope
	// has a hole between plain arithmetic and the variable jump) — documented in
	// docs/generator-versions.md, not builder bugs.
	if hitRate < 0.75 {
		t.Errorf("targeting hit-rate %.3f below gate 0.75", hitRate)
	}
	if mae > 1.5 {
		t.Errorf("targeting MAE %.3f above gate 1.5", mae)
	}
	if additiveEasyOver > 0 {
		t.Errorf("an additive envelope overshot an easy target %d time(s) — easy problems must be constructible there", additiveEasyOver)
	}

	shown := 0
	for _, g := range gaps {
		if shown >= 20 {
			t.Logf("... and %d more gaps", len(gaps)-shown)
			break
		}
		t.Logf("  GAP bm=%d target=%.0f got=%.2f features=%v",
			g.bm, g.target, g.got, mathcore.ProblemTypeToFeatures(g.bm))
		shown++
	}
}

// TestBuildProblem_CoversLLMOnlyConcepts proves the heuristic_2.0 LLM-offload
// claim: the bits that were LLM-only under heuristic_1.0 (#227) — DECIMALS,
// PEMDAS, PERCENTAGES, SINGLE_VARIABLE — are each actually produced by the
// heuristic when enabled and the target calls for them.
func TestBuildProblem_CoversLLMOnlyConcepts(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// A rich envelope so every concept is reachable; aim high so the inverter
	// reaches for concepts.
	bitmap := mathcore.ADDITION | mathcore.SUBTRACTION | mathcore.MULTIPLICATION |
		mathcore.DIVISION | mathcore.MEDIUM_NUMBERS | mathcore.CHAINED_OPERATIONS |
		mathcore.DECIMALS | mathcore.PEMDAS | mathcore.PERCENTAGES |
		mathcore.SINGLE_VARIABLE | mathcore.MISSING_NUMBER
	want := map[string]mathcore.ProblemType{
		"decimals":        mathcore.DECIMALS,
		"pemdas":          mathcore.PEMDAS,
		"percentages":     mathcore.PERCENTAGES,
		"single_variable": mathcore.SINGLE_VARIABLE,
		"missing_number":  mathcore.MISSING_NUMBER,
	}
	seen := map[string]bool{}
	for i := 0; i < 4000; i++ {
		target := 8.0 + rng.Float64()*12.0
		expr, _, err := BuildProblem(bitmap, target, rng)
		if err != nil {
			t.Fatalf("BuildProblem: %v", err)
		}
		bm := mathcore.ProblemType(mathcore.DetectProblemTypeBitmap(expr))
		for name, bit := range want {
			if bm&bit != 0 {
				seen[name] = true
			}
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("heuristic_2.0 never produced a %s problem for a rich high-target envelope", name)
		}
	}
}
