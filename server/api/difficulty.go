package api

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Universal difficulty scale: 1-20.
// See docs/generator-versions.md for the design rationale.
//
// Difficulty 1-3   ~ Grade 1   (counting, basic add within 20)
// Difficulty 3-5   ~ Grade 2   (add/sub within 100)
// Difficulty 5-8   ~ Grade 3   (multiplication facts, simple fractions)
// Difficulty 8-11  ~ Grade 4   (multi-digit mul, fraction add/sub)
// Difficulty 11-14 ~ Grade 5   (unlike-denom fractions, decimals)
// Difficulty 14-16 ~ Grade 6-7 (negatives, proportional reasoning)
// Difficulty 16-20 ~ Grade 8+  (algebra, exponents, systems)

// Patterns for feature extraction. Pre-compiled at package init for speed.
var (
	reNumber   = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	reFraction = regexp.MustCompile(`\d+/\d+`)
	reText     = regexp.MustCompile(`\\text\{[^}]*\}`)
	reVariable = regexp.MustCompile(`\b[a-zA-Z]\b`)
	reExponent = regexp.MustCompile(`\^`)
)

// problemFeatures holds the observable attributes of an expression used to
// compute its universal difficulty score.
type problemFeatures struct {
	maxMagnitude float64
	numOps       int
	hasSub       bool
	hasMul       bool
	hasDiv       bool
	hasExp       bool
	numFractions int
	sameDenom    bool
	hasNegatives bool
	hasVariables bool
	isWord       bool
	hasMissing   bool
}

// ComputeProblemDifficulty returns a universal difficulty score on a 1-20 scale
// for any problem expression. Works for both heuristic (plain arithmetic) and
// LLM (may include \text{} word problems, LaTeX, variables) formats.
//
// The score is a log-compressed composite of four factors:
//   - magnitude  (how big are the numbers)
//   - op_weight  (hardest operation present)
//   - concept    (fractions, negatives, variables, word problem)
//   - structure  (number of ops, missing-number templates)
func ComputeProblemDifficulty(expr string) float64 {
	if strings.TrimSpace(expr) == "" {
		return 3.0
	}
	f := parseProblemFeatures(expr)

	// Magnitude: log10 of the largest operand. +0.3 floor so small numbers still contribute.
	magnitude := math.Log10(f.maxMagnitude+1) + 0.3

	// Op weight: take the hardest operation present. Addition is base (1.0).
	opWeight := 1.0
	if f.hasSub {
		opWeight = math.Max(opWeight, 1.1)
	}
	if f.hasMul {
		opWeight = math.Max(opWeight, 2.2)
	}
	if f.hasDiv {
		opWeight = math.Max(opWeight, 2.8)
	}
	if f.hasExp {
		opWeight = math.Max(opWeight, 4.0)
	}

	// Concept load: multiplicative factors for each concept.
	concept := 1.0
	if f.numFractions > 0 {
		if f.sameDenom {
			concept *= 1.4
		} else {
			concept *= 3.0
		}
	}
	if f.hasNegatives {
		concept *= 1.3
	}
	if f.hasVariables {
		concept *= 5.0 // algebra is a big jump
	}
	if f.isWord && !f.hasVariables {
		// Word problem without variables. Variables already capture algebraic difficulty;
		// don't double-count.
		concept *= 1.3
	}

	// Structure: more operations and missing-number templates bump difficulty.
	structure := 1.0 + 0.15*float64(maxInt(0, f.numOps-1))
	if f.hasMissing {
		structure += 0.2
	}

	raw := magnitude * opWeight * concept * structure

	// Log-compress raw into the 1-20 scale. Tuned so 3+5 lands near 3.5,
	// multi-digit mul near 10, algebra near 15.
	const (
		rawMin   = 0.5
		rawMax   = 15.0
		scaleMin = 1.0
		scaleMax = 20.0
	)
	num := math.Log(raw+1) - math.Log(rawMin+1)
	den := math.Log(rawMax+1) - math.Log(rawMin+1)
	scaled := scaleMin + (scaleMax-scaleMin)*num/den
	if scaled < scaleMin {
		scaled = scaleMin
	}
	if scaled > scaleMax {
		scaled = scaleMax
	}
	return scaled
}

