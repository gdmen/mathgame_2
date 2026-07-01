package generator

import (
	"math"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"garydmenezes.com/mathgame/server/mathcore"
)

var reFracLiteral = regexp.MustCompile(`(\d+)/(\d+)`)

// TestBuildProblem_FractionMagnitudeOnIntegers pins the fraction/magnitude
// invariant: in a fraction envelope with mul/div and a magnitude bracket, every
// fraction operand stays within the SMALL bracket while the MEDIUM/LARGE
// magnitude is carried by an integer operand (24 / (2/3) = 36). It guards
// against a regression to inflated numerators (58/3, 385/12) and proves the
// integer-carry actually reaches the target (magnitude wasn't just dropped).
func TestBuildProblem_FractionMagnitudeOnIntegers(t *testing.T) {
	cases := []struct {
		name string
		bm   mathcore.ProblemType
	}{
		{"div frac medium", mathcore.DIVISION | mathcore.FRACTIONS | mathcore.MEDIUM_NUMBERS},
		{"div frac large", mathcore.DIVISION | mathcore.FRACTIONS | mathcore.MEDIUM_NUMBERS | mathcore.LARGE_NUMBERS},
		{"mul frac medium", mathcore.MULTIPLICATION | mathcore.FRACTIONS | mathcore.MEDIUM_NUMBERS},
	}
	rng := rand.New(rand.NewSource(20260701))
	for _, c := range cases {
		ceil := mathcore.MaxDiffForBitmap(uint64(c.bm))
		var maxDiff float64
		for i := 0; i < 2000; i++ {
			expr, _, err := BuildProblem(c.bm, ceil, rng)
			if err != nil {
				t.Fatalf("%s: BuildProblem: %v", c.name, err)
			}
			for _, m := range reFracLiteral.FindAllStringSubmatch(expr, -1) {
				num, _ := strconv.Atoi(m[1])
				den, _ := strconv.Atoi(m[2])
				if num > mathcore.SmallMaxOperand || den > mathcore.SmallMaxOperand {
					t.Errorf("%s: fraction operand %d/%d exceeds SMALL bracket (%d) in %q",
						c.name, num, den, mathcore.SmallMaxOperand, expr)
				}
			}
			if d := mathcore.ComputeProblemDifficulty(mathcore.AdmitExpression(expr).Expr, ""); d > maxDiff {
				maxDiff = d
			}
		}
		// The bracket magnitude must be reachable via an integer operand, so the
		// cell still hits its ceiling window despite the small-fraction cap.
		if maxDiff < ceil-targetWindow {
			t.Errorf("%s: max difficulty %.1f fell short of ceiling %.1f-window — magnitude not carried by an integer operand",
				c.name, maxDiff, ceil)
		}
	}
}

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

// TestDisplaySymbolicRoundTrip pins the storage invariant for heuristic_2.0's
// two-column split: the caller stores the canonical grammar (BuildProblem's
// output) in symbolic_expression and the display skin (\frac, \div) in
// expression. The two are bridged by NormalizeExpression — it folds both forms
// to the same canonical grammar — so NormalizeExpression(expression) ==
// NormalizeExpression(symbolic_expression). It also proves the skin never
// perturbs scoring: (display, grammar) scores identically to the grammar alone
// and earns no word bonus (the display carries no \text{}).
func TestDisplaySymbolicRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(20260701))
	var checked, withFrac, withDiv int
	for _, bm := range representativeBitmaps() {
		ceil := mathcore.MaxDiffForBitmap(uint64(bm))
		lo := math.Ceil(mathcore.MinTargetDifficulty)
		for target := lo; target <= ceil; target += 1.0 {
			expr, _, err := BuildProblem(bm, target, rng)
			if err != nil {
				continue
			}
			grammar := mathcore.AdmitExpression(expr).Expr
			display := mathcore.DisplayExpression(grammar)
			checked++

			if a, b := mathcore.NormalizeExpression(display), mathcore.NormalizeExpression(grammar); a != b {
				t.Errorf("drift: NormalizeExpression(display)=%q != NormalizeExpression(grammar)=%q (%q, bm=%d)", a, b, display, bm)
			}
			if strings.Contains(display, `\text`) {
				t.Errorf("display carries \\text (would score as WORD): %q (bm=%d)", display, bm)
			}
			if strings.Contains(display, " / ") {
				t.Errorf("display carries a bare division slash (should be \\div): %q (bm=%d)", display, bm)
			}
			grammarOnly := mathcore.ComputeProblemDifficulty(grammar, "")
			dual := mathcore.ComputeProblemDifficulty(display, grammar)
			if math.Abs(dual-grammarOnly) > 1e-9 {
				t.Errorf("scoring not preserved: grammar=%g display+symbolic=%g (%q, bm=%d)", grammarOnly, dual, display, bm)
			}
			if strings.Contains(display, `\frac`) {
				withFrac++
			}
			if strings.Contains(display, `\div`) {
				withDiv++
			}
		}
	}
	t.Logf("checked=%d withFracDisplay=%d withDivDisplay=%d", checked, withFrac, withDiv)
	if withFrac == 0 || withDiv == 0 {
		t.Errorf("skin not exercised: frac=%d div=%d", withFrac, withDiv)
	}
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

