// Package generator contains a grade-aware heuristic math problem generator.
// This is heuristic_1.0, a complete rewrite of heuristic_0.0. Unlike the LLM
// generator it runs in-process, is deterministic, and produces clean output.
//
// Design principles:
//   - Grade-aware: number ranges, operations, and templates vary by grade
//   - Template-based: multiple problem shapes (basic, missing-number, multi-term)
//   - Clean output: no redundant parens, spaces around operators
//   - No trivial problems: avoid a+0, a*1, a-a, 0/anything
package generator // import "garydmenezes.com/mathgame/server/generator"

// GradeConfig describes what's appropriate for a given grade level.
// Number ranges follow Common Core progression.
type GradeConfig struct {
	Label        string
	MinAddSub    int  // Minimum operand for add/sub (keeps problems non-trivial)
	MaxAddSub    int  // Maximum operand for add/sub
	MinMul       int  // Minimum operand for multiplication
	MaxMul       int  // Maximum operand for multiplication
	MaxDiv       int  // Maximum quotient for division (a/b where a <= MaxDiv)
	MaxDivisor   int  // Maximum divisor for division
	AllowFrac    bool // Fractions enabled
	MaxFracDenom int  // Largest fraction denominator
	AllowNeg     bool // Negative numbers enabled
	AllowMultiOp bool // Multi-term expressions (a + b - c)
	AllowMissing bool // Missing-number templates (? + b = c)
	MaxChainLen  int  // Maximum chain length for multi-op
}

// grades holds per-grade configuration. Grades 1-8 mirror the curriculum.json
// progression used by the LLM generator for consistency.
var grades = map[int]GradeConfig{
	1: {
		Label: "1st Grade", MinAddSub: 1, MaxAddSub: 20,
		MaxChainLen: 2,
	},
	2: {
		Label: "2nd Grade", MinAddSub: 2, MaxAddSub: 100,
		MinMul: 2, MaxMul: 5,
		AllowMissing: true, MaxChainLen: 3,
	},
	3: {
		Label: "3rd Grade", MinAddSub: 2, MaxAddSub: 1000,
		MinMul: 2, MaxMul: 10, MaxDiv: 100, MaxDivisor: 10,
		AllowFrac: true, MaxFracDenom: 8,
		AllowMultiOp: true, AllowMissing: true, MaxChainLen: 3,
	},
	4: {
		Label: "4th Grade", MinAddSub: 2, MaxAddSub: 10000,
		MinMul: 2, MaxMul: 12, MaxDiv: 1000, MaxDivisor: 12,
		AllowFrac: true, MaxFracDenom: 12,
		AllowMultiOp: true, AllowMissing: true, MaxChainLen: 4,
	},
	5: {
		Label: "5th Grade", MinAddSub: 2, MaxAddSub: 100000,
		MinMul: 2, MaxMul: 20, MaxDiv: 10000, MaxDivisor: 25,
		AllowFrac: true, MaxFracDenom: 16,
		AllowMultiOp: true, AllowMissing: true, MaxChainLen: 4,
	},
	6: {
		Label: "6th Grade", MinAddSub: 2, MaxAddSub: 100000,
		MinMul: 2, MaxMul: 25, MaxDiv: 10000, MaxDivisor: 50,
		AllowFrac: true, MaxFracDenom: 20,
		AllowNeg: true, AllowMultiOp: true, AllowMissing: true, MaxChainLen: 4,
	},
	7: {
		Label: "7th Grade", MinAddSub: 2, MaxAddSub: 100000,
		MinMul: 2, MaxMul: 50, MaxDiv: 10000, MaxDivisor: 100,
		AllowFrac: true, MaxFracDenom: 24,
		AllowNeg: true, AllowMultiOp: true, AllowMissing: true, MaxChainLen: 5,
	},
	8: {
		Label: "8th Grade", MinAddSub: 2, MaxAddSub: 100000,
		MinMul: 2, MaxMul: 100, MaxDiv: 100000, MaxDivisor: 100,
		AllowFrac: true, MaxFracDenom: 24,
		AllowNeg: true, AllowMultiOp: true, AllowMissing: true, MaxChainLen: 5,
	},
}

// defaultGradeConfig is used when GradeLevel is 0 or unrecognized.
// Mirrors grade 3: supports all four operations with moderate ranges.
var defaultGradeConfig = GradeConfig{
	Label: "default", MinAddSub: 2, MaxAddSub: 100,
	MinMul: 2, MaxMul: 10, MaxDiv: 100, MaxDivisor: 10,
	AllowFrac:    false,
	AllowMultiOp: true, AllowMissing: false, MaxChainLen: 3,
}

// getGradeConfig returns the config for a grade, or default if not found.
func getGradeConfig(grade int) GradeConfig {
	if c, ok := grades[grade]; ok {
		return c
	}
	return defaultGradeConfig
}
