// difficulty.go: the universal problem-difficulty formula and the per-bitmap
// difficulty ceiling.
//
// A change to the formula's output semantics (factors, multipliers, detection)
// requires a DifficultyVersion bump + a recompute_problem_difficulty run on
// deploy.

package mathcore

import (
	"math"
	"regexp"
	"strings"
)

// The difficulty scale (formula v0.3):
//
// Open-ended, floored at 1.0, NO upper clamp. Inputs are bounded by
// construction (MaxChainLen, LargeMaxOperand, the fixed multiplier set), so
// the system maximum is ~62, not infinity. 1-20 is the band for normal
// one/two-concept problems; scores above 20 mean the problem stacks multiple
// concepts. There is deliberately no clamp at the top: selection (the
// +/-1.5 window) and adaptive difficulty need resolution there, and a clamp
// would store a moderately hard equation (raw 22) and a six-concept monster
// (raw 2930) as the same value.
//
// Rough bands for one/two-concept problems:
//   1-3   counting, basic addition within 20
//   3-5   add/sub within 100
//   5-8   multiplication facts, simple fractions
//   8-11  multi-digit multiplication, fraction add/sub
//   11-14 mismatched-denominator fractions, decimals
//   14-16 negatives, proportional reasoning
//   16+   algebra and multi-concept stacks

// DifficultyVersion is stamped on every problem row whose stored
// difficulty was produced by the current code. Bump it whenever
// ComputeProblemDifficulty's output semantics change (any constant tweak,
// any new feature in parseProblemFeatures, any change to the compression
// curve). 0.x while the scale is still in active calibration; 1.0 once
// stable. Minor bumps for tuning, major bumps for structural rewrites.
const DifficultyVersion = "0.3"

// Shared shape constants - used by BOTH the generators' option mapping and
// MaxDiffForBitmap so the ceiling and what generation can actually produce
// stay in lockstep. Changing either is a formula-semantics change in
// ceiling terms: bump DifficultyVersion and recompute.
const (
	// MaxChainLen is the maximum operator count generators emit when
	// CHAINED_OPERATIONS is enabled.
	MaxChainLen = 5
	// LargeMaxOperand is the maximum operand when LARGE_NUMBERS is enabled
	// (digit-magnitude for decimals shares the bound).
	LargeMaxOperand = 9999
)

// Magnitude bracket boundaries (maxMagnitude; digit-based for decimals).
// Shared shape constants like MaxChainLen/LargeMaxOperand: the generators'
// option mapping and the ceiling both read them, so they are exported.
const (
	SmallMaxOperand  = 12 // default envelope: no magnitude bits enabled
	MediumMaxOperand = 99 // MEDIUM_NUMBERS
)

// MinTargetDifficulty is the lowest selectable target_difficulty. The
// easiest problems the default envelope produces score ~3.5 ("3 + 5" =
// 3.6), so a target below 3 aims the selection window at a band the pool
// barely populates. The settings slider mirrors this floor
// (web/src/bitmap_validation.js MIN_TARGET_DIFFICULTY).
const MinTargetDifficulty = 3.0

// Formula v0.3 factor constants, combined as
// magnitude * opWeight * concept * structure (see ComputeProblemDifficulty).
const (
	// Op weights: opWeight is the MAX over the operators present
	// (addition is the 1.0 baseline).
	weightSub = 1.1
	weightMul = 2.2
	weightDiv = 2.8

	// Concept multipliers: each enabled concept MULTIPLIES into the
	// concept factor.
	conceptFractions  = 2.0 // same denominators
	conceptMismatched = 1.5 // stacks on conceptFractions -> net 3.0; the
	// FRACTIONS dependency guarantees the base factor is always present
	conceptNegatives = 1.3
	conceptVariable  = 5.0 // coefficient / multi-occurrence letter forms
	conceptWord      = 1.3 // stacks with conceptVariable as of v0.2
	conceptPEMDAS    = 1.5
	conceptDecimals  = 2.0
	conceptPercent   = 2.0

	// Structure increments: ADDED to the structure factor's 1.0 base.
	structurePerExtraOp = 0.15 // per operator beyond the first
	structureMissing    = 0.2  // missing-number blank present
)

// Prose-number scanning (difficulty side of the prose rule): magnitude,
// decimal, and percent scanning for DIFFICULTY reads numerals inside
// \text{...} (a word problem about 47 apples has maxMagnitude 47). Bit
// detection never does - WORD problems' bits come from the validator.
var reProseNumber = regexp.MustCompile(`(\d+(?:\.\d+)?)(%?)`)