// TestBuildProblem_CoversLLMOnlyConcepts proves the LLM-offload claim: DECIMALS,
// PEMDAS, PERCENTAGES, and SINGLE_VARIABLE — concepts the LLM generator covers —
// are each actually produced by the heuristic when enabled and the target calls
// for them.
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

// TestBuildProblem_ComposesConcepts asserts the builder COMPOSES concepts —
// produces single problems carrying two or more concept/structure bits at once
// (e.g. division of a fraction, a decimal in a chain, a missing-number in a
// multiplication) rather than one concept per problem. Over many builds of a
// rich envelope it collects the distinct multi-concept combinations that occur
// and asserts a healthy variety, including division composed with a value
// concept.
func TestBuildProblem_ComposesConcepts(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	bitmap := mathcore.ADDITION | mathcore.SUBTRACTION | mathcore.MULTIPLICATION |
		mathcore.DIVISION | mathcore.MEDIUM_NUMBERS | mathcore.LARGE_NUMBERS |
		mathcore.CHAINED_OPERATIONS | mathcore.FRACTIONS | mathcore.MISMATCHED_DENOMINATORS |
		mathcore.DECIMALS | mathcore.PERCENTAGES | mathcore.NEGATIVES | mathcore.PEMDAS |
		mathcore.SINGLE_VARIABLE | mathcore.MISSING_NUMBER
	conceptBits := []mathcore.ProblemType{
		mathcore.FRACTIONS, mathcore.MISMATCHED_DENOMINATORS, mathcore.DECIMALS,
		mathcore.PERCENTAGES, mathcore.NEGATIVES, mathcore.PEMDAS,
		mathcore.SINGLE_VARIABLE, mathcore.MISSING_NUMBER,
	}
	combos := map[string]int{}
	divWithConcept := 0
	for i := 0; i < 6000; i++ {
		target := 6.0 + rng.Float64()*16.0
		expr, _, err := BuildProblem(bitmap, target, rng)
		if err != nil {
			t.Fatalf("BuildProblem: %v", err)
		}
		bm := mathcore.ProblemType(mathcore.DetectProblemTypeBitmap(expr))
		n := 0
		for _, c := range conceptBits {
			if bm&c != 0 {
				n++
			}
		}
		if n >= 2 {
			combos[concKey(bm, conceptBits)]++
		}
		// division composed with a value concept.
		if bm&mathcore.DIVISION != 0 && bm&(mathcore.FRACTIONS|mathcore.DECIMALS|mathcore.PERCENTAGES) != 0 {
			divWithConcept++
		}
	}
	t.Logf("distinct multi-concept combinations: %d; division+value-concept problems: %d", len(combos), divWithConcept)
	for k, n := range combos {
		t.Logf("  %s  x%d", k, n)
	}
	if len(combos) < 6 {
		t.Errorf("only %d distinct multi-concept combinations — builder is not composing", len(combos))
	}
	if divWithConcept == 0 {
		t.Errorf("never composed division with a value concept")
	}
}

// TestBuildProblem_ComposesValueConceptUnderMulDiv guards the specific silo the
// ComposesConcepts test can't see: that test builds from a fully-chained rich
// envelope, where a fraction can ride an additive term while '*'/'/' stay
// integer — so it passes even if a value concept never attaches directly to a
// multiplicative operator. These envelopes have NO additive operator, so the
// only way to compose is a genuine fraction/decimal operand under '*' or '/'
// (e.g. "3/8 * 5/3", "0.2 * 3"). A regression to integer-only multiplicative
// splits drops these to zero.
func TestBuildProblem_ComposesValueConceptUnderMulDiv(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	cases := []struct {
		name string
		bm   mathcore.ProblemType
	}{
		{"fraction under multiplication", mathcore.FRACTIONS | mathcore.MULTIPLICATION},
		{"fraction under division", mathcore.FRACTIONS | mathcore.DIVISION},
		{"mismatched fractions under multiplication",
			mathcore.FRACTIONS | mathcore.MISMATCHED_DENOMINATORS | mathcore.MULTIPLICATION},
		{"decimal under multiplication", mathcore.DECIMALS | mathcore.MULTIPLICATION},
	}
	for _, c := range cases {
		ceil := mathcore.MaxDiffForBitmap(uint64(c.bm))
		found := 0
		var ex string
		for i := 0; i < 3000; i++ {
			target := 6.0 + rng.Float64()*(ceil-6.0)
			expr, _, err := BuildProblem(c.bm, target, rng)
			if err != nil {
				t.Fatalf("%s: BuildProblem: %v", c.name, err)
			}
			if bm := mathcore.ProblemType(mathcore.DetectProblemTypeBitmap(expr)); bm&c.bm == c.bm {
				found++
				if ex == "" {
					ex = expr
				}
			}
		}
		if found == 0 {
			t.Errorf("%s: never composed the value concept with the operator — builder siloed", c.name)
		} else {
			t.Logf("%s: %d/3000 composed, e.g. %q", c.name, found, ex)
		}
	}
}

func concKey(bm mathcore.ProblemType, bits []mathcore.ProblemType) string {
	s := ""
	for _, c := range bits {
		if bm&c != 0 {
			if s != "" {
				s += "+"
			}
			s += mathcore.ProblemTypeToFeatures(c)[0]
		}
	}
	return s
}
