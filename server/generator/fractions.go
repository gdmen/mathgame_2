package generator // import "garydmenezes.com/mathgame/server/generator"

import (
	"fmt"
	"strconv"
)

// tFractionSameDenom produces a/c op b/c where op is + or -.
// Used for grade 3+ (same-denominator fraction arithmetic).
func tFractionSameDenom(cfg GradeConfig, ops []Op, rng randFunc) (string, string, bool) {
	if !cfg.AllowFrac || cfg.MaxFracDenom < 2 {
		return "", "", false
	}
	// Pick +/- (fraction mul/div at this level is different pedagogy).
	op := OpAdd
	if rng(2) == 1 {
		op = OpSub
	}
	// Operations filter: only emit if +/- are in the allowed ops
	hasAdd, hasSub := false, false
	for _, o := range ops {
		if o == OpAdd {
			hasAdd = true
		}
		if o == OpSub {
			hasSub = true
		}
	}
	if op == OpAdd && !hasAdd {
		op = OpSub
	}
	if op == OpSub && !hasSub {
		op = OpAdd
	}
	if (op == OpAdd && !hasAdd) || (op == OpSub && !hasSub) {
		return "", "", false
	}

	denom := randIntRange(2, cfg.MaxFracDenom)
	// Keep numerators smaller than denom for proper fractions (except sometimes we
	// allow improper to teach concept; keep it simple for now).
	aNum := randIntRange(1, denom-1)
	bNum := randIntRange(1, denom-1)
	if op == OpSub && bNum > aNum && !cfg.AllowNeg {
		aNum, bNum = bNum, aNum
	}
	resultNum := compute(aNum, op, bNum)
	// Simplify result
	resultNum, resultDenom := simplifyFrac(resultNum, denom)
	expr := fmt.Sprintf("%d/%d %s %d/%d", aNum, denom, opSymbol(op), bNum, denom)
	return expr, fracAnswer(resultNum, resultDenom), true
}

// tFractionDiffDenom produces a/c op b/d with unlike denominators.
// Used for grade 4+ (unlike-denominator fraction arithmetic).
func tFractionDiffDenom(cfg GradeConfig, ops []Op, rng randFunc) (string, string, bool) {
	if !cfg.AllowFrac || cfg.MaxFracDenom < 3 {
		return "", "", false
	}
	op := OpAdd
	if rng(2) == 1 {
		op = OpSub
	}
	hasAdd, hasSub := false, false
	for _, o := range ops {
		if o == OpAdd {
			hasAdd = true
		}
		if o == OpSub {
			hasSub = true
		}
	}
	if op == OpAdd && !hasAdd {
		op = OpSub
	}
	if op == OpSub && !hasSub {
		op = OpAdd
	}
	if (op == OpAdd && !hasAdd) || (op == OpSub && !hasSub) {
		return "", "", false
	}

	denomA := randIntRange(2, cfg.MaxFracDenom)
	denomB := randIntRange(2, cfg.MaxFracDenom)
	for i := 0; i < 5 && denomA == denomB; i++ {
		denomB = randIntRange(2, cfg.MaxFracDenom)
	}
	if denomA == denomB {
		return tFractionSameDenom(cfg, ops, rng)
	}

	aNum := randIntRange(1, denomA-1)
	bNum := randIntRange(1, denomB-1)
	commonDenom := lcm(denomA, denomB)
	aScaled := aNum * (commonDenom / denomA)
	bScaled := bNum * (commonDenom / denomB)
	if op == OpSub && bScaled > aScaled && !cfg.AllowNeg {
		aNum, bNum = bNum, aNum
		denomA, denomB = denomB, denomA
		aScaled = aNum * (commonDenom / denomA)
		bScaled = bNum * (commonDenom / denomB)
	}
	resultNum := compute(aScaled, op, bScaled)
	resultNum, resultDenom := simplifyFrac(resultNum, commonDenom)
	expr := fmt.Sprintf("%d/%d %s %d/%d", aNum, denomA, opSymbol(op), bNum, denomB)
	return expr, fracAnswer(resultNum, resultDenom), true
}

// simplifyFrac reduces a fraction to lowest terms.
func simplifyFrac(num, denom int) (int, int) {
	if denom == 0 {
		return num, 1
	}
	if num == 0 {
		return 0, 1
	}
	g := gcd(num, denom)
	if denom < 0 {
		num, denom = -num, -denom
	}
	return num / g, denom / g
}

// fracAnswer formats a fraction answer string. Integer results render
// without the denominator ("3" not "3/1"). Matches AnswersEquivalent which
// treats "3", "3/1", and "6/2" as equivalent.
func fracAnswer(num, denom int) string {
	if denom == 1 {
		return strconv.Itoa(num)
	}
	return fmt.Sprintf("%d/%d", num, denom)
}