// Legacy-fallback patterns, used only when the lexer rejects an expression
// (out-of-alphabet legacy rows, surfaced by the backfill census).
var (
	reNumber   = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	reFraction = regexp.MustCompile(`\d+/\d+`)
	reText     = regexp.MustCompile(`\\text\{[^}]*\}`)
)

// problemFeatures holds the observable attributes of an expression used to
// compute its difficulty score and (in the generation pipeline) its
// problem-type bits.
//
// The prose rule: structural fields (operators, unknowns, fractions, PEMDAS)
// are token-level only - \text{...} contents can never contribute to them.
// The magnitude/decimal/percent fields deliberately DO include prose
// numerals for the difficulty side; the *Symbolic variants are the
// bit-detection side and exclude prose.
type problemFeatures struct {
	// Difficulty-side (prose-inclusive).
	maxMagnitude float64 // digit-based for decimals (0.75 -> 75)
	hasDecimals  bool
	hasPercent   bool

	// Bit-detection side (symbolic tokens only).
	hasDecimalsSymbolic bool
	hasPercentSymbolic  bool

	// Structural (token-level only, per the prose rule).
	hasAdd, hasSub, hasMul, hasDiv bool
	numOps                         int
	numFractions                   int
	sameDenom                      bool
	hasNegatives                   bool
	isWord                         bool
	hasMissing                     bool
	questionMarks                  int // '?' occurrences (>1 is rejected at insert)
	distinctUnknowns               int // distinct letters + (1 if any '?')
	hasVariables                   bool
	requiresPEMDAS                 bool

	// rewritten is true when a lone bare variable was rewritten to '?'
	// (stage 1.5). rewrittenExpr is the spliced expression string.
	rewritten     bool
	rewrittenExpr string

	// lexFailed is true when the expression is outside the alphabet and the
	// legacy regex fallback produced these features instead.
	lexFailed bool
}

// parseProblemFeatures extracts observable attributes from an expression.
// Pipeline: normalize -> lex -> rewrite lone bare variable -> extract.
// On lex failure (out-of-alphabet legacy rows), falls back to regex
// extraction so the recompute tools never crash on the existing pool.
func parseProblemFeatures(expr string) problemFeatures {
	norm := NormalizeExpression(expr)
	toks, lexErr := LexExpression(norm)
	if lexErr != nil {
		return parseProblemFeaturesFallback(norm)
	}
	toks, rewrittenExpr, rewrote := RewriteLoneVariable(toks, norm)

	var f problemFeatures
	f.rewritten = rewrote
	f.rewrittenExpr = rewrittenExpr

	letters := map[byte]bool{}
	denoms := map[int64]bool{}

	for _, t := range toks {
		switch t.Kind {
		case TokText:
			f.isWord = true
			// Difficulty-side prose scan: numerals, decimals, percents.
			for _, m := range reProseNumber.FindAllStringSubmatch(t.Content, -1) {
				digits := strings.Replace(m[1], ".", "", 1)
				digits = strings.TrimLeft(digits, "0")
				var v float64
				for _, c := range digits {
					v = v*10 + float64(c-'0')
				}
				if v > f.maxMagnitude {
					f.maxMagnitude = v
				}
				if strings.Contains(m[1], ".") {
					f.hasDecimals = true
				}
				if m[2] == "%" {
					f.hasPercent = true
				}
			}
		case TokNumber:
			if t.DigitMagnitude > f.maxMagnitude {
				f.maxMagnitude = t.DigitMagnitude
			}
			if t.IsDecimal {
				f.hasDecimals = true
				f.hasDecimalsSymbolic = true
			}
			if t.IsPercent {
				f.hasPercent = true
				f.hasPercentSymbolic = true
			}
			if t.IsNegative {
				f.hasNegatives = true
			}
		case TokFraction:
			f.numFractions++
			denoms[t.Den] = true
			if t.IsNegative {
				f.hasNegatives = true
			}
			n := t.Num
			if n < 0 {
				n = -n
			}
			if float64(n) > f.maxMagnitude {
				f.maxMagnitude = float64(n)
			}
			if float64(t.Den) > f.maxMagnitude {
				f.maxMagnitude = float64(t.Den)
			}
		case TokOperator:
			f.numOps++
			switch t.Op {
			case '+':
				f.hasAdd = true
			case '-':
				f.hasSub = true
			case '*':
				f.hasMul = true
			case '/':
				f.hasDiv = true
			}
		case TokMissing:
			f.hasMissing = true
			f.questionMarks++
		case TokVariable:
			f.hasVariables = true
			letters[t.Letter] = true
		}
	}

	f.sameDenom = len(denoms) <= 1
	f.distinctUnknowns = len(letters)
	if f.questionMarks > 0 {
		f.distinctUnknowns++
	}
	f.requiresPEMDAS = requiresPEMDAS(toks)
	return f
}

