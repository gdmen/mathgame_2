package generator // import "garydmenezes.com/mathgame/server/generator"

import (
	"fmt"
	"strconv"
)

// Template produces a problem expression and its answer.
// Each template is parameterized by the grade config and a random source.
type Template func(cfg GradeConfig, ops []Op, rng randFunc) (expr string, answer string, ok bool)

// Missing-answer placeholder used in templates like "? + 5 = 12".
// KaTeX renders this as a readable question mark.
const blank = "?"

// tBasic produces a single binary expression: "a op b".
// For +/- keeps operands >= MinAddSub to avoid trivial (a+0, a-0) problems.
// For * keeps operands within [MinMul, MaxMul].
// For / picks a clean divisor so result is a whole number.
func tBasic(cfg GradeConfig, ops []Op, rng randFunc) (string, string, bool) {
	op := pickOp(ops, rng)
	switch op {
	case OpAdd, OpSub:
		a := randIntRange(cfg.MinAddSub, cfg.MaxAddSub)
		b := randIntRange(cfg.MinAddSub, cfg.MaxAddSub)
		if op == OpSub && !cfg.AllowNeg && b > a {
			a, b = b, a
		}
		if a == b && op == OpSub {
			// avoid a - a = 0
			b = randIntRange(cfg.MinAddSub, max(cfg.MinAddSub, a-1))
		}
		return formatBinary(a, op, b), strconv.Itoa(compute(a, op, b)), true
	case OpMul:
		if cfg.MaxMul < cfg.MinMul {
			return "", "", false
		}
		a := randIntRange(cfg.MinMul, cfg.MaxMul)
		b := randIntRange(cfg.MinMul, cfg.MaxMul)
		return formatBinary(a, op, b), strconv.Itoa(a * b), true
	case OpDiv:
		if cfg.MaxDivisor < 2 {
			return "", "", false
		}
		// Pick divisor and quotient first, then multiply to get dividend.
		// This guarantees a whole-number result.
		divisor := randIntRange(2, cfg.MaxDivisor)
		quotient := randIntRange(2, max(2, cfg.MaxDiv/divisor))
		dividend := divisor * quotient
		return formatBinary(dividend, op, divisor), strconv.Itoa(quotient), true
	}
	return "", "", false
}

// tMissing produces a missing-addend/factor template.
// Forms (picked randomly):
//
//	? + b = c   (answer: c - b)
//	a + ? = c   (answer: c - a)
//	? - b = c   (answer: c + b)
//	a - ? = c   (answer: a - c)
//	? * b = c   (answer: c / b, only if clean)
func tMissing(cfg GradeConfig, ops []Op, rng randFunc) (string, string, bool) {
	op := pickOp(ops, rng)
	// Position of the blank: 0 = left operand, 1 = right operand
	pos := rng(2)
	switch op {
	case OpAdd:
		a := randIntRange(cfg.MinAddSub, cfg.MaxAddSub)
		b := randIntRange(cfg.MinAddSub, cfg.MaxAddSub)
		c := a + b
		if pos == 0 {
			return fmt.Sprintf("%s + %d = %d", blank, b, c), strconv.Itoa(a), true
		}
		return fmt.Sprintf("%d + %s = %d", a, blank, c), strconv.Itoa(b), true
	case OpSub:
		a := randIntRange(cfg.MinAddSub, cfg.MaxAddSub)
		b := randIntRange(cfg.MinAddSub, a)
		c := a - b
		if pos == 0 {
			return fmt.Sprintf("%s - %d = %d", blank, b, c), strconv.Itoa(a), true
		}
		return fmt.Sprintf("%d - %s = %d", a, blank, c), strconv.Itoa(b), true
	case OpMul:
		if cfg.MaxMul < cfg.MinMul {
			return "", "", false
		}
		a := randIntRange(cfg.MinMul, cfg.MaxMul)
		b := randIntRange(cfg.MinMul, cfg.MaxMul)
		c := a * b
		if pos == 0 {
			return fmt.Sprintf("%s * %d = %d", blank, b, c), strconv.Itoa(a), true
		}
		return fmt.Sprintf("%d * %s = %d", a, blank, c), strconv.Itoa(b), true
	case OpDiv:
		if cfg.MaxDivisor < 2 {
			return "", "", false
		}
		divisor := randIntRange(2, cfg.MaxDivisor)
		quotient := randIntRange(2, max(2, cfg.MaxDiv/divisor))
		dividend := divisor * quotient
		if pos == 0 {
			return fmt.Sprintf("%s / %d = %d", blank, divisor, quotient), strconv.Itoa(dividend), true
		}
		return fmt.Sprintf("%d / %s = %d", dividend, blank, quotient), strconv.Itoa(divisor), true
	}
	return "", "", false
}

// tMultiOp produces a multi-term expression like "a + b - c" with 2-4 operations.
// Uses left-to-right evaluation (no precedence mixing) to keep mental math
// approachable. For higher grades we allow longer chains.
// Avoids producing negative intermediate results when negatives aren't allowed.
func tMultiOp(cfg GradeConfig, ops []Op, rng randFunc) (string, string, bool) {
	// Only use add/sub in chains - mixing in mul/div would require explicit
	// precedence handling that makes problems harder to read for this grade.
	chainOps := []Op{}
	for _, op := range ops {
		if op == OpAdd || op == OpSub {
			chainOps = append(chainOps, op)
		}
	}
	if len(chainOps) == 0 {
		return "", "", false
	}
	chainLen := randIntRange(2, max(2, cfg.MaxChainLen))

	// Start with an initial number large enough to avoid negatives
	running := randIntRange(cfg.MaxAddSub/2, cfg.MaxAddSub)
	expr := strconv.Itoa(running)
	for i := 0; i < chainLen; i++ {
		op := pickOp(chainOps, rng)
		b := randIntRange(cfg.MinAddSub, cfg.MaxAddSub/2)
		if op == OpSub && !cfg.AllowNeg && b > running {
			b = randIntRange(cfg.MinAddSub, max(cfg.MinAddSub, running-1))
		}
		running = compute(running, op, b)
		expr = fmt.Sprintf("%s %s %d", expr, opSymbol(op), b)
	}
	return expr, strconv.Itoa(running), true
}
