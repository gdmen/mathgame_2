// Package generator contains a bit-driven heuristic math problem generator.
// This is heuristic_1.0. Unlike the LLM generator it runs in-process, is
// deterministic, and produces clean output.
//
// Part of the problem-generation system - documented in
// docs/problem-generation.md. Behavior changes here (ranges, templates,
// option mapping) REQUIRE updating that doc in the same PR.
//
// Design principles:
//   - Bit-driven: number ranges, operations, and templates come from
//     the explicit Options fields mapped off the user's settings bitmap
//   - Template-based: multiple problem shapes (basic, missing-number, multi-term)
//   - Clean output: no redundant parens, spaces around operators
//   - No trivial problems: avoid a+0, a*1, a-a, 0/anything
package generator // import "garydmenezes.com/mathgame/server/generator"

// GenConfig describes the numeric ranges and allowed shapes for one
// generation call.
type GenConfig struct {
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

	// Bit-driven fields.
	SameDenomOnly bool // All fractions in a problem share one denominator
	MaxOperand    int  // Hard bound on every number in the expression (0 = unbounded)
}