// parseProblemFeaturesFallback is the degraded extraction for expressions
// the lexer rejects (out-of-alphabet legacy rows); the backfill census
// reports such rows for cull-or-fix.
//
// Deliberately cruder than the lexer path: no variable detection (a
// lex-rejected row never gets the x5.0 multiplier), no missing-number
// detection, no PEMDAS. These only affect rows that are outside the
// alphabet anyway - their difficulty is best-effort until the census
// disposes of them.
func parseProblemFeaturesFallback(expr string) problemFeatures {
	f := problemFeatures{lexFailed: true}
	f.isWord = reText.MatchString(expr)

	fracs := reFraction.FindAllString(expr, -1)
	f.numFractions = len(fracs)
	denoms := map[string]bool{}
	for _, fr := range fracs {
		parts := strings.SplitN(fr, "/", 2)
		if len(parts) == 2 {
			denoms[parts[1]] = true
		}
	}
	f.sameDenom = len(denoms) <= 1

	for _, m := range reNumber.FindAllString(expr, -1) {
		neg := strings.HasPrefix(m, "-")
		digits := strings.TrimPrefix(m, "-")
		if strings.Contains(digits, ".") {
			f.hasDecimals = true
		}
		digits = strings.Replace(digits, ".", "", 1)
		digits = strings.TrimLeft(digits, "0")
		var v float64
		for _, c := range digits {
			v = v*10 + float64(c-'0')
		}
		if v > f.maxMagnitude {
			f.maxMagnitude = v
		}
		_ = neg
	}

	// Operator counting, spaced form only (legacy rows write ops
	// space-delimited; unspaced ops here stay uncounted).
	f.hasSub = strings.Contains(expr, " - ")
	f.hasMul = strings.Contains(expr, " * ")
	f.hasDiv = strings.Contains(expr, " / ")
	f.hasAdd = strings.Contains(expr, " + ")
	f.numOps = strings.Count(expr, " + ") + strings.Count(expr, " - ") +
		strings.Count(expr, " * ") + strings.Count(expr, " / ")

	trimmed := strings.TrimSpace(expr)
	if strings.HasPrefix(trimmed, "-") || strings.Contains(expr, "(-") {
		f.hasNegatives = true
	}
	return f
}

// ComputeProblemDifficulty returns the universal difficulty score for a
// problem expression. Pure and deterministic in the expression alone - the
// DifficultyVersion machinery depends on this: rows stamped
// with the current version are skipped by recompute without re-evaluation.
//
// Formula v0.3 (canonical spec in docs/problem-generation.md):
//
//	magnitude = log10(maxMagnitude+1) + 0.3   (digit-based for decimals)
//	opWeight  = max over present ops: add 1.0 | sub 1.1 | mul 2.2 | div 2.8
//	concept   = product of the enabled concept multipliers (see constants)
//	structure = 1 + 0.15*max(0, numOps-1), +0.2 if missing-number
//	raw       = magnitude * opWeight * concept * structure
//	scaled    = 1 + 19 * (ln(raw+1) - ln(1.5)) / (ln(16) - ln(1.5))
//
// floored at 1.0; NO upper clamp (see the scale comment at the top of this
// file).
// ComputeProblemDifficulty scores a problem. For a word problem, pass its
// symbolic_expression (the bare symbolic form it computes) as symbolic and the
// difficulty is scored from that, with the word concept applied; the prose expr
// is ignored. For every other problem symbolic is empty and expr is scored
// directly.
func ComputeProblemDifficulty(expr, symbolic string) float64 {
	return ComputeDifficultyBreakdownFor(expr, symbolic).Scaled
}

// ComputeDifficultyBreakdownFor returns the breakdown a problem is scored by:
// from its symbolic form (a word problem's symbolic_expression, with the word
// concept applied) when symbolic is non-empty, else from expr directly. The
// single source of truth for the expr-vs-symbolic dispatch.
func ComputeDifficultyBreakdownFor(expr, symbolic string) DifficultyBreakdown {
	if symbolic != "" {
		return computeBreakdown(symbolic, true)
	}
	return computeBreakdown(expr, false)
}