// parseProblemFeatures extracts observable attributes from an expression.
// Handles plain arithmetic, LaTeX word-problem wrapping (\text{...}), fractions,
// single-letter variables, and missing-number placeholders (?).
func parseProblemFeatures(expr string) problemFeatures {
	var f problemFeatures

	// Word-problem detection via \text{}. Don't strip the content - numbers
	// inside the text are still real math content ("2 apples and 3 oranges").
	f.isWord = reText.MatchString(expr)

	// Fractions (a/b). Inspect denominators to distinguish same-denom (grade 3)
	// from unlike-denom (grade 4+).
	fracs := reFraction.FindAllString(expr, -1)
	f.numFractions = len(fracs)
	if f.numFractions > 0 {
		denoms := make(map[string]bool)
		for _, fr := range fracs {
			parts := strings.SplitN(fr, "/", 2)
			if len(parts) == 2 {
				denoms[parts[1]] = true
			}
		}
		f.sameDenom = len(denoms) <= 1
	}

	// Magnitude: largest absolute number. Parse through fractions as well so
	// "3/8" contributes magnitude 8 (the denominator).
	nums := reNumber.FindAllString(expr, -1)
	for _, n := range nums {
		v, err := strconv.ParseFloat(n, 64)
		if err != nil {
			continue
		}
		m := math.Abs(v)
		if m > f.maxMagnitude {
			f.maxMagnitude = m
		}
	}

	// Operations. Use spacing rules to distinguish division ops from fraction
	// slashes. This matches the formatting convention of heuristic_1.0.
	//   "a * b" -> multiplication
	//   "a / b" -> division
	//   "a/b"   -> fraction
	f.hasSub = strings.Contains(expr, " - ") ||
		strings.Contains(expr, "- ") && !strings.HasPrefix(strings.TrimSpace(expr), "-")
	f.hasMul = strings.Contains(expr, " * ") ||
		strings.Contains(expr, "*") && !reExponent.MatchString(expr)
	f.hasDiv = strings.Contains(expr, " / ") || strings.Contains(expr, "÷")
	f.hasExp = strings.Contains(expr, "^")

	// Count ops. Each top-level operator counts once; fraction internals don't.
	// Simple approach: count +, -, *, ÷ tokens when surrounded by spaces.
	f.numOps = strings.Count(expr, " + ") +
		strings.Count(expr, " - ") +
		strings.Count(expr, " * ") +
		strings.Count(expr, " / ") +
		strings.Count(expr, " ÷ ")
	// If there are fractions, the slashes inside the fraction aren't ops; our
	// space-based count already excludes them. But we need at least 1 op if
	// the expression has any ops at all (e.g., "1/2 + 1/2" has 1 op).
	// If no spaced ops detected but there's still an arithmetic operator
	// somewhere, count 1.
	if f.numOps == 0 {
		if strings.ContainsAny(expr, "+-*") && !strings.HasPrefix(strings.TrimSpace(expr), "-") {
			f.numOps = 1
		}
	}

	// Negatives: leading minus (e.g., "-3 + 5") or " -N" without a preceding number.
	trimmed := strings.TrimSpace(expr)
	if strings.HasPrefix(trimmed, "-") {
		f.hasNegatives = true
	}
	if strings.Contains(expr, "(-") {
		f.hasNegatives = true
	}

	// Variables: single-letter identifier outside of \text{}. We intentionally
	// look at the whole expression - "solve for x: 3x + 7 = 22" counts because
	// the x appears literally and represents algebra.
	if reVariable.MatchString(expr) {
		// Filter out common LaTeX command letters that aren't math variables.
		// \text matches but we don't want text letters to count. So check:
		// strip \text{...} and re-check.
		exprNoText := reText.ReplaceAllString(expr, "")
		if reVariable.MatchString(exprNoText) {
			f.hasVariables = true
		} else {
			// All single-letter matches were inside \text{}. Look again inside text
			// for standalone math variables (e.g., "Solve for x: ..." has x both
			// as word and as variable in "3x").
			// Heuristic: "Nx" pattern (digit followed by letter) indicates variable.
			if regexp.MustCompile(`\d[a-zA-Z]`).MatchString(expr) {
				f.hasVariables = true
			}
		}
	}

	// Missing-number placeholder (? template).
	f.hasMissing = strings.Contains(expr, "?")

	return f
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