// ConceptFactor is one enabled concept multiplier and the factor it contributed,
// for inspection tools. Presentation (labels/formatting) is the caller's job.
type ConceptFactor struct {
	Name   string  `json:"name"`
	Factor float64 `json:"factor"`
}

// DifficultyBreakdown exposes the intermediate factors that
// ComputeProblemDifficulty combines. It exists for inspection (the admin
// difficulty-calibration page) and does NOT affect scoring:
// ComputeProblemDifficulty returns Scaled, computed by exactly the formula
// below. Keep this output-identical to the documented v0.3 formula - any change
// to Scaled requires a DifficultyVersion bump (see docs/problem-generation.md).
type DifficultyBreakdown struct {
	Magnitude    float64         `json:"magnitude"`
	OpWeight     float64         `json:"op_weight"`
	Concept      float64         `json:"concept"`
	Structure    float64         `json:"structure"`
	Raw          float64         `json:"raw"`
	Scaled       float64         `json:"scaled"`
	MaxMagnitude float64         `json:"max_magnitude"`
	NumOps       int             `json:"num_ops"`
	HasMissing   bool            `json:"has_missing"`
	Concepts     []ConceptFactor `json:"concepts"` // enabled concept multipliers, in formula order
}

// ComputeDifficultyBreakdown computes the difficulty score and every
// intermediate factor. ComputeProblemDifficulty delegates here, so the two can
// never disagree.
func ComputeDifficultyBreakdown(expr string) DifficultyBreakdown {
	return computeBreakdown(expr, false)
}

// computeBreakdown scores scoredExpr through the universal formula. forceWord
// credits the word concept even when scoredExpr carries no \text{} marker -
// used to score a word problem from its (bare-symbolic) symbolic_expression
// while still applying the ×1.3 word concept.
func computeBreakdown(scoredExpr string, forceWord bool) DifficultyBreakdown {
	if strings.TrimSpace(scoredExpr) == "" {
		return DifficultyBreakdown{Scaled: 3.0}
	}
	f := parseProblemFeatures(scoredExpr)
	if forceWord {
		f.isWord = true
	}

	magnitude := math.Log10(f.maxMagnitude+1) + 0.3

	opWeight := 1.0
	if f.hasSub {
		opWeight = math.Max(opWeight, weightSub)
	}
	if f.hasMul {
		opWeight = math.Max(opWeight, weightMul)
	}
	if f.hasDiv {
		opWeight = math.Max(opWeight, weightDiv)
	}

	concept := 1.0
	var concepts []ConceptFactor
	if f.numFractions > 0 {
		concept *= conceptFractions
		concepts = append(concepts, ConceptFactor{"fractions", conceptFractions})
		if f.numFractions >= 2 && !f.sameDenom {
			concept *= conceptMismatched
			concepts = append(concepts, ConceptFactor{"mismatched", conceptMismatched})
		}
	}
	if f.hasNegatives {
		concept *= conceptNegatives
		concepts = append(concepts, ConceptFactor{"negatives", conceptNegatives})
	}
	if f.hasVariables {
		concept *= conceptVariable
		concepts = append(concepts, ConceptFactor{"variable", conceptVariable})
	}
	if f.isWord {
		concept *= conceptWord
		concepts = append(concepts, ConceptFactor{"word", conceptWord})
	}
	if f.requiresPEMDAS {
		concept *= conceptPEMDAS
		concepts = append(concepts, ConceptFactor{"pemdas", conceptPEMDAS})
	}
	if f.hasDecimals {
		concept *= conceptDecimals
		concepts = append(concepts, ConceptFactor{"decimals", conceptDecimals})
	}
	if f.hasPercent {
		concept *= conceptPercent
		concepts = append(concepts, ConceptFactor{"percent", conceptPercent})
	}

	structure := 1.0 + structurePerExtraOp*float64(maxInt(0, f.numOps-1))
	if f.hasMissing {
		structure += structureMissing
	}

	raw := magnitude * opWeight * concept * structure
	return DifficultyBreakdown{
		Magnitude:    magnitude,
		OpWeight:     opWeight,
		Concept:      concept,
		Structure:    structure,
		Raw:          raw,
		Scaled:       compressRaw(raw),
		MaxMagnitude: f.maxMagnitude,
		NumOps:       f.numOps,
		HasMissing:   f.hasMissing,
		Concepts:     concepts,
	}
}

// compressRaw maps a raw composite onto the difficulty scale with a log
// curve. The two anchor pairs define the curve's slope, NOT a range: raw 0.5
// maps to 1.0 and raw 15 maps to 20.0 (anchors chosen to preserve the
// scale's original 1-20 calibration), and the curve continues past the
// upper anchor (no clamp). The floor at 1.0 is the only cutoff: degenerate
// expressions (0 + 0) must not score below the scale minimum.
func compressRaw(raw float64) float64 {
	const (
		rawAnchorLo   = 0.5
		rawAnchorHi   = 15.0
		scaleAnchorLo = 1.0
		scaleAnchorHi = 20.0
		scaleFloor    = 1.0
	)
	num := math.Log(raw+1) - math.Log(rawAnchorLo+1)
	den := math.Log(rawAnchorHi+1) - math.Log(rawAnchorLo+1)
	scaled := scaleAnchorLo + (scaleAnchorHi-scaleAnchorLo)*num/den
	if scaled < scaleFloor {
		scaled = scaleFloor
	}
	return scaled
}

// MaxDiffForBitmap returns the difficulty ceiling for a settings bitmap: the
// difficulty of the HARDEST problem that can actually be constructed under
// the enabled bits (reachable problems - shapes the per-problem rules allow).
//
// WHY THIS EXISTS (do not break this property): adaptive difficulty ratchets
// target_difficulty upward on success. Without a ceiling, a kid's target can
// drift above the hardest problem their bitmap can express - into a band
// that is empty BY CONSTRUCTION. Selection's +/-1.5 window then never
// matches anything, every serve falls through to the synchronous fallback,
// and the system churns permanently, because generation cannot fill an
// unreachable band. The ceiling pins target_difficulty to what the envelope
// (the set of problem shapes the user's settings allow) can produce.
//
// Cheap enough to compute on demand; cacheable per bitmap value.
//
// Either/or ceiling rule: when two features can't appear in the same problem,
// compute the ceiling both ways and use the higher - never multiply both in.
// MISSING_NUMBER and SINGLE_VARIABLE are per-problem mutually exclusive (at
// most one distinct unknown per problem), so the variable branch (x5.0
// concept, no +0.2 structure) and the missing branch (+0.2 structure, no
// x5.0) are computed separately. Multiplying both in would claim a ceiling
// no constructible problem reaches - recreating the empty-band drift this
// function exists to prevent.
func MaxDiffForBitmap(bitmap uint64) float64 {
	pt := ProblemType(bitmap)

	maxOperand := float64(SmallMaxOperand)
	if pt&MEDIUM_NUMBERS != 0 {
		maxOperand = float64(MediumMaxOperand)
	}
	if pt&LARGE_NUMBERS != 0 {
		maxOperand = float64(LargeMaxOperand)
	}
	magnitude := math.Log10(maxOperand+1) + 0.3

	opWeight := 1.0
	if pt&SUBTRACTION != 0 {
		opWeight = math.Max(opWeight, weightSub)
	}
	if pt&MULTIPLICATION != 0 {
		opWeight = math.Max(opWeight, weightMul)
	}
	if pt&DIVISION != 0 {
		opWeight = math.Max(opWeight, weightDiv)
	}

	// Concept multipliers common to both either/or branches. A plain
	// product: sub-feature bits (MISMATCHED) stack their increment on their
	// parent's factor.
	concept := 1.0
	if pt&FRACTIONS != 0 {
		concept *= conceptFractions
	}
	if pt&MISMATCHED_DENOMINATORS != 0 {
		concept *= conceptMismatched
	}
	if pt&NEGATIVES != 0 {
		concept *= conceptNegatives
	}
	if pt&WORD != 0 {
		concept *= conceptWord
	}
	if pt&PEMDAS != 0 {
		concept *= conceptPEMDAS
	}
	if pt&DECIMALS != 0 {
		concept *= conceptDecimals
	}
	if pt&PERCENTAGES != 0 {
		concept *= conceptPercent
	}

	structure := 1.0
	if pt&CHAINED_OPERATIONS != 0 {
		structure = 1.0 + structurePerExtraOp*float64(MaxChainLen-1)
	}

	// Either/or branches over the reachable problem space.
	rawNeither := magnitude * opWeight * concept * structure
	rawBest := rawNeither
	if pt&SINGLE_VARIABLE != 0 {
		if r := magnitude * opWeight * concept * conceptVariable * structure; r > rawBest {
			rawBest = r
		}
	}
	if pt&MISSING_NUMBER != 0 {
		if r := magnitude * opWeight * concept * (structure + structureMissing); r > rawBest {
			rawBest = r
		}
	}
	return compressRaw(rawBest)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
